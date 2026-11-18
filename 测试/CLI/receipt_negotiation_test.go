// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cliNegotiationView(t *testing.T, r *cliReceiptFixture) receiptNegotiation {
	t.Helper()
	s, current, err := r.client.receiptContext()
	if err != nil {
		t.Fatal(err)
	}
	control, err := s.receiptControl(s.db, nil, current, true)
	if err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	q := "transport_only"
	if r.storage.Qualified {
		q = "qualified_content"
	}
	if r.storage.ObjectQualified {
		q = "qualified_content_and_objects"
	}
	storage := r.storage
	if storage.Qualified && storage.Contract == "" {
		storage.Contract = "native_sqlite_full_v1"
	}
	return receiptNegotiation{syncBinding: control.syncBinding, Protocol: receiptNegotiationProtocol, Schema: control.schema(), Qualification: q, Storage: storage, Qualified: &r.storage.Qualified, Content: &r.storage.Qualified, Object: &r.storage.ObjectQualified, After: &r.storage.Qualified, AfterObject: &r.storage.ObjectQualified, Default: "cloud_history", Deletion: &no, Target: &no, Readiness: receiptReadiness{TransportConfigured: &yes, PublishConnected: &yes, ConsumeConnected: &yes, MessageDurable: &yes, JournalDurable: &yes, JournalIndependent: &yes, EpochMatches: &yes, Ready: &yes, Blockers: []string{}}}
}

func cliNegotiationHook(t *testing.T, r *cliReceiptFixture, custom func(http.ResponseWriter, *http.Request) bool) {
	t.Helper()
	r.setHook(func(w http.ResponseWriter, q *http.Request) bool {
		if custom != nil && custom(w, q) {
			return true
		}
		switch q.URL.Path {
		case "/api/bootstrap":
			cliSyncJSON(w, 200, map[string]any{"deploymentId": r.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{"authoritative_sync": true, "sync_jetstream": true, "durable_receipt_protocol": true, "qualified_receipt_negotiation": true}})
			return true
		case "/api/sync/receipt-context":
			if q.Header.Get("X-JobsIM-Store-Generation") != r.physical {
				t.Error("new negotiation omitted the original physical store")
			}
			cliSyncJSON(w, 200, cliNegotiationView(t, r))
			return true
		case "/api/sync/receipt-profiles":
			if q.Header.Get("X-JobsIM-Store-Generation") != r.physical || q.URL.Query().Get("buildDigest") != r.storage.Build {
				t.Error("profiles omitted original store or exact local build")
			}
			no, yes := false, true
			cliSyncJSON(w, 200, receiptProfileList{Protocol: receiptNegotiationProtocol, Profiles: []receiptApprovedProfile{{r.storage.Profile, r.storage.Build, r.storage.Verification, "native_sqlite_full_v1"}}, Qualified: &no, Declaration: &yes, LocalProbe: &yes})
			return true
		}
		return r.serveReceipt(w, q)
	})
}

func TestReceiptNegotiationQualifiedStoreKeepsLegacyTransportCompatible(t *testing.T) {
	r := cliReceiptSetup(t)
	cliNegotiationHook(t, r, nil)
	if err := r.client.receiptCapabilities(); err != nil {
		t.Fatal(err)
	}
	s, current, _ := r.client.receiptContext()
	control, _ := s.receiptControl(s.db, nil, current, true)
	v, err := r.client.getReceiptNegotiation(control)
	if err != nil || !receiptBool(v.After, true) || !receiptBool(v.Qualified, true) {
		t.Fatal("new store context stayed permanently false", err)
	}
	legacy, err := r.client.getReceiptStorage(control)
	if err != nil || !receiptFalse(legacy.After) {
		t.Fatal("new context changed the legacy storage contract", err)
	}
	if _, err = r.client.receiptProfiles(control, r.storage.Build); err != nil {
		t.Fatal(err)
	}
	if len(r.calls("/api/sync/receipts")) != 0 || len(r.calls("/api/sync/storage")) != 1 {
		t.Fatal("capability inspection submitted a declaration or ACK")
	}
}

