package jobsim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real isolated PostgreSQL and private files; authorization, source plan and
// governance RPCs are explicit fixtures, not evidence of native qualification.
type mediaRetentionFixture struct {
	base                             *retentionPGTransport
	app                              *App
	plan                             retentionMediaPlan
	user                             User
	store                            string
	denyAccess, failPlan, failCancel bool
	cancelled                        int
	lastAccess                       map[string]string
}

func (f *mediaRetentionFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	w.Header().Set("X-JobsIM-Admission-Fence", f.app.admissionFenceIdentity())
	switch r.URL.Path {
	case "/internal/authorize":
		respond(w, 200, f.user)
	case "/internal/retention/media-plan":
		if f.failPlan {
			return nil, errors.New("injected source outage")
		}
		respond(w, 200, f.plan)
	case "/internal/retention/attachment-access":
		if json.NewDecoder(r.Body).Decode(&f.lastAccess) != nil {
			return nil, errors.New("invalid fixture access")
		}
		q := f.lastAccess
		if f.denyAccess || q["userId"] != f.user.ID || q["deviceId"] != f.user.DeviceID || q["restoreEpoch"] != f.user.RestoreEpoch || q["storeGeneration"] != "" && q["storeGeneration"] != f.store || q["messageId"] != f.plan.Message || q["objectId"] != f.plan.Object || f.plan.DeleteScope == "global" {
			respond(w, 403, map[string]any{"error": map[string]string{"code": "RETAINED_TARGET_REQUIRED", "message": "Exact frozen target required"}})
		} else {
			respond(w, 200, map[string]any{"allowed": true, "attachmentId": f.plan.Object, "contentVersion": f.plan.Content, "cloudOnly": f.plan.DeleteScope == "cloud_only"})
		}
	case "/internal/exports/invalidate":
		if f.failCancel {
			return nil, errors.New("injected exact archive cancellation failure")
		}
		f.cancelled++
		respond(w, 200, map[string]any{"state": "cleared"})
	default:
		return f.base.RoundTrip(r)
	}
	return w.Result(), nil
}

func mediaPlanFixture() retentionMediaPlan {
	now := time.Now().UTC()
	return retentionMediaPlan{Message: newID(), Object: newID(), Content: "1", Generation: "1", Policy: "1", Epoch: "1", Digest: strings.Repeat("a", 64), AttachmentMode: "fixed_retention", ObjectDeadline: now.Add(time.Hour), CleanupEligible: true}
}

func newMediaRetentionPG(t *testing.T) (context.Context, *App, *mediaRetentionFixture, []byte) {
	t.Helper()
	ctx, m, _, base := newRetentionPG(t)
	a := &App{Config: m.Config, Role: "media", DB: m.DB}
	a.Config.RoleURLs["media"] = "http://127.0.0.1:7"
	if err := a.migrate(ctx); err != nil {
		t.Fatal("production media retention migration:", err)
	}
	f := &mediaRetentionFixture{base: base, app: a, plan: mediaPlanFixture(), user: User{ID: newID(), DeviceID: newID(), RestoreEpoch: "1"}, store: newID()}
	a.Client = &http.Client{Transport: f}
	data := []byte("owned whole original attachment fixture\n")
	hash := sha256.Sum256(data)
	retentionExec(t, ctx, a, `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,message_id,purpose) VALUES($1::uuid,$2,'fixture.txt','text/plain',$3,$4,$1::uuid::text,$5,'chat_attachment')`, f.plan.Object, f.user.ID, len(data), hash[:], f.plan.Message)
	dir := filepath.Join(a.Config.StateDir, "Media")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, f.plan.Object), data, 0600); err != nil {
		t.Fatal(err)
	}
	return ctx, a, f, data
}

