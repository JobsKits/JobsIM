package jobsim

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Real isolated PostgreSQL and the production auth/journal handlers are used.
// Message/governance services are explicit in-process acknowledgement fixtures;
// this is not a live HTTPS, separate-cluster restore or native-client test.
type passwordChangePGTransport struct {
	app, journal            *App
	mu                      sync.Mutex
	failPhase               string
	beforeState             func()
	beforeAppend            func()
	forwards, invalidations int
}

func (f *passwordChangePGTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	switch r.URL.Path {
	case "/internal/authorize":
		if err := f.app.authorize(w, r); err != nil {
			writeError(w, r, err)
		}
	case "/internal/state", "/internal/append":
		if r.URL.Path == "/internal/state" {
			f.mu.Lock()
			hook := f.beforeState
			f.beforeState = nil
			f.mu.Unlock()
			if hook != nil {
				hook()
			}
		}
		if r.URL.Path == "/internal/append" {
			raw, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var value journalRecord
			_ = json.Unmarshal(raw, &value)
			f.mu.Lock()
			blocked := value.Phase == f.failPhase
			f.mu.Unlock()
			if blocked {
				return nil, errors.New("injected journal acknowledgement failure")
			}
			f.mu.Lock()
			hook := f.beforeAppend
			f.beforeAppend = nil
			f.mu.Unlock()
			if hook != nil {
				hook()
			}
		}
		f.journal.Handler().ServeHTTP(w, r)
	case "/internal/forwards/revoke-principal":
		f.mu.Lock()
		f.forwards++
		f.mu.Unlock()
		w.Header().Set("X-JobsIM-Admission-Fence", f.app.admissionFenceIdentity())
		respond(w, 200, map[string]any{})
	case "/internal/exports/invalidate":
		f.mu.Lock()
		f.invalidations++
		f.mu.Unlock()
		respond(w, 200, map[string]any{})
	case "/internal/capability":
		respond(w, 200, map[string]bool{"enabled": true})
	default:
		return nil, errors.New("unexpected dependency in isolated credential test")
	}
	return w.Result(), nil
}

func (f *passwordChangePGTransport) block(phase string) {
	f.mu.Lock()
	f.failPhase = phase
	f.mu.Unlock()
}

func pcExec(t *testing.T, ctx context.Context, a *App, sql string, args ...any) {
	t.Helper()
	if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
		t.Fatal("owned credential fixture SQL failed; private values suppressed")
	}
}

func newPasswordChangePGApp(t *testing.T) (context.Context, *App, User, passwordChangeRequest, string, *passwordChangePGTransport) {
	t.Helper()
	ctx, a, user := newSyncRegisterPGApp(t)
	a.Role = "identity"
	a.Config.StateDir, a.Config.PublicURL = t.TempDir(), "https://127.0.0.1:56443"
	a.Config.Key = bytes.Repeat([]byte{0x63}, 32)
	a.Config.RoleURLs = map[string]string{"identity": "http://127.0.0.1:1", "journal": "http://127.0.0.1:2", "message": "http://127.0.0.1:3", "governance": "http://127.0.0.1:4"}
	a.hashSlots = make(chan struct{}, 1)
	if err := a.migrate(ctx); err != nil {
		t.Fatal("full production identity migration failed: " + err.Error())
	}
	if err := a.migrate(ctx); err != nil {
		t.Fatal("repeat production identity migration failed")
	}
	journal := &App{Config: a.Config, Role: "journal", DB: a.DB}
	if err := journal.migrate(ctx); err != nil {
		t.Fatal("owned journal migration failed")
	}
	f := &passwordChangePGTransport{app: a, journal: journal}
	a.Client = &http.Client{Transport: f}
	journal.Client = a.Client
	q := passwordChangeTestRequest()
	user.Username, user.DisplayName = q.Username, "Non-secret test owner"
	pcExec(t, ctx, a, `INSERT INTO auth.users(id,username,display_name,password_hash) VALUES($1,$2,$3,$4)`, user.ID, user.Username, user.DisplayName, hashPassword(q.CurrentPassword))
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal("owned initial session transaction unavailable")
	}
	delivery, err := a.issueSession(ctx, tx, user, "native-test", 1, 1, 1, false)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal("production native family creation failed")
	}
	web, err := a.issueSession(ctx, tx, user, "browser-test", 1, 1, 1, true)
	if err != nil || tx.Commit(ctx) != nil {
		t.Fatal("production browser family creation failed")
	}
	user.DeviceID, q.DeviceID = delivery.Result.DeviceID, delivery.Result.DeviceID
	pcExec(t, ctx, a, `INSERT INTO auth.local_auth_requests(request_id,purpose,request_digest,user_id,device_id,delivery_ciphertext,expires_at,delivery_version,session_id,family_id)
VALUES($1,'login',$2,$3,$4,$5,now()+interval '5 minutes','refresh-auth-v1',$6,$7)`, "initial-"+newID(), tokenDigest("non-secret-proof"), user.ID, q.DeviceID, []byte("non-secret-initial-delivery"), delivery.SessionID, delivery.Result.FamilyID)
	pcExec(t, ctx, a, `UPDATE auth.bff_sessions SET state='active',browser_confirmed_at=now() WHERE session_id=$1`, web.SessionID)
	pcExec(t, ctx, a, `UPDATE auth.sessions SET web_pending=false WHERE id=$1`, web.SessionID)
	return ctx, a, user, q, delivery.Result.Token, f
}