func TestReceiptNegotiationRejectsWrongStoreAndContradictoryReadiness(t *testing.T) {
	r := cliReceiptSetup(t)
	s, current, _ := r.client.receiptContext()
	control, _ := s.receiptControl(s.db, nil, current, true)
	original := cliNegotiationView(t, r)
	for _, mutate := range []func(*receiptNegotiation){
		func(v *receiptNegotiation) { v.StoreGeneration = r.conversation },
		func(v *receiptNegotiation) { v.DeviceID = r.conversation },
		func(v *receiptNegotiation) { v.RestoreEpoch = "2" },
		func(v *receiptNegotiation) { v.SourceContextID = "imported" },
		func(v *receiptNegotiation) { v.Storage.Contract = "unknown_cache" },
		func(v *receiptNegotiation) { v.Readiness.JournalIndependent = nil },
		func(v *receiptNegotiation) { no := false; v.Readiness.JournalIndependent = &no },
		func(v *receiptNegotiation) { yes := true; v.Object = &yes },
		func(v *receiptNegotiation) { yes := true; v.Deletion = &yes },
		func(v *receiptNegotiation) { v.Readiness.Blockers = []string{"journal_not_independent"} },
	} {
		raw, _ := json.Marshal(original)
		var v receiptNegotiation
		_ = json.Unmarshal(raw, &v)
		mutate(&v)
		if receiptValidateNegotiation(v, control) == nil {
			t.Fatal("contradictory new receipt context was accepted")
		}
	}
	no := false
	v := original
	v.Storage.Qualified, v.Storage.ObjectQualified = false, false
	v.Qualification = "transport_only"
	v.Qualified, v.Content, v.Object, v.After, v.AfterObject = &no, &no, &no, &no, &no
	if receiptValidateNegotiation(v, control) != nil {
		t.Fatal("server readiness alone must not require a transport-only client to qualify")
	}
}

func cliReceiptGeneration(t *testing.T, token, generation, digest string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var proof map[string]any
	_ = json.Unmarshal(raw, &proof)
	proof["deliveryGeneration"], proof["manifestDigest"] = generation, digest
	raw, _ = json.Marshal(proof)
	parts[1] = base64.RawURLEncoding.EncodeToString(raw)
	return strings.Join(parts, ".")
}

func TestReceiptNegotiationGenerationIsRetainedAcrossUnknownResult(t *testing.T) {
	r := cliReceiptSetup(t)
	generation, manifest := "9007199254740993", strings.Repeat("d", 64)
	r.challenge.Challenge = cliReceiptGeneration(t, r.challenge.Challenge, generation, manifest)
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("server result silently dropped the original target generation")
	}
	id := r.receiptID()
	o, err := r.store.loadReceipt(id)
	if err != nil || o.State != "pending" {
		t.Fatal("bad result destroyed the original responsibility", err)
	}
	var result map[string]any
	_ = json.Unmarshal(r.result, &result)
	result["deliveryGeneration"], result["manifestDigest"] = generation, manifest
	r.result, _ = json.Marshal(result)
	if _, err = r.client.performReceipt(id, false, false); err != nil {
		t.Fatal(err)
	}
	if len(r.calls("/api/sync/receipts")) != 1 {
		t.Fatal("result reconciliation created another ACK")
	}
	for _, pair := range [][2]string{{"", manifest}, {generation, ""}, {"01", manifest}, {"0", manifest}, {generation, strings.Repeat("X", 64)}} {
		v := r.challenge
		v.Challenge = cliReceiptGeneration(t, v.Challenge, pair[0], pair[1])
		if _, err = receiptValidateChallenge(v, o.Binding, r.qualification, r.messageID, r.permission, time.Now()); err == nil {
			t.Fatal("incomplete or noncanonical target pair was accepted")
		}
	}
}

func TestReceiptNegotiationObjectMustKeepParentGeneration(t *testing.T) {
	o := cliObjectSetup(t)
	p, err := o.store.loadReceipt(o.parent)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	_ = json.Unmarshal([]byte(p.Body), &body)
	generation, manifest := "3", strings.Repeat("f", 64)
	body["challenge"] = cliReceiptGeneration(t, body["challenge"], generation, manifest)
	raw, _ := json.Marshal(body)
	p.Body = string(raw)
	challenge := o.challenge
	challenge.Challenge = cliReceiptGeneration(t, challenge.Challenge, generation, manifest)
	if err = receiptValidateObjectChallenge(challenge, p, o.permission, time.Now()); err != nil {
		t.Fatal("matching parent generation was rejected", err)
	}
	challenge.Challenge = cliReceiptGeneration(t, challenge.Challenge, "4", manifest)
	if err = receiptValidateObjectChallenge(challenge, p, o.permission, time.Now()); err == nil {
		t.Fatal("object inherited a different target generation")
	}
}

