// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func cliS2Hook(f *cliSyncFixture) func(http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/bootstrap":
			cliSyncJSON(w, 200, map[string]any{"deploymentId": f.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{"authoritative_sync": true, "sync_jetstream": true, "sync_self_state": true, "cloud_drafts": true, "send_admission": true}})
			return true
		case "/api/sync/device":
			v := f.binding()
			v["schemaVersion"] = "2"
			v["qualification"] = "transport_only"
			v["qualifiedAck"] = false
			cliSyncJSON(w, 200, v)
			return true
		case "/api/sync/context":
			v := f.binding()
			v["head"] = "0"
			v["permissionGeneration"] = f.permission
			v["qualification"] = "transport_only"
			v["qualifiedAck"] = false
			v["afterReceiptEnabled"] = false
			v["coverage"] = map[string]string{"schemaVersion": "2"}
			cliSyncJSON(w, 200, v)
			return true
		case "/api/sync/snapshots":
			v := f.manifest()
			v["schemaVersion"] = "2"
			v["ownerWatermarks"].(map[string]string)["sync"] = "1"
			cliSyncJSON(w, 201, v)
			return true
		}
		return false
	}
}
func cliS2Fixture(t *testing.T) *cliSyncFixture {
	f := newCLISyncFixture(t)
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	f.stream = "88888888-8888-4888-8888-888888888888"
	f.snapshot = "99999999-9999-4999-8999-999999999999"
	f.setHook(cliS2Hook(f))
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	if f.control().schema() != "2" {
		t.Fatal("ordinary sync failed automatic negotiation")
	}
	return f
}
func cliSelfOperation(f *cliSyncFixture, kind, method, route string, fields map[string]any) selfOperation {
	request := id()
	fields["requestId"] = request
	fields["revisionEpoch"] = "1"
	body, _ := json.Marshal(fields)
	op := selfOperation{ID: request, Kind: kind, Conversation: f.conversation, Epoch: "1", Method: method, Route: route, Body: string(body), Created: time.Now().UTC().Format(time.RFC3339Nano)}
	if kind == "font" {
		op.Conversation = ""
	}
	op.Hash = selfOperationHash(op)
	return op
}
func TestS2AutomaticTransitionArchivesStreamFactsAndPreservesPhysicalIdentity(t *testing.T) {
	f := newCLISyncFixture(t)
	if err := f.store.enqueue(f.pending()); err != nil {
		t.Fatal(err)
	}
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	old := f.control()
	if _, err := f.store.db.Exec("INSERT INTO sync_events VALUES('1','prior-origin','{\"old\":true}')"); err != nil {
		t.Fatal(err)
	}
	f.stream = "88888888-8888-4888-8888-888888888888"
	f.snapshot = "99999999-9999-4999-8999-999999999999"
	f.setHook(cliS2Hook(f))
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	current := f.control()
	if current.schema() != "2" || current.StreamID == old.StreamID || current.StoreGeneration != old.StoreGeneration || current.RegistrationRequestID == old.RegistrationRequestID || f.store.identity.StoreGeneration != strings.Repeat("1", 32) {
		t.Fatal("schema transition altered identity or reused old nonce/stream")
	}
	var count int
	if err := f.store.db.QueryRow("SELECT count(*) FROM sync_stream_archive WHERE stream=? AND kind='sync_events' AND id='prior-origin'", old.StreamID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("old fact not archived %d %v", count, err)
	}
	queue, err := f.store.outbox()
	pending := queue[f.nonce]
	if err != nil || pending.ClientMsgID != f.nonce || pending.Text != f.pending().Text {
		t.Fatalf("original changed %v", err)
	}
	if _, err = f.store.selectSyncSchema(current, "1"); err == nil {
		t.Fatal("downgrade accepted")
	}
}
func TestS2SchemaTransitionFailureRollsArchiveAndNonceBackTogether(t *testing.T) {
	f := newCLISyncFixture(t)
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	before := f.control()
	if _, err := f.store.db.Exec("CREATE TRIGGER refuse_schema_delete BEFORE DELETE ON sync_refs BEGIN SELECT RAISE(ABORT,'fault schema transition'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.selectSyncSchema(before, "2"); err == nil {
		t.Fatal("expected transaction failure")
	}
	if err := syncCheckControl(f.control(), before); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.store.db.QueryRow("SELECT count(*) FROM sync_stream_archive").Scan(&count); err != nil || count != 0 {
		t.Fatal("archive partially published")
	}
}
func TestS2RefAndObjectCountersAreIndependentAndOwnScopeIsRequired(t *testing.T) {
	f := cliS2Fixture(t)
	control := f.control()
	yes := true
	raw, _ := json.Marshal(selfPreferences{selfScope: selfScope{ScopeID: control.ScopeID, Source: "personal", User: control.PrincipalID, Epoch: "1", Version: "3"}, Font: "large", Explicit: &yes})
	ref := syncRef{Owner: "directory", Kind: "preferences", ID: control.PrincipalID, SubjectUserID: control.PrincipalID, Revision: "99", RevisionEpoch: "1", State: "active"}
	p := syncProjection{Ref: ref, State: "available", CurrentRevision: "99", Object: raw}
	if _, err := validateSelfProjection(p, control); err != nil {
		t.Fatal(err)
	}
	p.Ref.SubjectUserID = f.conversation
	if _, err := validateSelfProjection(p, control); err == nil {
		t.Fatal("foreign personal metadata accepted")
	}
	if err := ref.schemaCheck("1"); err == nil {
		t.Fatal("S1 accepted S2 reference")
	}
}
func TestS2DraftDefaultEmptyAndDurableProjectionContainNoTemporaryPermission(t *testing.T) {
	f := cliS2Fixture(t)
	control := f.control()
	yes := true
	empty := selfDraft{selfScope: selfScope{ID: f.conversation, ScopeID: control.ScopeID, Source: "personal", User: control.PrincipalID, Conversation: f.conversation, Epoch: "1", Version: "0"}, ContentState: "empty", ReplyState: "none", Writable: &yes, Admission: "secret-da1"}
	if err := selfDraftValid(empty); err != nil {
		t.Fatalf("default ownedText=null rejected: %v", err)
	}
	body, _ := json.Marshal(empty)
	p := syncProjection{Ref: syncRef{Owner: "sync", Kind: "draft", ID: f.conversation, SubjectUserID: control.PrincipalID, ConversationID: f.conversation, Revision: "20", RevisionEpoch: "1", State: "active"}, State: "available", CurrentRevision: "20", Object: body}
	saved, err := validateSelfProjection(p, control)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "secret-da1") {
		t.Fatal("draft authorization stored in projection")
	}
	display := map[string]any{"replyAdmission": "dr1-private", "draftAdmission": "da1-private", "nested": map[string]any{"replyAdmission": "nested-private"}}
	projectUnavailable(display)
	encoded, _ := json.Marshal(display)
	if strings.Contains(string(encoded), "private") {
		t.Fatal("temporary draft/reply permission exposed in ordinary output")
	}
	message := f.message()
	var object map[string]any
	_ = json.Unmarshal(message, &object)
	object["replyAdmission"] = "dr1-private"
	message, _ = json.Marshal(object)
	persisted, err := syncPersistable(message, false)
	if err != nil || strings.Contains(string(persisted), "dr1-private") {
		t.Fatal("temporary reply permission persisted with message")
	}
}
func TestS2OriginalPreparedDraftResumesByReadonlyFactWithoutReplacingLocalInput(t *testing.T) {
	f := cliS2Fixture(t)
	if err := f.store.draft(f.conversation, "newer independent local text"); err != nil {
		t.Fatal(err)
	}
	route := "/api/conversations/" + f.conversation + "/draft"
	op := cliSelfOperation(f, "draft", "PUT", route, map[string]any{"expectedVersion": "0", "draftAdmission": "original-da1", "ownedText": "original owned text", "reply": nil})
	if err := f.store.saveSelfOperation(op); err != nil {
		t.Fatal(err)
	}
	prepared := false
	operationID := "abababab-abab-4bab-8bab-abababababab"
	base := cliS2Hook(f)
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == route+"-results" {
			if !prepared {
				cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "DRAFT_OPERATION_NOT_FOUND"}})
			} else {
				cliSyncJSON(w, 200, map[string]any{"operationId": operationID, "status": "applied", "current": false, "result": map[string]string{"revisionEpoch": "1", "version": "1", "contentState": "owned", "replyState": "none"}})
			}
			return true
		}
		if r.URL.Path == route && r.Method == "PUT" {
			raw, _ := io.ReadAll(r.Body)
			if !syncEqual(raw, []byte(op.Body)) {
				t.Error("original body replaced")
			}
			prepared = true
			cliSyncJSON(w, 200, map[string]any{"operationId": operationID, "status": "prepared", "current": false, "result": map[string]string{"revisionEpoch": "1", "version": "0", "contentState": "empty", "replyState": "none"}})
			return true
		}
		return base(w, r)
	})
	if _, err := f.client.performSelfOperation(op.ID, true); err != nil {
		t.Fatal(err)
	}
	saved, err := f.store.loadSelfOperation(op.ID)
	if err != nil || saved.State != "pending" {
		t.Fatal("prepared falsely completed")
	}
	if _, err = f.client.performSelfOperation(op.ID, true); err != nil {
		t.Fatal(err)
	}
	saved, err = f.store.loadSelfOperation(op.ID)
	if err != nil || saved.State != "completed" {
		t.Fatal("applied original not completed")
	}
	result, err := f.client.performSelfOperation(op.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	var fact struct {
		Current bool `json:"current"`
	}
	if json.Unmarshal(result, &fact) != nil || fact.Current {
		t.Fatal("completed original failed to recheck current fact")
	}
	drafts, err := f.store.drafts()
	if err != nil || drafts[f.conversation] != "newer independent local text" {
		t.Fatal("old success overwrote newer local text")
	}
	var writes int
	for _, body := range f.calls(route) {
		if len(body) > 0 {
			writes++
		}
	}
	if writes != 1 {
		t.Fatalf("unexpected create retry count %d", writes)
	}
	if len(f.calls(route+"-results")) != 3 {
		t.Fatal("completed original bypassed readonly current recheck")
	}
}
func TestS2Lookup404InAutomaticModeNeverCreatesAndOriginalHashCannotChange(t *testing.T) {
	f := cliS2Fixture(t)
	route := "/api/me/preferences"
	op := cliSelfOperation(f, "font", "PATCH", route, map[string]any{"expectedVersion": "0", "fontPreset": "large"})
	if err := f.store.saveSelfOperation(op); err != nil {
		t.Fatal(err)
	}
	base := cliS2Hook(f)
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == route+"-results" {
			cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "PREFERENCE_OPERATION_NOT_FOUND"}})
			return true
		}
		return base(w, r)
	})
	if _, err := f.client.performSelfOperation(op.ID, false); err == nil {
		t.Fatal("unknown marked success")
	}
	if len(f.calls(route)) != 0 {
		t.Fatal("automatic readonly lookup created original write")
	}
	completed, _ := json.Marshal(map[string]any{"operationId": op.ID, "status": "applied", "current": true, "result": map[string]string{"revisionEpoch": "1", "version": "1", "fontPreset": "large"}})
	if _, err := f.store.db.Exec("UPDATE self_operations SET state='completed',result=? WHERE id=?", string(completed), op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.performSelfOperation(op.ID, true); err == nil {
		t.Fatal("completed missing original bypassed readonly check")
	}
	if len(f.calls(route)) != 0 {
		t.Fatal("completed original was recreated after lookup404")
	}
	if _, err := f.store.db.Exec("UPDATE self_operations SET body='{}' WHERE id=?", op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.performSelfOperation(op.ID, true); err == nil {
		t.Fatal("mutated original accepted")
	}
}
func TestS2LocalDraftAdoptionUsesExactOriginalInputCAS(t *testing.T) {
	f := cliS2Fixture(t)
	if err := f.store.draft(f.conversation, "current local"); err != nil {
		t.Fatal(err)
	}
	if err := f.client.adoptSelfDraft(f.conversation, "old local", "remote"); err == nil {
		t.Fatal("stale adoption overwrote current local")
	}
	if err := f.client.adoptSelfDraft(f.conversation, "current local", "remote"); err != nil {
		t.Fatal(err)
	}
}
func TestS2ReadProofRequiresOriginalObservedSetNotAClaimedMaximum(t *testing.T) {
	f := cliS2Fixture(t)
	op := cliSelfOperation(f, "read", "PUT", "/api/conversations/"+f.conversation+"/read", map[string]any{"membershipGeneration": "1", "observedMessageIds": []string{f.messageID}})
	bad := json.RawMessage(fmt.Sprintf(`{"result":{"revisionEpoch":"1","version":"1","membershipGeneration":"1","readPoint":{"epoch":"1","sequence":"999999"},"observedMessageIds":[],"countStatus":"not_computed","hasMoreEligible":false,"hasUnorderedEligible":false}}`))
	if err := validateSelfResult(op, bad); err == nil {
		t.Fatal("unrelated maximum read point treated as original observation proof")
	}
	if !selfResultIdentity("read", "12121212-1212-4212-8212-121212121212", op.ID) || !selfResultIdentity("draft", "12121212-1212-4212-8212-121212121212", op.ID) || selfResultIdentity("font", "12121212-1212-4212-8212-121212121212", op.ID) {
		t.Fatal("read service identity incorrectly treated as request ID")
	}
}
func TestS2OriginalOperationCannotChangeMethodRouteOrRequestIdentity(t *testing.T) {
	f := cliS2Fixture(t)
	op := cliSelfOperation(f, "font", "PATCH", "/api/me/preferences", map[string]any{"expectedVersion": "0", "fontPreset": "large"})
	if err := validateSelfOperation(op); err != nil {
		t.Fatal(err)
	}
	bad := op
	bad.Route = "/api/admin/capabilities"
	bad.Hash = selfOperationHash(bad)
	if err := validateSelfOperation(bad); err == nil {
		t.Fatal("wrong original route accepted")
	}
	bad = op
	bad.ID = id()
	if err := validateSelfOperation(bad); err == nil {
		t.Fatal("new ID with old body accepted")
	}
	if err := validateViewChanges(map[string]any{"mutedUntil": "2026-11-16T12:34:56.123456789+00:00"}); err != nil {
		t.Fatal("valid RFC3339 offset rejected", err)
	}
}
func TestS2ConversationViewsApplyHiddenArchiveAndExposeRestorationEntry(t *testing.T) {
	f := cliS2Fixture(t)
	control := f.control()
	yes, no := true, false
	v := selfView{selfScope: selfScope{ID: f.conversation, ScopeID: control.ScopeID, Source: "personal", User: control.PrincipalID, Conversation: f.conversation, Epoch: "1", Version: "1"}, Archived: &no, Hidden: &yes, Manual: &no, ManualGeneration: "0"}
	raw, _ := json.Marshal(map[string]any{"object": v})
	if _, err := f.store.db.Exec("INSERT INTO sync_objects VALUES('1',?,'conversation_view','available',?)", "sync|conversation_view|"+v.ID, string(raw)); err != nil {
		t.Fatal(err)
	}
	conversation := json.RawMessage(`{"id":"` + f.conversation + `"}`)
	hidden, err := f.store.selfConversationListing([]json.RawMessage{conversation}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden["items"].([]json.RawMessage)) != 0 {
		t.Fatal("hidden view ignored")
	}
	all, err := f.store.selfConversationListing([]json.RawMessage{conversation}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all["items"].([]json.RawMessage)) != 1 || all["terminalFontChanged"] != false {
		t.Fatal("restore listing or terminal-font boundary lost")
	}
}
