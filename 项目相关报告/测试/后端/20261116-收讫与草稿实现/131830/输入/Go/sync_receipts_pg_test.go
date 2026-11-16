package jobsim

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Only SQL below is real. The authentication, permission and message owners are
// explicit process-local fixtures, not an HTTPS or device durability proof.
type receiptOwnerFixture struct {
	mu      sync.Mutex
	app     *App
	user    User
	message Message
	permit  string
}

func (f *receiptOwnerFixture) RoundTrip(request *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var output any
	switch request.URL.Path {
	case "/internal/authorize":
		output = f.user
	case "/internal/sync/watermark":
		output = syncOwnerCapture{RestoreEpoch: f.user.RestoreEpoch, Watermark: "1", PermissionVersion: f.permit}
	case "/internal/read-scopes":
		output = map[string]any{"items": []map[string]string{{"id": f.message.ConversationID, "membershipGeneration": "1"}}}
	case "/internal/sync/hydrate":
		raw, _ := json.Marshal(f.message)
		output = syncProjection{State: "available", Object: f.message, BodyDigest: syncDigest(raw), BodyEnvelope: base64.RawURLEncoding.EncodeToString(raw), CurrentRevisionEpoch: f.user.RestoreEpoch, CurrentRevision: "1"}
	default:
		return nil, errors.New("unselected owner dependency in receipt PG fixture")
	}
	raw, _ := json.Marshal(output)
	header := http.Header{}
	header.Set("X-JobsIM-Admission-Fence", f.app.admissionFenceIdentity())
	return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader(raw)), Request: request}, nil
}

func callReceiptTest(app *App, method, path string, body any, handler func(http.ResponseWriter, *http.Request) error) (map[string]any, error) {
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	recorder := httptest.NewRecorder()
	if err := handler(recorder, request); err != nil {
		return nil, err
	}
	var output map[string]any
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &output) != nil {
		return nil, errors.New("receipt response shape rejected")
	}
	return output, nil
}