func TestReceiptNegotiationStorageUnknownFirstQueriesOriginalAfterWithdrawal(t *testing.T) {
	r := cliReceiptSetup(t)
	original := r.storage
	r.storageUnknown = true
	known := false
	cliNegotiationHook(t, r, func(w http.ResponseWriter, q *http.Request) bool {
		if q.URL.Path != "/api/sync/storage-results" {
			return false
		}
		if !known {
			cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "STORAGE_DECLARATION_NOT_FOUND"}})
		} else {
			no := false
			cliSyncJSON(w, 200, receiptStorageView{Storage: r.storage, Result: &original, Current: &no, Deletion: &no, After: &no})
		}
		return true
	})
	if _, err := r.client.receiptCommand("storage-declare", r.storage.Profile+" "+r.storage.Build+" "+r.storage.Verification); err == nil {
		t.Fatal("injected unknown declaration became success")
	}
	var id string
	if err := r.store.db.QueryRow("SELECT id FROM storage_declarations").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.performReceipt(id, false, false); err == nil || len(r.calls("/api/sync/storage")) != 1 {
		t.Fatal("readonly unknown declaration wrote again")
	}
	known = true
	r.storage.Qualified, r.storage.ObjectQualified = false, false
	if _, err := r.client.performReceipt(id, false, false); err != nil {
		t.Fatal("withdrawal hid the original committed declaration", err)
	}
	if len(r.calls("/api/sync/storage")) != 1 {
		t.Fatal("old committed declaration was resubmitted after withdrawal")
	}
}

func TestReceiptNegotiationNativeProbeChecksDatabaseAndFileDirectory(t *testing.T) {
	r := cliReceiptSetup(t)
	s, current, _ := r.client.receiptContext()
	control, _ := s.receiptControl(s.db, nil, current, true)
	if err := s.receiptNativeProbe(receiptFileContract, control, current); err != nil {
		t.Fatal("actual private SQLite/file/directory probe", err)
	}
	entries, err := os.ReadDir(filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".receipt-native-probe-") {
			t.Fatal("successful probe retained its temporary original bytes")
		}
	}
	if _, err = s.db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		t.Fatal(err)
	}
	if s.receiptNativeProbe(receiptFileContract, control, current) == nil {
		t.Fatal("non-FULL physical store declared native durability")
	}
}

func TestReceiptNegotiationPolicyUnknownKeepsOriginalCASAndNonce(t *testing.T) {
	r := cliReceiptSetup(t)
	var original string
	writes := 0
	cliNegotiationHook(t, r, func(w http.ResponseWriter, q *http.Request) bool {
		if q.URL.Path != "/api/conversations/"+r.conversation+"/retention" {
			return false
		}
		if q.Header.Get("X-JobsIM-Store-Generation") != r.physical {
			t.Error("policy publication omitted negotiated physical store")
		}
		raw, _ := io.ReadAll(q.Body)
		var request receiptPolicyRequest
		if json.Unmarshal(raw, &request) != nil {
			t.Fatal("policy fixture body")
		}
		var saved, state string
		if err := r.store.db.QueryRow("SELECT payload,state FROM receipt_policy_operations WHERE id=?", request.ID).Scan(&saved, &state); err != nil || state != "pending" {
			t.Error("policy HTTP preceded the original FULL intent", err)
		}
		writes++
		if writes == 1 {
			original = string(raw)
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
		} else {
			if string(raw) != original {
				t.Error("retry replaced original nonce, CAS or policy")
			}
			cliSyncJSON(w, 200, map[string]any{"policy": request.Policy, "version": "2", "appliesTo": "new_messages_only"})
		}
		return true
	})
	policy := `{"mode":"after_receipt","delaySeconds":0,"receiptScope":"all_target_devices","undeliveredMaxAgeSeconds":3600,"historyRetentionSeconds":null,"attachmentMode":"fixed_retention","attachmentRetentionSeconds":3600,"attachmentDelaySeconds":null}`
	if _, err := r.client.receiptCommand("retention-set", r.conversation+" 1 | "+policy); err == nil {
		t.Fatal("unknown policy publication became success")
	}
	var id string
	if err := r.store.db.QueryRow("SELECT id FROM receipt_policy_operations").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.receiptCommand("retention-set", r.conversation+" 1 | "+policy); err == nil || writes != 1 {
		t.Fatal("unknown original policy was replaced by a fresh nonce")
	}
	r.storage.Qualified = false
	if _, err := r.client.receiptCommand("retention-retry", id); err != nil {
		t.Fatal("original committed policy fact could not be reconciled after qualification withdrawal", err)
	}
	if _, err := r.client.receiptCommand("retention-retry", id); err != nil || writes != 2 {
		t.Fatal("completed policy was posted again", err)
	}
}

