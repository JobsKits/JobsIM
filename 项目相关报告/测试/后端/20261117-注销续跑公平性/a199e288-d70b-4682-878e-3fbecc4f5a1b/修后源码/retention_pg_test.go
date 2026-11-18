package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Real owned PostgreSQL; private RPC transport and qualification are explicit
// fixtures. These tests do not approve any shipped native build or claim HTTPS.
type retentionPGTransport struct {
	message, sync, journal *App
	audience               []receiptAudienceUser
	devices                []receiptDeviceState
	failJournal            bool
	invalidated            int
	onAppend               func(journalRecord)
}

func (f *retentionPGTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	w.Header().Set("X-JobsIM-Admission-Fence", f.message.admissionFenceIdentity())
	switch r.URL.Path {
	case "/internal/state":
		respond(w, 200, journalState{RestoreEpoch: 1})
	case "/internal/append":
		if f.onAppend != nil {
			raw, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var record journalRecord
			_ = json.Unmarshal(raw, &record)
			f.onAppend(record)
		}
		if f.failJournal {
			return nil, errors.New("injected independent journal acknowledgement failure")
		}
		f.journal.Handler().ServeHTTP(w, r)
	case "/internal/receipts/audience":
		respond(w, 200, map[string]any{"users": f.audience})
	case "/internal/receipts/devices":
		respond(w, 200, map[string]any{"items": f.devices})
	case "/internal/member":
		respond(w, 200, map[string]any{"kind": "direct", "membershipGeneration": "1"})
	case "/internal/retention/checkpoint", "/internal/retention/complete":
		f.message.Handler().ServeHTTP(w, r)
	case "/internal/receipts/freeze":
		f.sync.Handler().ServeHTTP(w, r)
	case "/internal/exports/invalidate":
		f.invalidated++
		respond(w, 200, map[string]any{})
	default:
		return nil, errors.New("unexpected dependency in retention PG fixture")
	}
	return w.Result(), nil
}
func newRetentionPG(t *testing.T) (context.Context, *App, *App, *retentionPGTransport) {
	t.Helper()
	ctx, syncApp, _ := newSyncRegisterPGApp(t)
	syncApp.Config.StateDir = t.TempDir()
	syncApp.Config.Key = bytes.Repeat([]byte{0x51}, 32)
	syncApp.Config.NATSURL = ""
	syncApp.Config.RoleURLs = map[string]string{"identity": "http://127.0.0.1:1", "conversation": "http://127.0.0.1:2", "sync": "http://127.0.0.1:3", "message": "http://127.0.0.1:4", "journal": "http://127.0.0.1:5", "governance": "http://127.0.0.1:6"}
	cfg, err := normalizeAdmissionState(syncApp.Config)
	if err != nil {
		t.Fatal(err)
	}
	syncApp.Config = cfg
	m := &App{Config: cfg, Role: "message", DB: syncApp.DB}
	j := &App{Config: cfg, Role: "journal", DB: syncApp.DB}
	for _, a := range []*App{m, j} {
		if err = a.migrate(ctx); err != nil {
			t.Fatal("production retention migration:", err)
		}
	}
	tx, err := syncApp.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrateSyncReceipts(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f := &retentionPGTransport{message: m, sync: syncApp, journal: j}
	client := &http.Client{Transport: f}
	m.Client, syncApp.Client, j.Client = client, client, client
	return ctx, m, syncApp, f
}
func retentionExec(t *testing.T, ctx context.Context, a *App, sql string, args ...any) {
	t.Helper()
	if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
		t.Fatal("isolated retention SQL:", err)
	}
}
func retentionManifestFixture() receiptDeliveryManifest {
	u := newID()
	now := time.Now().UTC().Add(-time.Second)
	m := receiptDeliveryManifest{Message: newID(), Conversation: newID(), Epoch: "1", Content: "1", Generation: "1", Policy: "1", Scope: "all_target_devices", Audience: []receiptAudienceUser{{u, "1"}}, Targets: []receiptDeliveryTarget{}, FrozenAt: now, Expires: now.Add(24 * time.Hour)}
	m.Digest = receiptManifestDigest(m)
	return m
}
func retentionInsertMessage(t *testing.T, ctx context.Context, a *App, p retentionPrepared) {
	t.Helper()
	m := p.Manifest
	retentionExec(t, ctx, a, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text','owned retention content',1)`, m.Message, m.Conversation, m.Audience[0].User, newID(), []byte("digest"))
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = a.persistMessageRetention(ctx, tx, m.Message, p); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
func retentionCompleteCall(a *App, m receiptDeliveryManifest, kind string, at time.Time) error {
	raw, _ := json.Marshal(receiptCompletion{Message: m.Message, Generation: m.Generation, Digest: m.Digest, Kind: kind, At: at})
	r := httptest.NewRequest("POST", "/internal/retention/complete", bytes.NewReader(raw))
	r.Header.Set("X-JobsIM-Caller", "sync")
	return a.retentionComplete(httptest.NewRecorder(), r)
}

func TestRetentionFrozenRangeNeverTreatsEmptyOrRevokedAsDelivered(t *testing.T) {
	m := retentionManifestFixture()
	u := m.Audience[0].User
	v := newID()
	m.Audience = append(m.Audience, receiptAudienceUser{v, "1"})
	target := receiptDeliveryTarget{User: u, Device: newID(), Store: newID(), Qualification: newID(), ObjectCapable: true}
	m.Targets = []receiptDeliveryTarget{target}
	key := u + ":" + target.Device
	for _, scope := range []string{"all_target_devices", "per_user_any_device"} {
		if receiptRangeComplete(m.Audience, m.Targets, scope, map[string]bool{key: true}, nil, false) {
			t.Fatal("zero-target human silently removed")
		}
		if receiptRangeComplete(m.Audience[:1], m.Targets, scope, map[string]bool{key: true}, map[string]bool{key: true}, false) {
			t.Fatal("all-revoked user falsely delivered")
		}
	}
	m.Audience = m.Audience[:1]
	if !receiptRangeComplete(m.Audience, m.Targets, m.Scope, map[string]bool{key: true}, nil, true) {
		t.Fatal("complete exact original object target rejected")
	}
	m.Targets[0].ObjectCapable = false
	if receiptRangeComplete(m.Audience, m.Targets, m.Scope, map[string]bool{key: true}, nil, true) {
		t.Fatal("body-only storage counted as complete object")
	}
}
func TestRetentionPolicyExplicitIndependentZeroAndBounds(t *testing.T) {
	p := defaultRetentionPolicy()
	p.Mode = "after_receipt"
	if validateRetentionPolicy(p) == nil {
		t.Fatal("missing n accepted")
	}
	zero := int64(0)
	p.Delay = &zero
	if validateRetentionPolicy(p) != nil {
		t.Fatal("explicit immediate queue rejected")
	}
	p.AttachmentMode = "after_object_receipt"
	if validateRetentionPolicy(p) == nil {
		t.Fatal("body delay reused for object")
	}
	p.AttachmentDelay = &zero
	if validateRetentionPolicy(p) != nil {
		t.Fatal("independent zero rejected")
	}
	bad := int64(604801)
	p.Delay = &bad
	if validateRetentionPolicy(p) == nil {
		t.Fatal("unbounded deletion delay accepted")
	}
}
func TestRetentionPGFirstCompletionImmutableAndJournalPending(t *testing.T) {
	ctx, a, _, f := newRetentionPG(t)
	m := retentionManifestFixture()
	zero := int64(0)
	p := defaultRetentionPolicy()
	p.Mode, p.Delay = "after_receipt", &zero
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: p, PolicyVersion: "1", Manifest: m})
	at := time.Now().UTC().Add(-100 * time.Millisecond)
	f.failJournal = true
	completionErr := retentionCompleteCall(a, m, "content", at)
	if completionErr == nil {
		t.Fatal("unprotected deadline reported effective")
	}
	var first time.Time
	var state string
	if a.DB.QueryRow(ctx, `SELECT receipt_complete_at,state FROM message.retention_assignments WHERE message_id=$1`, m.Message).Scan(&first, &state) != nil || state != "protection_pending" {
		t.Fatal("first completion lost:", completionErr)
	}
	if _, err := a.prepareRetentionEdit(ctx, Message{ID: m.Message, ContentVersion: "1"}); err == nil {
		t.Fatal("scheduled range edited")
	}
	f.failJournal = false
	if err := retentionCompleteCall(a, m, "content", at.Add(time.Millisecond)); err != nil {
		t.Fatal("stable completion retry:", err)
	}
	var due time.Time
	if a.DB.QueryRow(ctx, `SELECT due_at,state FROM message.lifecycle_jobs WHERE message_id=$1 AND reason='receipt_complete'`, m.Message).Scan(&due, &state) != nil || !due.Equal(first) || state != "scheduled" {
		t.Fatal("duplicate completion extended first n=0 deadline")
	}
	stale := m
	stale.Generation = "2"
	if retentionCompleteCall(a, stale, "content", at) == nil {
		t.Fatal("new generation accepted old manifest digest")
	}
	var count int
	if a.DB.QueryRow(ctx, `SELECT count(*) FROM journal.entries WHERE kind='message.retention_deadline'`).Scan(&count) != nil || count != 2 {
		t.Fatal("original hard and receipt deadlines not durably journalled")
	}
}
func TestRetentionPGDueCloudOnlyThenFutureGlobalAndNoDowngrade(t *testing.T) {
	ctx, a, _, f := newRetentionPG(t)
	m := retentionManifestFixture()
	p := defaultRetentionPolicy()
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: p, PolicyVersion: "1", Manifest: m})
	// A future global trigger must not make an earlier cloud expiry burn local history.
	if err := a.QueueRetentionErasure(ctx, m.Message, "future-global-operation", "global", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := a.QueueRetentionErasure(ctx, m.Message, "due-cloud-operation", "cloud_only", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	// Real export-holder rows exercise synchronous source withdrawal; the external
	// governance acknowledgement is an explicit fixture, never a fake HTTPS claim.
	retentionExec(t, ctx, a, `INSERT INTO message.export_holders(job_id,artifact_id,message_id,user_id,restore_epoch,build_generation,source_version,state,journal_seq,expires_at) VALUES($1,$2,$3,$4,1,1,1,'active',1,now()+interval '1 day')`, newID(), newID(), m.Message, m.Audience[0].User)
	if err := a.resumeRetention(ctx, 4); err != nil {
		t.Fatal("due worker:", err)
	}
	var body, scope string
	var deleted bool
	var gen int64
	if a.DB.QueryRow(ctx, `SELECT text,delete_scope,deleted,deletion_generation FROM message.messages WHERE id=$1`, m.Message).Scan(&body, &scope, &deleted, &gen) != nil || body != "" || scope != "cloud_only" || deleted || gen != 1 || f.invalidated != 1 {
		t.Fatal("cloud-only source or holder cleanup failed")
	}
	if err := a.QueueRetentionErasure(ctx, m.Message, "due-global-operation", "global", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := a.resumeRetention(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if err := a.QueueRetentionErasure(ctx, m.Message, "late-cloud-operation", "cloud_only", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := a.resumeRetention(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if a.DB.QueryRow(ctx, `SELECT delete_scope,deleted,deletion_generation FROM message.messages WHERE id=$1`, m.Message).Scan(&scope, &deleted, &gen) != nil || scope != "global" || !deleted || gen != 2 {
		t.Fatal("global downgraded or duplicate tombstone incremented")
	}
	if a.retentionSendFloor(ctx, m.Message) == nil {
		t.Fatal("erased logical identity could be resent")
	}
	var before, after, complete int
	if a.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM message.lifecycle_events WHERE phase='before'),(SELECT count(*) FROM message.lifecycle_events WHERE phase='after'),(SELECT count(*) FROM message.lifecycle_jobs WHERE state='complete')`).Scan(&before, &after, &complete) != nil || before != 3 || after != 3 || complete != 3 {
		t.Fatal("stable lifecycle events/results incomplete")
	}
}
func TestRetentionPGReadDeadlineAndEditPreserveOriginalBoundary(t *testing.T) {
	ctx, a, _, _ := newRetentionPG(t)
	m := retentionManifestFixture()
	p := defaultRetentionPolicy()
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: p, PolicyVersion: "1", Manifest: m})
	edit, err := a.prepareRetentionEdit(ctx, Message{ID: m.Message, ContentVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if edit.Manifest.Generation != "2" || edit.Manifest.Content != "2" || !edit.Manifest.FrozenAt.Equal(m.FrozenAt) || !edit.Manifest.Expires.Equal(m.Expires) || syncJSONDigest(edit.Manifest.Audience) != syncJSONDigest(m.Audience) {
		t.Fatal("editing extended send snapshot")
	}
	retentionExec(t, ctx, a, `INSERT INTO message.lifecycle_jobs(scope_id,id,message_id,generation,policy_version,restore_epoch,reason,delete_scope,due_at,before_event,after_event) VALUES($1,$2,$3,1,1,1,'history_expired','cloud_only',now()-interval '1 second',$4,$5)`, a.Config.DeploymentID, newID(), m.Message, newID(), newID())
	projected := Message{ID: m.Message, Text: "must not escape", AttachmentID: newID()}
	if err = a.retentionReadState(ctx, &projected); err != nil || projected.Text != "" || projected.AttachmentID != "" || !projected.ContentUnavailable || projected.DeleteScope != "" {
		t.Fatal("worker outage exposed overdue cloud body or claimed unprotected burn")
	}
	if err = a.retentionRequireCloud(ctx, m.Message); err == nil {
		t.Fatal("export ignored hard deadline")
	}
}

func retentionTargetFixture(t *testing.T, ctx context.Context, s *App, f *retentionPGTransport, user string) receiptDeliveryTarget {
	t.Helper()
	p := receiptTestProfile()
	p.StorageContract = receiptFileContract
	s.Config.ReceiptProfiles = []ReceiptProfile{p}
	target := receiptDeliveryTarget{User: user, Device: newID(), Store: newID(), Qualification: newID(), ObjectCapable: true}
	retentionExec(t, ctx, s, `INSERT INTO sync.device_stores(scope_id,user_id,device_id,restore_epoch,store_generation) VALUES($1,$2,$3,1,$4)`, s.Config.DeploymentID, user, target.Device, target.Store)
	retentionExec(t, ctx, s, `INSERT INTO sync.storage_qualifications(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation,qualification_id,profile_id,build_digest,verification_digest,storage_contract) VALUES($1,'personal',$2,$3,1,$4,$5,$6,$7,$8,$9)`, s.Config.DeploymentID, user, target.Device, target.Store, target.Qualification, p.ID, p.BuildDigest, p.VerificationDigest, p.StorageContract)
	f.devices = append(f.devices, receiptDeviceState{User: user, Device: target.Device, Active: true})
	return target
}
func retentionFreezeFixture(t *testing.T, a *App, m receiptDeliveryManifest) receiptDeliveryManifest {
	t.Helper()
	m.Digest = ""
	m.Targets = []receiptDeliveryTarget{}
	var frozen receiptDeliveryManifest
	if err := a.rpc(context.Background(), "sync", "/internal/receipts/freeze", m, &frozen); err != nil {
		t.Fatal("production target freeze:", err)
	}
	return frozen
}
func retentionACKFixture(t *testing.T, ctx context.Context, s *App, m receiptDeliveryManifest, target receiptDeliveryTarget, content string) {
	t.Helper()
	result, _ := json.Marshal(map[string]string{"deliveryGeneration": m.Generation, "manifestDigest": m.Digest})
	retentionExec(t, ctx, s, `INSERT INTO sync.content_receipts(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation,qualification_id,request_id,request_digest,message_id,conversation_id,content_version,body_digest,permission_generation,result) VALUES($1,'personal',$2,$3,1,$4,$5,$6,$7,$8,$9,$10,'digest','1',$11)`, s.Config.DeploymentID, target.User, target.Device, target.Store, target.Qualification, newID(), bytes.Repeat([]byte{3}, 32), m.Message, m.Conversation, content, result)
}
func TestRetentionPGAccumulatesExactFrozenOriginalRange(t *testing.T) {
	ctx, a, s, f := newRetentionPG(t)
	m := retentionManifestFixture()
	second := newID()
	m.Audience = append(m.Audience, receiptAudienceUser{second, "1"})
	f.audience = m.Audience
	one := retentionTargetFixture(t, ctx, s, f, m.Audience[0].User)
	two := retentionTargetFixture(t, ctx, s, f, m.Audience[0].User)
	three := retentionTargetFixture(t, ctx, s, f, second)
	m = retentionFreezeFixture(t, a, m)
	if len(m.Targets) != 3 {
		t.Fatal("freeze omitted original sender/receiver devices")
	}
	zero := int64(0)
	p := defaultRetentionPolicy()
	p.Mode, p.Delay = "after_receipt", &zero
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: p, PolicyVersion: "1", Manifest: m})
	retentionACKFixture(t, ctx, s, m, one, "1")
	retentionACKFixture(t, ctx, s, m, two, "1")
	retentionACKFixture(t, ctx, s, m, three, "2")
	if err := s.refreshReceiptDelivery(ctx, m); err != nil {
		t.Fatal("partial range:", err)
	}
	var completed *time.Time
	if s.DB.QueryRow(ctx, `SELECT receipt_complete_at FROM sync.delivery_manifests WHERE message_id=$1`, m.Message).Scan(&completed) != nil || completed != nil {
		t.Fatal("another user or stale content was skipped")
	}
	retentionACKFixture(t, ctx, s, m, three, "1")
	if err := s.refreshReceiptDelivery(ctx, m); err != nil {
		t.Fatal("complete range:", err)
	}
	if s.DB.QueryRow(ctx, `SELECT receipt_complete_at FROM sync.delivery_manifests WHERE message_id=$1`, m.Message).Scan(&completed) != nil || completed == nil {
		t.Fatal("exact cumulative range never completed")
	}
	first := *completed
	if err := s.refreshReceiptDelivery(ctx, m); err != nil {
		t.Fatal(err)
	}
	if s.DB.QueryRow(ctx, `SELECT receipt_complete_at FROM sync.delivery_manifests WHERE message_id=$1`, m.Message).Scan(&completed) != nil || !completed.Equal(first) {
		t.Fatal("repeat range reset first completion")
	}
}
func TestRetentionPGEditCopiesTargetsAndOldACKCannotFinishNewGeneration(t *testing.T) {
	ctx, a, s, f := newRetentionPG(t)
	m := retentionManifestFixture()
	f.audience = m.Audience
	target := retentionTargetFixture(t, ctx, s, f, m.Audience[0].User)
	m = retentionFreezeFixture(t, a, m)
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: defaultRetentionPolicy(), PolicyVersion: "1", Manifest: m})
	retentionACKFixture(t, ctx, s, m, target, "1")
	// A later device is qualified, but the earlier send never targeted it.
	_ = retentionTargetFixture(t, ctx, s, f, m.Audience[0].User)
	a.Config.NATSURL = "nats://127.0.0.1:59999"
	edit, err := a.prepareRetentionEdit(ctx, Message{ID: m.Message, ContentVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(edit.Manifest.Targets) != 1 || edit.Manifest.Targets[0] != target {
		t.Fatal("edit expanded first target snapshot")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.persistRetentionEdit(ctx, tx, m.Message, edit); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE message.messages SET content_version=2 WHERE id=$1`, m.Message); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.refreshReceiptDelivery(ctx, edit.Manifest); err != nil {
		t.Fatal(err)
	}
	var first *time.Time
	if s.DB.QueryRow(ctx, `SELECT receipt_complete_at FROM sync.delivery_manifests WHERE message_id=$1 AND generation=2`, m.Message).Scan(&first) != nil || first != nil {
		t.Fatal("old ACK completed edited generation")
	}
	retentionACKFixture(t, ctx, s, edit.Manifest, target, "2")
	if err = s.refreshReceiptDelivery(ctx, edit.Manifest); err != nil {
		t.Fatal(err)
	}
	if s.DB.QueryRow(ctx, `SELECT receipt_complete_at FROM sync.delivery_manifests WHERE message_id=$1 AND generation=2`, m.Message).Scan(&first) != nil || first == nil {
		t.Fatal("original target new-version ACK could not finish")
	}
}

func TestRetentionPGExpiredLeaseCannotCommitOrReportCompletion(t *testing.T) {
	ctx, a, _, f := newRetentionPG(t)
	m := retentionManifestFixture()
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: defaultRetentionPolicy(), PolicyVersion: "1", Manifest: m})
	if err := a.QueueRetentionErasure(ctx, m.Message, "lease-race-operation", "cloud_only", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	f.onAppend = func(q journalRecord) {
		if q.Kind == "message.retention_delete" && q.Phase == "intent" {
			f.onAppend = nil
			retentionExec(t, ctx, a, `UPDATE message.lifecycle_jobs SET lease_token=$2,lease_until=now()+interval '1 minute' WHERE id=$1`, q.Payload["jobId"], newID())
		}
	}
	if err := a.resumeRetention(ctx, 1); err == nil {
		t.Fatal("old lease committed after ownership changed")
	}
	var text string
	if a.DB.QueryRow(ctx, `SELECT text FROM message.messages WHERE id=$1`, m.Message).Scan(&text) != nil || text == "" {
		t.Fatal("lease fencing occurred after irreversible removal")
	}
}
func TestRetentionPGReplayKeepsAbsoluteDeadlineAndMissingSourceFloor(t *testing.T) {
	ctx, a, _, _ := newRetentionPG(t)
	m := retentionManifestFixture()
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: defaultRetentionPolicy(), PolicyVersion: "1", Manifest: m})
	due := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	j := retentionJob{ID: newID(), Message: m.Message, Generation: "1", Policy: "1", Epoch: "1", Reason: "history_expired", Scope: "cloud_only", Due: due, Before: newID(), After: newID()}
	connection, err := a.DB.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	record := journalRecord{Owner: "message", Kind: "message.retention_deadline", Payload: j.safety()}
	if handled, err := ReplayRetentionSafety(ctx, connection.Conn(), record); err != nil || !handled {
		t.Fatal("deadline replay:", err)
	}
	var restored time.Time
	if a.DB.QueryRow(ctx, `SELECT due_at FROM message.lifecycle_jobs WHERE id=$1`, j.ID).Scan(&restored) != nil || !restored.Equal(due) {
		t.Fatal("restore restarted original deadline")
	}
	if handled, err := ReplayRetentionSafety(ctx, connection.Conn(), record); err != nil || !handled {
		t.Fatal("repeat deadline replay:", err)
	}
	missing := newID()
	j.Message, j.ID = missing, newID()
	record.Kind, record.Payload = "message.retention_delete", j.safety()
	if handled, err := ReplayRetentionSafety(ctx, connection.Conn(), record); err != nil || !handled {
		t.Fatal("missing source replay:", err)
	}
	if a.retentionSendFloor(ctx, missing) == nil {
		t.Fatal("pre-source backup could recreate protected deleted identity")
	}
	retentionExec(t, ctx, a, `UPDATE message.instance SET restore_epoch=2 WHERE singleton`)
	if retentionCompleteCall(a, m, "content", time.Now()) == nil {
		t.Fatal("old restore-epoch completion accepted")
	}
}
func TestRetentionPGRetainedAttachmentRequiresExactMemberAndPhysicalStore(t *testing.T) {
	ctx, a, s, f := newRetentionPG(t)
	m := retentionManifestFixture()
	f.audience = m.Audience
	target := retentionTargetFixture(t, ctx, s, f, m.Audience[0].User)
	m = retentionFreezeFixture(t, a, m)
	retentionInsertMessage(t, ctx, a, retentionPrepared{Policy: defaultRetentionPolicy(), PolicyVersion: "1", Manifest: m})
	object := newID()
	retentionExec(t, ctx, a, `UPDATE message.messages SET attachment_id=$2,content_unavailable=true,text='',delete_scope='cloud_only' WHERE id=$1`, m.Message, object)
	retentionACKFixture(t, ctx, s, m, target, "1")
	if err := s.refreshReceiptDelivery(ctx, m); err != nil {
		t.Fatal(err)
	}
	call := func(store, member string) bool {
		raw, _ := json.Marshal(map[string]string{"messageId": m.Message, "userId": target.User, "deviceId": target.Device, "storeGeneration": store, "restoreEpoch": "1", "contentVersion": "1", "membershipGeneration": member})
		r := httptest.NewRequest("POST", "/internal/receipts/retained-access", bytes.NewReader(raw))
		r.Header.Set("X-JobsIM-Caller", "message")
		w := httptest.NewRecorder()
		if err := s.receiptRetainedAccess(w, r); err != nil {
			t.Fatal(err)
		}
		var result map[string]bool
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("retained result")
		}
		return result["allowed"]
	}
	if !call(target.Store, "1") || call(newID(), "1") || call(target.Store, "2") {
		t.Fatal("wrong physical store or rejoined member inherited original access")
	}
	s.Config.ReceiptProfiles = nil
	if call(target.Store, "1") {
		t.Fatal("revoked approved build continued receiving retained content")
	}
}
func TestRetentionPGClockAlarmSurvivesRestart(t *testing.T) {
	ctx, a, _, _ := newRetentionPG(t)
	if err := a.retentionClockGuard(ctx); err != nil {
		t.Fatal(err)
	}
	retentionExec(t, ctx, a, `UPDATE message.lifecycle_clock SET last_database_time=clock_timestamp()+interval '1 minute' WHERE singleton`)
	if a.retentionClockGuard(ctx) == nil {
		t.Fatal("rollback passed irreversible clock gate")
	}
	retentionClockObservations.Delete(a)
	other := &App{Config: a.Config, DB: a.DB, Role: a.Role}
	if other.retentionClockGuard(ctx) == nil {
		t.Fatal("restart cleared a detected durable clock alarm")
	}
}
func TestRetentionPGLegacyAllowlistPreservesOnlyPreActivationBindings(t *testing.T) {
	ctx, a, _, _ := newRetentionPG(t)
	old, newer, conversation, user, object := newID(), newID(), newID(), newID(), newID()
	retentionExec(t, ctx, a, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,attachment_id,sequence,created_at) VALUES($1,$2,$3,$4,$5,'file','',$6,1,(SELECT activated_at-interval '1 second' FROM message.retention_activation))`, old, conversation, user, newID(), []byte("legacy"), object)
	retentionExec(t, ctx, a, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,attachment_id,sequence) VALUES($1,$2,$3,$4,$5,'file','',$6,2)`, newer, conversation, user, newID(), []byte("new-unpaired"), object)
	if err := a.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	call := func(id string) (retentionMediaPlan, error) {
		raw, _ := json.Marshal(map[string]string{"messageId": id, "objectId": object})
		r := httptest.NewRequest("POST", "/internal/retention/media-plan", bytes.NewReader(raw))
		r.Header.Set("X-JobsIM-Caller", "media")
		w := httptest.NewRecorder()
		err := a.retentionMediaPlan(w, r)
		var result retentionMediaPlan
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return result, err
	}
	p, err := call(old)
	if err != nil || !p.Legacy || p.CleanupEligible || !p.ObjectDeadline.IsZero() || p.Digest != "" {
		t.Fatal("legacy source gained a fabricated new deadline or lost its existing read contract:", err)
	}
	if _, err = call(newer); err == nil {
		t.Fatal("new unpaired message impersonated original legacy source")
	}
	retentionExec(t, ctx, a, `INSERT INTO message.retention_floors VALUES($1,$2,'global',1,now())`, a.Config.DeploymentID, old)
	if _, err = call(old); err == nil {
		t.Fatal("legacy allowlist bypassed protected deletion floor")
	}
	if err = a.QueueRetentionErasure(ctx, old, "legacy-global-erasure-operation", "global", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = a.resumeRetention(ctx, 4); err != nil {
		t.Fatal(err)
	}
	erased, err := call(old)
	if err != nil || !erased.Legacy || !erased.CleanupEligible || !erased.ErasureOnly || !syncValidationID(erased.SourceJob) || !syncValidationID(erased.SourceEvent) || erased.Digest != "" || erased.Generation != "" || erased.ObjectDeadline.IsZero() {
		t.Fatal("confirmed legacy global erasure lacks its distinct durable cleanup fact:", err)
	}

}
