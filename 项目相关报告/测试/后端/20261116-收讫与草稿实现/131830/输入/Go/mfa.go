package jobsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func canonicalAdminPayload(raw []byte) ([]byte, error) {
	var object map[string]json.RawMessage
	if e := json.Unmarshal(raw, &object); e != nil {
		return nil, fail(400, "INVALID_ADMIN_PAYLOAD", "Operation payload must be a JSON object")
	}
	if object == nil {
		return nil, fail(400, "INVALID_ADMIN_PAYLOAD", "Operation payload must be a JSON object")
	}
	delete(object, "stepUpId")
	delete(object, "requestId")
	normalized := map[string]any{}
	for key, value := range object {
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var v any
		if e := decoder.Decode(&v); e != nil {
			return nil, e
		}
		normalized[key] = v
	}
	return json.Marshal(normalized)
}
func totpCode(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	h := hmac.New(sha1.New, secret)
	h.Write(counter[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000)
}
func (a *App) consumeMFA(ctx context.Context, user, code string) error {
	if len(code) != 6 {
		return fail(403, "MFA_REQUIRED", "A current six-digit administrator authenticator code is required")
	}
	if e := a.rate(ctx, "mfa:"+user, 10); e != nil {
		return e
	}
	var factor string
	var sealed []byte
	var last int64
	e := a.DB.QueryRow(ctx, `SELECT id::text,secret_ciphertext,last_accepted_step FROM auth.mfa_factors WHERE user_id=$1 AND revoked_at IS NULL`, user).Scan(&factor, &sealed, &last)
	if e != nil {
		return fail(403, "MFA_NOT_ENROLLED", "Administrator MFA enrollment is required")
	}
	secretText, e := a.unseal(sealed, "mfa:"+factor+":"+user)
	if e != nil {
		return e
	}
	secret, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretText)
	if e != nil {
		return e
	}
	now := time.Now().Unix() / 30
	accepted := int64(-1)
	for _, step := range []int64{now, now - 1, now + 1} {
		if hmac.Equal([]byte(totpCode(secret, step)), []byte(code)) {
			accepted = step
			break
		}
	}
	if accepted < 0 {
		return fail(403, "MFA_INVALID", "Authenticator code rejected")
	}
	if accepted <= last {
		return fail(403, "MFA_REPLAY", "Code time step already consumed; wait for a new code")
	}
	payload := map[string]any{"userId": user, "factorId": factor, "step": str(accepted)}
	operation := "mfa-step:" + factor + ":" + str(accepted)
	if e = a.protect(ctx, operation, "intent", "auth.mfa_step", payload); e != nil {
		return e
	}
	result, e := a.DB.Exec(ctx, `UPDATE auth.mfa_factors SET last_accepted_step=$2 WHERE id=$1 AND last_accepted_step<$2 AND revoked_at IS NULL`, factor, accepted)
	if e != nil {
		return e
	}
	if result.RowsAffected() != 1 {
		return fail(403, "MFA_REPLAY", "Concurrent code consumption rejected")
	}
	return a.protect(ctx, operation, "result", "auth.mfa_step", payload)
}

type stepUpCheck struct {
	AuthorizationID string          `json:"authorizationId"`
	UserID          string          `json:"userId"`
	DeviceID        string          `json:"deviceId"`
	Action          string          `json:"action"`
	TargetID        string          `json:"targetId"`
	RequestID       string          `json:"requestId"`
	Payload         json.RawMessage `json:"payload"`
}

