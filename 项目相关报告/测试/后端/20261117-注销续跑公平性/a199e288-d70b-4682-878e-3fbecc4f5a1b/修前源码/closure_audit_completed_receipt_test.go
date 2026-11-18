package jobsim

import (
	"bytes"
	"context"
	"encoding/base32"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A separate live administrator may recover only the completed receipt. The
// deleted user's credentials, cancellation and normal authorization stay shut.
func TestClosureAuditDeletionCompletedReceiptCanRenewOnlyThroughAuditedAdmin(t *testing.T) {
	f := newDeletionPG(t, 0)
	a := f.nodes["identity"]
	admin := User{ID: newID(), Admin: true, DisplayName: "Independent receipt auditor"}
	admin.Username = "receipt_" + strings.ReplaceAll(admin.ID, "-", "")[:20]
	deletionExec(t, f.ctx, a, `INSERT INTO auth.users(id,username,display_name,password_hash,admin) VALUES($1,$2,$3,$4,true)`, admin.ID, admin.Username, admin.DisplayName, pcHash(t, f.ctx, a, f.user.ID))
	tx, err := a.DB.Begin(f.ctx)
	if err != nil {
		t.Fatal("owned independent administrator transaction unavailable")
	}
	delivery, err := a.issueSession(f.ctx, tx, admin, "receipt-auditor", 1, 1, 1, false)
	if err != nil {
		_ = tx.Rollback(f.ctx)
		t.Fatal("owned independent administrator session unavailable")
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal("owned independent administrator session not committed")
	}
	factor := newID()
	secret := bytes.Repeat([]byte{0x39}, 20)
	sealed, err := a.seal(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), "mfa:"+factor+":"+admin.ID)
	if err != nil {
		t.Fatal("owned independent authenticator unavailable")
	}
	deletionExec(t, f.ctx, a, `INSERT INTO auth.mfa_factors(id,user_id,secret_ciphertext,last_accepted_step) VALUES($1,$2,$3,-1)`, factor, admin.ID, sealed)
	batch, code := newID(), newID()
	recoveryCode := "synthetic-completed-receipt-recovery-code"
	deletionExec(t, f.ctx, a, `INSERT INTO auth.recovery_code_batches(id,user_id,generation,active) VALUES($1,$2,1,true)`, batch, f.user.ID)
	deletionExec(t, f.ctx, a, `UPDATE auth.users SET recovery_generation=1 WHERE id=$1`, f.user.ID)
	deletionExec(t, f.ctx, a, `INSERT INTO auth.recovery_codes(id,batch_id,code_digest) VALUES($1,$2,$3)`, code, batch, keyedDigest(a.Config.Key, "recovery", []byte(recoveryCode)))

	f.request(t)
	deletionFence(t, f.nodes["governance"], func(ctx context.Context) error {
		return f.nodes["governance"].resumeAccountDeletions(ctx, 2)
	})
	d := deletionBody(t, deletionCall(a, "/api/auth/deletion-results", "", f.q), 200)
	if d["status"] != "completed" {
		t.Fatal("owned deletion did not reach its completed receipt")
	}
	var hash, state string
	if err = a.DB.QueryRow(f.ctx, `SELECT password_hash,deletion_state FROM auth.users WHERE id=$1`, f.user.ID).Scan(&hash, &state); err != nil || hash != "" || state != "completed" {
		t.Fatal("completed credential erasure not established")
	}
	request := map[string]string{
		"requestId": newID(), "deletionId": d["deletionId"].(string), "requestVersion": d["requestVersion"].(string),
		"purpose": "query", "reason": "Audited request to recover the original completion receipt",
	}
	path := "/api/admin/users/" + f.user.ID + "/data-rights-proofs"
	deletionBody(t, deletionCall(a, path, delivery.Result.Token, request), 403)
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM auth.data_rights_audits WHERE request_id=$1`, request["requestId"]) != 0 {
		t.Fatal("unauthenticated receipt audit was persisted")
	}
	request["otp"] = totpCode(secret, time.Now().Unix()/30)
	first := deletionBody(t, deletionCall(a, path, delivery.Result.Token, request), 200)
	proof := first["proof"].(string)
	if first["purpose"] != "query" || first["deletionId"] != d["deletionId"] || first["requestVersion"] != d["requestVersion"] {
		t.Fatal("completed receipt proof lost its original purpose or subject")
	}
	var expires time.Time
	if err = a.DB.QueryRow(f.ctx, `SELECT expires_at FROM auth.data_rights_proofs WHERE token_digest=$1 AND purpose='query' AND user_id=$2 AND deletion_id=$3`, keyedDigest(a.Config.Key, "data-rights-proof", []byte(proof)), f.user.ID, d["deletionId"]).Scan(&expires); err != nil || !expires.After(time.Now()) || expires.After(time.Now().Add(5*time.Minute+time.Second)) {
		t.Fatal("completed receipt proof was not bounded to five minutes")
	}
	read := deletionBody(t, deletionCall(a, "/api/auth/data-rights/query", "", map[string]string{"proof": proof}), 200)
	if read["status"] != "completed" || read["deletionId"] != d["deletionId"] {
		t.Fatal("audited proof could not query the original completion")
	}
	deletionExec(t, f.ctx, a, `UPDATE auth.data_rights_proofs SET expires_at=clock_timestamp()-interval '1 second' WHERE token_digest=$1`, keyedDigest(a.Config.Key, "data-rights-proof", []byte(proof)))
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/query", "", map[string]string{"proof": proof}), 403)
	request["requestId"], request["otp"] = newID(), totpCode(secret, time.Now().Unix()/30+1)
	renewed := deletionBody(t, deletionCall(a, path, delivery.Result.Token, request), 200)
	newProof := renewed["proof"].(string)
	if newProof == proof {
		t.Fatal("expired receipt token was reused instead of a fresh scoped proof")
	}
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/query", "", map[string]string{"proof": newProof}), 200)
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM auth.data_rights_audits WHERE actor_id=$1 AND user_id=$2 AND deletion_id=$3 AND purpose='query'`, admin.ID, f.user.ID, d["deletionId"]) != 2 {
		t.Fatal("completed receipt renewals were not independently audited")
	}

	request["requestId"], request["purpose"] = newID(), "cancel"
	deletionBody(t, deletionCall(a, path, delivery.Result.Token, request), 409)
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/cancel", "", map[string]string{"proof": newProof, "requestId": newID()}), 403)
	old := dataRightsProofRequest{Username: f.q.Username, DeletionID: d["deletionId"].(string), Version: d["requestVersion"].(string), Purpose: "query", Password: f.q.Password}
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/proofs", "", old), 403)
	old.Password, old.Code = "", recoveryCode
	deletionBody(t, deletionCall(a, "/api/auth/data-rights/proofs", "", old), 403)
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.Header.Set("Authorization", "Bearer "+newProof)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 401 || pcCount(t, f.ctx, a, `SELECT count(*) FROM auth.sessions WHERE user_id=$1 AND revoked_at IS NULL`, f.user.ID) != 0 || pcCount(t, f.ctx, a, `SELECT count(*) FROM auth.data_rights_proofs WHERE user_id=$1 AND purpose='cancel'`, f.user.ID) != 0 {
		t.Fatal("receipt renewal reopened normal sessions or cancellation")
	}
	if err = a.DB.QueryRow(f.ctx, `SELECT password_hash,deletion_state FROM auth.users WHERE id=$1`, f.user.ID).Scan(&hash, &state); err != nil || hash != "" || state != "completed" {
		t.Fatal("receipt renewal lowered the completed credential floor")
	}
}
