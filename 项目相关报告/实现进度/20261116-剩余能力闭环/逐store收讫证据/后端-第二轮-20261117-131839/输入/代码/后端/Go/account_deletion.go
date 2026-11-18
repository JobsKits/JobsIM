package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

// Identity owns credential denial; Governance owns cooldown/cancel arbitration.
// Neither role writes another role's schema during normal operation.
func migrateAccountDeletion(ctx context.Context, tx pgx.Tx, role string) error {
	schema := schemaFor[role]
	if schema == "" || role == "journal" || role == "control" {
		return nil
	}
	if _, err := tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+schema+`.account_erasure_floors(user_id uuid PRIMARY KEY,deletion_id uuid NOT NULL,request_version bigint NOT NULL CHECK(request_version>0),state text NOT NULL DEFAULT 'pending',updated_at timestamptz NOT NULL DEFAULT now(),last_object uuid); ALTER TABLE `+schema+`.account_erasure_floors ADD COLUMN IF NOT EXISTS last_object uuid;`); err != nil {
		return err
	}
	switch role {
	case "identity":
		_, err := tx.Exec(ctx, `ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS deletion_request_id uuid;
ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS deletion_version bigint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS auth.account_deletions(id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES auth.users(id),request_id text NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL,request_digest bytea NOT NULL,state text NOT NULL,
expected_security bigint NOT NULL,security_version bigint NOT NULL,request_version bigint NOT NULL DEFAULT 1,cancel_until timestamptz,cancel_request_id text,cancel_security bigint,
created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(user_id,request_id));
CREATE INDEX IF NOT EXISTS account_deletion_current ON auth.account_deletions(user_id,state);
ALTER TABLE auth.account_deletions ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz NOT NULL DEFAULT '-infinity';
CREATE INDEX IF NOT EXISTS account_deletion_retry ON auth.account_deletions(last_attempt_at,updated_at,id) WHERE state='prepared';
CREATE TABLE IF NOT EXISTS auth.data_rights_proofs(token_digest bytea PRIMARY KEY,user_id uuid NOT NULL,deletion_id uuid NOT NULL,request_version bigint NOT NULL,purpose text NOT NULL CHECK(purpose IN('query','cancel')),restore_epoch bigint NOT NULL,security_version bigint NOT NULL,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS auth.data_rights_audits(request_id text PRIMARY KEY,actor_id uuid NOT NULL,user_id uuid NOT NULL,deletion_id uuid NOT NULL,request_version bigint NOT NULL,purpose text NOT NULL,reason text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());`)
		return err
	case "governance":
		_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS governance.account_deletions(id uuid PRIMARY KEY,user_id uuid NOT NULL,request_id text NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL,state text NOT NULL DEFAULT 'prepared',row_version bigint NOT NULL DEFAULT 1,cancel_until timestamptz NOT NULL,cancel_request_id text,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(user_id,request_id));
CREATE INDEX IF NOT EXISTS account_deletion_due ON governance.account_deletions(state,cancel_until);
ALTER TABLE governance.account_deletions ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz NOT NULL DEFAULT '-infinity';
CREATE INDEX IF NOT EXISTS account_deletion_retry ON governance.account_deletions(last_attempt_at,updated_at,id) WHERE state IN('prepared','cancel_pending','processing','requested');
CREATE TABLE IF NOT EXISTS governance.account_deletion_acks(deletion_id uuid NOT NULL,owner text NOT NULL,request_version bigint NOT NULL,PRIMARY KEY(deletion_id,owner));`)
		return err
	}
	return nil
}

type accountDeletionRequest struct {
	RequestID    string `json:"requestId"`
	Username     string `json:"username"`
	DeviceID     string `json:"deviceId"`
	RestoreEpoch string `json:"restoreEpoch"`
	Password     string `json:"password"`
	OTP          string `json:"otp,omitempty"`
}

type accountDeletion struct {
	ID             string    `json:"deletionId"`
	User           string    `json:"userId,omitempty"`
	Request        string    `json:"requestId"`
	Device         string    `json:"deviceId,omitempty"`
	Epoch          string    `json:"restoreEpoch"`
	State          string    `json:"status"`
	Version        string    `json:"requestVersion"`
	Until          time.Time `json:"cancelUntil"`
	Cancel         string    `json:"cancelRequestId,omitempty"`
	Security       int64     `json:"-"`
	Expected       int64     `json:"-"`
	CancelSecurity int64     `json:"-"`
	Digest         []byte    `json:"-"`
}

const deletionIdentityColumns = `id::text,user_id::text,request_id,device_id::text,restore_epoch::text,state,request_version::text,COALESCE(cancel_until,'epoch'::timestamptz),COALESCE(cancel_request_id,''),security_version,expected_security,COALESCE(cancel_security,0),request_digest`
const deletionGovernanceColumns = `id::text,user_id::text,request_id,device_id::text,restore_epoch::text,state,row_version::text,cancel_until,COALESCE(cancel_request_id,'')`

func scanIdentityDeletion(row pgx.Row) (accountDeletion, error) {
	var d accountDeletion
	err := row.Scan(&d.ID, &d.User, &d.Request, &d.Device, &d.Epoch, &d.State, &d.Version, &d.Until, &d.Cancel, &d.Security, &d.Expected, &d.CancelSecurity, &d.Digest)
	return d, err
}
func scanGovernanceDeletion(row pgx.Row) (accountDeletion, error) {
	var d accountDeletion
	err := row.Scan(&d.ID, &d.User, &d.Request, &d.Device, &d.Epoch, &d.State, &d.Version, &d.Until, &d.Cancel)
	return d, err
}
func deletionOperation(d accountDeletion, stage string) string {
	return "account-deletion:" + d.ID + ":" + stage
}

func decodeAccountDeletion(r *http.Request) (accountDeletionRequest, error) {
	var q accountDeletionRequest
	if err := decodeReceiptRequest(r, &q); err != nil {
		return q, err
	}
	var err error
	q.Username, err = normalizeUsername(q.Username)
	q.Password = norm.NFC.String(q.Password)
	if err != nil || !validReceiptNonce(q.RequestID) || !validObjectID(q.DeviceID) || !syncValidationCounter(q.RestoreEpoch, false) || len(q.Password) == 0 || len(q.Password) > 512 || q.OTP != "" && (len(q.OTP) != 6 || strings.IndexFunc(q.OTP, func(r rune) bool { return r < '0' || r > '9' }) >= 0) {
		return q, fail(400, "INVALID_ACCOUNT_DELETION", "Original owner, device, epoch and current-password proof required")
	}
	return q, nil
}
func (a *App) deletionDigest(q accountDeletionRequest) []byte {
	q.OTP = ""
	raw, _ := json.Marshal(q)
	return keyedDigest(a.Config.Key, "own-account-deletion:"+a.Config.DeploymentID, raw)
}
func (a *App) deletionRate(r *http.Request, kind string) error {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return a.rate(r.Context(), "data-rights:"+kind+":"+host, 15)
}
func (a *App) accountDeletionRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	switch a.Role {
	case "identity":
		h("POST /api/auth/deletion-requests", a.requestAccountDeletion)
		h("POST /api/auth/deletion-results", a.accountDeletionResults)
		h("POST /api/auth/data-rights/proofs", a.issueDataRightsProof)
		h("POST /api/auth/data-rights/query", a.queryDataRights)
		h("POST /api/auth/data-rights/cancel", a.cancelDataRights)
		h("POST /api/admin/users/{id}/data-rights-proofs", a.adminDataRightsProof)
		h("POST /internal/deletion/activate", internal(a.activateDeletionInternal))
		h("POST /internal/deletion/cancel", internal(a.cancelDeletionInternal))
		h("POST /internal/deletion/identity-stage", internal(a.identityDeletionStageInternal))
	case "governance":
		h("POST /internal/deletion/prepare", internal(a.prepareDeletionInternal))
		h("POST /internal/deletion/state", internal(a.governanceDeletionState))
		h("POST /internal/deletion/activate-governance", internal(a.activateDeletionGovernance))
		h("POST /internal/deletion/begin-cancel", internal(a.beginDeletionCancel))
		h("POST /internal/deletion/confirm-cancel", internal(a.confirmDeletionCancel))
		h("POST /internal/deletion/qualify", internal(a.qualifyDeletion))
	}
	if a.Role != "identity" && a.Role != "control" && a.Role != "journal" {
		h("POST /internal/deletion/erase", internal(a.eraseAccountOwner))
	}
}

func (a *App) originalDeletion(ctx context.Context, q accountDeletionRequest) (accountDeletion, bool, error) {
	d, err := scanIdentityDeletion(a.DB.QueryRow(ctx, `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE user_id=(SELECT id FROM auth.users WHERE username=$1) AND request_id=$2`, q.Username, q.RequestID))
	if err == pgx.ErrNoRows {
		return d, false, nil
	}
	if err != nil {
		return d, false, err
	}
	if d.Device != q.DeviceID || d.Epoch != q.RestoreEpoch || !hmac.Equal(d.Digest, a.deletionDigest(q)) {
		return d, false, fail(403, "DATA_RIGHTS_REJECTED", "Original account-deletion proof does not match")
	}
	return d, true, nil
}
func (a *App) accountDeletionResults(w http.ResponseWriter, r *http.Request) error {
	if err := a.anonymousAuthOrigin(r); err != nil {
		return err
	}
	q, err := decodeAccountDeletion(r)
	if err != nil {
		return err
	}
	if err = a.deletionRate(r, "result"); err != nil {
		return err
	}
	d, ok, err := a.originalDeletion(r.Context(), q)
	if err != nil {
		return err
	}
	if !ok {
		return fail(404, "ACCOUNT_DELETION_NOT_FOUND", "No original deletion exists; none was created")
	}
	return a.respondDeletion(w, r, d)
}
func (a *App) respondDeletion(w http.ResponseWriter, r *http.Request, d accountDeletion) error {
	if d.State == "retired" {
		respond(w, 200, map[string]any{"deletionId": d.ID, "requestId": d.Request, "status": "retired", "requestVersion": d.Version, "restoreEpoch": d.Epoch, "cancelUntil": nil, "cancelRequestId": "", "requiresNewLogin": true})
		return nil
	}
	var current accountDeletion
	if err := a.rpc(r.Context(), "governance", "/internal/deletion/state", d, &current); err != nil {
		var e *apiError
		if errors.As(err, &e) && e.Status == 404 && d.State == "prepared" {
			respond(w, 200, map[string]any{"deletionId": d.ID, "requestId": d.Request, "status": "pending", "requestVersion": d.Version, "restoreEpoch": d.Epoch, "cancelUntil": nil, "cancelRequestId": "", "requiresNewLogin": true})
			return nil
		}
		return err
	}
	current.User, current.Device = "", ""
	if current.State == "prepared" {
		current.State = "pending"
	}
	respond(w, 200, map[string]any{"deletionId": current.ID, "requestId": current.Request, "status": current.State, "requestVersion": current.Version, "restoreEpoch": current.Epoch, "cancelUntil": current.Until, "cancelRequestId": current.Cancel, "requiresNewLogin": true})
	return nil
}
func (a *App) requestAccountDeletion(w http.ResponseWriter, r *http.Request) error {
	if err := a.anonymousAuthOrigin(r); err != nil {
		return err
	}
	q, err := decodeAccountDeletion(r)
	if err != nil {
		return err
	}
	if err = a.deletionRate(r, "request"); err != nil {
		return err
	}
	d, exists, err := a.originalDeletion(r.Context(), q)
	if err != nil {
		return err
	}
	if !exists {
		u, err := a.current(r)
		if err != nil {
			return err
		}
		if u.Username != q.Username || u.DeviceID != q.DeviceID || u.RestoreEpoch != q.RestoreEpoch {
			return fail(403, "DATA_RIGHTS_REJECTED", "The current original owner is required")
		}
		d, err = a.prepareIdentityDeletion(r.Context(), u, q)
		if err != nil {
			return err
		}
	}
	if err = a.finishDeletionRequest(r.Context(), d); err != nil {
		return err
	}
	d, _, err = a.originalDeletion(r.Context(), q)
	if err != nil {
		return err
	}
	return a.respondDeletion(w, r, d)
}
func (a *App) prepareIdentityDeletion(ctx context.Context, u User, q accountDeletionRequest) (accountDeletion, error) {
	d := accountDeletion{ID: newID(), User: u.ID, Request: q.RequestID, Device: q.DeviceID, Epoch: q.RestoreEpoch, Version: "1", State: "prepared", Digest: a.deletionDigest(q)}
	var hash string
	if err := a.DB.QueryRow(ctx, `SELECT password_hash,security_version FROM auth.users WHERE id=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none' AND pending_operation_id IS NULL`, u.ID).Scan(&hash, &d.Expected); err != nil {
		return d, fail(403, "DATA_RIGHTS_REJECTED", "Current trusted credentials required")
	}
	if d.Expected == math.MaxInt64 {
		return d, fail(409, "GENERATION_EXHAUSTED", "Controlled security maintenance required")
	}
	if err := a.passwordSlot(ctx); err != nil {
		return d, err
	}
	valid := verifyPassword(hash, q.Password)
	<-a.hashSlots
	if !valid {
		return d, fail(403, "DATA_RIGHTS_REJECTED", "Current trusted credentials rejected")
	}
	if u.Admin {
		if err := a.consumeMFA(ctx, u.ID, q.OTP); err != nil {
			return d, err
		}
	}
	d.Security = d.Expected + 1
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)
	var security int64
	var currentHash, epoch string
	var pending *string
	if err = tx.QueryRow(ctx, `SELECT security_version,password_hash,pending_operation_id FROM auth.users WHERE id=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none' FOR UPDATE`, u.ID).Scan(&security, &currentHash, &pending); err != nil || security != d.Expected || currentHash != hash || pending != nil {
		return d, fail(409, "ACCOUNT_DELETION_CONTEXT_CHANGED", "Reload current account before a new deletion")
	}
	if err = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM auth.instance WHERE singleton FOR SHARE`).Scan(&epoch); err != nil {
		return d, err
	}
	if epoch != d.Epoch {
		return d, fail(409, "RESTORE_EPOCH_CHANGED", "Deletion belongs to another restore epoch")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth.account_deletions(id,user_id,request_id,device_id,restore_epoch,request_digest,state,expected_security,security_version) VALUES($1,$2,$3,$4,$5,$6,'prepared',$7,$8)`, d.ID, d.User, d.Request, d.Device, d.Epoch, d.Digest, d.Expected, d.Security); err != nil {
		return d, err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.users SET pending_operation_id=$2 WHERE id=$1`, d.User, deletionOperation(d, "request")); err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}

func (a *App) deletionSafety(d accountDeletion, state string) map[string]any {
	security := d.Security
	if state == "cancelled" {
		security = d.CancelSecurity
	}
	return map[string]any{"userId": d.User, "deletionId": d.ID, "deviceId": d.Device, "requestKey": d.Request, "restoreEpoch": d.Epoch, "requestVersion": d.Version, "deletionState": state, "securityVersion": str(security), "cancelUntil": d.Until.UTC().Format(time.RFC3339Nano)}
}
