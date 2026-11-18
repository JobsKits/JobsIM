package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// These exercise real PostgreSQL and production role handlers. The RPC
// transport is in-process and the journal schema shares the isolated test PG;
// they do not claim separate-cluster, HTTPS, native UI or device verification.
type deletionPGTransport struct {
	nodes               map[string]*App
	mu                  sync.Mutex
	failKind, failPhase string
}

func (f *deletionPGTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/internal/append" {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var q journalRecord
		_ = json.Unmarshal(raw, &q)
		f.mu.Lock()
		blocked := q.Kind == f.failKind && q.Phase == f.failPhase
		f.mu.Unlock()
		if blocked {
			return nil, errors.New("injected journal acknowledgement failure")
		}
	}
	node := f.nodes[r.URL.Host]
	if node == nil {
		return nil, errors.New("unexpected dependency in isolated deletion test")
	}
	w := httptest.NewRecorder()
	node.Handler().ServeHTTP(w, r)
	return w.Result(), nil
}

type deletionPGFixture struct {
	ctx   context.Context
	nodes map[string]*App
	user  User
	q     accountDeletionRequest
	token string
	wire  *deletionPGTransport
}

func newDeletionPG(t *testing.T, days int) *deletionPGFixture {
	t.Helper()
	ctx, a, user, pc, token, _ := newPasswordChangePGApp(t)
	cfg := a.Config
	cfg.NATSURL = ""
	cfg.AccountDeletionCooldownDays = &days
	cfg.RoleURLs = map[string]string{}
	roles := []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance", "sync"}
	nodes := map[string]*App{}
	wire := &deletionPGTransport{nodes: map[string]*App{}}
	for i, role := range roles {
		cfg.RoleURLs[role] = "http://127.0.0.1:" + str(int64(62001+i))
	}
	for _, role := range roles {
		node := &App{Config: cfg, Role: role, DB: a.DB, Client: &http.Client{Transport: wire}, hashSlots: make(chan struct{}, 2)}
		if err := node.migrate(ctx); err != nil {
			t.Fatalf("production %s migration failed: %v", role, err)
		}
		nodes[role] = node
		wire.nodes[strings.TrimPrefix(cfg.RoleURLs[role], "http://")] = node
	}
	return &deletionPGFixture{ctx, nodes, user, accountDeletionRequest{RequestID: newID(), Username: pc.Username, DeviceID: pc.DeviceID, RestoreEpoch: pc.RestoreEpoch, Password: pc.CurrentPassword}, token, wire}
}

func deletionExec(t *testing.T, ctx context.Context, a *App, sql string, args ...any) {
	t.Helper()
	if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			t.Fatalf("owned deletion SQLSTATE=%s table=%s column=%s constraint=%s", pg.Code, pg.TableName, pg.ColumnName, pg.ConstraintName)
		}
		t.Fatal("owned deletion fixture SQL unavailable; private values suppressed")
	}
}

