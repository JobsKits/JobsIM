// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// These are injected HTTP/SQLite tests, not an actual JobsIM HTTPS/PG acceptance.
type cliSyncFixture struct {
	t            *testing.T
	store        *localStore
	client       *client
	identity     localIdentity
	server       *httptest.Server
	root         string
	physical     string
	conversation string
	messageID    string
	snapshot     string
	stream       string
	permission   string
	expires      string
	nonce        string
	mu           sync.Mutex
	requests     map[string][]json.RawMessage
	hook         func(http.ResponseWriter, *http.Request) bool
}

func newCLISyncFixture(t *testing.T) *cliSyncFixture {
	t.Helper()
	f := &cliSyncFixture{t: t, root: t.TempDir(), conversation: "11111111-1111-4111-8111-111111111111", messageID: "22222222-2222-4222-8222-222222222222", snapshot: "33333333-3333-4333-8333-333333333333", stream: "44444444-4444-4444-8444-444444444444", permission: strings.Repeat("a", 64), expires: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), nonce: "3a1542717a864d439cfe0db5da3918dd", requests: map[string][]json.RawMessage{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	f.identity = localIdentity{Origin: f.server.URL, DeploymentID: "55555555-5555-4555-8555-555555555555", UserID: "66666666-6666-4666-8666-666666666666", DeviceID: "77777777-7777-4777-8777-777777777777", Space: "personal", ConnectionProfile: strings.Repeat("b", 64), StoreGeneration: strings.Repeat("1", 32)}
	identity, _ := json.Marshal(f.identity)
	path := filepath.Join(f.root, f.identity.ConnectionProfile, fingerprint(string(identity)), "client.sqlite")
	var err error
	f.store, err = openStore(path, f.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.store.close() })
	if err = f.store.db.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&f.physical); err != nil {
		t.Fatal(err)
	}
	f.client = &client{base: f.server.URL, http: f.server.Client(), token: "unit-only-not-a-credential", deployment: f.identity.DeploymentID, user: f.identity.UserID, epoch: "1", device: f.identity.DeviceID, serverDevice: f.identity.DeviceID, store: f.store}
	return f
}
func (f *cliSyncFixture) setHook(h func(http.ResponseWriter, *http.Request) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hook = h
}
func (f *cliSyncFixture) calls(path string) []json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]json.RawMessage(nil), f.requests[path]...)
}
func (f *cliSyncFixture) binding() map[string]any {
	return map[string]any{"scopeId": f.identity.DeploymentID, "principalId": f.identity.UserID, "sourceContextId": "personal", "deviceId": f.identity.DeviceID, "storeGeneration": f.physical, "restoreEpoch": "1", "streamId": f.stream}
}
func (f *cliSyncFixture) ref(kind, id string) syncRef {
	owner := "conversation"
	if kind == "message" {
		owner = "message"
	}
	r := syncRef{Owner: owner, Kind: kind, ID: id, Revision: "1", RevisionEpoch: "1", State: "active"}
	if kind == "message" {
		r.ConversationID = f.conversation
		r.SubjectUserID = f.identity.UserID
	}
	return r
}
func (f *cliSyncFixture) message() json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"id": f.messageID, "conversationId": f.conversation, "senderId": f.identity.UserID, "clientMsgId": f.nonce, "type": "text", "text": "原始 bytes <&> 消息", "version": "9007199254740993", "contentVersion": "1", "sequence": "1", "createdAt": "2026-11-16T00:00:00Z", "deleted": false})
	return bytes.ReplaceAll(raw, []byte(`\u003c`), []byte("<"))
}
func cliSyncEnvelope(raw []byte) (string, string) {
	return base64.RawURLEncoding.EncodeToString(raw), fingerprint(string(raw))
}
func (f *cliSyncFixture) projection(ref syncRef, body json.RawMessage, state string) map[string]any {
	envelope, digest := cliSyncEnvelope(body)
	return map[string]any{"ref": ref, "state": state, "currentRevision": ref.Revision, "currentRevisionEpoch": "1", "revisionMatch": true, "bodyDigest": digest, "bodyEnvelope": envelope, "object": body}
}
func (f *cliSyncFixture) page(index int) map[string]any {
	ref := f.ref("conversation", f.conversation)
	body, _ := json.Marshal(map[string]any{"id": f.conversation, "kind": "group", "title": "测试群", "memberIds": []string{f.identity.UserID}, "sendAdmission": "unit-original-admission"})
	if index == 1 {
		ref = f.ref("message", f.messageID)
		body = f.message()
	}
	refs, _ := json.MarshalIndent([]syncRef{ref}, "", " ")
	envelope, digest := cliSyncEnvelope(refs)
	return map[string]any{"snapshotId": f.snapshot, "page": index, "refs": json.RawMessage(refs), "referenceEnvelope": envelope, "pageDigest": digest, "items": []any{f.projection(ref, body, "available")}, "permissionGeneration": f.permission, "restoreEpoch": "1", "installable": true, "requiresFinalValidation": true, "requiresAtomicInstall": true, "qualifiedAck": false, "digestScope": "immutable_references_only"}
}
func (f *cliSyncFixture) manifest() map[string]any {
	v := f.binding()
	v["snapshotId"] = f.snapshot
	v["schemaVersion"] = "1"
	v["boundary"] = "0"
	v["boundaryToken"] = "signed-original-boundary"
	v["ownerWatermarks"] = map[string]string{"auth": "0", "directory": "0", "conversation": "1", "message": "1"}
	v["permissionGeneration"] = f.permission
	v["classificationWatermark"] = "9"
	v["coverage"] = map[string]any{}
	v["expiresAt"] = f.expires
	v["pageManifest"] = []any{map[string]any{"page": 0, "count": 1, "digest": f.page(0)["pageDigest"]}, map[string]any{"page": 1, "count": 1, "digest": f.page(1)["pageDigest"]}}
	return v
}
func (f *cliSyncFixture) changes() map[string]any {
	return map[string]any{"items": []any{}, "cursor": "signed-contiguous-0", "position": "0", "head": "0", "hasMore": false, "restoreEpoch": "1", "permissionGeneration": f.permission, "snapshotRequired": false, "qualifiedAck": false}
}
func cliSyncJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (f *cliSyncFixture) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	f.mu.Lock()
	f.requests[r.URL.Path] = append(f.requests[r.URL.Path], bytes.Clone(raw))
	hook := f.hook
	f.mu.Unlock()
	if hook != nil && hook(w, r) {
		return
	}
	switch {
	case r.URL.Path == "/api/bootstrap":
		cliSyncJSON(w, 200, map[string]any{"deploymentId": f.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{"authoritative_sync": true, "sync_jetstream": true, "send_admission": true}})
	case r.URL.Path == "/api/sync/device":
		v := f.binding()
		v["qualification"] = "transport_only"
		v["qualifiedAck"] = false
		cliSyncJSON(w, 200, v)
	case r.URL.Path == "/api/sync/context":
		v := f.binding()
		v["head"] = "0"
		v["permissionGeneration"] = f.permission
		v["qualification"] = "transport_only"
		v["qualifiedAck"] = false
		v["afterReceiptEnabled"] = false
		cliSyncJSON(w, 200, v)
	case r.URL.Path == "/api/sync/snapshots":
		cliSyncJSON(w, 201, f.manifest())
	case strings.Contains(r.URL.Path, "/pages/"):
		index, _ := strconv.Atoi(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		cliSyncJSON(w, 200, f.page(index))
	case strings.HasSuffix(r.URL.Path, "/validate"):
		cliSyncJSON(w, 200, map[string]any{"canInstall": true, "boundaryToken": "signed-original-boundary", "restoreEpoch": "1", "permissionGeneration": f.permission, "classificationWatermark": "10", "requiresLocalAtomicInstall": true, "qualifiedAck": false})
	case r.URL.Path == "/api/sync/changes":
		cliSyncJSON(w, 200, f.changes())
	case strings.HasSuffix(r.URL.Path, "/message-results"):
		cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "MESSAGE_RESULT_NOT_FOUND"}})
	default:
		cliSyncJSON(w, 500, map[string]any{"error": map[string]string{"code": "UNEXPECTED_TEST_ROUTE"}})
	}
}
func (f *cliSyncFixture) pending() pending {
	return pending{DeploymentID: f.identity.DeploymentID, UserID: f.identity.UserID, ConversationID: f.conversation, ClientMsgID: f.nonce, Type: "text", Text: "原始 bytes <&> 消息", RestoreEpoch: "1", State: "pending", SendAdmission: "unit-original-admission"}
}
func (f *cliSyncFixture) control() syncControl {
	v, ok, err := syncLoadControl(f.store.db, "1")
	if err != nil || !ok {
		f.t.Fatalf("sync control absent: %v", err)
	}
	return v
}