func pcCall(a *App, path, token string, q passwordChangeRequest) *httptest.ResponseRecorder {
	body, _ := json.Marshal(q)
	r := httptest.NewRequest("POST", path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func pcBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if json.Unmarshal(w.Body.Bytes(), &value) != nil {
		t.Fatal("credential result was not JSON")
	}
	return value
}

func pcCount(t *testing.T, ctx context.Context, a *App, sql string, args ...any) int {
	t.Helper()
	var count int
	if a.DB.QueryRow(ctx, sql, args...).Scan(&count) != nil {
		t.Fatal("owned credential count unavailable")
	}
	return count
}

func pcHash(t *testing.T, ctx context.Context, a *App, user string) string {
	t.Helper()
	var hash string
	if a.DB.QueryRow(ctx, `SELECT password_hash FROM auth.users WHERE id=$1`, user).Scan(&hash) != nil {
		t.Fatal("owned credential hash unavailable")
	}
	return hash
}

func TestPasswordChangePGRevokesEveryFamilyWithoutCredentialDelivery(t *testing.T) {
	ctx, a, user, q, token, _ := newPasswordChangePGApp(t)
	batch := newID()
	pcExec(t, ctx, a, `INSERT INTO auth.recovery_code_batches(id,user_id,generation,active) VALUES($1,$2,1,true)`, batch, user.ID)
	pcExec(t, ctx, a, `INSERT INTO auth.reset_grants(id,user_id,purpose,credential_digest,issued_security_version,restore_epoch,expires_at,state,issue_request_id) VALUES($1,$2,'password_reset',$3,1,1,now()+interval '5 minutes','active',$4)`, newID(), user.ID, tokenDigest("non-secret-reset"), newID())
	w := pcCall(a, "/api/auth/password-change", token, q)
	if w.Code != 200 {
		t.Fatalf("password change status %d: %s", w.Code, pcBody(t, w)["error"])
	}
	body := pcBody(t, w)
	if body["status"] != "changed" || body["current"] != true || body["requiresNewLogin"] != true || body["credentialGeneration"] != "2" {
		t.Fatal("changed result lost current status or forced login")
	}
	if bytes.Contains(w.Body.Bytes(), []byte(q.NewPassword)) || body["token"] != nil || body["refreshToken"] != nil {
		t.Fatal("password change delivered credential material")
	}
	hash := pcHash(t, ctx, a, user.ID)
	if !verifyPassword(hash, q.NewPassword) || verifyPassword(hash, q.CurrentPassword) {
		t.Fatal("replacement credential was not installed exclusively")
	}
	for _, sql := range []string{
		`SELECT count(*) FROM auth.sessions WHERE user_id=$1 AND revoked_at IS NULL`,
		`SELECT count(*) FROM auth.refresh_token_history h JOIN auth.sessions s ON s.id=h.session_id WHERE s.user_id=$1 AND h.revoked_at IS NULL`,
		`SELECT count(*) FROM auth.bff_sessions WHERE user_id=$1 AND (state<>'revoked' OR upstream_credentials_ciphertext IS NOT NULL)`,
		`SELECT count(*) FROM auth.local_auth_requests WHERE user_id=$1 AND octet_length(delivery_ciphertext)>0`,
		`SELECT count(*) FROM auth.recovery_code_batches WHERE user_id=$1 AND active`,
		`SELECT count(*) FROM auth.reset_grants WHERE user_id=$1 AND state='active'`,
	} {
		if pcCount(t, ctx, a, sql, user.ID) != 0 {
			t.Fatal("old family, delivery, recovery code or reset credential survived")
		}
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.sessions WHERE user_id=$1`, user.ID) != 2 || pcCount(t, ctx, a, `SELECT count(*) FROM auth.session_revocations WHERE user_id=$1`, user.ID) != 2 {
		t.Fatal("revocation was not exercised on both real session families")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE user_id=$1 AND state='completed' AND octet_length(replacement_hash_ciphertext)=0`, user.ID) != 1 {
		t.Fatal("completed operation retained replacement ciphertext")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries WHERE kind='auth.invalidate_user' AND payload->>'userId'=$1 AND (SELECT count(*) FROM jsonb_object_keys(payload))=3`, user.ID) != 2 {
		t.Fatal("typed credential denial intent/result missing")
	}
	proof, _ := json.Marshal(map[string]string{"token": token, "authorization": "Bearer " + token, "method": "GET", "path": "/api/me"})
	r := httptest.NewRequest("POST", "/internal/authorize", bytes.NewReader(proof))
	var denied *apiError
	if err := a.authorize(httptest.NewRecorder(), r); !errors.As(err, &denied) || denied.Status != 401 {
		t.Fatal("original unexpired access credential still authorized")
	}
	before := pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`)
	if original := pcCall(a, "/api/auth/password-change-results", "", q); original.Code != 200 || pcBody(t, original)["current"] != true {
		t.Fatal("original proof could not read result after all families were revoked")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`) != before || pcHash(t, ctx, a, user.ID) != hash {
		t.Fatal("pure result query mutated original operation")
	}
	raw, _ := json.Marshal(q)
	retry := httptest.NewRequest("POST", "/api/auth/password-change", bytes.NewReader(raw))
	retry.Header.Set("Content-Type", "application/json")
	retry.AddCookie(&http.Cookie{Name: "__Host-jobsim_session", Value: "non-secret-later-family-cookie"})
	late := httptest.NewRecorder()
	a.Handler().ServeHTTP(late, retry)
	if late.Code != 200 || len(late.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("historical original intent cleared a later browser identity")
	}
	pcExec(t, ctx, a, `UPDATE auth.users SET security_version=3 WHERE id=$1`, user.ID)
	if old := pcCall(a, "/api/auth/password-change-results", "", q); old.Code != 200 || pcBody(t, old)["current"] != false {
		t.Fatal("historical success was presented as the current security generation")
	}
}

func TestPasswordChangePGJournalFailureCanResumeWithoutPlaintext(t *testing.T) {
	ctx, a, user, q, token, f := newPasswordChangePGApp(t)
	old := pcHash(t, ctx, a, user.ID)
	f.block("intent")
	if w := pcCall(a, "/api/auth/password-change", token, q); w.Code != 503 {
		t.Fatal("missing independent protection acknowledgement did not fail closed")
	}
	if pcHash(t, ctx, a, user.ID) != old || pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE state='prepared' AND octet_length(replacement_hash_ciphertext)>0`) != 1 {
		t.Fatal("failed intent installed a password or lost durable prepared recovery")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.users WHERE id=$1 AND pending_operation_id IS NOT NULL`, user.ID) != 1 {
		t.Fatal("unprotected pending operation failed to close old authorization")
	}
	f.block("")
	// The worker is a new App value and receives no password request or token.
	restarted := &App{Config: a.Config, Role: a.Role, DB: a.DB, Client: a.Client, hashSlots: make(chan struct{}, 1)}
	if err := restarted.resumePasswordChanges(ctx, 2); err != nil {
		t.Fatal("prepared credential operation could not resume from sealed state")
	}
	if !verifyPassword(pcHash(t, ctx, a, user.ID), q.NewPassword) || pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE state='completed'`) != 1 {
		t.Fatal("worker did not finish the original replacement")
	}
	if retry := pcCall(a, "/api/auth/password-change", "", q); retry.Code != 200 || pcBody(t, retry)["status"] != "changed" {
		t.Fatal("lost original response could not be recovered without a new family")
	}
	q.NewPassword += "x"
	if pcCall(a, "/api/auth/password-change-results", "", q).Code != 403 {
		t.Fatal("another password pair read the original result")
	}
}

func TestPasswordChangePGCommittedResultRetryDoesNotRehash(t *testing.T) {
	ctx, a, user, q, token, f := newPasswordChangePGApp(t)
	f.block("result")
	if pcCall(a, "/api/auth/password-change", token, q).Code != 503 || pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE state='committed' AND octet_length(replacement_hash_ciphertext)=0`) != 1 {
		t.Fatal("lost result acknowledgement did not preserve committed cleanup state")
	}
	hash := pcHash(t, ctx, a, user.ID)
	before := pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`)
	if w := pcCall(a, "/api/auth/password-change-results", "", q); w.Code != 200 || pcBody(t, w)["status"] != "pending" || pcBody(t, w)["requiresNewLogin"] != true {
		t.Fatal("read-only pending committed result is incorrect")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`) != before {
		t.Fatal("result lookup performed a protection retry")
	}
	f.block("")
	if err := a.resumePasswordChanges(ctx, 2); err != nil || pcHash(t, ctx, a, user.ID) != hash {
		t.Fatal("committed retry rehashed or failed to finish")
	}
}

func TestPasswordChangePGRejectsNewUnownedIntentAndUnknownLookup(t *testing.T) {
	ctx, a, _, q, token, _ := newPasswordChangePGApp(t)
	for _, mode := range []string{"no-auth", "wrong-device", "wrong-epoch", "wrong-password", "reuse"} {
		changed, proof := q, token
		changed.RequestID = newID()
		switch mode {
		case "no-auth":
			proof = ""
		case "wrong-device":
			changed.DeviceID = newID()
		case "wrong-epoch":
			changed.RestoreEpoch = "2"
		case "wrong-password":
			changed.CurrentPassword += "x"
		case "reuse":
			changed.NewPassword = changed.CurrentPassword
		}
		if w := pcCall(a, "/api/auth/password-change", proof, changed); w.Code < 400 || w.Code >= 500 {
			t.Fatal("invalid fresh intent was accepted or failed outside its expected authorization boundary: " + mode)
		}
	}
	if pcCall(a, "/api/auth/password-change-results", "", q).Code != 404 || pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes`) != 0 || pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`) != 0 {
		t.Fatal("rejected intent or unknown lookup created a protected operation")
	}
}

func TestPasswordChangePGOldEpochCannotInstallSealedCredential(t *testing.T) {
	ctx, a, user, q, _, _ := newPasswordChangePGApp(t)
	value, err := a.preparePasswordChange(ctx, user, q)
	if err != nil {
		t.Fatal("old epoch preparation failed")
	}
	old := pcHash(t, ctx, a, user.ID)
	pcExec(t, ctx, a, `UPDATE auth.instance SET restore_epoch=2; UPDATE journal.restore_epochs SET restore_epoch=2`)
	pcExec(t, ctx, a, `UPDATE auth.users SET state='banned',recovery_state='recovery_required',credential_generation=8,security_version=9 WHERE id=$1`, user.ID)
	if err = a.finishPasswordChange(ctx, value); err != nil {
		t.Fatal("old epoch retirement failed")
	}
	if pcHash(t, ctx, a, user.ID) != old || pcCount(t, ctx, a, `SELECT count(*) FROM auth.users WHERE id=$1 AND state='banned' AND recovery_state='recovery_required' AND credential_generation=8 AND security_version=9 AND pending_operation_id IS NULL`, user.ID) != 1 {
		t.Fatal("old sealed operation changed a newer denial floor or account state")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE state='retired' AND octet_length(replacement_hash_ciphertext)=0`) != 1 || pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`) != 0 {
		t.Fatal("retirement replayed stale facts or retained old sealed credential")
	}
}

func TestPasswordChangePGAdminNeedsCurrentAuthenticator(t *testing.T) {
	ctx, a, user, q, token, _ := newPasswordChangePGApp(t)
	pcExec(t, ctx, a, `UPDATE auth.users SET admin=true WHERE id=$1`, user.ID)
	q.OTP = "123456"
	if pcCall(a, "/api/auth/password-change", token, q).Code != 403 || pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes`) != 0 {
		t.Fatal("administrator changed password without an enrolled active authenticator")
	}
}