func TestMediaRetentionIndependentModesRejectImplicitDelay(t *testing.T) {
	now := time.Now().UTC()
	p := mediaPlanFixture()
	p.DeleteScope = "cloud_only"
	due, err := mediaPlanDeadline(p, now)
	if err != nil || !due.Equal(p.ObjectDeadline) {
		t.Fatal("body erasure changed independent attachment lifetime")
	}
	p.AttachmentMode = "after_object_receipt"
	if _, err = mediaPlanDeadline(p, now); err == nil {
		t.Fatal("absent delay became immediate cleanup")
	}
	zero := int64(0)
	p.ObjectDelay = &zero
	due, err = mediaPlanDeadline(p, now)
	if err != nil || !due.Equal(p.ObjectDeadline) {
		t.Fatal("missing object completion became a body receipt")
	}
	completed := now.Add(-time.Minute)
	p.ObjectCompleteAt = &completed
	due, err = mediaPlanDeadline(p, now)
	if err != nil || !due.Equal(completed) {
		t.Fatal("explicit zero did not preserve object completion deadline")
	}
	future := now.Add(time.Hour)
	p.ObjectCompleteAt = &future
	if _, err = mediaPlanDeadline(p, now); err == nil {
		t.Fatal("future completion accepted")
	}
	p.ObjectCompleteAt = nil
	p.DeleteScope = "global"
	due, err = mediaPlanDeadline(p, now)
	if err != nil || !due.Equal(now) {
		t.Fatal("global erasure retained an object grant")
	}
}

func TestMediaRetentionStableTimestampAndStreamingDeadline(t *testing.T) {
	due := time.Now().UTC().Add(-time.Second)
	p := mediaPlanFixture()
	if mediaRetentionOperation("delete", p.Object, due) != mediaRetentionOperation("delete", p.Object, due.Truncate(time.Microsecond)) || mediaRetentionPayload(p, due)["dueAt"] != mediaRetentionPayload(p, due.Truncate(time.Microsecond))["dueAt"] {
		t.Fatal("database microsecond roundtrip changed original typed intent")
	}
	path := filepath.Join(t.TempDir(), "private-original")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := mediaRetainedReader{File: file, ctx: t.Context(), due: due}
	buffer := make([]byte, 8)
	if n, err := reader.Read(buffer); n != 0 || err == nil {
		t.Fatal("stream returned bytes after independent hard deadline")
	}
}

func TestMediaRetentionPGCloudOnlyExactDeviceResumeAndRange(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	f.plan.DeleteScope = "cloud_only"
	request := httptest.NewRequest("GET", "/api/media/"+f.plan.Object, nil)
	request.Header.Set("X-JobsIM-Store-Generation", f.store)
	request.Header.Set("Range", "bytes=6-10")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, request)
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[6:11]) || f.lastAccess["deviceId"] != f.user.DeviceID || f.lastAccess["storeGeneration"] != f.store {
		t.Fatal("original descriptor target cannot resume exact original range:", w.Code)
	}
	for _, mutate := range []func(*http.Request){func(r *http.Request) { r.Header.Set("X-JobsIM-Store-Generation", newID()) }, func(r *http.Request) { r.Header.Set("X-JobsIM-Store-Generation", "not-a-store") }} {
		r := request.Clone(ctx)
		mutate(r)
		denied := httptest.NewRecorder()
		a.Handler().ServeHTTP(denied, r)
		if denied.Code < 400 || bytes.Equal(denied.Body.Bytes(), data) {
			t.Fatal("another physical store received original bytes")
		}
	}
	f.denyAccess = true
	denied := httptest.NewRecorder()
	a.Handler().ServeHTTP(denied, request.Clone(ctx))
	if denied.Code != 403 {
		t.Fatal("revoked membership or target still downloaded")
	}
	f.denyAccess = false
	f.plan.DeleteScope = "global"
	denied = httptest.NewRecorder()
	a.Handler().ServeHTTP(denied, request.Clone(ctx))
	if denied.Code != 403 {
		t.Fatal("global source erasure retained old grant")
	}
}