func TestReceiptNegotiationNoQualificationCannotStageReceiptPolicy(t *testing.T) {
	r := cliReceiptSetup(t)
	r.storage.Qualified = false
	cliNegotiationHook(t, r, nil)
	policy := `{"mode":"after_receipt","delaySeconds":0,"receiptScope":"all_target_devices","undeliveredMaxAgeSeconds":3600,"attachmentMode":"fixed_retention","attachmentRetentionSeconds":3600}`
	if _, err := r.client.receiptCommand("retention-set", r.conversation+" 1 | "+policy); err == nil {
		t.Fatal("runtime readiness supplied missing native qualification")
	}
	var count int
	if err := r.store.db.QueryRow("SELECT count(*) FROM receipt_policy_operations").Scan(&count); err != nil || count != 0 {
		t.Fatal("unqualified policy was staged", err)
	}
	for _, raw := range []string{
		`{"mode":"after_receipt","receiptScope":"all_target_devices","undeliveredMaxAgeSeconds":3600,"attachmentMode":"fixed_retention","attachmentRetentionSeconds":3600}`,
		`{"mode":"cloud_history","delaySeconds":0,"receiptScope":"all_target_devices","undeliveredMaxAgeSeconds":3600,"attachmentMode":"fixed_retention","attachmentRetentionSeconds":3600}`,
		`{"mode":"cloud_history","receiptScope":"all_target_devices","undeliveredMaxAgeSeconds":3600,"attachmentMode":"after_object_receipt","attachmentRetentionSeconds":3600}`,
	} {
		if _, err := r.client.receiptCommand("retention-set", r.conversation+" 1 | "+raw); err == nil {
			t.Fatal("missing explicit receipt delay or contradictory policy was accepted")
		}
	}
}