func TestPasswordChangePGAdminKeepsMFAAndCanQueryWithoutReplayingOTP(t *testing.T) {
	ctx, a, user, q, token, _ := newPasswordChangePGApp(t)
	pcExec(t, ctx, a, `UPDATE auth.users SET admin=true WHERE id=$1`, user.ID)
	factor := newID()
	secret := bytes.Repeat([]byte{0x72}, 20)
	sealed, err := a.seal(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), "mfa:"+factor+":"+user.ID)
	if err != nil {
		t.Fatal("owned MFA fixture could not be sealed")
	}
	pcExec(t, ctx, a, `INSERT INTO auth.mfa_factors(id,user_id,secret_ciphertext,last_accepted_step) VALUES($1,$2,$3,-1)`, factor, user.ID, sealed)
	step := time.Now().Unix() / 30
	q.OTP = totpCode(secret, step)
	if w := pcCall(a, "/api/auth/password-change", token, q); w.Code != 200 {
		t.Fatal("current enrolled administrator proof did not authorize the replacement")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.mfa_factors WHERE id=$1 AND revoked_at IS NULL AND last_accepted_step=$2`, factor, step) != 1 {
		t.Fatal("password change reset MFA or failed to consume its step")
	}
	q.OTP = ""
	if w := pcCall(a, "/api/auth/password-change-results", "", q); w.Code != 200 || pcBody(t, w)["current"] != true {
		t.Fatal("original result required replaying a consumed authenticator step")
	}
}

func TestPasswordChangePGBoundedOriginalKeyAndWorkerFairness(t *testing.T) {
	ctx, a, user, q, _, f := newPasswordChangePGApp(t)
	q.RequestID = strings.Repeat("a", 128)
	value, err := a.preparePasswordChange(ctx, user, q)
	if err != nil || len("principal:"+user.ID+":"+value.Operation) > 200 {
		t.Fatal("maximum legal request cannot pass downstream protection limits")
	}
	f.block("intent")
	if err = a.resumePasswordChanges(ctx, 1); err == nil {
		t.Fatal("injected pending attempt unexpectedly completed")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE last_attempt_at IS NOT NULL AND state='prepared'`) != 1 {
		t.Fatal("failed original cannot yield its bounded recovery position")
	}
	pcExec(t, ctx, a, `UPDATE auth.users SET state='banned' WHERE id=$1`, user.ID)
	f.block("")
	if err = a.resumePasswordChanges(ctx, 1); err != nil || pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE state='retired'`) != 1 {
		t.Fatal("superseded pending operation wedged background recovery or unbanned its owner")
	}
}

func TestPasswordChangePGResultProofOracleIsRateBounded(t *testing.T) {
	ctx, a, _, q, _, _ := newPasswordChangePGApp(t)
	for i := 0; i < 21; i++ {
		q.RequestID = "unknown-" + newID()
		want := 404
		if i == 20 {
			want = 429
		}
		if w := pcCall(a, "/api/auth/password-change-results", "", q); w.Code != want {
			t.Fatal("anonymous result proof lookup escaped its per-IP bound")
		}
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes`) != 0 || pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`) != 0 {
		t.Fatal("rate-limited unknown lookup created a credential operation or journal fact")
	}
}

func TestPasswordChangePGCompletedLookupCannotBypassIndependentEpochGate(t *testing.T) {
	ctx, a, _, q, token, _ := newPasswordChangePGApp(t)
	if pcCall(a, "/api/auth/password-change", token, q).Code != 200 {
		t.Fatal("original credential change failed")
	}
	pcExec(t, ctx, a, `UPDATE journal.restore_epochs SET restore_epoch=2`)
	if w := pcCall(a, "/api/auth/password-change-results", "", q); w.Code != 503 {
		t.Fatal("completed historical result called an ungated restored account current")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE state='completed'`) != 1 {
		t.Fatal("read-only epoch mismatch rewrote historical operation")
	}
}