func (a *App) checkStepUpTx(ctx context.Context, tx pgx.Tx, q stepUpCheck, currentEpoch int64, consume bool) error {
	canonical, e := canonicalAdminPayload(q.Payload)
	if e != nil {
		return e
	}
	digest := keyedDigest(a.Config.Key, "admin-operation", canonical)
	var actor, device, action, target, request string
	var original []byte
	var generation, epoch int64
	var expires time.Time
	var consumed *time.Time
	e = tx.QueryRow(ctx, `SELECT user_id::text,device_id::text,action,target_id,request_id,request_digest,security_version,restore_epoch,expires_at,consumed_at FROM auth.step_up_authorizations WHERE id=$1 FOR UPDATE`, q.AuthorizationID).Scan(&actor, &device, &action, &target, &request, &original, &generation, &epoch, &expires, &consumed)
	if e != nil {
		return fail(403, "STEP_UP_REQUIRED", "Bound authorization required")
	}
	var currentGeneration int64
	var currentAdmin, activeSession bool
	e = tx.QueryRow(ctx, `SELECT security_version,admin,EXISTS(SELECT 1 FROM auth.sessions s WHERE s.user_id=auth.users.id AND s.device_id=$2 AND s.revoked_at IS NULL AND s.expires_at>now() AND s.restore_epoch=$3 AND s.security_version=auth.users.security_version AND s.generation=auth.users.credential_generation) FROM auth.users WHERE id=$1 AND state='active' AND recovery_state='normal'`, actor, device, epoch).Scan(&currentGeneration, &currentAdmin, &activeSession)
	validOriginalConsumption := consumed != nil && !consumed.After(expires)
	if e != nil || !currentAdmin || !activeSession || currentGeneration != generation || time.Now().After(expires) && !validOriginalConsumption || actor != q.UserID || device != q.DeviceID || action != q.Action || target != q.TargetID || request != q.RequestID || !bytes.Equal(digest, original) {
		return fail(403, "STEP_UP_MISMATCH", "Authorization expired, revoked or bound to another operation")
	}
	if currentEpoch != epoch {
		return fail(403, "STEP_UP_EPOCH", "Authorization belongs to an older restore epoch")
	}
	if consume {
		_, e = tx.Exec(ctx, `UPDATE auth.step_up_authorizations SET consumed_at=COALESCE(consumed_at,now()) WHERE id=$1`, q.AuthorizationID)
	}
	return e
}
func (a *App) mfaRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /api/auth/step-up", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !u.Admin {
			return fail(403, "ADMIN_REQUIRED", "Administrator required")
		}
		var q struct {
			Action    string          `json:"action"`
			TargetID  string          `json:"targetId"`
			RequestID string          `json:"requestId"`
			OTP       string          `json:"otp"`
			Payload   json.RawMessage `json:"payload"`
		}
		if e = decode(r, &q); e != nil {
			return e
		}
		actions := map[string]bool{"capability.update": true, "account.state": true, "report.resolve": true, "account.reset_grant": true}
		if !actions[q.Action] || q.TargetID == "" || len(q.RequestID) < 8 || len(q.RequestID) > 128 {
			return fail(400, "INVALID_STEP_UP_TARGET", "Known action, target and stable requestId required")
		}
		canonical, e := canonicalAdminPayload(q.Payload)
		if e != nil {
			return e
		}
		digest := keyedDigest(a.Config.Key, "admin-operation", canonical)
		var original []byte
		var oldID string
		var expires time.Time
		e = a.DB.QueryRow(r.Context(), `SELECT id::text,request_digest,expires_at FROM auth.step_up_authorizations WHERE user_id=$1 AND request_id=$2 AND action=$3 AND target_id=$4`, u.ID, q.RequestID, q.Action, q.TargetID).Scan(&oldID, &original, &expires)
		if e == nil {
			if !bytes.Equal(original, digest) || time.Now().After(expires) {
				return fail(409, "STEP_UP_CONFLICT", "Original authorization differs or expired")
			}
			respond(w, 200, map[string]any{"authorizationId": oldID, "expiresAt": expires})
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if e = a.consumeMFA(r.Context(), u.ID, q.OTP); e != nil {
			return e
		}
		var security int64
		if e = a.DB.QueryRow(r.Context(), `SELECT security_version FROM auth.users WHERE id=$1 AND state='active'`, u.ID).Scan(&security); e != nil {
			return e
		}
		epoch, e := strconv.ParseInt(u.RestoreEpoch, 10, 64)
		if e != nil {
			return e
		}
		id := newID()
		expires = time.Now().UTC().Add(120 * time.Second)
		_, e = a.DB.Exec(r.Context(), `INSERT INTO auth.step_up_authorizations(id,user_id,device_id,action,target_id,request_id,request_digest,security_version,restore_epoch,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, u.ID, u.DeviceID, q.Action, q.TargetID, q.RequestID, digest, security, epoch, expires)
		if e != nil {
			return fail(409, "STEP_UP_RACE", "Query the original operation")
		}
		respond(w, 200, map[string]any{"authorizationId": id, "expiresAt": expires})
		return nil
	})
	for _, path := range []string{"/internal/step-up/check", "/internal/step-up/consume"} {
		endpoint := path
		h("POST "+endpoint, internal(func(w http.ResponseWriter, r *http.Request) error {
			var q stepUpCheck
			if e := decode(r, &q); e != nil {
				return e
			}
			currentEpoch, e := a.epoch(r.Context())
			if e != nil {
				return e
			}
			tx, e := a.DB.Begin(r.Context())
			if e != nil {
				return e
			}
			defer tx.Rollback(r.Context())
			if e = a.checkStepUpTx(r.Context(), tx, q, currentEpoch, endpoint == "/internal/step-up/consume"); e != nil {
				return e
			}
			if e = tx.Commit(r.Context()); e != nil {
				return e
			}
			respond(w, 200, map[string]bool{"authorized": true})
			return nil
		}))
	}
}
func (a *App) adminStepUp(r *http.Request, u User, authorization, action, target, request string, raw []byte) error {
	if !u.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	return a.rpc(r.Context(), "identity", "/internal/step-up/consume", stepUpCheck{authorization, u.ID, u.DeviceID, action, target, request, raw}, nil)
}
func readAdminBody(r *http.Request, target any) ([]byte, error) {
	raw, e := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if e != nil || len(raw) > 1<<20 {
		return nil, fail(413, "BODY_LIMIT", "Administrative request too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(target); e != nil {
		return nil, fail(400, "INVALID_JSON", "Invalid administrative request")
	}
	return raw, nil
}
func BootstrapAdministrator(ctx context.Context, c Config, username, password, secretFile, operation string) error {
	name, e := normalizeUsername(username)
	if e != nil {
		return e
	}
	password, e = normalizePassword(password)
	if e != nil {
		return e
	}
	if secretFile == "" || operation == "" {
		return fmt.Errorf("private TOTP enrollment file and stable operation id required")
	}
	text, e := os.ReadFile(secretFile)
	if os.IsNotExist(e) {
		if e = os.MkdirAll(filepath.Dir(secretFile), 0700); e != nil {
			return e
		}
		secret := make([]byte, 20)
		if _, e = rand.Read(secret); e != nil {
			return e
		}
		text = []byte(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret))
		f, e := os.OpenFile(secretFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(text)
		if e == nil {
			e = f.Sync()
		}
		f.Close()
		if e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	info, e := os.Stat(secretFile)
	if e != nil || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("TOTP enrollment file must be private mode 600")
	}
	secret, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimSpace(string(text)))
	if e != nil || len(secret) < 20 {
		return fmt.Errorf("invalid TOTP enrollment material")
	}
	app, e := NewApp(ctx, c, "identity")
	if e != nil {
		return e
	}
	defer app.Close()
	digest := keyedDigest(c.Key, "bootstrap-admin", []byte(name+"\x00"+password+"\x00"+string(text)))
	tx, e := app.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('jobsim-admin-bootstrap'))`); e != nil {
		return e
	}
	var priorActor, priorState string
	var priorDigest []byte
	e = tx.QueryRow(ctx, `SELECT actor_id::text,state,request_digest FROM auth.admin_operations WHERE request_id=$1 AND action='bootstrap' FOR UPDATE`, operation).Scan(&priorActor, &priorState, &priorDigest)
	if e == nil {
		if !hmac.Equal(priorDigest, digest) {
			return fmt.Errorf("bootstrap operation content differs from its original request")
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		if priorState == "completed" {
			return nil
		}
		if e = app.completeRegistration(ctx, priorActor, name, "bootstrap:"+operation); e != nil {
			return e
		}
		_, e = app.DB.Exec(ctx, `UPDATE auth.admin_operations SET state='completed',result='{"status":"initialized"}' WHERE request_id=$1 AND actor_id=$2 AND action='bootstrap'`, operation, priorActor)
		return e
	}
	if e != pgx.ErrNoRows {
		return e
	}
	var adminCount int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM auth.users WHERE admin`).Scan(&adminCount); e != nil {
		return e
	}
	if adminCount != 0 {
		return fmt.Errorf("administrator already initialized; existing identities are never promoted or overwritten")
	}
	user, factor := newID(), newID()
	sealed, e := app.seal(strings.TrimSpace(string(text)), "mfa:"+factor+":"+user)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO auth.users(id,username,display_name,password_hash,admin,state) VALUES($1,$2,$2,$3,true,'pending')`, user, name, hashPassword(password))
	if e != nil {
		return fmt.Errorf("bootstrap username already reserved")
	}
	_, e = tx.Exec(ctx, `INSERT INTO auth.mfa_factors(id,user_id,secret_ciphertext,last_accepted_step) VALUES($1,$2,$3,-1)`, factor, user, sealed)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO auth.admin_operations(request_id,actor_id,target_id,action,request_digest,state,result) VALUES($1,$2,$2,'bootstrap',$3,'prepared','{}')`, operation, user, digest)
	if e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	if e = app.completeRegistration(ctx, user, name, "bootstrap:"+operation); e != nil {
		return e
	}
	_, e = app.DB.Exec(ctx, `UPDATE auth.admin_operations SET state='completed',result='{"status":"initialized"}' WHERE request_id=$1`, operation)
	return e
}

func (a *App) previewAdminStepUp(r *http.Request, u User, authorization, action, target, request string, raw []byte) error {
	if !u.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	return a.rpc(r.Context(), "identity", "/internal/step-up/check", stepUpCheck{authorization, u.ID, u.DeviceID, action, target, request, raw}, nil)
}