func newReceiptPGApp(t *testing.T) (*App, *receiptOwnerFixture, string, storageDeclaration) {
	t.Helper()
	ctx, app, user := newSyncRegisterPGApp(t)
	profile := receiptTestProfile()
	app.Config.Key = bytes.Repeat([]byte{0x37}, 32)
	app.Config.StateDir = t.TempDir()
	app.Config.ReceiptProfiles = []ReceiptProfile{profile}
	app.Config.RoleURLs = map[string]string{"identity": "http://127.0.0.1:1", "directory": "http://127.0.0.1:1", "conversation": "http://127.0.0.1:1", "message": "http://127.0.0.1:1"}
	fixture := &receiptOwnerFixture{app: app, user: user, permit: "1", message: Message{ID: newID(), ConversationID: newID(), SenderID: newID(), ClientMsgID: "receipt-test-message", Type: "text", Text: "independent fixture body", ContentVersion: "1", Version: "1", CreationEpoch: "1", CreationSequence: "1", CreatedAt: time.Now().UTC()}}
	app.Client = &http.Client{Transport: fixture}
	tx, err := app.DB.Begin(ctx)
	if err != nil {
		t.Fatal("receipt migration transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if migrateSyncReceipts(ctx, tx) != nil || migrateSyncReceipts(ctx, tx) != nil || tx.Commit(ctx) != nil {
		t.Fatal("receipt migration or repeated migration failed")
	}
	store := newID()
	if _, err = callSyncRegisterPG(app, "1", store, newID()); err != nil {
		t.Fatal("receipt physical store fixture registration failed")
	}
	declaration := storageDeclaration{RequestID: newID(), StoreGeneration: store, RevisionEpoch: "1", ProfileID: profile.ID, BuildDigest: profile.BuildDigest, VerificationDigest: profile.VerificationDigest}
	return app, fixture, store, declaration
}

func receiptPGCount(t *testing.T, app *App) int {
	t.Helper()
	var count int
	if app.DB.QueryRow(t.Context(), "SELECT count(*) FROM sync.content_receipts").Scan(&count) != nil {
		t.Fatal("receipt row count unavailable")
	}
	return count
}

func TestReceiptPGOriginalFactsAndVersionRevocation(t *testing.T) {
	app, fixture, store, declaration := newReceiptPGApp(t)
	_, err := callReceiptTest(app, "POST", "/api/sync/receipt-challenges", map[string]string{"messageId": fixture.message.ID}, app.syncReceiptChallenge)
	receiptTestError(t, err, "STORAGE_NOT_QUALIFIED")
	registered, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil || registered["current"] != true || registered["deletionEligible"] != false {
		t.Fatal("approved storage declaration failed or granted deletion eligibility")
	}
	qualification := registered["result"].(map[string]any)["qualificationId"].(string)
	unchanged, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil || !reflectReceiptResult(unchanged["result"], registered["result"]) {
		t.Fatal("original declaration retry changed its storage identity")
	}
	conflict := declaration
	conflict.StoreGeneration = newID()
	_, err = callReceiptTest(app, "PUT", "/api/sync/storage", conflict, app.declareSyncStorage)
	receiptTestError(t, err, "STORE_GENERATION_CHANGED")
	challenge, err := callReceiptTest(app, "POST", "/api/sync/receipt-challenges", map[string]string{"messageId": fixture.message.ID}, app.syncReceiptChallenge)
	if err != nil || challenge["requiresAtomicLocalCommit"] != true || challenge["objectComplete"] != false {
		t.Fatal("complete original body receipt challenge failed")
	}
	request := contentReceiptRequest{RequestID: newID(), Store: store, Epoch: "1", Qualification: qualification, Kind: "message_content", Challenge: challenge["challenge"].(string), BodyDigest: challenge["bodyDigest"].(string)}
	_, err = callReceiptTest(app, "POST", "/api/sync/receipt-results", request, app.syncReceiptResults)
	receiptTestError(t, err, "RECEIPT_OPERATION_NOT_FOUND")
	if receiptPGCount(t, app) != 0 {
		t.Fatal("read-only unknown lookup created a receipt")
	}
	result, err := callReceiptTest(app, "POST", "/api/sync/receipts", request, app.submitSyncReceipt)
	if err != nil || result["storageQualifiedAtCommit"] != true || result["targetRangeComplete"] != false || result["deletionEligible"] != false || receiptPGCount(t, app) != 1 {
		t.Fatal("qualified body fact was not committed independently of deletion and objects")
	}
	fixture.mu.Lock()
	fixture.message.ContentVersion = "2"
	fixture.message.Text = "edited later fixture"
	fixture.mu.Unlock()
	prior, err := callReceiptTest(app, "POST", "/api/sync/receipt-results", request, app.syncReceiptResults)
	if err != nil || !reflectReceiptResult(prior, result) || prior["contentVersion"] != "1" {
		t.Fatal("original result was rewritten by later content")
	}
	later := request
	later.RequestID = newID()
	_, err = callReceiptTest(app, "POST", "/api/sync/receipts", later, app.submitSyncReceipt)
	receiptTestError(t, err, "RECEIPT_CONTENT_CHANGED")
	conflicted := request
	conflicted.BodyDigest = stringsRepeatDigest('e')
	_, err = callReceiptTest(app, "POST", "/api/sync/receipts", conflicted, app.submitSyncReceipt)
	receiptTestError(t, err, "IDEMPOTENCY_CONFLICT")
	if receiptPGCount(t, app) != 1 {
		t.Fatal("rejected old content or conflicting nonce inserted a receipt")
	}
}

func stringsRepeatDigest(letter byte) string { return string(bytes.Repeat([]byte{letter}, 64)) }

func reflectReceiptResult(left, right any) bool {
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return bytes.Equal(l, r)
}

func TestReceiptPGPermissionPolicyAndDeviceDoNotInherit(t *testing.T) {
	app, fixture, store, declaration := newReceiptPGApp(t)
	registered, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil {
		t.Fatal("storage declaration fixture failed")
	}
	qualification := registered["result"].(map[string]any)["qualificationId"].(string)
	challenge, err := callReceiptTest(app, "POST", "/api/sync/receipt-challenges", map[string]string{"messageId": fixture.message.ID}, app.syncReceiptChallenge)
	if err != nil {
		t.Fatal("challenge fixture failed")
	}
	request := contentReceiptRequest{RequestID: newID(), Store: store, Epoch: "1", Qualification: qualification, Kind: "message_content", Challenge: challenge["challenge"].(string), BodyDigest: challenge["bodyDigest"].(string)}
	fixture.mu.Lock()
	fixture.permit = "3" // ABA has a new owner permission generation.
	fixture.mu.Unlock()
	_, err = callReceiptTest(app, "POST", "/api/sync/receipts", request, app.submitSyncReceipt)
	receiptTestError(t, err, "RECEIPT_PERMISSION_CHANGED")
	fixture.mu.Lock()
	fixture.permit = "1"
	fixture.mu.Unlock()
	app.Config.ReceiptProfiles = nil
	_, err = callReceiptTest(app, "POST", "/api/sync/receipts", request, app.submitSyncReceipt)
	receiptTestError(t, err, "STORAGE_NOT_QUALIFIED")
	app.Config.ReceiptProfiles = []ReceiptProfile{receiptTestProfile()}
	fixture.mu.Lock()
	fixture.user.DeviceID = newID()
	fixture.mu.Unlock()
	otherStore := newID()
	if _, err = callSyncRegisterPG(app, "1", otherStore, newID()); err != nil {
		t.Fatal("second physical device registration failed")
	}
	request.Store = otherStore
	_, err = callReceiptTest(app, "POST", "/api/sync/receipts", request, app.submitSyncReceipt)
	receiptTestError(t, err, "RECEIPT_CONTEXT_CHANGED")
	if _, err = app.DB.Exec(t.Context(), "UPDATE sync.instance SET restore_epoch=2"); err != nil {
		t.Fatal("owned SQL epoch fixture failed")
	}
	fixture.mu.Lock()
	fixture.user.RestoreEpoch = "2"
	fixture.mu.Unlock()
	request.Epoch = "1"
	_, err = callReceiptTest(app, "POST", "/api/sync/receipts", request, app.submitSyncReceipt)
	receiptTestError(t, err, "RESTORE_EPOCH_CHANGED")
	if receiptPGCount(t, app) != 0 {
		t.Fatal("revoked permission, policy, device or epoch fabricated a receipt")
	}
}

func TestReceiptPGConcurrentDuplicateHasOneOriginalFact(t *testing.T) {
	app, fixture, store, declaration := newReceiptPGApp(t)
	registered, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil {
		t.Fatal("storage declaration fixture failed")
	}
	challenge, err := callReceiptTest(app, "POST", "/api/sync/receipt-challenges", map[string]string{"messageId": fixture.message.ID}, app.syncReceiptChallenge)
	if err != nil {
		t.Fatal("challenge fixture failed")
	}
	request := contentReceiptRequest{RequestID: newID(), Store: store, Epoch: "1", Qualification: registered["result"].(map[string]any)["qualificationId"].(string), Kind: "message_content", Challenge: challenge["challenge"].(string), BodyDigest: challenge["bodyDigest"].(string)}
	type outcome struct {
		value map[string]any
		err   error
	}
	start, done := make(chan struct{}), make(chan outcome, 8)
	for i := 0; i < 8; i++ {
		go func() {
			<-start
			value, failure := callReceiptTest(app, "POST", "/api/sync/receipts", request, app.submitSyncReceipt)
			done <- outcome{value, failure}
		}()
	}
	close(start)
	var first map[string]any
	for i := 0; i < 8; i++ {
		result := <-done
		if result.err != nil {
			t.Fatal("concurrent duplicate receipt failed; private details suppressed")
		}
		if first == nil {
			first = result.value
		} else if !reflectReceiptResult(first, result.value) {
			t.Fatal("concurrent retries received different original results")
		}
	}
	if receiptPGCount(t, app) != 1 {
		t.Fatal("concurrent duplicate receipts created multiple original facts")
	}
}