func deletionCall(a *App, path, token string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func deletionBody(t *testing.T, w *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	if w.Code != status {
		t.Fatalf("deletion expected %d got %d: %s", status, w.Code, w.Body.String())
	}
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal("deletion response is not JSON")
	}
	return out
}
func (f *deletionPGFixture) request(t *testing.T) map[string]any {
	return deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/deletion-requests", f.token, f.q), 200)
}
func (f *deletionPGFixture) proof(t *testing.T, d map[string]any, purpose string) string {
	t.Helper()
	q := dataRightsProofRequest{Username: f.q.Username, DeletionID: d["deletionId"].(string), Version: d["requestVersion"].(string), Purpose: purpose, Password: f.q.Password}
	p := deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/data-rights/proofs", "", q), 200)
	return p["proof"].(string)
}
func deletionFence(t *testing.T, a *App, fn func(context.Context) error) {
	t.Helper()
	lock, err := a.admissionFence(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer closeExportFence(lock)
	ctx, done := a.heldAdmissionScope(context.Background(), "exclusive")
	defer done()
	if err = fn(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAccountDeletionPGRevokesFamiliesAndOriginalLookupNeverCreates(t *testing.T) {
	f := newDeletionPG(t, 7)
	d := f.request(t)
	if d["status"] != "requested" || d["requestVersion"] != "1" || d["requiresNewLogin"] != true {
		t.Fatal("original deletion was not acknowledged")
	}
	var deletion string
	var live int
	if err := f.nodes["identity"].DB.QueryRow(f.ctx, `SELECT deletion_state,(SELECT count(*) FROM auth.sessions WHERE user_id=$1 AND revoked_at IS NULL) FROM auth.users WHERE id=$1`, f.user.ID).Scan(&deletion, &live); err != nil || deletion != "requested" || live != 0 {
		t.Fatal("all-family immediate denial missing", err)
	}
	q := f.q
	q.RequestID = newID()
	deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/deletion-results", "", q), 404)
	q = f.q
	q.Password = "different-private-fixture"
	deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/deletion-results", "", q), 403)
	read := deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/deletion-results", "", f.q), 200)
	if read["deletionId"] != d["deletionId"] {
		t.Fatal("original lookup changed identity")
	}
	deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/deletion-requests", "", f.q), 200)
	u := httptest.NewRequest("GET", "/api/me", nil)
	u.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.nodes["identity"].Handler().ServeHTTP(w, u)
	if w.Code != 401 {
		t.Fatalf("old family authorization remained usable: %d", w.Code)
	}
}
func TestAccountDeletionPGPurposeProofAndCancelPreserveIndependentDenials(t *testing.T) {
	f := newDeletionPG(t, 7)
	d := f.request(t)
	query := f.proof(t, d, "query")
	deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/data-rights/cancel", "", map[string]string{"proof": query, "requestId": newID()}), 403)
	deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/data-rights/query", "", map[string]string{"proof": query}), 200)
	proof := f.proof(t, d, "cancel")
	nonce := newID()
	deletionExec(t, f.ctx, f.nodes["identity"], `UPDATE auth.users SET state='banned',recovery_state='reset_required' WHERE id=$1`, f.user.ID)
	body := map[string]string{"proof": proof, "requestId": nonce}
	result := deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/data-rights/cancel", "", body), 200)
	if result["status"] != "cancelled" || result["requestVersion"] != "2" || result["cancelRequestId"] != nonce {
		t.Fatal("original cancellation not acknowledged")
	}
	deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/data-rights/cancel", "", body), 200)
	var state, recovery, deletion string
	var live int
	if err := f.nodes["identity"].DB.QueryRow(f.ctx, `SELECT state,recovery_state,deletion_state,(SELECT count(*) FROM auth.sessions WHERE user_id=$1 AND revoked_at IS NULL) FROM auth.users WHERE id=$1`, f.user.ID).Scan(&state, &recovery, &deletion, &live); err != nil || state != "banned" || recovery != "reset_required" || deletion != "none" || live != 0 {
		t.Fatal("cancellation undid independent denial or restored sessions", err)
	}
}
func TestAccountDeletionPGCancelUnknownWaitsForJournalResult(t *testing.T) {
	f := newDeletionPG(t, 7)
	d := f.request(t)
	proof := f.proof(t, d, "cancel")
	nonce := newID()
	f.wire.failKind, f.wire.failPhase = "auth.deletion", "result"
	deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/data-rights/cancel", "", map[string]string{"proof": proof, "requestId": nonce}), 503)
	result := deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/deletion-results", "", f.q), 200)
	if result["status"] != "cancel_pending" {
		t.Fatal("unknown cancellation was falsely acknowledged")
	}
	var pending *string
	if err := f.nodes["identity"].DB.QueryRow(f.ctx, `SELECT pending_operation_id FROM auth.users WHERE id=$1`, f.user.ID).Scan(&pending); err != nil || pending == nil {
		t.Fatal("new-login fence disappeared before protected result")
	}
	f.wire.failKind = ""
	deletionFence(t, f.nodes["governance"], func(ctx context.Context) error { return f.nodes["governance"].resumeAccountDeletions(ctx, 2) })
	result = deletionBody(t, deletionCall(f.nodes["identity"], "/api/auth/deletion-results", "", f.q), 200)
	if result["status"] != "cancelled" {
		t.Fatal("original cancellation did not recover")
	}
}
func TestAccountDeletionPGZeroCooldownCleansOwnerDataAndCannotCancel(t *testing.T) {
	f := newDeletionPG(t, 0)
	a := f.nodes["identity"]
	message, conversation, object := newID(), newID(), newID()
	deletionExec(t, f.ctx, a, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text','private deletion fixture content',1)`, message, conversation, f.user.ID, newID(), []byte("synthetic-digest"))
	deletionExec(t, f.ctx, a, `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,purpose) VALUES($1,$2,'private-fixture-name','image/jpeg',8,$3,$4,'profile_avatar')`, object, f.user.ID, bytes.Repeat([]byte{0x51}, 32), object)
	if err := os.MkdirAll(filepath.Join(a.Config.StateDir, "Media"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(a.Config.StateDir, "Media", object)
	if err := os.WriteFile(file, []byte("fixture-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	d := f.request(t)
	proof := f.proof(t, d, "cancel")
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/cancel", "", map[string]string{"proof": proof, "requestId": newID()}), 409)
	deletionFence(t, f.nodes["governance"], func(ctx context.Context) error { return f.nodes["governance"].resumeAccountDeletions(ctx, 2) })
	result := deletionBody(t, deletionCall(a, "/api/auth/deletion-results", "", f.q), 200)
	if result["status"] != "completed" {
		t.Fatal("qualified deletion did not complete")
	}
	var text, hash, state, scope string
	var deleted bool
	if err := a.DB.QueryRow(f.ctx, `SELECT text,deleted,delete_scope FROM message.messages WHERE id=$1`, message).Scan(&text, &deleted, &scope); err != nil || text != "" || !deleted || scope != "global" {
		t.Fatal("message source not globally erased", err)
	}
	if err := a.DB.QueryRow(f.ctx, `SELECT password_hash,deletion_state FROM auth.users WHERE id=$1`, f.user.ID).Scan(&hash, &state); err != nil || hash != "" || state != "completed" {
		t.Fatal("final credential facts were not erased", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("owned avatar bytes remained after completion")
	}
}
func TestAccountDeletionPGRecoveryCodeIsOneUseAndPurposeCannotAuthorizeChat(t *testing.T) {
	f := newDeletionPG(t, 7)
	d := f.request(t)
	a := f.nodes["identity"]
	batch, code := newID(), newID()
	secret := "synthetic-data-rights-recovery-code"
	deletionExec(t, f.ctx, a, `INSERT INTO auth.recovery_code_batches(id,user_id,generation,active) VALUES($1,$2,1,true)`, batch, f.user.ID)
	deletionExec(t, f.ctx, a, `UPDATE auth.users SET recovery_generation=1 WHERE id=$1`, f.user.ID)
	deletionExec(t, f.ctx, a, `INSERT INTO auth.recovery_codes(id,batch_id,code_digest) VALUES($1,$2,$3)`, code, batch, keyedDigest(a.Config.Key, "recovery", []byte(secret)))
	q := dataRightsProofRequest{Username: f.q.Username, DeletionID: d["deletionId"].(string), Version: "1", Purpose: "query", Code: secret}
	p := deletionBody(t, deletionCall(a, "/api/auth/data-rights/proofs", "", q), 200)
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/proofs", "", q), 403)
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.Header.Set("Authorization", "Bearer "+p["proof"].(string))
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("restricted proof was accepted as a chat session")
	}
	conn, err := a.DB.Acquire(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	deletionExec(t, f.ctx, a, `UPDATE auth.recovery_codes SET consumed_at=NULL WHERE id=$1`, code)
	if _, err = ReplayAccountDeletionSafety(f.ctx, conn.Conn(), journalRecord{Owner: "identity", Phase: "intent", Kind: "auth.data_rights_code", Payload: map[string]any{"userId": f.user.ID, "codeId": code}}); err != nil {
		t.Fatal(err)
	}
	var consumed bool
	if err = a.DB.QueryRow(f.ctx, `SELECT consumed_at IS NOT NULL FROM auth.recovery_codes WHERE id=$1`, code).Scan(&consumed); err != nil || !consumed {
		t.Fatal("restored recovery proof became reusable")
	}
}
func TestAccountDeletionPGSafetyCancelIntentCannotReopenAndCompletedCannotResurrect(t *testing.T) {
	f := newDeletionPG(t, 7)
	d := f.request(t)
	a := f.nodes["identity"]
	record, err := scanIdentityDeletion(a.DB.QueryRow(f.ctx, `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE id=$1`, d["deletionId"]))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := a.DB.Acquire(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	record.Version = "2"
	record.CancelSecurity = record.Security + 1
	q := journalRecord{Owner: "identity", Phase: "intent", Kind: "auth.deletion", Payload: a.deletionSafety(record, "cancelled")}
	if _, err = ReplayAccountDeletionSafety(f.ctx, conn.Conn(), q); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = a.DB.QueryRow(f.ctx, `SELECT deletion_state FROM auth.users WHERE id=$1`, f.user.ID).Scan(&state); err != nil || state != "requested" {
		t.Fatal("cancel intent reopened a deletion")
	}
	q.Phase = "result"
	if _, err = ReplayAccountDeletionSafety(f.ctx, conn.Conn(), q); err != nil {
		t.Fatal(err)
	}
	if err = a.DB.QueryRow(f.ctx, `SELECT deletion_state FROM auth.users WHERE id=$1`, f.user.ID).Scan(&state); err != nil || state != "none" {
		t.Fatal("protected original cancel result did not replay")
	}
	deletionExec(t, f.ctx, a, `UPDATE auth.users SET deletion_state='completed',deletion_request_id=$2 WHERE id=$1`, f.user.ID, record.ID)
	if _, err = ReplayAccountDeletionSafety(f.ctx, conn.Conn(), q); err != nil {
		t.Fatal(err)
	}
	if err = a.DB.QueryRow(f.ctx, `SELECT deletion_state FROM auth.users WHERE id=$1`, f.user.ID).Scan(&state); err != nil || state != "completed" {
		t.Fatal("completed deletion resurrected")
	}
}

func TestAccountDeletionPGExpiredAndOldEpochProofCannotMutate(t *testing.T) {
	f := newDeletionPG(t, 7)
	d := f.request(t)
	a := f.nodes["identity"]
	proof := f.proof(t, d, "cancel")
	deletionExec(t, f.ctx, a, `UPDATE auth.data_rights_proofs SET expires_at=clock_timestamp()-interval '1 second'`)
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/cancel", "", map[string]string{"proof": proof, "requestId": newID()}), 403)
	proof = f.proof(t, d, "cancel")
	deletionExec(t, f.ctx, a, `UPDATE journal.restore_epochs SET restore_epoch=2 WHERE singleton`)
	deletionExec(t, f.ctx, a, `UPDATE auth.instance SET restore_epoch=2 WHERE singleton`)
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/cancel", "", map[string]string{"proof": proof, "requestId": newID()}), 403)
	var state string
	if err := a.DB.QueryRow(f.ctx, `SELECT state FROM governance.account_deletions WHERE id=$1`, d["deletionId"]).Scan(&state); err != nil || state != "requested" {
		t.Fatal("expired proof changed the original deletion")
	}
}

func TestAccountDeletionPGOldVersionCanReverifyQueryButCannotRebaseCancellation(t *testing.T) {
	f := newDeletionPG(t, 7)
	d := f.request(t)
	a := f.nodes["identity"]
	proof := f.proof(t, d, "cancel")
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/cancel", "", map[string]string{"proof": proof, "requestId": newID()}), 200)
	q := dataRightsProofRequest{Username: f.q.Username, DeletionID: d["deletionId"].(string), Version: "1", Purpose: "query", Password: f.q.Password}
	current := deletionBody(t, deletionCall(a, "/api/auth/data-rights/proofs", "", q), 200)
	if current["requestVersion"] != "2" {
		t.Fatal("trusted query re-verification did not bind the current original version")
	}
	result := deletionBody(t, deletionCall(a, "/api/auth/data-rights/query", "", map[string]string{"proof": current["proof"].(string)}), 200)
	if result["status"] != "cancelled" {
		t.Fatal("unknown cancellation cannot be rediscovered using its old public version")
	}
	q.Purpose = "cancel"
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/proofs", "", q), 409)
	q.Purpose = "query"
	q.Version = "3"
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/proofs", "", q), 409)
}

func TestAccountDeletionPGInitialJournalUnknownKeepsOriginalAndWorkerResumes(t *testing.T) {
	f := newDeletionPG(t, 7)
	a := f.nodes["identity"]
	f.wire.failKind, f.wire.failPhase = "auth.deletion", "intent"
	w := deletionCall(a, "/api/auth/deletion-requests", f.token, f.q)
	deletionBody(t, w, 503)
	if len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("unknown deletion wrote browser lifecycle cookies")
	}
	var pending *string
	if err := a.DB.QueryRow(f.ctx, `SELECT pending_operation_id FROM auth.users WHERE id=$1`, f.user.ID).Scan(&pending); err != nil || pending == nil {
		t.Fatal("unknown request lost its ordinary-access fence")
	}
	d := deletionBody(t, deletionCall(a, "/api/auth/deletion-results", "", f.q), 200)
	if d["status"] != "pending" {
		t.Fatal("unknown initial journal was falsely acknowledged")
	}
	f.wire.failKind = ""
	deletionFence(t, a, func(ctx context.Context) error { return a.resumeAccountDeletions(ctx, 2) })
	read := deletionBody(t, deletionCall(a, "/api/auth/deletion-results", "", f.q), 200)
	if read["deletionId"] != d["deletionId"] || read["status"] != "requested" {
		t.Fatal("worker replaced the original request")
	}
}

func TestAccountDeletionPGMultipleOwnerPagesAndRestoredMediaStayErased(t *testing.T) {
	f := newDeletionPG(t, 0)
	a := f.nodes["identity"]
	conversation := newID()
	other := newID()
	deletionExec(t, f.ctx, a, `INSERT INTO conversation.conversations(id,kind,title,owner_id) VALUES($1,'group','retained lawful group history',$2)`, conversation, f.user.ID)
	deletionExec(t, f.ctx, a, `INSERT INTO conversation.members(conversation_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')`, conversation, f.user.ID, other)
	deletionExec(t, f.ctx, a, `INSERT INTO directory.profiles(user_id,display_name) VALUES($1,'private former display')`, f.user.ID)
	var files []string
	if err := os.MkdirAll(filepath.Join(a.Config.StateDir, "Media"), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 33; i++ {
		id, object := newID(), newID()
		deletionExec(t, f.ctx, a, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text','private bounded-page fixture',$6)`, id, conversation, f.user.ID, newID(), []byte("synthetic-digest"), i+1)
		deletionExec(t, f.ctx, a, `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,purpose) VALUES($1,$2,'owned fixture','image/jpeg',8,$3,$4,'profile_avatar')`, object, f.user.ID, bytes.Repeat([]byte{0x42}, 32), object)
		file := filepath.Join(a.Config.StateDir, "Media", object)
		if err := os.WriteFile(file, []byte("owned-fixture-bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	d := f.request(t)
	queryProof := f.proof(t, d, "query")
	g := f.nodes["governance"]
	completed := false
	for i := 0; i < 6; i++ {
		deletionFence(t, g, func(ctx context.Context) error {
			err := g.resumeAccountDeletions(ctx, 2)
			var e *apiError
			if errors.As(err, &e) && e.Code == "DELETION_CLEANUP_PENDING" {
				return nil
			}
			return err
		})
		var state string
		if err := a.DB.QueryRow(f.ctx, `SELECT state FROM governance.account_deletions WHERE id=$1`, d["deletionId"]).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "completed" {
			completed = true
			break
		}
	}
	if !completed {
		t.Fatal("bounded owner pages did not finish")
	}
	var group, member, display string
	var erased int
	if err := a.DB.QueryRow(f.ctx, `SELECT state FROM conversation.conversations WHERE id=$1`, conversation).Scan(&group); err != nil || group != "owner_unavailable" {
		t.Fatal("ownerless group blocked deletion or remained writable")
	}
	if err := a.DB.QueryRow(f.ctx, `SELECT state FROM conversation.members WHERE conversation_id=$1 AND user_id=$2`, conversation, f.user.ID).Scan(&member); err != nil || member != "left" {
		t.Fatal("deleted member remained active")
	}
	if err := a.DB.QueryRow(f.ctx, `SELECT display_name FROM directory.profiles WHERE user_id=$1`, f.user.ID).Scan(&display); err != nil || display != "" {
		t.Fatal("private display survived deletion")
	}
	if err := a.DB.QueryRow(f.ctx, `SELECT count(*) FROM message.messages WHERE sender_id=$1 AND text='' AND deleted AND delete_scope='global'`, f.user.ID).Scan(&erased); err != nil || erased != 33 {
		t.Fatal("message page omitted owned source content")
	}
	for _, file := range files {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("media page omitted owned bytes")
		}
	}
	if err := os.WriteFile(files[0], []byte("restored old backup bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	neighbor := filepath.Join(a.Config.StateDir, "Media", newID())
	if err := os.WriteFile(neighbor, []byte("unowned unrelated bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	conn, err := a.DB.Acquire(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if err = ClearRestoredAccountMedia(f.ctx, conn.Conn(), a.Config.StateDir); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(files[0]); !os.IsNotExist(err) {
		t.Fatal("protected owner backup bytes resurrected")
	}
	if _, err = os.Stat(neighbor); err != nil {
		t.Fatal("unowned media was removed")
	}
	// A current query proof may observe completion during its short deadline;
	// the final stage retires only cancellation proofs and keeps the account closed.
	read := deletionBody(t, deletionCall(a, "/api/auth/data-rights/query", "", map[string]string{"proof": queryProof}), 200)
	if read["status"] != "completed" {
		t.Fatal("unexpired query proof could not observe final status")
	}
}
