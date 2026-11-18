package jobsim

import (
	"context"
	"crypto/hmac"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

type dataRightsProofRequest struct {
	Username   string `json:"username"`
	DeletionID string `json:"deletionId"`
	Version    string `json:"requestVersion"`
	Purpose    string `json:"purpose"`
	Password   string `json:"password,omitempty"`
	Code       string `json:"recoveryCode,omitempty"`
	OTP        string `json:"otp,omitempty"`
}
type dataRightsProof struct {
	User, Deletion, Version, Purpose, Epoch string
	Security                                int64
	Expires                                 time.Time
}

func rightsPurpose(value string) bool { return value == "query" || value == "cancel" }
func (a *App) rightsSubject(ctx context.Context, id, version, purpose string) (accountDeletion, string, int64, bool, string, error) {
	d, err := scanIdentityDeletion(a.DB.QueryRow(ctx, `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE id=$1`, id))
	if err != nil {
		return d, "", 0, false, "", fail(403, "DATA_RIGHTS_REJECTED", "Trusted original subject proof required")
	}
	var hash, recovery string
	var security int64
	var admin bool
	err = a.DB.QueryRow(ctx, `SELECT password_hash,security_version,admin,recovery_state FROM auth.users WHERE id=$1`, d.User).Scan(&hash, &security, &admin, &recovery)
	if err != nil {
		return d, "", 0, false, "", err
	}
	var g accountDeletion
	if err = a.rpc(ctx, "governance", "/internal/deletion/state", d, &g); err != nil {
		return d, "", 0, false, "", err
	}
	if len(version) > len(g.Version) || len(version) == len(g.Version) && version > g.Version || g.Version != version && purpose != "query" {
		return d, "", 0, false, "", fail(409, "DELETION_VERSION_CHANGED", "Query the original deletion and verify its current request version")
	}
	d.Version = g.Version
	d.State = g.State
	d.Until = g.Until
	return d, hash, security, admin, recovery, nil
}
func (a *App) issueDataRightsProof(w http.ResponseWriter, r *http.Request) error {
	if err := a.anonymousAuthOrigin(r); err != nil {
		return err
	}
	var q dataRightsProofRequest
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validObjectID(q.DeletionID) || !syncValidationCounter(q.Version, false) || !rightsPurpose(q.Purpose) || q.Password == "" && q.Code == "" || q.Password != "" && q.Code != "" || len(q.Password) > 512 || len(q.Code) > 128 {
		return fail(400, "INVALID_DATA_RIGHTS_PROOF", "One current credential and a bound purpose required")
	}
	if err := a.deletionRate(r, "proof"); err != nil {
		return err
	}
	name, err := normalizeUsername(q.Username)
	if err != nil {
		return err
	}
	d, hash, security, admin, recovery, err := a.rightsSubject(r.Context(), q.DeletionID, q.Version, q.Purpose)
	if err != nil {
		return err
	}
	var storedName string
	if err = a.DB.QueryRow(r.Context(), `SELECT username FROM auth.users WHERE id=$1`, d.User).Scan(&storedName); err != nil {
		return err
	}
	if storedName != name || d.State == "completed" || d.State == "processing" && q.Purpose == "cancel" {
		return fail(403, "DATA_RIGHTS_REJECTED", "Trusted original subject proof required")
	}
	if q.Password != "" {
		if recovery != "normal" || hash == "" {
			return fail(403, "DATA_RIGHTS_REJECTED", "Current trusted credentials required")
		}
		if err = a.passwordSlot(r.Context()); err != nil {
			return err
		}
		valid := verifyPassword(hash, norm.NFC.String(q.Password))
		<-a.hashSlots
		if !valid {
			return fail(403, "DATA_RIGHTS_REJECTED", "Current trusted credentials rejected")
		}
		if admin {
			if err = a.consumeMFA(r.Context(), d.User, q.OTP); err != nil {
				return err
			}
		}
	} else {
		// Consumption is independently journaled, so restoring an earlier business
		// snapshot cannot make the same recovery code a second proof.
		var code string
		if err = a.DB.QueryRow(r.Context(), `SELECT c.id::text FROM auth.recovery_codes c JOIN auth.recovery_code_batches b ON b.id=c.batch_id JOIN auth.users u ON u.id=b.user_id WHERE b.user_id=$1 AND b.active AND b.generation=u.recovery_generation AND c.consumed_at IS NULL AND c.code_digest=$2`, d.User, keyedDigest(a.Config.Key, "recovery", []byte(q.Code))).Scan(&code); err != nil {
			return fail(403, "DATA_RIGHTS_REJECTED", "Current trusted recovery proof rejected")
		}
		payload := map[string]any{"userId": d.User, "codeId": code}
		if err = a.protect(r.Context(), "data-rights-code:"+code, "intent", "auth.data_rights_code", payload); err != nil {
			return err
		}
		tag, err := a.DB.Exec(r.Context(), `UPDATE auth.recovery_codes SET consumed_at=now() WHERE id=$1 AND consumed_at IS NULL`, code)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fail(403, "DATA_RIGHTS_REJECTED", "Recovery proof was already consumed")
		}
		if err = a.protect(r.Context(), "data-rights-code:"+code, "result", "auth.data_rights_code", payload); err != nil {
			return err
		}
	}
	return a.deliverRightsProof(w, r, d, q.Purpose, security)
}
func (a *App) deliverRightsProof(w http.ResponseWriter, r *http.Request, d accountDeletion, purpose string, security int64) error {
	epoch, err := a.currentIdentityEpoch(r.Context())
	if err != nil {
		return err
	}
	token := randomToken(32)
	var expiry time.Time
	// Completed accounts retain a minimal receipt, not login credentials. Only
	// the separately authenticated audit path can reach this query-only case.
	err = a.DB.QueryRow(r.Context(), `INSERT INTO auth.data_rights_proofs(token_digest,user_id,deletion_id,request_version,purpose,restore_epoch,security_version,expires_at) SELECT $1,id,$3,$4,$5,$6,$7,clock_timestamp()+interval '5 minutes' FROM auth.users WHERE id=$2 AND security_version=$7 AND (deletion_state<>'completed' OR $5='query') RETURNING expires_at`, keyedDigest(a.Config.Key, "data-rights-proof", []byte(token)), d.User, d.ID, d.Version, purpose, epoch, security).Scan(&expiry)
	if err == pgx.ErrNoRows {
		return fail(409, "DATA_RIGHTS_CONTEXT_CHANGED", "Credentials changed before proof delivery")
	}
	if err != nil {
		return err
	}
	respond(w, 200, map[string]any{"proof": token, "deletionId": d.ID, "requestVersion": d.Version, "purpose": purpose, "expiresAt": expiry, "restoreEpoch": str(epoch)})
	return nil
}
func (a *App) readRightsProof(ctx context.Context, token, purpose string) (dataRightsProof, error) {
	var p dataRightsProof
	if len(token) < 32 || len(token) > 128 {
		return p, fail(403, "DATA_RIGHTS_PROOF_REQUIRED", "A current purpose-bound data-rights proof is required")
	}
	epoch, err := a.currentIdentityEpoch(ctx)
	if err != nil {
		return p, err
	}
	err = a.DB.QueryRow(ctx, `SELECT user_id::text,deletion_id::text,request_version::text,purpose,restore_epoch::text,security_version,expires_at FROM auth.data_rights_proofs WHERE token_digest=$1 AND purpose=$2 AND restore_epoch=$3 AND expires_at>clock_timestamp()`, keyedDigest(a.Config.Key, "data-rights-proof", []byte(token)), purpose, epoch).Scan(&p.User, &p.Deletion, &p.Version, &p.Purpose, &p.Epoch, &p.Security, &p.Expires)
	if err == pgx.ErrNoRows {
		return p, fail(403, "DATA_RIGHTS_PROOF_REQUIRED", "Proof is expired, changed or bound to another purpose")
	}
	return p, err
}
func (a *App) queryDataRights(w http.ResponseWriter, r *http.Request) error {
	if err := a.anonymousAuthOrigin(r); err != nil {
		return err
	}
	if err := a.deletionRate(r, "query"); err != nil {
		return err
	}
	var q struct {
		Proof string `json:"proof"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	p, err := a.readRightsProof(r.Context(), q.Proof, "query")
	if err != nil {
		return err
	}
	var security int64
	if err = a.DB.QueryRow(r.Context(), `SELECT security_version FROM auth.users WHERE id=$1`, p.User).Scan(&security); err != nil {
		return err
	}
	if security != p.Security {
		return fail(403, "DATA_RIGHTS_CONTEXT_CHANGED", "Reverify the current trusted credential")
	}
	return a.respondDeletion(w, r, accountDeletion{ID: p.Deletion, User: p.User})
}
func (a *App) cancelDataRights(w http.ResponseWriter, r *http.Request) error {
	if err := a.anonymousAuthOrigin(r); err != nil {
		return err
	}
	if err := a.deletionRate(r, "cancel"); err != nil {
		return err
	}
	var q struct {
		Proof   string `json:"proof"`
		Request string `json:"requestId"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validReceiptNonce(q.Request) {
		return fail(400, "INVALID_DELETION_CANCEL", "Original cancellation nonce required")
	}
	p, err := a.readRightsProof(r.Context(), q.Proof, "cancel")
	if err != nil {
		return err
	}
	d := accountDeletion{ID: p.Deletion, User: p.User, Version: p.Version, Cancel: q.Request}
	var security int64
	if err = a.DB.QueryRow(r.Context(), `SELECT security_version FROM auth.users WHERE id=$1`, p.User).Scan(&security); err != nil {
		return err
	}
	if security != p.Security {
		var old accountDeletion
		if err = a.rpc(r.Context(), "governance", "/internal/deletion/state", d, &old); err != nil {
			return err
		}
		if old.Cancel != q.Request || old.State != "cancel_pending" && old.State != "cancelled" {
			return fail(403, "DATA_RIGHTS_CONTEXT_CHANGED", "Reverify current credentials before a new cancellation")
		}
		return a.respondDeletion(w, r, d)
	}
	var qualified accountDeletion
	if err = a.rpc(r.Context(), "governance", "/internal/deletion/begin-cancel", d, &qualified); err != nil {
		return err
	}
	if err = a.finishDeletionCancellation(r.Context(), qualified); err != nil {
		return err
	}
	return a.respondDeletion(w, r, d)
}
func (a *App) adminDataRightsProof(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	if !u.Admin {
		return fail(403, "ADMIN_REQUIRED", "Audited administrator verification required")
	}
	if err = a.deletionRate(r, "admin-proof"); err != nil {
		return err
	}
	var q struct {
		Request  string `json:"requestId"`
		Deletion string `json:"deletionId"`
		Version  string `json:"requestVersion"`
		Purpose  string `json:"purpose"`
		Reason   string `json:"reason"`
		OTP      string `json:"otp"`
	}
	if err = decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	q.Reason = strings.TrimSpace(q.Reason)
	if !validReceiptNonce(q.Request) || !validObjectID(q.Deletion) || !syncValidationCounter(q.Version, false) || !rightsPurpose(q.Purpose) || len(q.Reason) < 10 || len(q.Reason) > 512 {
		return fail(400, "INVALID_DATA_RIGHTS_AUDIT", "Original audit, target, purpose and reason required")
	}
	d, _, security, _, _, err := a.rightsSubject(r.Context(), q.Deletion, q.Version, q.Purpose)
	if err != nil {
		return err
	}
	if d.User != r.PathValue("id") {
		return fail(403, "DATA_RIGHTS_REJECTED", "Original audit target differs")
	}
	if d.State == "completed" && q.Purpose != "query" {
		return fail(409, "DELETION_CANCEL_CLOSED", "Completed deletion permits only its audited receipt query")
	}
	if err = a.consumeMFA(r.Context(), u.ID, q.OTP); err != nil {
		return err
	}
	_, err = a.DB.Exec(r.Context(), `INSERT INTO auth.data_rights_audits(request_id,actor_id,user_id,deletion_id,request_version,purpose,reason) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, q.Request, u.ID, d.User, d.ID, d.Version, q.Purpose, q.Reason)
	if err != nil {
		return err
	}
	var actor, user, deletion, version, purpose, reason string
	if err = a.DB.QueryRow(r.Context(), `SELECT actor_id::text,user_id::text,deletion_id::text,request_version::text,purpose,reason FROM auth.data_rights_audits WHERE request_id=$1`, q.Request).Scan(&actor, &user, &deletion, &version, &purpose, &reason); err != nil {
		return err
	}
	if !hmac.Equal([]byte(strings.Join([]string{actor, user, deletion, version, purpose, reason}, "\x00")), []byte(strings.Join([]string{u.ID, d.User, d.ID, d.Version, q.Purpose, q.Reason}, "\x00"))) {
		return fail(409, "IDEMPOTENCY_CONFLICT", "Original data-rights audit differs")
	}
	return a.deliverRightsProof(w, r, d, q.Purpose, security)
}
