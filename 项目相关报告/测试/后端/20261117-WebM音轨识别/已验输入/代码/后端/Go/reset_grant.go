package jobsim

import (
	"bytes"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func init() {
	migrations["identity"] += `
ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS recovery_state text NOT NULL DEFAULT 'normal';
UPDATE auth.users SET recovery_state='recovery_required' WHERE recovery_state='reset_required';
CREATE TABLE IF NOT EXISTS auth.reset_grants(id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES auth.users(id),purpose text NOT NULL CHECK(purpose='password_reset'),credential_digest bytea NOT NULL UNIQUE,issued_security_version bigint NOT NULL,restore_epoch bigint NOT NULL,expires_at timestamptz NOT NULL,state text NOT NULL,consumed_at timestamptz,revoked_at timestamptz,issue_request_id text NOT NULL UNIQUE);
`
}

func (a *App) adminResetGrant(w http.ResponseWriter, r *http.Request) error {
	actor, e := a.current(r)
	if e != nil {
		return e
	}
	if !actor.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	target := r.PathValue("id")
	if target == actor.ID {
		return fail(409, "SELF_RESET_GRANT", "Use a different controlled administrator for an administrative reset")
	}
	var q struct {
		Reason          string `json:"reason"`
		ExpectedVersion string `json:"expectedVersion"`
		RequestID       string `json:"requestId"`
		StepUpID        string `json:"stepUpId"`
	}
	raw, e := readAdminBody(r, &q)
	if e != nil {
		return e
	}
	if strings.TrimSpace(q.Reason) == "" || len(q.Reason) > 1000 || len(q.RequestID) < 8 || len(q.RequestID) > 128 || q.ExpectedVersion == "" {
		return fail(400, "INVALID_RESET_OPERATION", "Reason, expectedVersion and stable requestId required")
	}
	canonical, e := canonicalAdminPayload(raw)
	if e != nil {
		return e
	}
	digest := keyedDigest(a.Config.Key, "reset-grant:"+actor.ID+":"+target, canonical)
	var oldActor, oldTarget, state string
	var previous, result, payloadRaw []byte
	e = a.DB.QueryRow(r.Context(), `SELECT actor_id::text,target_id::text,request_digest,state,result,journal_payload FROM auth.admin_operations WHERE request_id=$1 AND action='account.reset_grant'`, q.RequestID).Scan(&oldActor, &oldTarget, &previous, &state, &result, &payloadRaw)
	exists := e == nil
	if exists {
		if oldActor != actor.ID || oldTarget != target || !bytes.Equal(digest, previous) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original reset request differs")
		}
		if state == "completed" {
			var payload map[string]any
			if e = json.Unmarshal(payloadRaw, &payload); e != nil {
				return e
			}
			if e = a.protect(r.Context(), "reset-grant:"+q.RequestID, "result", "auth.invalidate_user", payload); e != nil {
				return e
			}
			var body map[string]any
			if e = json.Unmarshal(result, &body); e != nil {
				return e
			}
			body["credentialAvailable"] = false
			respond(w, 200, body)
			return nil
		}
	} else if e != pgx.ErrNoRows {
		return e
	}
	credential := ""
	if !exists {
		epoch, e := strconv.ParseInt(actor.RestoreEpoch, 10, 64)
		if e != nil {
			return e
		}
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			return e
		}
		defer tx.Rollback(r.Context())
		rows, e := tx.Query(r.Context(), `SELECT id FROM auth.users WHERE id=ANY($1::uuid[]) ORDER BY id FOR UPDATE`, []string{actor.ID, target})
		if e != nil {
			return e
		}
		for rows.Next() {
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		var version, generation, security int64
		var pending *string
		var targetState, deletion string
		e = tx.QueryRow(r.Context(), `SELECT row_version,credential_generation,security_version,pending_operation_id,state,deletion_state FROM auth.users WHERE id=$1`, target).Scan(&version, &generation, &security, &pending, &targetState, &deletion)
		if e != nil {
			return fail(404, "USER_NOT_FOUND", "Target unavailable")
		}
		if (targetState != "active" && targetState != "banned") || deletion != "none" {
			return fail(409, "ACCOUNT_NOT_SETTLED", "Restore the account identity before resetting its password")
		}
		if pending != nil || str(version) != q.ExpectedVersion {
			return fail(409, "VERSION_CONFLICT", "Refresh the settled current account")
		}
		if e = a.checkStepUpTx(r.Context(), tx, stepUpCheck{q.StepUpID, actor.ID, actor.DeviceID, "account.reset_grant", target, q.RequestID, raw}, epoch, true); e != nil {
			return e
		}
		id := newID()
		credential = randomToken(32)
		expires := time.Now().UTC().Add(600 * time.Second)
		_, e = tx.Exec(r.Context(), `INSERT INTO auth.reset_grants(id,user_id,purpose,credential_digest,issued_security_version,restore_epoch,expires_at,state,issue_request_id) VALUES($1,$2,'password_reset',$3,$4,$5,$6,'prepared',$7)`, id, target, keyedDigest(a.Config.Key, "password-reset-grant", []byte(credential)), security+1, epoch, expires, q.RequestID)
		if e != nil {
			return e
		}
		payloadRaw, _ = json.Marshal(map[string]any{"userId": target, "generation": str(generation + 1), "securityVersion": str(security + 1)})
		result, _ = json.Marshal(map[string]any{"id": id, "userId": target, "expiresAt": expires, "status": "issued", "version": str(version + 1), "requestId": q.RequestID, "credentialAvailable": false})
		metadata, _ := json.Marshal(map[string]any{"reason": q.Reason, "actorId": actor.ID, "purpose": "password_reset", "priorState": targetState, "doesNotResetMFA": true})
		_, e = tx.Exec(r.Context(), `INSERT INTO auth.admin_operations(request_id,actor_id,target_id,action,request_digest,state,result,journal_payload,request_metadata) VALUES($1,$2,$3,'account.reset_grant',$4,'prepared',$5,$6,$7)`, q.RequestID, actor.ID, target, digest, result, payloadRaw, metadata)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE auth.users SET pending_operation_id=$2 WHERE id=$1`, target, q.RequestID)
		if e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
	}
	var payload map[string]any
	if e = json.Unmarshal(payloadRaw, &payload); e != nil {
		return e
	}
	var owns bool
	if e = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.users WHERE id=$1 AND state IN('active','banned') AND deletion_state='none' AND pending_operation_id=$2)`, target, q.RequestID).Scan(&owns); e != nil {
		return e
	}
	if !owns {
		return fail(409, "RESET_GRANT_CONTEXT_CHANGED", "Original reset no longer owns an account eligible for ordinary credential changes")
	}
	if e = a.protect(r.Context(), "reset-grant:"+q.RequestID, "intent", "auth.invalidate_user", payload); e != nil {
		return e
	}
	if e = a.freezeUserExports(r.Context(), target, "reset-grant:"+q.RequestID); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var pending *string
	var targetState, deletion string
	if e = tx.QueryRow(r.Context(), `SELECT pending_operation_id,state,deletion_state FROM auth.users WHERE id=$1 FOR UPDATE`, target).Scan(&pending, &targetState, &deletion); e != nil {
		return e
	}
	if pending == nil || *pending != q.RequestID || deletion != "none" || (targetState != "active" && targetState != "banned") {
		return fail(409, "RESET_SUPERSEDED", "Original reset no longer owns the target")
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.reset_grants SET revoked_at=now(),state='revoked' WHERE user_id=$1 AND issue_request_id<>$2 AND state='active'`, target, q.RequestID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.reset_grants SET state='active' WHERE user_id=$1 AND issue_request_id=$2 AND state='prepared'`, target, q.RequestID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.users SET credential_generation=$2,security_version=$3,recovery_state='recovery_required',row_version=row_version+1,pending_operation_id=NULL WHERE id=$1`, target, payload["generation"], payload["securityVersion"])
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=now() WHERE user_id=$1`, target)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1`, target)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.admin_operations SET state='completed' WHERE request_id=$1`, q.RequestID)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if e = a.protect(r.Context(), "reset-grant:"+q.RequestID, "result", "auth.invalidate_user", payload); e != nil {
		return e
	}
	var body map[string]any
	if e = json.Unmarshal(result, &body); e != nil {
		return e
	}
	if credential != "" {
		body["credential"] = credential
		body["credentialAvailable"] = true
	}
	respond(w, 200, body)
	return nil
}