func TestReceiptNegotiationOriginalEnvelopeCommitsWithIntentAndGlobalErasesIt(t *testing.T) {
	r := cliReceiptSetup(t)
	generation, manifest := "5", strings.Repeat("e", 64)
	r.challenge.Challenge = cliReceiptGeneration(t, r.challenge.Challenge, generation, manifest)
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("fixture result omitted target pair")
	}
	id := r.receiptID()
	var result map[string]any
	_ = json.Unmarshal(r.result, &result)
	result["deliveryGeneration"], result["manifestDigest"] = generation, manifest
	r.result, _ = json.Marshal(result)
	if _, err := r.client.performReceipt(id, false, false); err != nil {
		t.Fatal(err)
	}
	messageCurrent := true
	cliNegotiationHook(t, r, func(w http.ResponseWriter, q *http.Request) bool {
		if q.URL.Path == "/api/messages/"+r.messageID {
			var live map[string]any
			_ = json.Unmarshal(r.message(), &live)
			live["text"], live["contentUnavailable"], live["deleteScope"] = "", true, "cloud_only"
			cliSyncJSON(w, 200, live)
			return true
		}
		if q.URL.Path != "/api/sync/receipt-targets/"+r.messageID {
			return false
		}
		yes, no := true, false
		if q.URL.Query().Get("qualificationId") != r.qualification {
			t.Error("history fact lookup replaced the original qualification")
		}
		cliSyncJSON(w, 200, receiptTargetView{Protocol: receiptNegotiationProtocol, Message: r.messageID, Content: "1", Epoch: "1", Qualification: r.qualification, Generation: generation, Digest: manifest, Scope: "all_target_devices", Original: &no, ContentFor: &no, ObjectFor: &no, ContentACK: &yes, ObjectACK: &no, Current: &no, MessageNow: &messageCurrent, Deletion: &no})
		return true
	})
	source, err := r.client.localQualifiedMigrationMessage(r.messageID, "1")
	if err != nil || source.Envelope != r.challenge.Envelope || source.BodyDigest != r.challenge.Digest || source.ContentReceiptID != id || source.Original != nil {
		t.Fatal("qualified local source reconstructed or lost the original raw envelope", err)
	}
	r.storage.Qualified = false
	if historical, err := r.client.localQualifiedMigrationMessage(r.messageID, "1"); err != nil || historical.ContentReceiptID != id {
		t.Fatal("withdrawn approval erased an original qualified-at-commit fact", err)
	}
	binding := dataExportBinding{Identity: r.identity, Physical: r.physical, Epoch: "1", Family: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}
	selection := cliMigrationIntent{Selection: []cliMigrationSelection{{r.messageID, "1", "none"}}}
	inputs, objects, sourceDigest, err := r.client.cliMigrationSourceInputs(selection, binding, func() bool { return true })
	if err != nil || len(inputs) != 1 || len(objects) != 0 || inputs[0].Envelope != r.challenge.Envelope || inputs[0].Receipt != id || sourceDigest == "" {
		t.Fatal("fresh cloud-only GET did not use the exact original envelope and qualified receipt", err)
	}
	messageCurrent = false
	if _, err = r.client.localQualifiedMigrationMessage(r.messageID, "1"); err == nil {
		t.Fatal("source-owner pending global gate released an old qualified body")
	}
	messageCurrent = true
	// Expired delivery-range current=false does not erase a still-current lawful
	// qualified body; messageCurrent and the original membership remain required.
	tx, err := r.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = syncEraseTx(tx, "1", r.messageID); err != nil || tx.Commit() != nil {
		t.Fatal("global erase failed", err)
	}
	var count int
	if r.store.db.QueryRow("SELECT count(*) FROM receipt_bodies WHERE message_id=?", r.messageID).Scan(&count) != nil || count != 0 {
		t.Fatal("global deletion retained a raw receipt-body copy")
	}
	if _, err = r.client.localQualifiedMigrationMessage(r.messageID, "1"); err == nil {
		t.Fatal("globally removed receipt bytes became a migration source")
	}
}

func TestReceiptNegotiationRawEnvelopeSQLFailureCannotSendACK(t *testing.T) {
	r := cliReceiptSetup(t)
	if _, err := r.store.db.Exec("CREATE TRIGGER fail_receipt_raw BEFORE INSERT ON receipt_bodies BEGIN SELECT RAISE(ABORT,'injected raw holder failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("raw-holder failure became a successful local receipt")
	}
	var count int
	if r.store.db.QueryRow("SELECT count(*) FROM receipt_outbox").Scan(&count) != nil || count != 0 || len(r.calls("/api/sync/receipts")) != 0 {
		t.Fatal("raw envelope and original receipt intent did not roll back together")
	}
}

func TestReceiptNegotiationLegacyMigrationSourceRequiresOriginalPermission(t *testing.T) {
	r := cliReceiptSetup(t)
	if _, err := r.client.receiveReceiptText(r.messageID); err != nil {
		t.Fatal(err)
	}
	permission := r.permission
	cliNegotiationHook(t, r, func(w http.ResponseWriter, q *http.Request) bool {
		if q.URL.Path == "/api/sync/context" {
			control, _, err := syncLoadControl(r.store.db, "1")
			if err != nil {
				t.Error(err)
			}
			raw, _ := json.Marshal(control.syncBinding)
			var view map[string]any
			_ = json.Unmarshal(raw, &view)
			view["permissionGeneration"], view["qualification"], view["qualifiedAck"], view["afterReceiptEnabled"] = permission, "transport_only", false, false
			cliSyncJSON(w, 200, view)
			return true
		}
		if q.URL.Path == "/api/messages/"+r.messageID {
			var message map[string]any
			_ = json.Unmarshal(r.message(), &message)
			message["contentUnavailable"], message["deleteScope"], message["text"] = true, "cloud_only", ""
			cliSyncJSON(w, 200, message)
			return true
		}
		return false
	})
	if source, err := r.client.localQualifiedMigrationMessage(r.messageID, "1"); err != nil || source.Envelope != r.challenge.Envelope || source.ContentReceiptID != r.receiptID() {
		t.Fatal("lawful legacy receipt was forced into a fabricated target range", err)
	}
	permission = strings.Repeat("f", 64)
	if _, err := r.client.localQualifiedMigrationMessage(r.messageID, "1"); err == nil {
		t.Fatal("old no-manifest receipt survived a changed full permission snapshot")
	}
	if len(r.calls("/api/sync/receipt-targets/"+r.messageID)) != 0 {
		t.Fatal("legacy migration manufactured a missing target range")
	}
}