func TestMediaRetentionPGDescriptorRequiresOriginalPhysicalTuple(t *testing.T) {
	_, a, f, data := newMediaRetentionPG(t)
	call := func(device, store, epoch string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"userId": f.user.ID, "deviceId": device, "storeGeneration": store, "restoreEpoch": epoch, "contentVersion": f.plan.Content, "messageId": f.plan.Message, "objectId": f.plan.Object})
		r := httptest.NewRequest("POST", "/internal/receipts/object", bytes.NewReader(body))
		r.Header.Set("X-JobsIM-Service-Key", encodeTestServiceKey(a.Config.Key))
		r.Header.Set("X-JobsIM-Caller", "sync")
		r.Header.Set("X-JobsIM-Projection-Fence-Mode", "shared")
		r.Header.Set("X-JobsIM-Projection-Fence", a.gatewayProjectionProof(r, "shared"))
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		return w
	}
	f.plan.DeleteScope = "cloud_only"
	w := call(f.user.DeviceID, f.store, "1")
	hash := sha256.Sum256(data)
	if w.Code != 200 || !strings.Contains(w.Body.String(), hex.EncodeToString(hash[:])) {
		t.Fatal("original complete descriptor refused:", w.Code)
	}
	for _, tuple := range [][3]string{{newID(), f.store, "1"}, {f.user.DeviceID, newID(), "1"}, {f.user.DeviceID, f.store, "2"}} {
		if w = call(tuple[0], tuple[1], tuple[2]); w.Code < 400 {
			t.Fatal("descriptor crossed original device/store/epoch")
		}
	}
}

func TestMediaRetentionPGDeadlineNeverExtendsAndJournalFailurePreservesBytes(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	_, first, err := a.mediaRetentionPlan(ctx, f.plan.Message, f.plan.Object)
	if err != nil {
		t.Fatal(err)
	}
	f.plan.Content = "2"
	f.plan.Generation = "2"
	f.plan.ObjectDeadline = f.plan.ObjectDeadline.Add(time.Hour)
	_, next, err := a.mediaRetentionPlan(ctx, f.plan.Message, f.plan.Object)
	if err != nil || !first.Equal(next) {
		t.Fatal("edit extended original object deadline")
	}
	f.plan.ObjectDeadline = time.Now().Add(-time.Minute)
	f.base.failJournal = true
	if err = a.resumeMediaRetention(ctx); err == nil {
		t.Fatal("independent journal outage silently cleaned object")
	}
	if got, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); err != nil || !bytes.Equal(got, data) {
		t.Fatal("unprotected deletion lost original bytes")
	}
	f.base.failJournal = false
	if err = a.resumeMediaRetention(ctx); err != nil {
		t.Fatal("durable retry:", err)
	}
	if _, err = os.Stat(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); !os.IsNotExist(err) {
		t.Fatal("due original bytes survived acknowledged cleanup")
	}
	var terminal, cleared, result bool
	if err = a.DB.QueryRow(ctx, `SELECT terminal,bytes_cleared,result_recorded FROM media.retention_floors WHERE object_id=$1`, f.plan.Object).Scan(&terminal, &cleared, &result); err != nil || !terminal || !cleared || !result {
		t.Fatal("durable cleanup did not close all stages")
	}
	if err = a.resumeMediaRetention(ctx); err != nil {
		t.Fatal("restart idempotency:", err)
	}
}

func TestMediaRetentionPGArchivesBlockUnlinkUntilExactCancellation(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	job, artifact := newID(), newID()
	retentionExec(t, ctx, a, `INSERT INTO media.export_holders(job_id,artifact_id,object_id,message_id,user_id,restore_epoch,build_generation,state,journal_seq,expires_at) VALUES($1,$2,$3,$4,$5,1,1,'active',1,now()+interval '1 hour')`, job, artifact, f.plan.Object, f.plan.Message, f.user.ID)
	f.plan.ObjectDeadline = time.Now().Add(-time.Minute)
	f.failCancel = true
	if err := a.resumeMediaRetention(ctx); err == nil {
		t.Fatal("archive failure acknowledged physical cleanup")
	}
	if got, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); err != nil || !bytes.Equal(got, data) {
		t.Fatal("archive cancellation failure removed quarantined source")
	}
	if err := a.requireMediaRetention(ctx, f.plan.Message, f.plan.Object); err == nil {
		t.Fatal("quarantined source retained grant")
	}
	f.failCancel = false
	f.failPlan = true
	if err := a.resumeMediaRetention(ctx); err != nil {
		t.Fatal("journal-protected retirement could not resume without old body:", err)
	}
	if f.cancelled != 1 {
		t.Fatal("exact archive cancellation was skipped")
	}
	if _, err := os.Stat(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); !os.IsNotExist(err) {
		t.Fatal("acknowledged retirement retained source")
	}
}

