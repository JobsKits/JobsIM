package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func objectReceiptTestProof(app *App, now time.Time) objectReceiptProof {
	return objectReceiptProof{receiptProof: receiptProof{Scope: app.Config.DeploymentID, User: newID(), Device: newID(), Store: newID(), Epoch: "9007199254740993", Qualification: newID(), Message: newID(), Conversation: newID(), Content: "9007199254740995", BodyDigest: strings.Repeat("1", 64), Permission: strings.Repeat("2", 64), Expires: now.Add(time.Hour).Unix()}, Parent: newID(), Object: newID(), Size: "67108864", Digest: strings.Repeat("3", 64)}
}

func TestObjectReceiptProofIsPurposeAndByteBound(t *testing.T) {
	now := time.Unix(1794825000, 0)
	app := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x36}, 32)}}
	proof := objectReceiptTestProof(app, now)
	token := app.encodeObjectReceiptProof(proof)
	decoded, err := app.decodeObjectReceiptProof(token, now)
	if err != nil || decoded != proof {
		t.Fatal("exact object proof identity or large content counters changed")
	}
	for _, invalid := range []string{app.encodeReceiptProof(proof.receiptProof), strings.Replace(token, "ro1.", "rc1.", 1), token + ".extra", token[:len(token)-4]} {
		if _, err = app.decodeObjectReceiptProof(invalid, now); err == nil {
			t.Fatal("wrong purpose or damaged signature became an object receipt")
		}
	}
	for _, size := range []string{"", "-1", "01", "+1", "1.0", "1e1", " 1", "67108865", "9007199254740993"} {
		changed := proof
		changed.Size = size
		if _, err = app.decodeObjectReceiptProof(app.encodeObjectReceiptProof(changed), now); err == nil {
			t.Fatal("noncanonical or oversized original byte count accepted")
		}
	}
	changed := proof
	changed.Object = "preview"
	if _, err = app.decodeObjectReceiptProof(app.encodeObjectReceiptProof(changed), now); err == nil {
		t.Fatal("thumbnail label substituted for original object identity")
	}
	_, err = app.decodeObjectReceiptProof(token, now.Add(time.Hour))
	receiptTestError(t, err, "RECEIPT_PROOF_EXPIRED")
	other := *app
	other.Config.DeploymentID = newID()
	if _, err = other.decodeObjectReceiptProof(token, now); err == nil {
		t.Fatal("another deployment accepted the object proof")
	}
}

func TestObjectReceiptPolicyDoesNotUpgradeSQLiteOnly(t *testing.T) {
	profile := receiptTestProfile()
	if profile.StorageContract == receiptFileContract {
		t.Fatal("old profile fixture unexpectedly includes file durability")
	}
	profile.StorageContract = receiptFileContract
	if validateReceiptProfiles([]ReceiptProfile{profile}) != nil {
		t.Fatal("explicit combined SQLite and file durability approval rejected")
	}
	profile.StorageContract = "file_downloaded"
	if validateReceiptProfiles([]ReceiptProfile{profile}) == nil {
		t.Fatal("network download was accepted as durable file storage")
	}
}

type objectReceiptOwnerFixture struct {
	base     *receiptOwnerFixture
	object   receiptObject
	observed chan struct{}
}

func (f *objectReceiptOwnerFixture) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/internal/receipts/object" && request.URL.Path != "/internal/attachment-confirm" {
		return f.base.RoundTrip(request)
	}
	var value any = f.object
	if request.URL.Path == "/internal/receipts/object" && f.observed != nil {
		select {
		case f.observed <- struct{}{}:
		default:
		}
	}
	if request.URL.Path == "/internal/attachment-confirm" {
		value = map[string]bool{"committed": true}
	}
	raw, _ := json.Marshal(value)
	header := http.Header{}
	header.Set("X-JobsIM-Admission-Fence", f.base.app.admissionFenceIdentity())
	return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader(raw)), Request: request}, nil
}

