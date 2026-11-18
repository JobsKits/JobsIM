package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func conversationFixture(id, kind string, members ...string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"id": id, "kind": kind, "memberIds": members, "sendAdmission": "original-local-snapshot", "membershipGeneration": "1"})
	return raw
}
func enqueueFixture(t *testing.T, s *localStore, conversation string) pending {
	t.Helper()
	p := pending{DeploymentID: s.identity.DeploymentID, UserID: s.identity.UserID, ConversationID: conversation, ClientMsgID: "original-" + conversation, Type: "text", Text: "original " + conversation, RestoreEpoch: s.epoch, SendAdmission: "original-token", State: "pending"}
	if err := s.enqueue(p); err != nil {
		t.Fatal(err)
	}
	return p
}
func rawOutbox(t *testing.T, s *localStore, p pending) string {
	t.Helper()
	var raw, digest string
	if err := s.db.QueryRow("SELECT payload,payload_hash FROM outbox WHERE client_msg_id=?", p.ClientMsgID).Scan(&raw, &digest); err != nil {
		t.Fatal(err)
	}
	return raw + "|" + digest
}
func TestGovernanceOriginalOperationsSurviveUnknownResultAndReopen(t *testing.T) {
	for _, kind := range []string{"request", "reopen"} {
		t.Run(kind, func(t *testing.T) {
			database := filepath.Join(t.TempDir(), "client.sqlite")
			s := testStore(t, database, "1")
			defer func() { s.close() }()
			posts := 0
			original := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/bootstrap":
					fmt.Fprint(w, `{"deploymentId":"deployment","restoreEpoch":"1"}`)
				case strings.HasSuffix(r.URL.Path, "/relationship"):
					fmt.Fprint(w, `{"peerId":"peer","friend":false,"blockedByMe":false,"inboundGate":"denied","generation":"9007199254740993123","inboundReopened":false}`)
				case r.Method == "POST":
					posts++
					var payload map[string]string
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
						return
					}
					records, err := s.operations()
					if err != nil || len(records) != 1 || records[0].State != "pending" {
						t.Error("HTTP preceded durable original operation")
					}
					if original == "" {
						original = payload["requestId"]
					} else if original != payload["requestId"] {
						t.Error("unknown result changed original operation ID")
					}
					if (kind == "request" && (len(payload) != 2 || payload["userId"] != "peer")) || (kind == "reopen" && len(payload) != 1) {
						t.Error("unsupported payload field")
					}
					if posts == 1 {
						w.WriteHeader(503)
						fmt.Fprint(w, `{"error":{"code":"UNKNOWN","message":"result lost"}}`)
						return
					}
					if kind == "request" {
						fmt.Fprint(w, `{"id":"request-fact","state":"pending"}`)
					} else {
						fmt.Fprint(w, `{"status":"opened","current":false,"generation":"1"}`)
					}
				default:
					t.Error("unexpected route")
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			c := &client{base: server.URL, http: server.Client(), store: s, deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", epoch: "1"}
			if err := c.command(kind, "peer"); err == nil {
				t.Fatal("unknown operation reported success")
			}
			records, err := s.operations()
			if err != nil || len(records) != 1 || records[0].State != "pending" {
				t.Fatal("original operation not retained")
			}
			hash := records[0].PayloadHash
			s.close()
			s = testStore(t, database, "1")
			c.store = s
			if err = c.command("resume", original); err != nil {
				t.Fatal(err)
			}
			op, err := s.operation(original)
			if err != nil || op.State != "completed" || op.PayloadHash != hash {
				t.Fatal("original operation changed after reopen")
			}
			if err = c.command("resume", original); err != nil || posts != 2 {
				t.Fatal("completed original operation was sent again")
			}
			s.close()
			s = testStore(t, database, "2")
			c.store = s
			c.epoch = "2"
			if _, err = s.prepareOperation(kind, "another-peer"); err != nil {
				t.Fatal(err)
			}
			s.close()
			s = testStore(t, database, "3")
			c.store = s
			c.epoch = "3"
			all, _ := s.operations()
			var frozen governanceOperation
			for _, item := range all {
				if item.PeerID == "another-peer" {
					frozen = item
				}
			}
			if frozen.State != "frozen" || frozen.ErrorCode != "RESTORE_EPOCH_CHANGED" {
				t.Fatal("old governance intent survived restore as sendable")
			}
			if err = c.command("resume", frozen.ID); err == nil || posts != 2 {
				t.Fatal("old governance intent reached HTTP")
			}
		})
	}
}