func TestPasswordChangePGConcurrentOriginalRetriesCommitOnce(t *testing.T) {
	ctx, a, user, q, token, _ := newPasswordChangePGApp(t)
	start, done := make(chan struct{}), make(chan int, 2)
	for range 2 {
		go func() { <-start; done <- pcCall(a, "/api/auth/password-change", token, q).Code }()
	}
	close(start)
	if <-done != 200 || <-done != 200 {
		t.Fatal("concurrent original intent did not reconcile to its single result")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes`) != 1 || pcCount(t, ctx, a, `SELECT count(*) FROM auth.users WHERE id=$1 AND credential_generation=2 AND security_version=2`, user.ID) != 1 || pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries WHERE kind='auth.invalidate_user'`) != 2 {
		t.Fatal("concurrent original intent duplicated credential rotation or journal facts")
	}
}

func TestPasswordChangePGResultBindsEpochAcrossItsReadAndJournalCheck(t *testing.T) {
	ctx, a, _, q, token, f := newPasswordChangePGApp(t)
	if pcCall(a, "/api/auth/password-change", token, q).Code != 200 {
		t.Fatal("original credential change failed")
	}
	f.mu.Lock()
	f.beforeState = func() {
		pcExec(t, ctx, a, `UPDATE auth.instance SET restore_epoch=2; UPDATE journal.restore_epochs SET restore_epoch=2`)
	}
	f.mu.Unlock()
	if w := pcCall(a, "/api/auth/password-change-results", "", q); w.Code != 503 {
		t.Fatal("old metadata read adopted a newer matching business/journal epoch")
	}
}