func TestReceiptNegotiationUpgradeCanReconcileAndDeclareNewApprovedBuild(t *testing.T) {
	r := cliReceiptSetup(t)
	if _, err := r.client.receiveReceiptText(r.messageID); err != nil {
		t.Fatal(err)
	}
	originalID := r.receiptID()
	storageBefore := len(r.calls("/api/sync/storage"))
	currentBuild, err := receiptExecutableDigest()
	if err != nil {
		t.Fatal(err)
	}
	// The store still has an approved declaration for the previous executable.
	r.storage.Build = strings.Repeat("a", 64)
	profile, verification := "cli-new-build-approved", strings.Repeat("d", 64)
	cliNegotiationHook(t, r, func(w http.ResponseWriter, q *http.Request) bool {
		switch q.URL.Path {
		case "/api/sync/receipt-profiles":
			if q.URL.Query().Get("buildDigest") != currentBuild {
				t.Error("upgrade queried the old server build instead of its actual executable")
			}
			no, yes := false, true
			cliSyncJSON(w, 200, receiptProfileList{Protocol: receiptNegotiationProtocol, Profiles: []receiptApprovedProfile{{profile, currentBuild, verification, "native_sqlite_full_v1"}}, Qualified: &no, Declaration: &yes, LocalProbe: &yes})
			return true
		case "/api/sync/storage-results":
			cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "STORAGE_DECLARATION_NOT_FOUND"}})
			return true
		case "/api/sync/storage":
			if q.Method != "PUT" {
				return false
			}
			var body receiptStorage
			data, _ := io.ReadAll(q.Body)
			if syncDecode(data, &body) != nil || body.Profile != profile || body.Build != currentBuild || body.Verification != verification {
				t.Error("upgrade declaration replaced its measured build or approved report")
			}
			r.storage.Profile, r.storage.Build, r.storage.Verification = body.Profile, body.Build, body.Verification
			r.storage.Contract = "native_sqlite_full_v1"
			current := r.storage
			yes, no := true, false
			cliSyncJSON(w, 200, receiptStorageView{Storage: current, Result: &current, Current: &yes, After: &no, Deletion: &no})
			return true
		}
		return false
	})
	if _, err = r.client.performReceipt(originalID, false, false); err != nil {
		t.Fatal("a previous build declaration blocked readonly original receipt reconciliation", err)
	}
	if _, err = r.client.receiveReceiptText(r.messageID); err == nil || len(r.calls("/api/sync/receipt-challenges")) != 1 || len(r.calls("/api/sync/receipts")) != 1 {
		t.Fatal("new ACK borrowed the old executable's qualification")
	}
	policy := `{"mode":"after_receipt","delaySeconds":0,"receiptScope":"all_target_devices","undeliveredMaxAgeSeconds":3600,"attachmentMode":"fixed_retention","attachmentRetentionSeconds":3600}`
	if _, err = r.client.receiptCommand("retention-set", r.conversation+" 1 | "+policy); err == nil {
		t.Fatal("new policy borrowed the previous executable's native approval")
	}
	var policies int
	if r.store.db.QueryRow("SELECT count(*) FROM receipt_policy_operations").Scan(&policies) != nil || policies != 0 {
		t.Fatal("old build was allowed to stage a new after-receipt policy")
	}
	if _, err = r.client.receiptCommand("storage-declare", profile+" "+currentBuild+" "+verification); err != nil {
		t.Fatal("old active declaration made the new approved build impossible to declare", err)
	}
	if r.storage.Build != currentBuild || len(r.calls("/api/sync/storage")) != storageBefore+2 {
		// One GET was made while refusing the old-build ACK; one PUT declares the upgrade.
		t.Fatal("the measured new build was not declared exactly once")
	}
}