func TestRelationshipFreezeTargetsOnlyCorrespondingDirectAndKeepsPayloadBytes(t *testing.T) {
	s := testStore(t, filepath.Join(t.TempDir(), "client.sqlite"), "1")
	defer s.close()
	target := enqueueFixture(t, s, "direct-peer")
	other := enqueueFixture(t, s, "direct-other")
	group := enqueueFixture(t, s, "group-peer")
	original := rawOutbox(t, s, target)
	fresh := []json.RawMessage{conversationFixture("direct-peer", "direct", "66666666-6666-4666-8666-666666666666", "peer"), conversationFixture("direct-other", "direct", "66666666-6666-4666-8666-666666666666", "another"), conversationFixture("group-peer", "group", "66666666-6666-4666-8666-666666666666", "peer")}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/conversations" {
			json.NewEncoder(w).Encode(map[string]any{"items": fresh})
			return
		}
		calls++
		items, _ := s.outbox()
		if items[target.ClientMsgID].State != "frozen" || items[target.ClientMsgID].FreezeReason != "CONTACT_REMOVED" {
			t.Error("relationship HTTP preceded local freeze")
		}
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"code":"UNKNOWN","message":"lost"}}`)
	}))
	defer server.Close()
	c := &client{base: server.URL, http: server.Client(), store: s, deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", epoch: "1"}
	if err := c.command("remove-contact", "peer"); err == nil || calls != 1 {
		t.Fatal("unknown DELETE was replayed or reported successful")
	}
	items, _ := s.outbox()
	if items[other.ClientMsgID].State != "pending" || items[group.ClientMsgID].State != "pending" {
		t.Fatal("unrelated direct or group was frozen")
	}
	if rawOutbox(t, s, target) != original {
		t.Fatal("freeze changed original payload bytes/hash/token/ID")
	}
	if err := s.prepareRelationshipRevocation("another", "PEER_BLOCKED", fresh); err != nil {
		t.Fatal(err)
	}
	items, _ = s.outbox()
	if items[other.ClientMsgID].FreezeReason != "PEER_BLOCKED" || items[target.ClientMsgID].FreezeReason != "CONTACT_REMOVED" {
		t.Fatal("distinct governance reasons overwritten")
	}
}

func TestLeavePurgesAtomicallyBeforeUnknownDeleteAndReconcileCannotRefillHistory(t *testing.T) {
	for _, command := range []string{"leave", "remove-member"} {
		t.Run(command, func(t *testing.T) {
			s := testStore(t, filepath.Join(t.TempDir(), "client.sqlite"), "1")
			defer s.close()
			target := enqueueFixture(t, s, "99999999-9999-4999-8999-999999999999")
			other := enqueueFixture(t, s, "other-group")
			before := rawOutbox(t, s, target)
			for _, p := range []pending{target, other} {
				if err := s.savePage(p.ConversationID, "1:9007199254740993123", []json.RawMessage{authoritative("cached-"+p.ConversationID, "cached-client", p.ConversationID, "1")}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.draft("99999999-9999-4999-8999-999999999999", "retain my private draft"); err != nil {
				t.Fatal(err)
			}
			generation := "9007199254740993123456789"
			var current map[string]any
			json.Unmarshal(conversationFixture("99999999-9999-4999-8999-999999999999", "group", "66666666-6666-4666-8666-666666666666", "peer"), &current)
			current["membershipGeneration"] = generation
			groupSnapshot, _ := json.Marshal(current)
			fresh := []json.RawMessage{groupSnapshot, conversationFixture("other-group", "group", "66666666-6666-4666-8666-666666666666", "peer")}
			deletes := 0
			posts := 0
			lookups := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/conversations" {
					json.NewEncoder(w).Encode(map[string]any{"items": fresh})
					return
				}
				if r.URL.Path == "/api/bootstrap" {
					fmt.Fprint(w, `{"deploymentId":"deployment","restoreEpoch":"1","features":{"send_admission":true}}`)
					return
				}
				if r.Method == "DELETE" {
					deletes++
					if r.URL.Query().Get("membershipGeneration") != generation {
						t.Error("original generation replaced or rounded")
					}
					items, _ := s.outbox()
					history, _ := s.history("99999999-9999-4999-8999-999999999999")
					cursor, _ := s.cursor("99999999-9999-4999-8999-999999999999")
					if items[target.ClientMsgID].FreezeReason != "CONVERSATION_LEFT" || len(history) != 0 || cursor != "0" {
						t.Error("DELETE preceded atomic freeze/history withdrawal")
					}
					w.WriteHeader(503)
					fmt.Fprint(w, `{"error":{"code":"UNKNOWN","message":"lost"}}`)
					return
				}
				if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/messages") {
					json.NewEncoder(w).Encode(map[string]any{"items": []json.RawMessage{authoritative("original-success", target.ClientMsgID, "99999999-9999-4999-8999-999999999999", "1")}, "cursor": "1:7"})
					return
				}
				if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/message-results") {
					lookups++
					var q map[string]string
					json.NewDecoder(r.Body).Decode(&q)
					if q["clientMsgId"] != target.ClientMsgID || q["sendAdmission"] != target.SendAdmission {
						t.Error("explicit reconciliation changed original")
					}
					w.Write(authoritative("original-success", target.ClientMsgID, "99999999-9999-4999-8999-999999999999", "1"))
					return
				}
				if r.Method == "POST" {
					posts++
				}
				t.Error("unexpected route")
				w.WriteHeader(404)
			}))
			defer server.Close()
			c := &client{base: server.URL, http: server.Client(), store: s, deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", epoch: "1"}
			args := "99999999-9999-4999-8999-999999999999 " + generation
			if command == "remove-member" {
				args = "99999999-9999-4999-8999-999999999999 66666666-6666-4666-8666-666666666666 " + generation
			}
			if err := c.command(command, args); err == nil || deletes != 1 {
				t.Fatal("unknown member DELETE automatically replayed")
			}
			if rawOutbox(t, s, target) != before {
				t.Fatal("leave altered original payload bytes or hash")
			}
			items, _ := s.outbox()
			history, _ := s.history("other-group")
			drafts, _ := s.drafts()
			if items[other.ClientMsgID].State != "pending" || len(history) != 1 || drafts["99999999-9999-4999-8999-999999999999"] != "retain my private draft" {
				t.Fatal("leave affected another group or lost draft")
			}
			if err := c.retry(target.ClientMsgID, false); err == nil || posts != 0 {
				t.Fatal("ordinary retry revived departed intent")
			}
			if err := c.retry(target.ClientMsgID, true); err != nil || posts != 0 || lookups != 1 {
				t.Fatal("original result was not checked using read-only original-body route")
			}
			history, _ = s.history("99999999-9999-4999-8999-999999999999")
			cursor, _ := s.cursor("99999999-9999-4999-8999-999999999999")
			if len(history) != 0 || cursor != "0" {
				t.Fatal("reconciliation refilled withdrawn local history")
			}
		})
	}
}

func TestLeaveFailureRollsBackFreezeHistoryAndCursorTogether(t *testing.T) {
	s := testStore(t, filepath.Join(t.TempDir(), "client.sqlite"), "1")
	defer s.close()
	p := enqueueFixture(t, s, "group")
	before := rawOutbox(t, s, p)
	if err := s.savePage("group", "1:5", []json.RawMessage{authoritative("message", "original", "group", "1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_cursor_delete BEFORE DELETE ON cursors BEGIN SELECT RAISE(ABORT,'injected rollback');END`); err != nil {
		t.Fatal(err)
	}
	if err := s.prepareLeave("group", "1", []json.RawMessage{conversationFixture("group", "group", "66666666-6666-4666-8666-666666666666", "peer")}); err == nil {
		t.Fatal("fault did not abort transaction")
	}
	items, _ := s.outbox()
	history, _ := s.history("group")
	cursor, _ := s.cursor("group")
	if items[p.ClientMsgID].State != "pending" || len(history) != 1 || cursor != "1:5" || rawOutbox(t, s, p) != before {
		t.Fatal("leave transaction partially committed")
	}
}

