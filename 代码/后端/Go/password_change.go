package jobsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

func init() {
	migrations["identity"] += `CREATE TABLE IF NOT EXISTS auth.password_changes(
user_id uuid NOT NULL REFERENCES auth.users(id),request_id text NOT NULL,device_id uuid NOT NULL,
restore_epoch bigint NOT NULL CHECK(restore_epoch>0),request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
operation_id text NOT NULL UNIQUE,expected_generation bigint NOT NULL CHECK(expected_generation>0),
expected_security_version bigint NOT NULL CHECK(expected_security_version>0),
new_generation bigint NOT NULL CHECK(new_generation>expected_generation),
new_security_version bigint NOT NULL CHECK(new_security_version>expected_security_version),
replacement_hash_ciphertext bytea NOT NULL,state text NOT NULL CHECK(state IN('prepared','committed','completed','retired')),
created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(user_id,request_id));
ALTER TABLE auth.password_changes ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz;
CREATE INDEX IF NOT EXISTS password_change_pending ON auth.password_changes(updated_at) WHERE state IN('prepared','committed');`
}

type passwordChangeRequest struct {
	RequestID       string `json:"requestId"`
	Username        string `json:"username"`
	DeviceID        string `json:"deviceId"`
	RestoreEpoch    string `json:"restoreEpoch"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	OTP             string `json:"otp,omitempty"`
}

type passwordChangeRecord struct {
	User, Request, Device, Epoch, Operation, State string
	ExpectedGeneration, ExpectedSecurity           int64
	Generation, Security                           int64
	Digest, Ciphertext                             []byte
}

const passwordChangeColumns = `user_id::text,request_id,device_id::text,restore_epoch::text,operation_id,state,
expected_generation,expected_security_version,new_generation,new_security_version,request_digest,replacement_hash_ciphertext`

func scanPasswordChange(row pgx.Row) (passwordChangeRecord, error) {
	var value passwordChangeRecord
	err := row.Scan(&value.User, &value.Request, &value.Device, &value.Epoch, &value.Operation, &value.State,
		&value.ExpectedGeneration, &value.ExpectedSecurity, &value.Generation, &value.Security, &value.Digest, &value.Ciphertext)
	return value, err
}

func decodePasswordChange(r *http.Request) (passwordChangeRequest, error) {
	var q passwordChangeRequest
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 8<<10))
	if err != nil || !utf8.Valid(raw) {
		return q, fail(400, "INVALID_PASSWORD_CHANGE", "One bounded UTF-8 password-change request is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&q) != nil || decoder.Decode(new(any)) != io.EOF {
		return q, fail(400, "INVALID_PASSWORD_CHANGE", "One complete bounded password-change request is required")
	}
	q.Username, err = normalizeUsername(q.Username)
	if err != nil {
		return q, err
	}
	q.NewPassword, err = normalizePassword(q.NewPassword)
	if err != nil {
		return q, err
	}
	q.CurrentPassword = norm.NFC.String(q.CurrentPassword)
	if !utf8.ValidString(q.CurrentPassword) || len(q.CurrentPassword) == 0 || len(q.CurrentPassword) > 512 || !validReceiptNonce(q.RequestID) || !syncValidationID(q.DeviceID) || !syncValidationCounter(q.RestoreEpoch, false) {
		return q, fail(400, "INVALID_PASSWORD_CHANGE", "Original request, device, epoch and current-password proof are required")
	}
	if q.OTP != "" && (len(q.OTP) != 6 || strings.IndexFunc(q.OTP, func(r rune) bool { return r < '0' || r > '9' }) >= 0) {
		return q, fail(400, "INVALID_PASSWORD_CHANGE", "Authenticator code must contain six digits")
	}
	return q, nil
}

// OTP authorizes the first submission, but is not the durable business intent.
// Original-result queries need the exact password pair and binding, not a code
// which has already expired or been consumed. No password is persisted here.
func (a *App) passwordChangeDigest(q passwordChangeRequest) []byte {
	q.OTP = ""
	raw, _ := json.Marshal(q)
	return keyedDigest(a.Config.Key, "own-password-change:"+a.Config.DeploymentID, raw)
}

func (a *App) passwordChangeAAD(value passwordChangeRecord) string {
	return strings.Join([]string{"own-password-change", a.Config.DeploymentID, value.User, value.Device, value.Epoch, value.Operation}, "\x00")
}

func (a *App) originalPasswordChange(ctx context.Context, q passwordChangeRequest) (passwordChangeRecord, bool, error) {
	value, err := scanPasswordChange(a.DB.QueryRow(ctx, `SELECT `+passwordChangeColumns+` FROM auth.password_changes p
WHERE p.user_id=(SELECT id FROM auth.users WHERE username=$1) AND p.request_id=$2`, q.Username, q.RequestID))
	if err == pgx.ErrNoRows {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	if !hmac.Equal(value.Digest, a.passwordChangeDigest(q)) || value.Device != q.DeviceID || value.Epoch != q.RestoreEpoch {
		return value, false, fail(403, "PASSWORD_CHANGE_REJECTED", "Original password-change proof does not match")
	}
	return value, true, nil
}

func (a *App) passwordChangeRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("POST /api/auth/password-change", a.changeOwnPassword)
	h("POST /api/auth/password-change-results", a.ownPasswordChangeResults)
}

func (a *App) ownPasswordChangeResults(w http.ResponseWriter, r *http.Request) error {
	if err := a.anonymousAuthOrigin(r); err != nil {
		return err
	}
	q, err := decodePasswordChange(r)
	if err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if err = a.rate(r.Context(), "password-change-result-ip:"+host, 20); err != nil {
		return err
	}
	value, exists, err := a.originalPasswordChange(r.Context(), q)
	if err != nil {
		return err
	}
	if !exists {
		return fail(404, "PASSWORD_CHANGE_OPERATION_NOT_FOUND", "No original password change exists; none was created")
	}
	return a.respondPasswordChange(w, r, value)
}

func (a *App) respondPasswordChange(w http.ResponseWriter, r *http.Request, value passwordChangeRecord) error {
	var epoch, generation, security string
	var state, recovery, deletion string
	err := a.DB.QueryRow(r.Context(), `SELECT i.restore_epoch::text,u.credential_generation::text,u.security_version::text,u.state,u.recovery_state,u.deletion_state
FROM auth.users u CROSS JOIN auth.instance i WHERE u.id=$1 AND i.singleton`, value.User).Scan(&epoch, &generation, &security, &state, &recovery, &deletion)
	if err != nil {
		return err
	}
	status := "pending"
	if value.State == "completed" {
		status = "changed"
	} else if value.State == "retired" || value.Epoch != epoch {
		status = "retired"
	}
	current := value.State == "completed" && value.Epoch == epoch && generation == str(value.Generation) && security == str(value.Security) && state == "active" && recovery == "normal" && deletion == "none"
	if current {
		currentEpoch, gateError := a.currentIdentityEpoch(r.Context())
		if gateError != nil {
			return gateError
		}
		if str(currentEpoch) != value.Epoch || str(currentEpoch) != epoch {
			return fail(503, "RESTORE_GATE_CLOSED", "Original result is not current in the independently verified epoch")
		}
	}
	respond(w, 200, map[string]any{"requestId": value.Request, "status": status, "restoreEpoch": value.Epoch, "credentialGeneration": str(value.Generation), "securityVersion": str(value.Security), "current": current, "requiresNewLogin": value.State == "committed" || value.State == "completed"})
	return nil
}

func (a *App) changeOwnPassword(w http.ResponseWriter, r *http.Request) error {
	if err := a.anonymousAuthOrigin(r); err != nil {
		return err
	}
	q, err := decodePasswordChange(r)
	if err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if err = a.rate(r.Context(), "password-change-ip:"+host, 20); err != nil {
		return err
	}
	value, exists, err := a.originalPasswordChange(r.Context(), q)
	if err != nil {
		return err
	}
	clearBrowser := false
	if !exists {
		u, err := a.current(r)
		if err != nil {
			return err
		}
		if u.Username != q.Username || u.DeviceID != q.DeviceID || u.RestoreEpoch != q.RestoreEpoch {
			return fail(403, "PASSWORD_CHANGE_REJECTED", "Current authenticated owner and original device are required")
		}
		clearBrowser = !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && bearer(r) != ""
		if err = a.rate(r.Context(), "password-change:"+u.ID, 5); err != nil {
			return err
		}
		value, err = a.preparePasswordChange(r.Context(), u, q)
		if err != nil {
			return err
		}
	}
	if err = a.finishPasswordChange(r.Context(), value); err != nil {
		return err
	}
	value, _, err = a.originalPasswordChange(r.Context(), q)
	if err != nil {
		return err
	}
	// Only the fresh authenticated browser submission owns its current cookie.
	// Historical retries must not clear a later login in this browser.
	if clearBrowser && value.State == "completed" {
		a.clearCookie(w)
	}
	return a.respondPasswordChange(w, r, value)
}

func (a *App) preparePasswordChange(ctx context.Context, u User, q passwordChangeRequest) (passwordChangeRecord, error) {
	value := passwordChangeRecord{User: u.ID, Request: q.RequestID, Device: q.DeviceID, Epoch: q.RestoreEpoch, Operation: "password-change:" + u.ID + ":" + hex.EncodeToString(tokenDigest(q.RequestID)), State: "prepared", Digest: a.passwordChangeDigest(q)}
	var previous string
	err := a.DB.QueryRow(ctx, `SELECT password_hash,credential_generation,security_version FROM auth.users
WHERE id=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none' AND pending_operation_id IS NULL`, u.ID).Scan(&previous, &value.ExpectedGeneration, &value.ExpectedSecurity)
	if err != nil {
		return value, fail(403, "PASSWORD_CHANGE_REJECTED", "The current active account is required")
	}
	if value.ExpectedGeneration >= math.MaxInt64 || value.ExpectedSecurity >= math.MaxInt64 {
		return value, fail(409, "GENERATION_EXHAUSTED", "The account requires controlled security maintenance")
	}
	if err = a.passwordSlot(ctx); err != nil {
		return value, err
	}
	valid := verifyPassword(previous, q.CurrentPassword)
	var replacement string
	if valid && !verifyPassword(previous, q.NewPassword) {
		replacement = hashPassword(q.NewPassword)
	}
	<-a.hashSlots
	if !valid {
		return value, fail(403, "PASSWORD_CHANGE_REJECTED", "Current-password proof was rejected")
	}
	if replacement == "" {
		return value, fail(400, "PASSWORD_REUSED", "Choose a different new password")
	}
	if u.Admin {
		if err = a.consumeMFA(ctx, u.ID, q.OTP); err != nil {
			return value, err
		}
	}
	value.Generation, value.Security = value.ExpectedGeneration+1, value.ExpectedSecurity+1
	value.Ciphertext, err = a.seal(replacement, a.passwordChangeAAD(value))
	if err != nil {
		return value, err
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return value, err
	}
	defer tx.Rollback(ctx)
	var generation, security int64
	var hash string
	var pending *string
	err = tx.QueryRow(ctx, `SELECT credential_generation,security_version,password_hash,pending_operation_id FROM auth.users
WHERE id=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none' FOR UPDATE`, u.ID).Scan(&generation, &security, &hash, &pending)
	if err != nil || pending != nil || generation != value.ExpectedGeneration || security != value.ExpectedSecurity || !bytes.Equal([]byte(hash), []byte(previous)) {
		return value, fail(409, "PASSWORD_CHANGE_CONTEXT_CHANGED", "Account state changed before the original password change was prepared")
	}
	var epoch string
	if err = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM auth.instance WHERE singleton FOR SHARE`).Scan(&epoch); err != nil {
		return value, err
	}
	if epoch != q.RestoreEpoch {
		return value, fail(409, "RESTORE_EPOCH_CHANGED", "Original password change belongs to another restore epoch")
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth.password_changes(user_id,request_id,device_id,restore_epoch,request_digest,operation_id,
expected_generation,expected_security_version,new_generation,new_security_version,replacement_hash_ciphertext,state)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'prepared')`, value.User, value.Request, value.Device, value.Epoch, value.Digest, value.Operation, value.ExpectedGeneration, value.ExpectedSecurity, value.Generation, value.Security, value.Ciphertext)
	if err != nil {
		return value, err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.users SET pending_operation_id=$2 WHERE id=$1`, value.User, value.Operation); err != nil {
		return value, err
	}
	return value, tx.Commit(ctx)
}