func TestMediaRetentionPGObjectReceiptZeroAndSiblingIsolation(t *testing.T) {
	ctx, a, f, _ := newMediaRetentionPG(t)
	sibling := newID()
	other := []byte("independent non-source bytes")
	if err := os.WriteFile(filepath.Join(a.Config.StateDir, "Media", sibling), other, 0600); err != nil {
		t.Fatal(err)
	}
	f.plan.AttachmentMode = "after_object_receipt"
	zero := int64(0)
	f.plan.ObjectDelay = &zero
	if err := a.resumeMediaRetention(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); err != nil {
		t.Fatal("body-only or absent object completion caused cleanup")
	}
	completed := time.Now().Add(-time.Second)
	f.plan.ObjectCompleteAt = &completed
	if err := a.resumeMediaRetention(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); !os.IsNotExist(err) {
		t.Fatal("explicit zero object delay failed to clean")
	}
	if got, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", sibling)); err != nil || !bytes.Equal(got, other) {
		t.Fatal("another source's private bytes were removed")
	}
}

func TestMediaRetentionPGReplayKeepsOldSnapshotClosed(t *testing.T) {
	ctx, a, f, _ := newMediaRetentionPG(t)
	due := time.Now().Add(-time.Minute).UTC()
	q := journalRecord{Owner: "media", OperationKey: mediaRetentionOperation("delete", f.plan.Object, due), Phase: "intent", Kind: "media.retention_delete", Payload: mediaRetentionPayload(f.plan, due)}
	if err := a.protect(ctx, q.OperationKey, q.Phase, q.Kind, q.Payload); err != nil {
		t.Fatal("real independent typed intent fixture:", err)
	}
	connection, err := a.DB.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	for range 2 {
		handled, replayErr := ReplayMediaRetentionSafety(ctx, connection.Conn(), q)
		if !handled || replayErr != nil {
			t.Fatal("original delete fact failed to replay:", replayErr)
		}
	}
	f.plan.ObjectDeadline = time.Now().Add(24 * time.Hour)
	if err = a.requireMediaRetention(ctx, f.plan.Message, f.plan.Object); err == nil {
		t.Fatal("restored old media row resurrected a terminal grant")
	}
	if err = a.resumeMediaRetention(ctx); err != nil {
		t.Fatal("restored original bytes were not re-cleared:", err)
	}
	if _, err = os.Stat(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); !os.IsNotExist(err) {
		t.Fatal("restore retained retired bytes")
	}
}

