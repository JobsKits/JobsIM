package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnavailableContentNeverExposesOlderBodyInPresentation(t *testing.T) {
	raw := json.RawMessage(`{"id":"9007199254740993123","contentUnavailable":true,"text":"stale body retained by an older cache","attachmentId":"older-object","deleted":false}`)
	original := bytes.Clone(raw)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	err = printJSON(map[string]any{"items": []json.RawMessage{raw}})
	os.Stdout = previous
	writer.Close()
	output, readErr := io.ReadAll(reader)
	reader.Close()
	if err != nil || readErr != nil {
		t.Fatalf("render failed: %v %v", err, readErr)
	}
	if bytes.Contains(output, []byte("stale body")) || bytes.Contains(output, []byte("older-object")) {
		t.Fatal("recovery placeholder exposed an older body or attachment")
	}
	if !bytes.Contains(output, []byte("Content unavailable after restore")) || !bytes.Contains(output, []byte("9007199254740993123")) || !bytes.Contains(output, []byte(`"deleted": false`)) {
		t.Fatal("placeholder lost its identity or became a deletion")
	}
	if !bytes.Equal(raw, original) {
		t.Fatal("presentation changed canonical data")
	}
}

func identityForTest() localIdentity {
	return localIdentity{Origin: "https://verified.example", DeploymentID: "deployment", UserID: "66666666-6666-4666-8666-666666666666", DeviceID: "physical-installation", Space: "personal"}
}
func testStore(t *testing.T, path, epoch string) *localStore {
	t.Helper()
	store, err := openStore(path, identityForTest(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func authoritative(id, clientID, conversation, version string) json.RawMessage {
	if !decimal.MatchString(id) && !syncUUIDPattern.MatchString(id) {
		digest := fingerprint(id)[:32]
		id = digest[:8] + "-" + digest[8:12] + "-" + digest[12:16] + "-" + digest[16:20] + "-" + digest[20:]
	}
	raw, _ := json.Marshal(map[string]any{"id": id, "clientMsgId": clientID, "conversationId": conversation, "senderId": "66666666-6666-4666-8666-666666666666", "createdAt": time.Now().UTC().Format(time.RFC3339Nano), "type": "text", "deleted": false, "contentVersion": "1", "version": version, "sequence": "9007199254740993", "text": "authoritative"})
	return raw
}

func TestOutboxPersistsBeforeSendAndKeepsUnknownIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.sqlite")
	store := testStore(t, path, "1")
	defer store.close()
	var seen string
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveAdmissionMetadata(w, r) {
			return
		}
		count := calls.Add(1)
		var q map[string]string
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		if seen != "" && seen != q["clientMsgId"] {
			t.Error("retry changed original ID")
		}
		seen = q["clientMsgId"]
		var state string
		if err := store.db.QueryRow("SELECT state FROM outbox WHERE client_msg_id=?", seen).Scan(&state); err != nil || state != "pending" {
			t.Error("HTTP preceded committed SQLite Outbox")
		}
		if count == 1 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":"TEMPORARY","message":"temporary"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(authoritative("server-message", seen, "11111111-1111-4111-8111-111111111111", "1"))
	}))
	defer server.Close()
	c := &client{base: server.URL, http: server.Client(), token: "never-persist-this", deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", store: store, epoch: "1"}
	if err := c.send("11111111-1111-4111-8111-111111111111", "durable world", "stable-original-id", "text"); err == nil {
		t.Fatal("failed send reported success")
	}
	items, err := c.readOutbox()
	if err != nil || items["stable-original-id"].State != "pending" {
		t.Fatal("unknown send lost")
	}
	if err = c.send("other-conversation", "durable world", "stable-original-id", "text"); err == nil {
		t.Fatal("same ID changed target")
	}
	if calls.Load() != 1 {
		t.Fatal("changed original payload reached HTTP")
	}
	if err = c.retry("stable-original-id", false); err != nil {
		t.Fatal(err)
	}
	items, err = c.readOutbox()
	if err != nil || items["stable-original-id"].State != "confirmed" {
		t.Fatal("retry not confirmed")
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		t.Fatal("not a real SQLite database")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("database not private")
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		raw, _ := os.ReadFile(path + suffix)
		if bytes.Contains(raw, []byte(c.token)) {
			t.Fatal("credential persisted")
		}
	}
}
func TestOutboxCrashReopenAndRestoreFreeze(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.sqlite")
	store := testStore(t, path, "1")
	p := pending{DeploymentID: "deployment", UserID: "66666666-6666-4666-8666-666666666666", ConversationID: "11111111-1111-4111-8111-111111111111", ClientMsgID: "original", Type: "audio", AttachmentID: "private-object", ReplyToID: "source", RestoreEpoch: "1", State: "pending"}
	if err := store.enqueue(p); err != nil {
		t.Fatal(err)
	}
	store.close()
	store = testStore(t, path, "1")
	items, err := store.outbox()
	if err != nil || items["original"].AttachmentID != "private-object" || items["original"].ReplyToID != "source" {
		t.Fatal("payload did not survive reopen")
	}
	store.close()
	store = testStore(t, path, "2")
	defer store.close()
	items, err = store.outbox()
	if err != nil || items["original"].State != "frozen" || items["original"].RestoreEpoch != "1" {
		t.Fatal("old queue not frozen with original epoch")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveAdmissionMetadata(w, r) {
			return
		}
		calls.Add(1)
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["clientMsgId"] != "original" || body["restoreEpoch"] != "1" || body["attachmentId"] != "private-object" {
			t.Error("reconciliation changed original")
		}
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":{"code":"RESTORE_EPOCH_CHANGED","message":"frozen"}}`))
	}))
	defer server.Close()
	c := &client{store: store, epoch: "2", base: server.URL, http: server.Client(), deployment: "deployment", user: "66666666-6666-4666-8666-666666666666"}
	if err = c.retry("original", false); err == nil || calls.Load() != 0 {
		t.Fatal("frozen operation retried automatically")
	}
	if err = c.retry("original", true); err == nil || calls.Load() != 0 {
		t.Fatal("old epoch reconciliation reached network")
	}
	other := identityForTest()
	other.UserID = "another-user"
	if foreign, err := openStore(path, other, "2"); err == nil {
		foreign.close()
		t.Fatal("cross-account cache reused")
	}
}
func TestCursorAndMessagesCommitTogetherWithoutVersionRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.sqlite")
	store := testStore(t, path, "1")
	defer store.close()
	valid := authoritative("9007199254740993123456", "original", "11111111-1111-4111-8111-111111111111", "9")
	invalid := authoritative("other", "other", "different-conversation", "1")
	if err := store.savePage("11111111-1111-4111-8111-111111111111", "1:9007199254740993123", []json.RawMessage{valid, invalid}); err == nil {
		t.Fatal("cross-conversation page accepted")
	}
	cursor, err := store.cursor("11111111-1111-4111-8111-111111111111")
	if err != nil || cursor != "0" {
		t.Fatal("failed page advanced cursor")
	}
	history, _ := store.history("11111111-1111-4111-8111-111111111111")
	if len(history) != 0 {
		t.Fatal("failed page partially committed")
	}
	if err = store.savePage("11111111-1111-4111-8111-111111111111", "1:9007199254740993123", []json.RawMessage{valid}); err != nil {
		t.Fatal(err)
	}
	older := authoritative("9007199254740993123456", "original", "11111111-1111-4111-8111-111111111111", "2")
	if err = store.savePage("11111111-1111-4111-8111-111111111111", "1:3", []json.RawMessage{older}); err != nil {
		t.Fatal(err)
	}
	cursor, _ = store.cursor("11111111-1111-4111-8111-111111111111")
	if cursor != "1:9007199254740993123" {
		t.Fatal("cursor lost precision or regressed")
	}
	history, _ = store.history("11111111-1111-4111-8111-111111111111")
	if len(history) != 1 || !strings.Contains(string(history[0]), `"version":"9"`) {
		t.Fatal("authoritative version regressed")
	}
	if err = store.savePage("11111111-1111-4111-8111-111111111111", "2:4", nil); err == nil {
		t.Fatal("old database accepted other restore epoch")
	}
	if err = store.draft("11111111-1111-4111-8111-111111111111", "unsent draft"); err != nil {
		t.Fatal(err)
	}
	drafts, _ := store.drafts()
	if drafts["11111111-1111-4111-8111-111111111111"] != "unsent draft" {
		t.Fatal("draft not durable")
	}
}
func TestLegacyImportIsExplicitAtomicAndRetainsOriginal(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "outbox.jsonl")
	p := pending{DeploymentID: "deployment", UserID: "66666666-6666-4666-8666-666666666666", ConversationID: "11111111-1111-4111-8111-111111111111", ClientMsgID: "old-id", Type: "text", Text: "old pending", State: "pending"}
	raw, _ := json.Marshal(p)
	original := append(raw, '\n')
	if err := os.WriteFile(source, original, 0600); err != nil {
		t.Fatal(err)
	}
	store := testStore(t, filepath.Join(directory, "client.sqlite"), "1")
	defer store.close()
	if err := store.importLegacy(source, "wrong-instance"); err == nil {
		t.Fatal("legacy origin not confirmed")
	}
	if err := store.importLegacy(source, "deployment"); err != nil {
		t.Fatal(err)
	}
	items, _ := store.outbox()
	if len(items) != 1 || items["old-id"].ClientMsgID != "old-id" {
		t.Fatal("legacy identity changed")
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(original, after) {
		t.Fatal("original legacy file changed")
	}
	if err := store.importLegacy(source, "deployment"); err == nil {
		t.Fatal("same legacy source imported twice")
	}
	broken := filepath.Join(directory, "broken.jsonl")
	os.WriteFile(broken, []byte(fmt.Sprintf("%s\n%s\n", raw, "not-json")), 0600)
	if err := store.importLegacy(broken, "deployment"); err == nil {
		t.Fatal("broken source accepted")
	}
	items, _ = store.outbox()
	if len(items) != 1 {
		t.Fatal("broken source partly imported")
	}
}