func TestSyncSchemaSeparatesPhysicalUUIDFromLegacyAuthAndSurvivesReopen(t *testing.T) {
	f := newCLISyncFixture(t)
	before, _ := json.Marshal(f.identity)
	if !syncUUIDPattern.MatchString(f.physical) || len(f.identity.StoreGeneration) != 32 {
		t.Fatal("sync and legacy generation not separated")
	}
	control, err := f.store.ensureSync()
	if err != nil {
		t.Fatal(err)
	}
	other, err := openStore(f.store.path, f.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer other.close()
	again, err := other.ensureSync()
	if err != nil {
		t.Fatal(err)
	}
	if again.StoreGeneration != control.StoreGeneration || again.RegistrationRequestID != control.RegistrationRequestID {
		t.Fatal("reopen replaced original registration")
	}
	var identity string
	if err = other.db.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&identity); err != nil {
		t.Fatal(err)
	}
	if identity != string(before) {
		t.Fatal("auth/filename identity silently migrated")
	}
}
func TestSyncFullManifestOriginalBytesAndFirstChangesBeforeAtomicInstall(t *testing.T) {
	f := newCLISyncFixture(t)
	checked := false
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/sync/changes" {
			var count int
			if err := f.store.db.QueryRow("SELECT count(*) FROM messages").Scan(&count); err != nil || count != 0 {
				t.Error("body installed before current permission")
			}
			checked = true
		}
		return false
	})
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	if !checked || f.control().Position != "0" || f.control().Classification != "10" {
		t.Fatal("boundary/classification not installed")
	}
	rows, err := f.store.history(f.conversation)
	if err != nil || len(rows) != 1 {
		t.Fatalf("body missing: %v", err)
	}
	var m cacheMessage
	_ = json.Unmarshal(rows[0], &m)
	if m.Version != "9007199254740993" || m.ClientMsgID != f.nonce {
		t.Fatal("opaque nonce or precise version changed")
	}
}
func TestSyncUnknownSnapshotPreservesOriginalRequestAcrossReopen(t *testing.T) {
	f := newCLISyncFixture(t)
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/sync/snapshots" {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "UNKNOWN"}})
			return true
		}
		return false
	})
	if err := f.client.authoritativeSync(); err == nil {
		t.Fatal("unknown snapshot accepted")
	}
	before := f.control()
	other, err := openStore(f.store.path, f.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer other.close()
	after, _, err := syncLoadControl(other.db, "1")
	if err != nil || !bytes.Equal(before.SnapshotRequest, after.SnapshotRequest) {
		t.Fatal("original snapshot intent lost")
	}
	f.setHook(nil)
	if err = f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	requests := f.calls("/api/sync/snapshots")
	if len(requests) != 2 || !syncEqual(requests[0], requests[1]) {
		t.Fatal("unknown retried with a different request")
	}
}
func TestSyncMissingOrCorruptPageRejectsWholeBoundary(t *testing.T) {
	for _, mode := range []string{"missing", "reference", "body"} {
		t.Run(mode, func(t *testing.T) {
			f := newCLISyncFixture(t)
			f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
				if !strings.HasSuffix(r.URL.Path, "/pages/1") {
					return false
				}
				if mode == "missing" {
					cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "MISSING"}})
				} else {
					page := f.page(1)
					if mode == "reference" {
						page["referenceEnvelope"] = "e30"
					} else {
						page["items"].([]any)[0].(map[string]any)["object"] = map[string]any{"id": f.messageID, "text": "changed"}
					}
					cliSyncJSON(w, 200, page)
				}
				return true
			})
			if err := f.client.authoritativeSync(); err == nil {
				t.Fatal("incomplete snapshot installed")
			}
			if f.control().Cursor != "" {
				t.Fatal("incomplete cursor advanced")
			}
			var count int
			_ = f.store.db.QueryRow("SELECT count(*) FROM messages").Scan(&count)
			if count != 0 {
				t.Fatal("partial cloud body installed")
			}
		})
	}
}
func TestSyncFinalWatermarkPermissionAndLocalGuardBlockLateInstall(t *testing.T) {
	for _, mode := range []string{"watermark", "permission", "guard"} {
		t.Run(mode, func(t *testing.T) {
			f := newCLISyncFixture(t)
			f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
				if mode == "watermark" && strings.HasSuffix(r.URL.Path, "/validate") {
					cliSyncJSON(w, 200, map[string]any{"canInstall": true, "boundaryToken": "signed-original-boundary", "restoreEpoch": "1", "permissionGeneration": f.permission, "classificationWatermark": "8", "requiresLocalAtomicInstall": true, "qualifiedAck": false})
					return true
				}
				if r.URL.Path == "/api/sync/changes" {
					if mode == "guard" {
						if err := f.store.restrictSync("local revoke", false); err != nil {
							t.Error(err)
						}
					} else if mode == "permission" {
						delta := f.changes()
						delta["permissionGeneration"] = strings.Repeat("c", 64)
						cliSyncJSON(w, 200, delta)
						return true
					}
				}
				return false
			})
			if err := f.client.authoritativeSync(); err == nil {
				t.Fatal("changed cut installed")
			}
			if f.control().Cursor != "" {
				t.Fatal("late cursor committed")
			}
		})
	}
}
func TestSyncSameClientIDBadResultAnd503NeverAckOrBlockVerifiedGlobal(t *testing.T) {
	for _, status := range []int{404, 503, 200} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f := newCLISyncFixture(t)
			p := f.pending()
			if err := f.store.enqueue(p); err != nil {
				t.Fatal(err)
			}
			tx, err := f.store.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = writeCached(tx, "1", f.message()); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
				if strings.HasSuffix(r.URL.Path, "/pages/1") {
					page := f.page(1)
					var body map[string]any
					_ = syncDecode(f.message(), &body)
					body["deleted"] = true
					raw, _ := json.Marshal(body)
					page["items"] = []any{f.projection(f.ref("message", f.messageID), raw, "global")}
					cliSyncJSON(w, 200, page)
					return true
				}
				if strings.HasSuffix(r.URL.Path, "/message-results") {
					if status == 200 {
						cliSyncJSON(w, 200, map[string]string{"clientMsgId": p.ClientMsgID, "senderId": p.UserID, "conversationId": p.ConversationID})
					} else {
						cliSyncJSON(w, status, map[string]any{"error": map[string]string{"code": "RESULT_UNKNOWN"}})
					}
					return true
				}
				return false
			})
			if err = f.client.authoritativeSync(); err != nil {
				t.Fatal(err)
			}
			outbox, err := f.store.outbox()
			if err != nil {
				t.Fatal(err)
			}
			original, ok := outbox[p.ClientMsgID]
			if !ok || original.State != "pending" || operationDigest(original) != operationDigest(p) {
				t.Fatal("projection or bad/unknown proof acknowledged original")
			}
			if len(f.calls("/api/conversations/"+f.conversation+"/messages")) != 0 {
				t.Fatal("sync created a message")
			}
			var count int
			_ = f.store.db.QueryRow("SELECT count(*) FROM sync_terminal WHERE id=?", f.messageID).Scan(&count)
			if count != 1 {
				t.Fatal("auxiliary proof blocked global")
			}
			_ = f.store.db.QueryRow("SELECT count(*) FROM sync_retained WHERE id=?", f.messageID).Scan(&count)
			if count != 0 {
				t.Fatal("global retained body survived")
			}
		})
	}
}
func TestSyncCompleteReadonlyOriginalProofAndHashCAS(t *testing.T) {
	f := newCLISyncFixture(t)
	p := f.pending()
	if err := f.store.enqueue(p); err != nil {
		t.Fatal(err)
	}
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/message-results") {
			cliSyncJSON(w, 200, f.message())
			return true
		}
		return false
	})
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	outbox, _ := f.store.outbox()
	if outbox[p.ClientMsgID].State != "confirmed" {
		t.Fatal("complete original proof not confirmed")
	}
	wrong := p
	wrong.Text = "different body"
	guard := f.control().Guard
	tx, err := f.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = f.store.confirmSyncTx(tx, syncProof{Original: wrong, Result: f.message(), Guard: guard}); err == nil {
		t.Fatal("changed original passed transaction CAS")
	}
}
func TestSyncGlobalOverridesHigherCacheVersionAndCannotRevive(t *testing.T) {
	f := newCLISyncFixture(t)
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = syncDecode(f.message(), &body)
	body["version"] = "1"
	body["deleted"] = true
	body["text"] = "must erase even with lower version"
	raw, _ := json.Marshal(body)
	tx, err := f.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writeCached(tx, "1", raw); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, err = f.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writeCached(tx, "1", f.message()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := f.store.history(f.conversation)
	if err != nil || len(rows) != 0 {
		t.Fatal("known global revived through cached")
	}
	var stored string
	if err = f.store.db.QueryRow("SELECT payload FROM messages WHERE epoch='1' AND id=?", f.messageID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "原始 bytes") || strings.Contains(stored, "must erase") {
		t.Fatal("global plaintext survived")
	}
}
func TestSyncUnknownRetainsOnlyVerifiedOrdinaryHistory(t *testing.T) {
	f := newCLISyncFixture(t)
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	if err := f.store.restrictSync("SYNC_UNAVAILABLE", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.history(f.conversation); err == nil {
		t.Fatal("cached bypassed restriction")
	}
	local, err := f.store.localHistory(f.conversation)
	if err != nil || len(local) != 1 {
		t.Fatalf("verified ordinary copy lost: %v", err)
	}
	if _, err = f.store.db.Exec("UPDATE sync_retained SET payload=?", `{"id":"22222222-2222-4222-8222-222222222222","version":"9007199254740993","text":"unverified restored copy"}`); err != nil {
		t.Fatal(err)
	}
	local, err = f.store.localHistory(f.conversation)
	if err != nil || len(local) != 0 {
		t.Fatal("unverified backup exposed through retained entry")
	}
}
func TestSyncCapabilityLossNeverFallsBackToHistoryOrCreate(t *testing.T) {
	t.Run("no responsibility on old capability", func(t *testing.T) {
		old := newCLISyncFixture(t)
		old.setHook(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/api/bootstrap" {
				cliSyncJSON(w, 200, map[string]any{"deploymentId": old.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{}})
				return true
			}
			return false
		})
		if _, err := old.client.syncCommand("sync", ""); err == nil {
			t.Fatal("old service accepted schema1")
		}
		if responsible, err := old.store.syncResponsible(); err != nil || responsible {
			t.Fatal("old capability stranded new store")
		}
		if len(old.calls("/api/sync/device")) != 0 {
			t.Fatal("registered without capability")
		}
	})
	f := newCLISyncFixture(t)
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/bootstrap" {
			cliSyncJSON(w, 200, map[string]any{"deploymentId": f.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{}})
			return true
		}
		return false
	})
	if handled, err := f.client.syncCommand("history", f.conversation); !handled || err == nil {
		t.Fatal("authority fell back to legacy")
	}
	if len(f.calls("/api/conversations/"+f.conversation+"/messages")) != 0 {
		t.Fatal("legacy history route used after responsibility")
	}
}
func TestSyncProtocolRejectsCounterGapUnknownAndWrongOriginalDigest(t *testing.T) {
	for _, value := range []string{"01", "-1", "9223372036854775808"} {
		if _, err := syncCounter(value); err == nil {
			t.Fatal("bad counter accepted")
		}
	}
	if n, err := syncCounter("9007199254740993"); err != nil || n != 9007199254740993 {
		t.Fatal("counter precision lost")
	}
	f := newCLISyncFixture(t)
	delta := f.changes()
	delta["position"] = "3"
	delta["head"] = "3"
	delta["items"] = []any{map[string]string{"kind": "redacted", "from": "2", "to": "3", "rangeDigest": f.permission}}
	raw, _ := json.Marshal(delta)
	if _, err := syncReadChanges(raw, "0", "1", f.permission); err == nil {
		t.Fatal("gap accepted")
	}
	if _, err := syncEnvelope("e30", strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong original digest accepted")
	}
	var malformed map[string]any
	_ = syncDecode(f.message(), &malformed)
	malformed["text"] = nil
	raw, _ = json.Marshal(malformed)
	if _, err := syncValidateMessage(raw, false); err == nil {
		t.Fatal("null text is not a complete proof")
	}
	for _, key := range []string{"contentVersion", "deleted"} {
		_ = syncDecode(f.message(), &malformed)
		delete(malformed, key)
		raw, _ = json.Marshal(malformed)
		if _, err := syncValidateMessage(raw, false); err == nil {
			t.Fatalf("missing %s became full proof", key)
		}
	}
	_ = syncDecode(f.message(), &malformed)
	malformed["type"] = "forward"
	malformed["forwarded"] = true
	raw, _ = json.Marshal(malformed)
	if _, err := syncValidateMessage(raw, false); err != nil {
		t.Fatal("fresh available forward cannot prove original result")
	}
	if _, err := syncValidateProjectionMessage(raw, false); err == nil {
		t.Fatal("non-placeholder forward persisted by sync")
	}
	_ = syncDecode(f.message(), &malformed)
	malformed["deleted"] = true
	raw, _ = json.Marshal(malformed)
	for _, route := range []string{"/api/messages/88888888-8888-4888-8888-888888888888", "/api/conversations/88888888-8888-4888-8888-888888888888/messages", "/api/search?conversationId=88888888-8888-4888-8888-888888888888"} {
		if err := f.client.observeMessageGlobals("GET", route, nil, raw); err == nil {
			t.Fatal("wrong route wrote global")
		}
	}
	var terminal int
	_ = f.store.db.QueryRow("SELECT count(*) FROM sync_terminal").Scan(&terminal)
	if terminal != 0 {
		t.Fatal("wrong route caused irreversible cleanup")
	}
}
func TestSyncBodyAndCursorRollbackTogetherOnSQLFailure(t *testing.T) {
	f := newCLISyncFixture(t)
	if _, err := f.store.db.Exec("CREATE TRIGGER refuse_cursor BEFORE UPDATE ON metadata WHEN NEW.key='sync-control:1' AND json_extract(NEW.value,'$.cursor') IS NOT NULL BEGIN SELECT RAISE(ABORT,'injected cursor failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := f.client.authoritativeSync(); err == nil {
		t.Fatal("injected SQL failure accepted")
	}
	var count int
	_ = f.store.db.QueryRow("SELECT count(*) FROM messages").Scan(&count)
	if count != 0 || f.control().Cursor != "" {
		t.Fatal("body/cursor did not roll back together")
	}
}
func TestSyncCrossDeviceCleanupResumesWithoutNetworkAndQuarantinesUnknownFile(t *testing.T) {
	f := newCLISyncFixture(t)
	otherIdentity := f.identity
	otherIdentity.DeviceID = "88888888-8888-4888-8888-888888888888"
	identityRaw, _ := json.Marshal(otherIdentity)
	oldPath := filepath.Join(f.root, otherIdentity.ConnectionProfile, fingerprint(string(identityRaw)), "client.sqlite")
	old, err := openStore(oldPath, otherIdentity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer old.close()
	unknown, err := openStore(filepath.Join(f.root, otherIdentity.ConnectionProfile, "unproven-directory", "client.sqlite"), otherIdentity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer unknown.close()
	for _, store := range []*localStore{old, unknown} {
		tx, e := store.db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		if _, e = writeCached(tx, "1", f.message()); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	p := f.pending()
	if err = old.enqueue(p); err != nil {
		t.Fatal(err)
	}
	before, _ := old.outbox()
	tx, err := f.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = syncRecordGlobalTx(tx, "1", f.messageID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// Reopen before any drain: the SQLite cleanup intent, not an in-memory event, owns recovery.
	reopened, err := openStore(f.store.path, f.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if err = reopened.drainSyncCleanup(); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = old.db.QueryRow("SELECT count(*) FROM messages WHERE id=?", f.messageID).Scan(&count)
	if count != 0 {
		t.Fatal("proven old device holder survived")
	}
	_ = unknown.db.QueryRow("SELECT count(*) FROM messages WHERE id=?", f.messageID).Scan(&count)
	if count != 1 {
		t.Fatal("unproven filename was modified")
	}
	after, _ := old.outbox()
	if operationDigest(before[p.ClientMsgID]) != operationDigest(after[p.ClientMsgID]) || after[p.ClientMsgID].State != before[p.ClientMsgID].State {
		t.Fatal("global implicitly acknowledged or changed unknown original")
	}
	if len(f.calls("/api/sync/device")) != 0 {
		t.Fatal("cleanup unexpectedly called API")
	}
}
func TestSyncPhysicalGenerationMissingAfterRegistrationFailsClosed(t *testing.T) {
	f := newCLISyncFixture(t)
	if _, err := f.store.ensureSync(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec("DELETE FROM metadata WHERE key='sync-physical-generation'"); err != nil {
		t.Fatal(err)
	}
	if _, err := openStore(f.store.path, f.identity, "1"); err == nil {
		t.Fatal("registered store silently received a new UUID")
	}
	var failure *syncFailure
	if !errors.As(syncRefuse("SYNC_STORE_INVALID"), &failure) {
		t.Fatal("typed failure unavailable")
	}
}
