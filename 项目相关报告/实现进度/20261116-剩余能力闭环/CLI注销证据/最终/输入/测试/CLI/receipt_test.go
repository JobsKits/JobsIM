// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type cliReceiptFixture struct {
	*cliSyncFixture
	qualification  string
	storage        receiptStorage
	challenge      receiptChallenge
	result         json.RawMessage
	submitStatus   int
	resultStatus   int
	submitCheck    func(*testing.T, receiptOperation)
	storageUnknown bool
	badResult      bool
	feature        bool
}

func cliReceiptSetup(t *testing.T) *cliReceiptFixture {
	f := newCLISyncFixture(t)
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	r := &cliReceiptFixture{cliSyncFixture: f, qualification: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", feature: true, resultStatus: 404, submitStatus: 200}
	build, err := receiptExecutableDigest()
	if err != nil {
		t.Fatal(err)
	}
	r.storage = receiptStorage{Qualification: r.qualification, Store: f.physical, Epoch: "1", Profile: "cli-native-test", Build: build, Verification: strings.Repeat("c", 64), Qualified: true}
	r.challenge = r.makeChallenge(f.message(), time.Now().Add(30*time.Minute).Truncate(time.Second))
	f.setHook(r.serveReceipt)
	return r
}
func (r *cliReceiptFixture) makeChallenge(raw json.RawMessage, expires time.Time) receiptChallenge {
	envelope, digest := cliSyncEnvelope(raw)
	p := receiptProofTuple{Scope: r.identity.DeploymentID, User: r.identity.UserID, Device: r.identity.DeviceID, Store: r.physical, Epoch: "1", Qualification: r.qualification, Message: r.messageID, Conversation: r.conversation, Content: "1", Digest: digest, Permission: r.permission, Expires: expires.Unix()}
	body, _ := json.Marshal(p)
	// Injected HTTP fixture token, not a genuine server signature/receipt.
	token := "rc1." + base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	yes, no := true, false
	return receiptChallenge{Challenge: token, Message: r.messageID, Conversation: r.conversation, Content: "1", Epoch: "1", Store: r.physical, Qualification: r.qualification, Envelope: envelope, Digest: digest, Expires: expires.Format(time.RFC3339Nano), Atomic: &yes, Object: &no, Deletion: &no}
}
func (r *cliReceiptFixture) receiptID() string {
	var id string
	if err := r.store.db.QueryRow("SELECT id FROM receipt_outbox ORDER BY rowid DESC LIMIT 1").Scan(&id); err != nil {
		r.t.Fatal(err)
	}
	return id
}
func (r *cliReceiptFixture) serveReceipt(w http.ResponseWriter, q *http.Request) bool {
	no := false
	switch q.URL.Path {
	case "/api/bootstrap":
		cliSyncJSON(w, 200, map[string]any{"deploymentId": r.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{"authoritative_sync": true, "sync_jetstream": true, "durable_receipt_protocol": r.feature}})
	case "/api/sync/storage":
		if q.Method == "GET" {
			cliSyncJSON(w, 200, receiptStorageView{Storage: r.storage, Deletion: &no, After: &no})
			return true
		}
		if r.storageUnknown {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
			return true
		}
		var body receiptStorage
		raw, _ := io.ReadAll(q.Body)
		if syncDecode(raw, &body) != nil {
			r.t.Error("invalid storage body")
		}
		original := r.storage
		original.Profile, original.Build, original.Verification = body.Profile, body.Build, body.Verification
		yes := true
		cliSyncJSON(w, 200, receiptStorageView{Storage: original, Result: &original, Current: &yes, Deletion: &no, After: &no})
	case "/api/sync/receipt-challenges":
		cliSyncJSON(w, 200, r.challenge)
	case "/api/sync/receipt-results":
		if r.resultStatus == 200 {
			w.Header().Set("Content-Type", "application/json")
			w.Write(r.result)
		} else {
			cliSyncJSON(w, r.resultStatus, map[string]any{"error": map[string]string{"code": "RECEIPT_OPERATION_NOT_FOUND"}})
		}
	case "/api/sync/receipts":
		id := r.receiptID()
		o, err := r.store.loadReceipt(id)
		if err != nil {
			r.t.Error(err)
		}
		if r.submitCheck != nil {
			r.submitCheck(r.t, o)
		}
		if r.submitStatus != 200 {
			cliSyncJSON(w, r.submitStatus, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
			return true
		}
		v := map[string]any{"operationId": o.ID, "status": "recorded", "messageId": o.Message, "conversationId": o.Conversation, "contentVersion": o.Content, "restoreEpoch": o.Binding.RestoreEpoch, "deviceId": o.Binding.DeviceID, "storeGeneration": o.Binding.StoreGeneration, "qualificationId": r.qualification, "bodyDigest": o.Digest, "receiptKind": "message_content", "storageQualifiedAtCommit": true, "objectComplete": false, "targetRangeComplete": false, "deletionEligible": false}
		if r.badResult {
			v["deletionEligible"] = true
		}
		r.result, _ = json.Marshal(v)
		r.resultStatus = 200
		w.Header().Set("Content-Type", "application/json")
		w.Write(r.result)
	default:
		return false
	}
	return true
}
func TestReceiptTextAtomicCommitPrecedesHTTPAndNeverConfirmsMessageOutbox(t *testing.T) {
	r := cliReceiptSetup(t)
	if err := r.store.enqueue(r.pending()); err != nil {
		t.Fatal(err)
	}
	r.submitCheck = func(t *testing.T, o receiptOperation) {
		var message, payload string
		if err := r.store.db.QueryRow("SELECT payload FROM messages WHERE epoch='1' AND id=?", r.messageID).Scan(&message); err != nil {
			t.Fatal(err)
		}
		if fingerprint(message) != o.CachedDigest {
			t.Fatal("HTTP preceded durable body")
		}
		if err := r.store.db.QueryRow("SELECT payload FROM receipt_outbox WHERE id=?", o.ID).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload, "原始 bytes") || strings.Contains(payload, "bodyEnvelope") {
			t.Fatal("receipt intent contains a second body copy")
		}
	}
	if _, err := r.client.receiveReceiptText(r.messageID); err != nil {
		t.Fatal(err)
	}
	o, err := r.store.loadReceipt(r.receiptID())
	if err != nil || o.State != "completed" {
		t.Fatalf("%s %v", o.State, err)
	}
	queue, err := r.store.outbox()
	if err != nil || queue[r.nonce].State != "pending" {
		t.Fatal("content receipt incorrectly ACKed a sending intent")
	}
	if len(r.calls("/api/sync/receipts")) != 1 || len(r.calls("/api/sync/receipt-results")) != 1 {
		t.Fatal("fresh intent did not first check original result")
	}
}
func TestReceiptSQLFailureRollsBodyAndIntentBackTogether(t *testing.T) {
	r := cliReceiptSetup(t)
	if _, err := r.store.db.Exec("DELETE FROM messages WHERE id=?", r.messageID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.store.db.Exec("CREATE TRIGGER receipt_disk_failure BEFORE INSERT ON receipt_outbox BEGIN SELECT RAISE(FAIL,'injected receipt persistence failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("SQL failure hidden")
	}
	for _, table := range []string{"messages", "receipt_outbox"} {
		var n int
		if err := r.store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("partial %s commit %d %v", table, n, err)
		}
	}
	if len(r.calls("/api/sync/receipts")) != 0 {
		t.Fatal("receipt sent before durable transaction")
	}
}
func TestReceiptUnknownReopensOriginalAndAutomatic404NeverWrites(t *testing.T) {
	r := cliReceiptSetup(t)
	r.submitStatus = 503
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("unknown hidden")
	}
	id := r.receiptID()
	before, _ := r.store.loadReceipt(id)
	path := r.store.path
	if err := r.store.close(); err != nil {
		t.Fatal(err)
	}
	var err error
	r.store, err = openStore(path, r.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer r.store.close()
	r.client.store = r.store
	if _, err = r.client.performReceipt(id, false, false); err == nil {
		t.Fatal("404 passed")
	}
	if len(r.calls("/api/sync/receipts")) != 1 {
		t.Fatal("automatic reconciliation created receipt")
	}
	r.submitStatus = 200
	if _, err = r.client.performReceipt(id, true, false); err != nil {
		t.Fatal(err)
	}
	after, _ := r.store.loadReceipt(id)
	if before.Body != after.Body || before.Hash != after.Hash {
		t.Fatal("original receipt was rebound")
	}
	calls := r.calls("/api/sync/receipts")
	if len(calls) != 2 || !bytes.Equal(calls[0], calls[1]) {
		t.Fatal("retry did not preserve exact original body")
	}
	if _, err = r.client.performReceipt(id, true, false); err != nil {
		t.Fatal(err)
	}
	if len(r.calls("/api/sync/receipts")) != 2 {
		t.Fatal("completed receipt re-created")
	}
}
func TestReceiptChallengeRejectsWrongTupleDigestAndUnsupportedBodies(t *testing.T) {
	r := cliReceiptSetup(t)
	b := r.control().syncBinding
	for _, name := range []string{"wrong-device", "wrong-epoch", "wrong-qualification", "wrong-digest", "expired", "forward", "attachment", "missing-content-version", "deleted", "bad-signature-shape"} {
		t.Run(name, func(t *testing.T) {
			q := r.challenge
			switch name {
			case "wrong-device":
				b.DeviceID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
			case "wrong-epoch":
				q.Epoch = "2"
			case "wrong-qualification":
				q.Qualification = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
			case "wrong-digest":
				q.Digest = strings.Repeat("f", 64)
			case "expired":
				q.Expires = time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
			case "bad-signature-shape":
				q.Challenge = "rc1.e30.Zg"
			default:
				var body map[string]any
				_ = syncDecode(r.message(), &body)
				switch name {
				case "forward":
					body["forwarded"] = true
				case "attachment":
					body["attachmentId"] = r.messageID
				case "missing-content-version":
					delete(body, "contentVersion")
				case "deleted":
					body["deleted"] = true
				}
				raw, _ := json.Marshal(body)
				q = r.makeChallenge(raw, time.Now().Add(time.Minute).Truncate(time.Second))
			}
			if _, err := receiptValidateChallenge(q, b, r.qualification, r.messageID, r.permission, time.Now()); err == nil {
				t.Fatal("bad challenge accepted")
			}
			b = r.control().syncBinding
		})
	}
}
func TestReceiptCapabilityAndQualificationCannotRebindAnUnknownIntent(t *testing.T) {
	r := cliReceiptSetup(t)
	r.resultStatus = 503
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("unknown hidden")
	}
	id := r.receiptID()
	original, _ := r.store.loadReceipt(id)
	r.feature = false
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("missing capability accepted")
	}
	r.feature = true
	r.resultStatus = 404
	r.storage.Build = strings.Repeat("b", 64)
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("old approved binary profile was borrowed by a different executable")
	}
	r.storage.Build, _ = receiptExecutableDigest()
	r.storage.Qualification = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("new qualification substituted")
	}
	after, _ := r.store.loadReceipt(id)
	if after.Body != original.Body || after.Hash != original.Hash || len(r.calls("/api/sync/receipts")) != 0 {
		t.Fatal("old intent changed or sent")
	}
}
func TestReceiptGlobalAndGuardPreventLateBodyInstallationOrRetry(t *testing.T) {
	r := cliReceiptSetup(t)
	raw, err := receiptValidateChallenge(r.challenge, r.control().syncBinding, r.qualification, r.messageID, r.permission, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r.resultStatus = 503
	if _, err = r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("unknown hidden")
	}
	id := r.receiptID()
	tx, err := r.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = syncRecordGlobalTx(tx, "1", r.messageID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r.resultStatus = 404
	if _, err = r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("global receipt retried")
	}
	if len(r.calls("/api/sync/receipts")) != 0 {
		t.Fatal("global content submitted")
	}
	o, _ := r.store.loadReceipt(id)
	o.ID, _ = syncNewUUID()
	o.Hash = receiptHash(o)
	if err = r.store.stageReceipt(r.control(), o, raw, func() bool { return true }); err == nil {
		t.Fatal("global body resurrected")
	}
	control := r.control()
	next := control
	next.Guard, _ = syncNewUUID()
	if _, err = r.store.updateSync(control, next, nil); err != nil {
		t.Fatal(err)
	}
	if err = r.store.stageReceipt(control, o, raw, func() bool { return true }); err == nil {
		t.Fatal("late guard installed receipt")
	}
}
func TestReceiptExpiredUnknownOnlyChecksOriginalAndNeverRefreshesProof(t *testing.T) {
	r := cliReceiptSetup(t)
	r.resultStatus = 503
	r.challenge = r.makeChallenge(r.message(), time.Now().Truncate(time.Second).Add(2*time.Second))
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("unknown hidden")
	}
	id := r.receiptID()
	before, _ := r.store.loadReceipt(id)
	expires, _ := time.Parse(time.RFC3339Nano, before.Expires)
	time.Sleep(time.Until(expires) + 20*time.Millisecond)
	r.resultStatus = 404
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("expired receipt recreated")
	}
	after, _ := r.store.loadReceipt(id)
	if after.State != "frozen" || after.Body != before.Body || len(r.calls("/api/sync/receipt-challenges")) != 1 || len(r.calls("/api/sync/receipts")) != 0 {
		t.Fatal("expired original proof replaced or submitted")
	}
}
func TestReceiptWrongIdentityAndCorruptOriginalNeverContactReceiptRoute(t *testing.T) {
	r := cliReceiptSetup(t)
	r.resultStatus = 503
	_, _ = r.client.receiveReceiptText(r.messageID)
	id := r.receiptID()
	count := len(r.calls("/api/sync/receipt-results"))
	r.client.serverDevice = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("wrong device accepted")
	}
	r.client.serverDevice = r.identity.DeviceID
	original, _ := r.store.loadReceipt(id)
	wrong := original
	wrong.Route = "/api/messages"
	wrong.ID, _ = syncNewUUID()
	var body map[string]string
	_ = json.Unmarshal([]byte(wrong.Body), &body)
	body["requestId"] = wrong.ID
	raw, _ := json.Marshal(body)
	wrong.Body = string(raw)
	wrong.Hash = receiptHash(wrong)
	if err := r.store.stageReceipt(r.control(), wrong, nil, func() bool { return true }); err == nil {
		t.Fatal("wrong original route staged despite fresh hash")
	}
	if _, err := r.store.db.Exec("UPDATE receipt_outbox SET payload_hash=? WHERE id=?", strings.Repeat("f", 64), id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("corrupt intent accepted")
	}
	if len(r.calls("/api/sync/receipt-results")) != count {
		t.Fatal("invalid intent contacted server")
	}
}
func TestReceiptBadSuccessDoesNotCompleteAndHistoricalProofNeverCreates(t *testing.T) {
	r := cliReceiptSetup(t)
	r.badResult = true
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("deletion eligibility accepted")
	}
	id := r.receiptID()
	o, _ := r.store.loadReceipt(id)
	if o.State != "pending" {
		t.Fatal("bad result completed")
	}
	var result map[string]any
	_ = json.Unmarshal(r.result, &result)
	result["deletionEligible"] = false
	r.result, _ = json.Marshal(result)
	r.storage.Qualified = false
	if _, err := r.client.performReceipt(id, false, false); err != nil {
		t.Fatal("committed historical fact rejected after policy withdrawal", err)
	}
	r.resultStatus = 404
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("completed missing result re-created")
	}
	if len(r.calls("/api/sync/receipts")) != 1 {
		t.Fatal("historical result created another receipt")
	}
}
func TestStorageDeclarationUnknownGETIsOnlyObservationAndOriginalPUTSurvives(t *testing.T) {
	r := cliReceiptSetup(t)
	b := r.control().syncBinding
	request, _ := syncNewUUID()
	raw, _ := json.Marshal(map[string]string{"requestId": request, "storeGeneration": b.StoreGeneration, "revisionEpoch": b.RestoreEpoch, "profileId": r.storage.Profile, "buildDigest": r.storage.Build, "verificationDigest": r.storage.Verification})
	o := receiptOperation{ID: request, Kind: "storage", Binding: b, Method: "PUT", Route: "/api/sync/storage", Body: string(raw), State: "pending"}
	o.Hash = receiptHash(o)
	if err := r.store.stageReceipt(r.control(), o, nil, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	r.storageUnknown = true
	if _, err := r.client.performReceipt(request, true, true); err == nil {
		t.Fatal("unknown declaration hidden")
	}
	if _, err := r.client.performReceipt(request, false, false); err != nil {
		t.Fatal(err)
	}
	after, _ := r.store.loadReceipt(request)
	if after.State != "pending" {
		t.Fatal("GET guessed original PUT success")
	}
	r.storageUnknown = false
	if _, err := r.client.performReceipt(request, true, false); err != nil {
		t.Fatal(err)
	}
	after, _ = r.store.loadReceipt(request)
	if after.State != "completed" || after.Body != o.Body || after.Hash != o.Hash {
		t.Fatal("declaration changed")
	}
	puts := []json.RawMessage{}
	for _, body := range r.calls("/api/sync/storage") {
		if len(body) > 0 {
			puts = append(puts, body)
		}
	}
	if len(puts) != 2 || !bytes.Equal(puts[0], puts[1]) {
		t.Fatal("declaration did not reuse original body")
	}
}
func TestReceiptUnqualifiedAndWrongLocalDurabilityDoNotMakeDeclaration(t *testing.T) {
	r := cliReceiptSetup(t)
	r.storage.Qualified = false
	r.storage.Qualification = ""
	if _, err := r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("unqualified storage received challenge")
	}
	if len(r.calls("/api/sync/receipt-challenges")) != 0 {
		t.Fatal("unqualified challenge requested")
	}
	build, err := receiptExecutableDigest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.store.db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.client.receiptCommand("storage-declare", "cli-native-test "+build+" "+strings.Repeat("c", 64)); err == nil {
		t.Fatal("NORMAL approved")
	}
	if len(r.calls("/api/sync/storage")) != 1 {
		t.Fatal("invalid local durability declared")
	}
	r.storage.Qualified = true
	r.storage.Qualification = r.qualification
	if _, err = r.client.receiveReceiptText(r.messageID); err == nil {
		t.Fatal("a declaration cannot waive live FULL durability")
	}
	var count int
	if err = r.store.db.QueryRow("SELECT count(*) FROM receipt_outbox").Scan(&count); err != nil || count != 0 {
		t.Fatal("undurable receipt staged", err)
	}
}
func TestReceiptReplacedDatabaseAndRestoreEpochCannotInheritOriginalIntent(t *testing.T) {
	r := cliReceiptSetup(t)
	r.resultStatus = 503
	_, _ = r.client.receiveReceiptText(r.messageID)
	id := r.receiptID()
	before, _ := r.store.loadReceipt(id)
	r.client.epoch = "2"
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("old epoch receipt inherited")
	}
	r.client.epoch = "1"
	if _, err := r.store.db.Exec("UPDATE metadata SET value=? WHERE key='sync-physical-generation'", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.performReceipt(id, true, false); err == nil {
		t.Fatal("replaced physical store inherited intent")
	}
	after, err := r.store.loadReceipt(id)
	if err != nil || after.Body != before.Body {
		t.Fatal("old original lost", err)
	}
	if len(r.calls("/api/sync/receipts")) != 0 {
		t.Fatal("old identity receipt written")
	}
}