func serveAdmissionMetadata(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/bootstrap":
		_, _ = w.Write([]byte(`{"deploymentId":"deployment","restoreEpoch":"1","features":{"send_admission":true}}`))
		return true
	case "/api/conversations":
		_, _ = w.Write([]byte(`{"items":[{"id":"11111111-1111-4111-8111-111111111111","kind":"group","sendAdmission":"original-admission"}]}`))
		return true
	}
	return false
}

func TestSendingAdmissionSurvivesRestartAndNeverUpgradesOriginalIntent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission.sqlite")
	store := testStore(t, path, "1")
	defer func() { _ = store.close() }()
	var current = "admission-before-exit"
	var enabled = true
	var posts int
	var originalBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/bootstrap":
			_ = json.NewEncoder(w).Encode(map[string]any{"deploymentId": "deployment", "restoreEpoch": "1", "features": map[string]bool{"send_admission": enabled}})
			return
		case "/api/conversations":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]string{{"id": "11111111-1111-4111-8111-111111111111", "kind": "group", "sendAdmission": current}}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/message-results") {
			raw, _ := io.ReadAll(r.Body)
			if len(originalBody) > 0 && string(raw) != originalBody && strings.Contains(string(raw), "original-id") {
				t.Error("readonly lookup replaced original admission")
			}
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":{"code":"MESSAGE_RESULT_NOT_FOUND"}}`)
			return
		}
		posts++
		raw, _ := io.ReadAll(r.Body)
		if posts == 1 {
			originalBody = string(raw)
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":"TEMPORARY","message":"unknown"}}`))
			return
		}
		if posts <= 3 && string(raw) != originalBody {
			t.Error("original sending payload or admission was replaced")
		}
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":{"code":"SEND_ADMISSION_CHANGED","message":"old admission"}}`))
	}))
	defer server.Close()
	c := &client{base: server.URL, http: server.Client(), deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", epoch: "1", store: store}
	if err := c.send("11111111-1111-4111-8111-111111111111", "original text", "original-id", "text"); err == nil {
		t.Fatal("unknown send succeeded")
	}
	before, _ := store.outbox()
	digest := operationDigest(before["original-id"])
	current = "admission-after-rejoin"
	_ = store.close()
	store = testStore(t, path, "1")
	c.store = store
	if err := c.retry("original-id", false); err == nil {
		t.Fatal("old admission accepted")
	}
	after, _ := store.outbox()
	item := after["original-id"]
	if item.State != "frozen" || item.FreezeReason != "SEND_ADMISSION_CHANGED" || item.SendAdmission != "admission-before-exit" || operationDigest(item) != digest {
		t.Fatal("original intent not durably frozen intact")
	}
	if err := c.retry("original-id", false); err == nil || posts != 2 {
		t.Fatal("frozen intent was resent automatically")
	}
	if err := c.retry("original-id", true); err == nil || posts != 2 {
		t.Fatal("explicit reconciliation did not preserve original payload")
	}
	enabled = false
	if err := c.retry("original-id", true); err == nil || posts != 2 {
		t.Fatal("capability loss created an original intent")
	}
	legacy := pending{DeploymentID: "deployment", UserID: "66666666-6666-4666-8666-666666666666", ConversationID: "11111111-1111-4111-8111-111111111111", ClientMsgID: "legacy-id", Type: "text", Text: "legacy", RestoreEpoch: "1", State: "pending"}
	if err := store.enqueue(legacy); err != nil {
		t.Fatal(err)
	}
	enabled = true
	if err := c.retry("legacy-id", false); err == nil {
		t.Fatal("legacy absent snapshot was accepted")
	}
	after, _ = store.outbox()
	if after["legacy-id"].State != "frozen" || after["legacy-id"].SendAdmission != "" || posts != 2 {
		t.Fatal("legacy snapshot was inferred or not frozen")
	}
}