func newObjectReceiptPGApp(t *testing.T) (*App, *objectReceiptOwnerFixture, string, storageDeclaration, string) {
	t.Helper()
	app, base, store, declaration := newReceiptPGApp(t)
	app.Config.ReceiptProfiles[0].StorageContract = receiptFileContract
	app.Config.RoleURLs["media"] = "http://127.0.0.1:1"
	base.message.Type = "file"
	base.message.AttachmentID = newID()
	fixture := &objectReceiptOwnerFixture{base: base, object: receiptObject{ID: base.message.AttachmentID, Size: "4", Digest: syncDigest([]byte("data")), ContentType: "application/octet-stream"}}
	app.Client.Transport = fixture
	registered, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil || registered["storage"].(map[string]any)["objectQualified"] != true {
		t.Fatal("combined file storage qualification failed")
	}
	qualification := registered["result"].(map[string]any)["qualificationId"].(string)
	challenge, err := callReceiptTest(app, "POST", "/api/sync/receipt-challenges", map[string]string{"messageId": base.message.ID}, app.syncReceiptChallenge)
	if err != nil {
		t.Fatal("parent descriptor challenge failed")
	}
	parent := contentReceiptRequest{RequestID: newID(), Store: store, Epoch: "1", Qualification: qualification, Kind: "message_content", Challenge: challenge["challenge"].(string), BodyDigest: challenge["bodyDigest"].(string)}
	result, err := callReceiptTest(app, "POST", "/api/sync/receipts", parent, app.submitSyncReceipt)
	if err != nil || result["objectComplete"] != false {
		t.Fatal("descriptor receipt implied complete attachment bytes")
	}
	return app, fixture, store, declaration, parent.RequestID
}

func objectReceiptPGRequest(t *testing.T, app *App, parent string) objectReceiptRequest {
	t.Helper()
	challenge, err := callReceiptTest(app, "POST", "/api/sync/object-receipt-challenges", map[string]string{"parentReceiptId": parent}, app.syncObjectReceiptChallenge)
	if err != nil {
		t.Fatal("original object challenge unavailable")
	}
	object := challenge["object"].(map[string]any)
	return objectReceiptRequest{RequestID: newID(), Store: challenge["storeGeneration"].(string), Epoch: challenge["restoreEpoch"].(string), Qualification: challenge["qualificationId"].(string), Kind: "object_received", Parent: parent, Challenge: challenge["challenge"].(string), Object: object["objectId"].(string), Size: object["sizeBytes"].(string), Digest: object["sha256"].(string)}
}

func objectReceiptPGCount(t *testing.T, app *App) int {
	t.Helper()
	var count int
	if app.DB.QueryRow(t.Context(), `SELECT count(*) FROM sync.object_receipts`).Scan(&count) != nil {
		t.Fatal("object receipt facts unavailable")
	}
	return count
}

func TestObjectReceiptPGOriginalResultsConcurrencyAndDescriptorSeparation(t *testing.T) {
	app, _, _, _, parent := newObjectReceiptPGApp(t)
	_, err := callReceiptTest(app, "POST", "/api/sync/object-receipt-challenges", map[string]string{"parentReceiptId": newID()}, app.syncObjectReceiptChallenge)
	receiptTestError(t, err, "CONTENT_RECEIPT_REQUIRED")
	request := objectReceiptPGRequest(t, app, parent)
	_, err = callReceiptTest(app, "POST", "/api/sync/object-receipt-results", request, app.syncObjectReceiptResults)
	receiptTestError(t, err, "OBJECT_RECEIPT_OPERATION_NOT_FOUND")
	if objectReceiptPGCount(t, app) != 0 {
		t.Fatal("read-only original query created an object receipt")
	}
	var wait sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := callReceiptTest(app, "POST", "/api/sync/object-receipts", request, app.submitSyncObjectReceipt)
			if err == nil && (result["objectComplete"] != true || result["targetRangeComplete"] != false || result["deletionEligible"] != false || result["sizeBytes"] != request.Size) {
				err = fail(500, "TEST_RESULT", "Complete-object result crossed the target deletion boundary")
			}
			failures <- err
		}()
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("concurrent original object receipt failed")
		}
	}
	if objectReceiptPGCount(t, app) != 1 {
		t.Fatal("duplicate object acknowledgements were counted as separate facts")
	}
	changed := request
	changed.Size = "3"
	_, err = callReceiptTest(app, "POST", "/api/sync/object-receipt-results", changed, app.syncObjectReceiptResults)
	receiptTestError(t, err, "IDEMPOTENCY_CONFLICT")
	app.Config.ReceiptProfiles = nil
	result, err := callReceiptTest(app, "POST", "/api/sync/object-receipt-results", request, app.syncObjectReceiptResults)
	if err != nil || result["objectComplete"] != true || result["deletionEligible"] != false {
		t.Fatal("historical object fact was lost or promoted after approval removal")
	}
}

