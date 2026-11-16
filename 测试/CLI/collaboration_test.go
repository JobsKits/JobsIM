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
	"testing"
)

func collaborationStore(t *testing.T, origin, path, epoch string) *localStore {
	t.Helper()
	identity := identityForTest()
	identity.Origin = origin
	s, e := openStore(path, identity, epoch)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func captureCollaboration(t *testing.T, run func() error) (string, error) {
	t.Helper()
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	old := os.Stdout
	os.Stdout = w
	err := run()
	os.Stdout = old
	w.Close()
	raw, e := io.ReadAll(r)
	r.Close()
	if e != nil {
		t.Fatal(e)
	}
	return string(raw), err
}
func TestCollaborationPayloadCompatibilityAndDurableMentionQualification(t *testing.T) {
	old := pending{ConversationID: "g", ClientMsgID: "old", Type: "text", Text: "legacy", RestoreEpoch: "1", SendAdmission: "old-token"}
	original, _ := json.Marshal(map[string]any{"conversationId": "g", "payload": map[string]string{"clientMsgId": "old", "type": "text", "text": "legacy", "restoreEpoch": "1", "sendAdmission": "old-token"}})
	if operationDigest(old) != fingerprint(string(original)) {
		t.Fatal("old JSON digest changed when adding typed fields")
	}
	var s *localStore
	posts := 0
	role := "1"
	sa := "original-token"
	denied := ""
	feature := true
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/bootstrap":
			fmt.Fprintf(w, `{"deploymentId":"deployment","restoreEpoch":"1","features":{"send_admission":true,"mentions":true,"role_admission":%t}}`, feature)
		case r.Method == "GET" && r.URL.Path == "/api/conversations/g":
			fmt.Fprintf(w, `{"id":"g","kind":"group","ownerId":"66666666-6666-4666-8666-666666666666","sendAdmission":%q,"membershipGeneration":"1","roleGeneration":%q}`, sa, role)
		case strings.HasSuffix(r.URL.Path, "/members"):
			fmt.Fprint(w, `{"items":[{"id":"peer","state":"active"}]}`)
		case strings.HasSuffix(r.URL.Path, "/message-results"):
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":{"code":"MESSAGE_RESULT_NOT_FOUND"}}`)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/messages"):
			posts++
			raw, _ := io.ReadAll(r.Body)
			bodies = append(bodies, raw)
			var body map[string]any
			json.Unmarshal(raw, &body)
			id, _ := body["clientMsgId"].(string)
			items, e := s.outbox()
			if e != nil || items[id].SendAdmission != "original-token" || items[id].RoleGeneration != "1" || !items[id].MentionAll {
				t.Error("HTTP preceded original role/target durability")
			}
			if denied != "" {
				w.WriteHeader(409)
				fmt.Fprintf(w, `{"error":{"code":%q}}`, denied)
			} else {
				w.WriteHeader(503)
				fmt.Fprint(w, `{"error":{"code":"MENTION_POLICY_INVALID"}}`)
			}
		default:
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "client.sqlite")
	s = collaborationStore(t, server.URL, path, "1")
	defer func() { s.close() }()
	c := &client{base: server.URL, http: server.Client(), store: s, user: "66666666-6666-4666-8666-666666666666", deployment: "deployment", epoch: "1"}
	if e := c.command("mention", "g all | owner announcement mention"); e == nil {
		t.Fatal("unknown write reported success")
	}
	items, _ := s.outbox()
	if len(items) != 1 {
		t.Fatal("mention not retained")
	}
	var p pending
	for _, q := range items {
		p = q
	}
	before := rawOutbox(t, s, p)
	s.close()
	s = collaborationStore(t, server.URL, path, "1")
	c.store = s
	role, sa = "3", "new-token"
	feature = false
	if e := c.retry(p.ClientMsgID, false); e == nil || posts != 1 {
		t.Fatal("missing role flag allowed old @all creation")
	}
	if rawOutbox(t, s, p) != before {
		t.Fatal("capability check changed original payload")
	}
	feature = true
	denied = "SEND_ADMISSION_CHANGED"
	if e := c.retry(p.ClientMsgID, false); e == nil {
		t.Fatal("stale role intent accepted")
	}
	items, _ = s.outbox()
	if items[p.ClientMsgID].State != "frozen" || rawOutbox(t, s, p) != before {
		t.Fatal("original stale mention not frozen without mutation")
	}
	if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatal("old @all body/proof changed")
	}
	count := posts
	if e := c.retry(p.ClientMsgID, false); e == nil || posts != count {
		t.Fatal("frozen @all retransmitted")
	}
	if e := c.command("mention", "g peer,peer | invalid"); e == nil || posts != count {
		t.Fatal("duplicate targets accepted")
	}
}
func TestCollaborationSignedPagingExactSourceAndContextFence(t *testing.T) {
	requests := 0
	cursor := "opaque+/= %汉"
	deny := false
	version := "2"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case r.URL.Path == "/api/bootstrap":
			fmt.Fprint(w, `{"deploymentId":"deployment","restoreEpoch":"1"}`)
		case r.URL.Path == "/api/search" || r.URL.Path == "/api/favorites":
			if r.URL.Query().Get("limit") != "50" || r.URL.Query().Get("conversationId") != "beyond100" {
				t.Error("paging condition lost")
			}
			if r.URL.Path == "/api/search" && r.URL.Query().Get("q") != "hello 世界" {
				t.Error("query altered")
			}
			if r.URL.Query().Get("cursor") == "" {
				fmt.Fprintf(w, `{"items":[{"id":"m1","conversationId":"beyond100","forwardAdmission":"temporary-proof","text":"old list body"}],"cursor":%q,"hasMore":true}`, cursor)
			} else {
				if r.URL.Query().Get("cursor") != cursor {
					t.Error("opaque cursor changed")
				}
				fmt.Fprint(w, `{"items":[{"id":"m2","conversationId":"beyond100"}],"cursor":"end","hasMore":false}`)
			}
		case r.URL.Path == "/api/conversations/beyond100":
			if deny {
				w.WriteHeader(403)
				fmt.Fprint(w, `{"error":{"code":"HISTORY_NOT_ALLOWED"}}`)
			} else {
				fmt.Fprint(w, `{"id":"beyond100","kind":"group"}`)
			}
		case r.URL.Path == "/api/messages/m1":
			fmt.Fprintf(w, `{"id":"m1","conversationId":"beyond100","contentVersion":%q,"text":"fresh exact body","forwardAdmission":"temporary-proof"}`, version)
		default:
			t.Error("exact source must not use first directory page")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	s := collaborationStore(t, server.URL, filepath.Join(t.TempDir(), "c.sqlite"), "1")
	defer s.close()
	c := &client{base: server.URL, http: server.Client(), store: s, user: "66666666-6666-4666-8666-666666666666", deployment: "deployment", epoch: "1"}
	out, e := captureCollaboration(t, func() error { return c.command("search", "hello 世界 | beyond100") })
	if e != nil || strings.Contains(out, "temporary-proof") {
		t.Fatal("search failed or exposed temporary proof")
	}
	if e = c.command("more", ""); e != nil {
		t.Fatal(e)
	}
	out, e = captureCollaboration(t, func() error { return c.command("open-result", "m1") })
	if e != nil || !strings.Contains(out, "fresh exact body") || strings.Contains(out, "old list body") || strings.Contains(out, "temporary-proof") {
		t.Fatal("exact source reused a list body or leaked proof")
	}
	if e = c.command("favorites", "beyond100"); e != nil {
		t.Fatal(e)
	}
	deny = true
	if e = c.command("open-result", "m1"); e == nil || c.lookup != nil {
		t.Fatal("ACL denial retained old result session")
	}
	deny = false
	if e = c.command("favorites", "beyond100"); e != nil {
		t.Fatal(e)
	}
	before := requests
	c.user = "another"
	if e = c.command("more", ""); e == nil || requests != before {
		t.Fatal("cross-account cursor reused")
	}
	c.user = "66666666-6666-4666-8666-666666666666"
	before = requests
	if e = c.command("search", strings.Repeat("藏", 86)); e == nil || requests != before {
		t.Fatal("UTF8 oversized search sent")
	}
}
func TestAnnouncementDurableOriginalFactAndNoBodyRollback(t *testing.T) {
	var s *localStore
	writes := 0
	found := false
	currentVersion := "0"
	originalID := ""
	originalBody := ""
	role := "1"
	var c *client
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/bootstrap":
			fmt.Fprint(w, `{"deploymentId":"deployment","restoreEpoch":"1","features":{"announcements":true,"role_admission":true}}`)
		case r.URL.Path == "/api/conversations/g":
			fmt.Fprintf(w, `{"id":"g","kind":"group","ownerId":"66666666-6666-4666-8666-666666666666","membershipGeneration":"1","roleGeneration":%q}`, role)
		case r.URL.Path == "/api/messages/source":
			fmt.Fprint(w, `{"id":"source","conversationId":"g","version":"99","contentVersion":"2","text":"must not be persisted"}`)
		case strings.Contains(r.URL.Path, "/operations/"):
			if !found {
				w.WriteHeader(404)
				fmt.Fprint(w, `{"error":{"code":"NOT_FOUND"}}`)
			} else {
				fmt.Fprintf(w, `{"operationId":%q,"status":"applied","current":false,"result":{"conversationId":"g","version":"1","state":"active","messageId":"source","sourceVersion":"2","message":{"text":"forbidden retained copy"}}}`, originalID)
			}
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/announcement"):
			fmt.Fprintf(w, `{"conversationId":"g","version":%q,"state":"empty","canManage":true}`, currentVersion)
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/announcement"):
			writes++
			raw, _ := io.ReadAll(r.Body)
			var body map[string]string
			json.Unmarshal(raw, &body)
			originalID = body["requestId"]
			ops, e := s.announcementOperations()
			if e != nil || len(ops) != 1 || ops[0].State != "pending" || string(ops[0].Payload) != string(raw) || body["sourceVersion"] != "2" || body["roleGeneration"] != "1" {
				t.Error("announcement HTTP preceded exact durable reference")
			}
			if strings.Contains(string(raw), "must not") {
				t.Error("announcement copied source text")
			}
			originalBody = string(raw)
			found = true
			currentVersion = "4"
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"code":"RESULT_UNKNOWN"}}`)
		default:
			t.Error("unexpected announcement route")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "c.sqlite")
	s = collaborationStore(t, server.URL, path, "1")
	defer func() { s.close() }()
	c = &client{base: server.URL, http: server.Client(), store: s, user: "66666666-6666-4666-8666-666666666666", deployment: "deployment", epoch: "1"}
	if e := c.command("announce", "g source 2 1d"); e == nil {
		t.Fatal("lost response reported success")
	}
	op, e := s.announcementOperation(originalID)
	if e != nil || op.State != "pending" {
		t.Fatal("unknown original not pending")
	}
	hash := op.Hash
	s.close()
	s = collaborationStore(t, server.URL, path, "1")
	c.store = s
	role = "3"
	output, e := captureCollaboration(t, func() error { return c.command("resume-announcement", originalID) })
	if e != nil || writes != 1 || !strings.Contains(output, `"version": "4"`) || strings.Contains(output, "forbidden retained copy") {
		t.Fatal("old fact rewrote current announcement")
	}
	op, e = s.announcementOperation(originalID)
	if e != nil || op.State != "completed" || op.Hash != hash || string(op.Payload) != originalBody || strings.Contains(string(op.Result), "forbidden retained copy") {
		t.Fatal("original reference changed or fact persisted body")
	}
	if e = c.command("resume-announcement", originalID); e != nil || writes != 1 {
		t.Fatal("completed operation replayed")
	}
}
func TestAnnouncementCASFreezeEpochAndTransactionRollback(t *testing.T) {
	writes := 0
	roleFlag := true
	rejection := "QUALIFICATION_CHANGED"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/bootstrap":
			fmt.Fprintf(w, `{"deploymentId":"deployment","restoreEpoch":"1","features":{"announcements":true,"role_admission":%t}}`, roleFlag)
		case strings.Contains(r.URL.Path, "/operations/"):
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":{"code":"NOT_FOUND"}}`)
		case r.Method == "PUT":
			writes++
			w.WriteHeader(409)
			fmt.Fprintf(w, `{"error":{"code":%q}}`, rejection)
		default:
			t.Error("unexpected write fallback")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "c.sqlite")
	s := collaborationStore(t, server.URL, path, "1")
	defer func() { s.close() }()
	c := &client{base: server.URL, http: server.Client(), store: s, user: "66666666-6666-4666-8666-666666666666", deployment: "deployment", epoch: "1"}
	makeOp := func(id, conv string, legacy bool) announcementOperation {
		body := map[string]string{"requestId": id, "messageId": "source", "sourceVersion": "1", "expectedVersion": "0", "membershipGeneration": "1"}
		if !legacy {
			body["roleGeneration"] = "1"
		}
		raw, _ := json.Marshal(body)
		return announcementOperation{ID: id, Conversation: conv, Epoch: "1", Method: "PUT", Payload: raw, State: "pending"}
	}
	invalid := makeOp("body-copy", "invalid", false)
	var copied map[string]string
	json.Unmarshal(invalid.Payload, &copied)
	copied["text"] = "forbidden source body"
	invalid.Payload, _ = json.Marshal(copied)
	if _, err := s.prepareAnnouncement(invalid); err == nil {
		t.Fatal("source body entered durable announcement operation")
	}
	op, e := s.prepareAnnouncement(makeOp("original", "g", false))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.prepareAnnouncement(makeOp("replacement", "g", false)); e == nil {
		t.Fatal("unknown original was replaced")
	}
	if e = c.resumeAnnouncement(op.ID); e == nil {
		t.Fatal("old role accepted")
	}
	saved, _ := s.announcementOperation(op.ID)
	if saved.State != "frozen" || saved.ErrorCode != "QUALIFICATION_CHANGED" || saved.Hash != op.Hash {
		t.Fatal("qualification did not freeze original")
	}
	if e = c.resumeAnnouncement(op.ID); e == nil || writes != 1 {
		t.Fatal("frozen old role replayed")
	}
	legacy, e := s.prepareAnnouncement(makeOp("legacy", "other", true))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.resumeAnnouncement(legacy.ID); e == nil || writes != 1 {
		t.Fatal("legacy missing role gained latest qualification")
	}
	roleFlag = false
	fresh, e := s.prepareAnnouncement(makeOp("no-flag", "third", false))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.resumeAnnouncement(fresh.ID); e == nil || writes != 1 {
		t.Fatal("missing protection allowed PUT")
	}
	roleFlag = true
	rejection = "INVALID_EXPIRY"
	expired, e := s.prepareAnnouncement(makeOp("expired", "expiry", false))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.resumeAnnouncement(expired.ID); e == nil {
		t.Fatal("expired original intent reported success")
	}
	expiryState, _ := s.announcementOperation(expired.ID)
	if expiryState.State != "frozen" || expiryState.Hash != expired.Hash {
		t.Fatal("expired original was not frozen with unchanged body")
	}
	expiryWrites := writes
	if e = c.resumeAnnouncement(expired.ID); e == nil || writes != expiryWrites {
		t.Fatal("expired original was replayed")
	}
	if _, e = s.db.Exec(`CREATE TRIGGER fail_announcement BEFORE INSERT ON announcement_operations WHEN NEW.id='fail' BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.prepareAnnouncement(makeOp("fail", "fourth", false)); e == nil {
		t.Fatal("SQL failure lost")
	}
	var n int
	s.db.QueryRow("SELECT count(*) FROM announcement_operations WHERE id='fail'").Scan(&n)
	if n != 0 {
		t.Fatal("transaction left partial operation")
	}
	s.close()
	s = collaborationStore(t, server.URL, path, "2")
	frozen, _ := s.announcementOperation(legacy.ID)
	if frozen.State != "frozen" || frozen.ErrorCode != "RESTORE_EPOCH_CHANGED" {
		t.Fatal("old epoch remained sendable")
	}
}