func TestAdmissionLookupFailureKeepsDraftWithoutInventingQualification(t *testing.T) {
	store := testStore(t, filepath.Join(t.TempDir(), "draft.sqlite"), "1")
	defer store.close()
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
		}
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"code":"OFFLINE","message":"unavailable"}}`))
	}))
	defer server.Close()
	c := &client{base: server.URL, http: server.Client(), deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", epoch: "1", store: store}
	if err := c.send("11111111-1111-4111-8111-111111111111", "keep this draft", "new-original", "text"); err == nil {
		t.Fatal("unqualified intent sent")
	}
	drafts, err := store.drafts()
	if err != nil || drafts["11111111-1111-4111-8111-111111111111"] != "keep this draft" {
		t.Fatal("new draft lost before qualification lookup")
	}
	rows, err := store.outbox()
	if err != nil || len(rows) != 0 || posts != 0 {
		t.Fatal("invented qualification or sent before authoritative admission")
	}
}

func TestForwardUnknownResultsUseOnlyOriginalReferencesAfterSQLiteReopen(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprintf("committed_%v", committed), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "forward.sqlite")
			store := testStore(t, path, "1")
			defer func() { store.close() }()
			if err := store.draft("11111111-1111-4111-8111-111111111111", "keep destination draft"); err != nil {
				t.Fatal(err)
			}
			var original []byte
			posts, lookups := 0, 0
			hasResult := committed
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/bootstrap":
					fmt.Fprint(w, `{"deploymentId":"deployment","restoreEpoch":"1","features":{"send_admission":true,"forwarding":true}}`)
				case "/api/messages/source":
					fmt.Fprint(w, `{"id":"source","contentVersion":"9007199254740997","forwardAdmission":"original-source-proof","text":"source body must not be copied"}`)
				case "/api/conversations/11111111-1111-4111-8111-111111111111":
					fmt.Fprint(w, `{"id":"11111111-1111-4111-8111-111111111111","kind":"direct","memberIds":["66666666-6666-4666-8666-666666666666","peer"],"sendAdmission":"original-target-proof"}`)
				case "/api/conversations/11111111-1111-4111-8111-111111111111/message-results":
					lookups++
					body, _ := io.ReadAll(r.Body)
					if !bytes.Equal(body, original) {
						t.Error("lookup changed original payload")
					}
					if !hasResult {
						w.WriteHeader(404)
						fmt.Fprint(w, `{"error":{"code":"MESSAGE_RESULT_NOT_FOUND"}}`)
						return
					}
					var q map[string]string
					json.Unmarshal(body, &q)
					w.Write(authoritative("result", q["clientMsgId"], "11111111-1111-4111-8111-111111111111", "1"))
				case "/api/conversations/11111111-1111-4111-8111-111111111111/messages":
					posts++
					body, _ := io.ReadAll(r.Body)
					var q map[string]string
					json.Unmarshal(body, &q)
					if posts == 1 {
						original = bytes.Clone(body)
					} else if !bytes.Equal(body, original) {
						t.Error("retry changed original body")
					}
					for _, key := range []string{"text", "attachmentId", "replyToId", "mentionAll", "mentionedUserIds"} {
						if _, ok := q[key]; ok {
							t.Error("copied source content", key)
						}
					}
					if q["forwardSourceVersion"] != "9007199254740997" || q["forwardAdmission"] != "original-source-proof" || q["sendAdmission"] != "original-target-proof" {
						t.Error("lost original source/target proof")
					}
					var raw string
					if err := store.db.QueryRow("SELECT payload FROM outbox WHERE client_msg_id=?", q["clientMsgId"]).Scan(&raw); err != nil {
						t.Error("POST preceded commit", err)
					}
					attempted, err := store.forwardAttempted(q["clientMsgId"])
					if err != nil || !attempted {
						t.Error("POST preceded durable attempt marker")
					}
					if posts == 1 {
						w.WriteHeader(503)
						fmt.Fprint(w, `{"error":{"code":"FORWARD_QUALIFICATION_UNAVAILABLE"}}`)
						return
					}
					hasResult = true
					w.Write(authoritative("result", q["clientMsgId"], "11111111-1111-4111-8111-111111111111", "1"))
				default:
					t.Error("unexpected route", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			c := &client{base: server.URL, http: server.Client(), deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", store: store, epoch: "1"}
			if err := c.forwardMessage("11111111-1111-4111-8111-111111111111", "source", "9007199254740997"); err == nil {
				t.Fatal("unknown send reported confirmed")
			}
			items, _ := store.outbox()
			if len(items) != 1 {
				t.Fatal("lost pending original")
			}
			var identity string
			for key := range items {
				identity = key
			}
			drafts, _ := store.drafts()
			if drafts["11111111-1111-4111-8111-111111111111"] != "keep destination draft" {
				t.Fatal("forward erased destination draft")
			}
			store.close()
			store = testStore(t, path, "1")
			c.store = store
			if err := c.retry(identity, false); err != nil {
				t.Fatal(err)
			}
			expected := 2
			if committed {
				expected = 1
			}
			expectedLookups := 3
			if committed {
				expectedLookups = 1
			}
			if posts != expected || lookups != expectedLookups {
				t.Fatalf("wrong unknown reconciliation: posts=%d lookups=%d", posts, lookups)
			}
			items, _ = store.outbox()
			if items[identity].State != "confirmed" {
				t.Fatal("original not confirmed")
			}
		})
	}
}

func TestForwardVersionChangesAndDefiniteRejectionNeverRefreshOldProof(t *testing.T) {
	for _, rejection := range []string{"FORWARD_ADMISSION_CHANGED", "NOT_A_MEMBER", "HISTORY_NOT_ALLOWED"} {
		t.Run(rejection, func(t *testing.T) {
			store := testStore(t, filepath.Join(t.TempDir(), "forward.sqlite"), "1")
			defer store.close()
			posts, lookups := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/bootstrap":
					fmt.Fprint(w, `{"deploymentId":"deployment","restoreEpoch":"1","features":{"send_admission":true,"forwarding":true}}`)
				case "/api/messages/source":
					fmt.Fprint(w, `{"id":"source","contentVersion":"2","forwardAdmission":"source-proof"}`)
				case "/api/conversations/11111111-1111-4111-8111-111111111111":
					fmt.Fprint(w, `{"id":"11111111-1111-4111-8111-111111111111","kind":"direct","memberIds":["66666666-6666-4666-8666-666666666666","peer"],"sendAdmission":"target-proof"}`)
				case "/api/conversations/11111111-1111-4111-8111-111111111111/messages":
					posts++
					if rejection == "FORWARD_ADMISSION_CHANGED" {
						w.WriteHeader(409)
					} else {
						w.WriteHeader(403)
					}
					fmt.Fprintf(w, `{"error":{"code":%q}}`, rejection)
				case "/api/conversations/11111111-1111-4111-8111-111111111111/message-results":
					lookups++
					w.WriteHeader(404)
					fmt.Fprint(w, `{"error":{"code":"MESSAGE_RESULT_NOT_FOUND"}}`)
				default:
					t.Error(r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			c := &client{base: server.URL, http: server.Client(), deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", store: store, epoch: "1"}
			if err := c.forwardMessage("11111111-1111-4111-8111-111111111111", "source", "1"); err == nil {
				t.Fatal("source changed without reselection")
			}
			items, _ := store.outbox()
			if len(items) != 0 || posts != 0 {
				t.Fatal("changed source was queued")
			}
			if err := c.forwardMessage("11111111-1111-4111-8111-111111111111", "source", "2"); err == nil {
				t.Fatal("definite rejection reported success")
			}
			items, _ = store.outbox()
			var p pending
			for _, v := range items {
				p = v
			}
			if p.State != "frozen" || p.FreezeReason != rejection {
				t.Fatal("rejected original not frozen")
			}
			before := operationDigest(p)
			if err := c.retry(p.ClientMsgID, true); err == nil {
				t.Fatal("missing original result claimed success")
			}
			after, _ := store.outbox()
			if operationDigest(after[p.ClientMsgID]) != before || posts != 1 || lookups != 1 {
				t.Fatal("frozen reconcile upgraded or first-created old intent")
			}
		})
	}
}

func TestForwardCacheAndPresentationCannotRetainProjectedBodyOrProof(t *testing.T) {
	store := testStore(t, filepath.Join(t.TempDir(), "cache.sqlite"), "1")
	defer store.close()
	raw := json.RawMessage(`{"id":"f","conversationId":"11111111-1111-4111-8111-111111111111","clientMsgId":"original","senderId":"peer","createdAt":"2026-11-15T00:00:00Z","version":"9007199254740997","forwarded":true,"text":"private-source-body","attachmentId":"source-media","attachmentURL":"/api/messages/f/attachment","forwardAdmission":"transient-proof"}`)
	if err := store.savePage("11111111-1111-4111-8111-111111111111", "1:1", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.history("11111111-1111-4111-8111-111111111111")
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-source-body", "source-media", "/api/messages/f/attachment", "transient-proof"} {
		if bytes.Contains(rows[0], []byte(secret)) {
			t.Fatal("offline cache retained projection", secret)
		}
	}
	var cached map[string]any
	json.Unmarshal(rows[0], &cached)
	if cached["forwardUnavailable"] != true || cached["version"] != "9007199254740997" {
		t.Fatal("cache lost placeholder identity")
	}
	view := map[string]any{"id": "plain", "forwardAdmission": "transient", "text": "ordinary text"}
	projectUnavailable(view)
	if _, ok := view["forwardAdmission"]; ok {
		t.Fatal("presentation retained transient permit")
	}
	if view["text"] != "ordinary text" {
		t.Fatal("ordinary source body lost")
	}
}