func TestObjectReceiptPGRejectsOldQualificationContentPermissionAndBytes(t *testing.T) {
	app, fixture, _, declaration, parent := newObjectReceiptPGApp(t)
	request := objectReceiptPGRequest(t, app, parent)
	fixture.object.Digest = syncDigest([]byte("edit"))
	_, err := callReceiptTest(app, "POST", "/api/sync/object-receipts", request, app.submitSyncObjectReceipt)
	receiptTestError(t, err, "RECEIPT_OBJECT_CHANGED")
	fixture.object.Digest = request.Digest
	fixture.base.message.ContentVersion = "2"
	_, err = callReceiptTest(app, "POST", "/api/sync/object-receipts", request, app.submitSyncObjectReceipt)
	receiptTestError(t, err, "RECEIPT_CONTENT_CHANGED")
	fixture.base.message.ContentVersion = "1"
	fixture.base.permit = "2"
	_, err = callReceiptTest(app, "POST", "/api/sync/object-receipts", request, app.submitSyncObjectReceipt)
	receiptTestError(t, err, "RECEIPT_PERMISSION_CHANGED")
	fixture.base.permit = "1"
	app.Config.ReceiptProfiles[0].StorageContract = receiptSQLiteContract
	_, err = callReceiptTest(app, "POST", "/api/sync/object-receipts", request, app.submitSyncObjectReceipt)
	receiptTestError(t, err, "STORAGE_NOT_QUALIFIED")
	declaration.RequestID = newID()
	registered, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil || registered["result"].(map[string]any)["qualificationId"] == request.Qualification || registered["storage"].(map[string]any)["objectQualified"] != false {
		t.Fatal("changed durability contract did not rotate and downgrade qualification")
	}
	if objectReceiptPGCount(t, app) != 0 {
		t.Fatal("rejected content, bytes, permission or approval created an object fact")
	}
}

func TestObjectReceiptPGSQLiteApprovalNeedsNewQualificationAndParent(t *testing.T) {
	app, _, store, declaration := newReceiptPGApp(t)
	registered, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil {
		t.Fatal("old SQLite-only declaration failed")
	}
	old := registered["result"].(map[string]any)["qualificationId"]
	app.Config.ReceiptProfiles[0].StorageContract = receiptFileContract
	qualification, err := app.getStorageQualification(t.Context(), User{ID: app.Client.Transport.(*receiptOwnerFixture).user.ID, DeviceID: app.Client.Transport.(*receiptOwnerFixture).user.DeviceID, RestoreEpoch: "1"}, store)
	if err != nil || qualification.Qualified || qualification.ObjectQualified {
		t.Fatal("policy edit silently promoted an old storage declaration")
	}
	retried, err := callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil || retried["current"] != false || retried["result"].(map[string]any)["qualificationId"] != old {
		t.Fatal("original declaration retry was rewritten into new approval")
	}
	declaration.RequestID = newID()
	registered, err = callReceiptTest(app, "PUT", "/api/sync/storage", declaration, app.declareSyncStorage)
	if err != nil || registered["result"].(map[string]any)["qualificationId"] == old || registered["storage"].(map[string]any)["objectQualified"] != true {
		t.Fatal("fresh explicit combined declaration did not create a new qualification")
	}
}