func TestGroupCommandsKeepExactContractAndDoNotReplayUnknownPatch(t *testing.T) {
	s := testStore(t, filepath.Join(t.TempDir(), "client.sqlite"), "1")
	defer s.close()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]string
		if r.Method == "PATCH" || r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&body)
		}
		switch r.Method {
		case "PATCH":
			if len(body) != 1 || (body["title"] != "New title with spaces" && body["ownerId"] != "peer") {
				t.Error("PATCH has unsupported or altered fields")
			}
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"code":"UNKNOWN","message":"lost"}}`)
		case "POST":
			if len(body) != 1 || body["userId"] != "peer" {
				t.Error("member add payload changed")
			}
			fmt.Fprint(w, `{"status":"added"}`)
		case "DELETE":
			if r.URL.Query().Get("membershipGeneration") != "9007199254740993123456" {
				t.Error("member original generation changed")
			}
			fmt.Fprint(w, `{"status":"removed"}`)
		case "GET":
			fmt.Fprint(w, `{"items":[]}`)
		default:
			t.Error("unexpected method")
		}
	}))
	defer server.Close()
	c := &client{base: server.URL, http: server.Client(), store: s, deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", epoch: "1"}
	for _, command := range []struct {
		cmd, args string
		fail      bool
	}{{"members", "group", false}, {"add-member", "group peer", false}, {"remove-member", "group peer 9007199254740993123456", false}, {"rename", "group New title with spaces", true}, {"transfer", "group peer", true}} {
		before := calls
		err := c.command(command.cmd, command.args)
		expectedCalls := 1
		if command.cmd == "transfer" {
			expectedCalls = 0
		}
		if (err != nil) != command.fail || calls != before+expectedCalls {
			t.Fatal("command contract failed or unknown write replayed")
		}
	}
	before := calls
	for _, args := range []string{"group peer", "group peer 0", "group peer 1.5"} {
		if err := c.command("remove-member", args); err == nil {
			t.Fatal("missing/invalid original generation accepted")
		}
	}
	if calls != before {
		t.Fatal("invalid original generation reached HTTP")
	}
}

func TestSchemaOneMigrationPreservesPendingAndAddsIndependentFreezeReason(t *testing.T) {
	file := filepath.Join(t.TempDir(), "client.sqlite")
	s := testStore(t, file, "1")
	p := enqueueFixture(t, s, "group")
	before := rawOutbox(t, s, p)
	if _, err := s.db.Exec("DROP TABLE governance_operations; ALTER TABLE outbox DROP COLUMN freeze_reason; PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	s.close()
	s = testStore(t, file, "1")
	defer s.close()
	var version int
	s.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 4 || rawOutbox(t, s, p) != before {
		t.Fatal("schema migration altered original outbox")
	}
	if err := s.prepareRelationshipRevocation("peer", "PEER_BLOCKED", []json.RawMessage{conversationFixture("group", "direct", "66666666-6666-4666-8666-666666666666", "peer")}); err != nil {
		t.Fatal(err)
	}
	items, _ := s.outbox()
	if items[p.ClientMsgID].FreezeReason != "PEER_BLOCKED" || rawOutbox(t, s, p) != before {
		t.Fatal("reason storage rewrote original payload")
	}
}

func TestUnknownLocalRevocationCannotCreateAnOldIntentDuringReconciliation(t *testing.T) {
	s := testStore(t, filepath.Join(t.TempDir(), "client.sqlite"), "1")
	defer s.close()
	p := enqueueFixture(t, s, "direct")
	if err := s.prepareRelationshipRevocation("peer", "CONTACT_REMOVED", []json.RawMessage{conversationFixture("direct", "direct", "66666666-6666-4666-8666-666666666666", "peer")}); err != nil {
		t.Fatal(err)
	}
	posts := 0
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/bootstrap" {
			fmt.Fprint(w, `{"deploymentId":"deployment","restoreEpoch":"1","features":{"send_admission":true}}`)
			return
		}
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/message-results") {
			reads++
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":{"code":"MESSAGE_RESULT_NOT_FOUND"}}`)
			return
		}
		if r.Method == "POST" {
			posts++
			w.Write(authoritative("would-create-old-intent", p.ClientMsgID, "direct", "1"))
			return
		}
		reads++
		fmt.Fprint(w, `{"items":[],"cursor":"1:0"}`)
	}))
	defer server.Close()
	c := &client{base: server.URL, http: server.Client(), store: s, deployment: "deployment", user: "66666666-6666-4666-8666-666666666666", epoch: "1"}
	if err := c.retry(p.ClientMsgID, true); err == nil || posts != 0 || reads == 0 {
		t.Fatal("read-only reconciliation created an old intent")
	}
	items, _ := s.outbox()
	if items[p.ClientMsgID].State != "frozen" || items[p.ClientMsgID].SendAdmission != p.SendAdmission {
		t.Fatal("unknown result revived original qualification")
	}
}