func TestMediaRetentionPGCleanupWaitsForLiveProjectionAndRefusesSymlink(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	f.plan.ObjectDeadline = time.Now().Add(-time.Minute)
	lock, err := a.admissionFence(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	err = a.resumeMediaRetention(short)
	cancel()
	closeExportFence(lock)
	if err == nil {
		t.Fatal("live shared projection did not block physical cleanup")
	}
	path := filepath.Join(a.Config.StateDir, "Media", f.plan.Object)
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
		t.Fatal("active response source was removed")
	}
	other := filepath.Join(t.TempDir(), "unrelated-original")
	if err = os.WriteFile(other, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(other, path); err != nil {
		t.Fatal(err)
	}
	if err = a.resumeMediaRetention(ctx); err == nil {
		t.Fatal("cleanup followed a replaced storage symlink")
	}
	if got, err := os.ReadFile(other); err != nil || !bytes.Equal(got, data) {
		t.Fatal("unrelated original was altered")
	}
}

func TestMediaRetentionPGLegacyRequiresExplicitOwnerWhitelist(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	message, object := f.plan.Message, f.plan.Object
	f.plan = retentionMediaPlan{Message: message, Object: object, Legacy: true, CleanupEligible: false}
	r := httptest.NewRequest("GET", "/api/media/"+object, nil)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatal("explicit owner-whitelisted legacy original was locked:", w.Code)
	}
	if err := a.resumeMediaRetention(ctx); err != nil {
		t.Fatal("legacy no-cleanup scan:", err)
	}
	var count int
	if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM media.retention_floors WHERE object_id=$1`, object).Scan(&count); err != nil || count != 0 {
		t.Fatal("legacy branch invented a cleanup deadline")
	}
	f.failPlan = true
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r.Clone(ctx))
	if w.Code < 400 {
		t.Fatal("missing new assignment was silently treated as legacy")
	}
	f.failPlan = false
	f.plan.CleanupEligible = true
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r.Clone(ctx))
	if w.Code < 400 {
		t.Fatal("ambiguous legacy/cleanup flags accepted")
	}
	if got, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", object)); err != nil || !bytes.Equal(got, data) {
		t.Fatal("unknown or legacy plan removed historical bytes")
	}
}

func TestMediaRetentionPGClockAlarmSurvivesRestartAndPreservesBytes(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	f.plan.ObjectDeadline = time.Now().Add(-time.Minute)
	retentionExec(t, ctx, a, `INSERT INTO media.lifecycle_clock(singleton,last_database_time,alarmed) VALUES(true,now()+interval '10 minutes',false)`)
	if err := a.resumeMediaRetention(ctx); err == nil {
		t.Fatal("backward/unverified clock irreversibly cleared bytes")
	}
	var alarm bool
	if err := a.DB.QueryRow(ctx, `SELECT alarmed FROM media.lifecycle_clock WHERE singleton`).Scan(&alarm); err != nil || !alarm {
		t.Fatal("clock anomaly did not persist its closure")
	}
	retentionExec(t, ctx, a, `UPDATE media.lifecycle_clock SET last_database_time=now()`)
	retentionClockObservations.Delete(a)
	if err := a.resumeMediaRetention(ctx); err == nil {
		t.Fatal("worker restart bypassed durable clock alarm")
	}
	if got, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", f.plan.Object)); err != nil || !bytes.Equal(got, data) {
		t.Fatal("clock alarm lost original bytes")
	}
}

func TestMediaRetentionPGLegacyGlobalErasureNeedsConfirmedSourceEvent(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	f.plan = retentionMediaPlan{Message: f.plan.Message, Object: f.plan.Object, Epoch: "1", Legacy: true, CleanupEligible: true, ErasureOnly: true, SourceJob: newID(), SourceEvent: newID(), ObjectDeadline: time.Now().Add(-time.Minute), DeleteScope: "global"}
	path := filepath.Join(a.Config.StateDir, "Media", f.plan.Object)
	event := f.plan.SourceEvent
	f.plan.SourceEvent = ""
	if err := a.resumeMediaRetention(ctx); err == nil {
		t.Fatal("legacy erasure without confirmed source event was accepted")
	}
	f.plan.SourceEvent = event
	f.base.failJournal = true
	if err := a.resumeMediaRetention(ctx); err == nil {
		t.Fatal("legacy erasure bypassed independent journal")
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
		t.Fatal("unprotected legacy erasure lost original")
	}
	f.base.failJournal = false
	if err := a.resumeMediaRetention(ctx); err != nil {
		t.Fatal("confirmed legacy global erasure:", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("confirmed global legacy source retained original bytes")
	}
	var invented int
	if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM media.retention_floors WHERE object_id=$1`, f.plan.Object).Scan(&invented); err != nil || invented != 0 {
		t.Fatal("legacy erasure fabricated receipt manifest or targets")
	}
	q := journalRecord{Owner: "media", OperationKey: "media-legacy-delete:" + f.plan.Object + ":" + f.plan.SourceJob, Phase: "intent", Kind: "media.legacy_retention_delete", Payload: legacyMediaErasurePayload(f.plan)}
	connection, err := a.DB.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	handled, err := ReplayMediaRetentionSafety(ctx, connection.Conn(), q)
	connection.Release()
	if err != nil || !handled {
		t.Fatal("legacy deletion failed independent replay:", err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f.failPlan = true
	if err = a.requireMediaRetention(ctx, f.plan.Message, f.plan.Object); err == nil {
		t.Fatal("restored older legacy plan resurrected a terminal grant")
	}
	if err = a.resumeMediaRetention(ctx); err != nil {
		t.Fatal("legacy retirement could not recover without old source body:", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("replayed legacy erasure retained restored bytes")
	}
}

// Use the same encoded key shape as internal(), without involving any account.
func encodeTestServiceKey(key []byte) string { return base64.RawURLEncoding.EncodeToString(key) }