func TestObjectReceiptPGMediaChecksRealOriginalFile(t *testing.T) {
	app, fixture, _, _, _ := newObjectReceiptPGApp(t)
	app.Role = "media"
	if _, err := app.DB.Exec(t.Context(), `CREATE SCHEMA media; CREATE TABLE media.objects(id uuid PRIMARY KEY,message_id uuid,purpose text,storage_key text,size_bytes bigint,sha256 bytea,content_type text)`); err != nil {
		t.Fatal("owned media metadata fixture unavailable")
	}
	data := []byte("data")
	if os.MkdirAll(filepath.Join(app.Config.StateDir, "Media"), 0700) != nil {
		t.Fatal("owned media directory unavailable")
	}
	path := filepath.Join(app.Config.StateDir, "Media", fixture.object.ID)
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("owned original media fixture unavailable")
	}
	if _, err := app.DB.Exec(t.Context(), `INSERT INTO media.objects VALUES($1,$2,'chat_attachment',$4,4,decode($3,'hex'),'application/octet-stream')`, fixture.object.ID, fixture.base.message.ID, fixture.object.Digest, fixture.object.ID); err != nil {
		t.Fatal("owned original media fixture metadata unavailable")
	}
	call := func(caller string) (*httptest.ResponseRecorder, error) {
		raw, _ := json.Marshal(map[string]string{"userId": fixture.base.user.ID, "messageId": fixture.base.message.ID, "objectId": fixture.object.ID})
		r := httptest.NewRequest("POST", "/internal/receipts/object", bytes.NewReader(raw))
		r.Header.Set("X-JobsIM-Caller", caller)
		r.Header.Set("X-JobsIM-Projection-Fence-Mode", "shared")
		r.Header.Set("X-JobsIM-Projection-Fence", app.gatewayProjectionProof(r, "shared"))
		w := httptest.NewRecorder()
		return w, app.receiptMediaObject(w, r)
	}
	w, err := call("sync")
	var descriptor receiptObject
	if err != nil || json.Unmarshal(w.Body.Bytes(), &descriptor) != nil || descriptor != fixture.object {
		t.Fatal("real original file descriptor differs from verified bytes")
	}
	_, err = call("message")
	receiptTestError(t, err, "RECEIPT_OWNER_REQUIRED")
	if os.WriteFile(path, []byte("EDIT"), 0600) != nil {
		t.Fatal("same-size corruption fixture unavailable")
	}
	w, err = call("sync")
	receiptTestError(t, err, "RECEIPT_OBJECT_INTEGRITY")
	if w.Body.Len() != 0 {
		t.Fatal("corrupt object leaked a successful descriptor")
	}
	if os.Remove(path) != nil || os.Symlink(filepath.Join(app.Config.StateDir, "absent"), path) != nil {
		t.Fatal("private symlink rejection fixture unavailable")
	}
	_, err = call("sync")
	receiptTestError(t, err, "RECEIPT_OBJECT_INTEGRITY")
	// No binary data or original proof is persisted by these receipt facts.
	var copied bool
	if app.DB.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM sync.object_receipts WHERE result ? 'bodyEnvelope')`).Scan(&copied) != nil || copied {
		t.Fatal("object acknowledgement persisted message or object bytes")
	}
}

func TestObjectReceiptPGDeadlineRecheckedAfterLocalLock(t *testing.T) {
	app, fixture, _, _, parent := newObjectReceiptPGApp(t)
	request := objectReceiptPGRequest(t, app, parent)
	proof, err := app.decodeObjectReceiptProof(request.Challenge, time.Now())
	if err != nil {
		t.Fatal("initial object challenge fixture unavailable")
	}
	proof.Expires = time.Now().Unix() + 2
	request.Challenge = app.encodeObjectReceiptProof(proof)
	lock, err := app.DB.Begin(t.Context())
	if err != nil {
		t.Fatal("independent storage qualification latch unavailable")
	}
	defer lock.Rollback(t.Context())
	if _, err = lock.Exec(t.Context(), `SELECT qualification_id FROM sync.storage_qualifications FOR UPDATE`); err != nil {
		t.Fatal("storage qualification latch was not acquired")
	}
	fixture.observed = make(chan struct{}, 1)
	finished := make(chan error, 1)
	go func() {
		_, err := callReceiptTest(app, "POST", "/api/sync/object-receipts", request, app.submitSyncObjectReceipt)
		finished <- err
	}()
	select {
	case <-fixture.observed:
	case <-time.After(3 * time.Second):
		t.Fatal("object validation did not reach the real storage commit latch")
	}
	timer := time.NewTimer(time.Until(time.Unix(proof.Expires, 0).Add(10 * time.Millisecond)))
	defer timer.Stop()
	select {
	case err := <-finished:
		_ = err
		t.Fatal("object receipt bypassed the held qualification row lock")
	case <-timer.C:
	}
	if lock.Commit(t.Context()) != nil {
		t.Fatal("qualification latch release failed")
	}
	select {
	case err := <-finished:
		receiptTestError(t, err, "RECEIPT_PROOF_EXPIRED")
	case <-time.After(3 * time.Second):
		t.Fatal("expired object receipt did not leave its local transaction")
	}
	if objectReceiptPGCount(t, app) != 0 {
		t.Fatal("a challenge that expired under the row lock created a receipt")
	}
}

func TestObjectReceiptMediaRequiresSourceFence(t *testing.T) {
	app := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x38}, 32), StateDir: t.TempDir()}}
	r := httptest.NewRequest("POST", "/internal/receipts/object", strings.NewReader(`{}`))
	r.Header.Set("X-JobsIM-Caller", "sync")
	receiptTestError(t, app.receiptMediaObject(httptest.NewRecorder(), r), "ADMISSION_FENCE_REQUIRED")
	r.Header.Set("X-JobsIM-Projection-Fence-Mode", "shared")
	r.Header.Set("X-JobsIM-Projection-Fence", "forged")
	receiptTestError(t, app.receiptMediaObject(httptest.NewRecorder(), r), "ADMISSION_FENCE_REQUIRED")
	for _, method := range []string{"GET", "HEAD"} {
		request := httptest.NewRequest(method, "/api/media/"+newID(), nil)
		if gatewayProjectionMode(request) != "shared" {
			t.Fatal("original media response can outlive its source authorization fence")
		}
	}
}

type objectReceiptBlockedWriter struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *objectReceiptBlockedWriter) Write(raw []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.ResponseRecorder.Write(raw)
}

func TestObjectReceiptPGOriginalDownloadDrainsBeforeSourceWrite(t *testing.T) {
	app, fixture, _, _, _ := newObjectReceiptPGApp(t)
	app.Role = "media"
	if _, err := app.DB.Exec(t.Context(), `CREATE SCHEMA media; CREATE TABLE media.objects(id uuid PRIMARY KEY,owner_id uuid,name text,content_type text,storage_key text,message_id uuid,purpose text,size_bytes bigint,sha256 bytea)`); err != nil {
		t.Fatal("owned download metadata fixture unavailable")
	}
	if _, err := app.DB.Exec(t.Context(), `INSERT INTO media.objects VALUES($1,$2,'original.bin','application/octet-stream',$3,$4,'chat_attachment',4,decode($5,'hex'))`, fixture.object.ID, fixture.base.user.ID, fixture.object.ID, fixture.base.message.ID, fixture.object.Digest); err != nil {
		t.Fatal("owned bound download metadata unavailable")
	}
	if os.MkdirAll(filepath.Join(app.Config.StateDir, "Media"), 0700) != nil || os.WriteFile(filepath.Join(app.Config.StateDir, "Media", fixture.object.ID), []byte("data"), 0600) != nil {
		t.Fatal("owned bound download bytes unavailable")
	}
	w := &objectReceiptBlockedWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(w.release) }) }
	defer unblock()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		app.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/media/"+fixture.object.ID, nil))
	}()
	select {
	case <-w.entered:
	case <-finished:
		t.Fatal("production media handler did not reach original object bytes")
	case <-time.After(3 * time.Second):
		t.Fatal("production media handler stalled before controlled response write")
	}
	ctx, stop := context.WithTimeout(t.Context(), 80*time.Millisecond)
	lock, err := app.admissionFence(ctx, true)
	stop()
	if err == nil {
		closeExportFence(lock)
		t.Fatal("source mutation acquired EX while original media bytes were still being written")
	}
	if ctx.Err() == nil {
		t.Fatal("source fence did not wait for the real download response")
	}
	unblock()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("production media response failed to drain")
	}
	if w.Code != 200 || w.Body.String() != "data" {
		t.Fatal("production media response did not preserve original complete bytes")
	}
	ctx, stop = context.WithTimeout(t.Context(), time.Second)
	defer stop()
	lock, err = app.admissionFence(ctx, true)
	if err != nil {
		t.Fatal("source fence remained held after the original download drained")
	}
	closeExportFence(lock)
}
