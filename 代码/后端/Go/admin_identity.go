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

type AdminUser struct {
	ID                   string     `json:"id"`
	Username             string     `json:"username"`
	DisplayName          string     `json:"displayName"`
	State                string     `json:"state"`
	Version              string     `json:"version"`
	SecurityVersion      string     `json:"securityVersion"`
	Admin                bool       `json:"admin"`
	RecoveryState        string     `json:"recoveryState"`
	StateBeforeTombstone *string    `json:"stateBeforeTombstone"`
	BanUntil             *time.Time `json:"banUntil"`
	DeletionState        string     `json:"deletionState"`
}

func (a *App) adminIdentityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/admin/users", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !u.Admin {
			return fail(403, "ADMIN_REQUIRED", "Administrator required")
		}
		rows, e := a.DB.Query(r.Context(), `SELECT id::text,username,display_name,state,row_version,security_version,admin,recovery_state,state_before_tombstone,ban_until,deletion_state FROM auth.users WHERE strpos(username,$1)>0 ORDER BY username LIMIT 100`, strings.ToLower(r.URL.Query().Get("q")))
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []AdminUser{}
		for rows.Next() {
			var v AdminUser
			var version, security int64
			if e = rows.Scan(&v.ID, &v.Username, &v.DisplayName, &v.State, &version, &security, &v.Admin, &v.RecoveryState, &v.StateBeforeTombstone, &v.BanUntil, &v.DeletionState); e != nil {
				return e
			}
			v.Version = str(version)
			v.SecurityVersion = str(security)
			items = append(items, v)
		}
		respond(w, 200, map[string]any{"items": items})
		return rows.Err()
	})
	h("PATCH /api/admin/users/{id}/state", a.adminAccountState)
	h("POST /api/admin/users/{id}/reset-grants", a.adminResetGrant)
	h("GET /api/admin/operations/{id}", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !u.Admin {
			return fail(403, "ADMIN_REQUIRED", "Administrator required")
		}
		var state string
		var result []byte
		e = a.DB.QueryRow(r.Context(), `SELECT state,result FROM auth.admin_operations WHERE request_id=$1 AND actor_id=$2`, r.PathValue("id"), u.ID).Scan(&state, &result)
		if e != nil {
			return fail(404, "OPERATION_NOT_FOUND", "Operation unavailable")
		}
		var body any
		json.Unmarshal(result, &body)
		respond(w, 200, map[string]any{"state": state, "result": body})
		return nil
	})
}
func (a *App) adminAccountState(w http.ResponseWriter, r *http.Request) error {
	actor, e := a.current(r)
	if e != nil {
		return e
	}
	if !actor.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	target := r.PathValue("id")
	if target == actor.ID {
		return fail(409, "SELF_ADMIN_STATE", "Self-disable is not allowed")
	}
	var q struct {
		State           string     `json:"state"`
		Reason          string     `json:"reason"`
		ExpectedVersion string     `json:"expectedVersion"`
		RequestID       string     `json:"requestId"`
		StepUpID        string     `json:"stepUpId"`
		Permanent       bool       `json:"permanent,omitempty"`
		BanUntil        *time.Time `json:"banUntil,omitempty"`
	}
	raw, e := readAdminBody(r, &q)
	if e != nil {
		return e
	}
	if (q.State != "active" && q.State != "banned" && q.State != "tombstoned") || len(strings.TrimSpace(q.Reason)) == 0 || len(q.Reason) > 1000 || len(q.RequestID) < 8 || len(q.RequestID) > 128 || q.ExpectedVersion == "" {
		return fail(400, "INVALID_ACCOUNT_OPERATION", "State, reason, version and requestId required")
	}
	if q.State == "banned" && ((q.Permanent && q.BanUntil != nil) || (!q.Permanent && (q.BanUntil == nil || !q.BanUntil.After(time.Now())))) {
		return fail(400, "BAN_DURATION_REQUIRED", "Choose a future banUntil or explicit permanent=true")
	}
	canonical, e := canonicalAdminPayload(raw)
	if e != nil {
		return e
	}
	digest := keyedDigest(a.Config.Key, "account-state:"+actor.ID+":"+target, canonical)
	var oldDigest, saved, payloadRaw []byte
	var oldActor, oldTarget, operationState string
	e = a.DB.QueryRow(r.Context(), `SELECT actor_id::text,target_id::text,request_digest,state,result,journal_payload FROM auth.admin_operations WHERE request_id=$1`, q.RequestID).Scan(&oldActor, &oldTarget, &oldDigest, &operationState, &saved, &payloadRaw)
	prepared := e == nil
	if prepared {
		if oldActor != actor.ID || oldTarget != target || !bytes.Equal(oldDigest, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original account operation differs")
		}
		if operationState == "completed" {
			var payload map[string]any
			json.Unmarshal(payloadRaw, &payload)
			if e = a.protect(r.Context(), "account-state:"+q.RequestID, "result", "auth.account_state", payload); e != nil {
				return e
			}
			var result any
			json.Unmarshal(saved, &result)
			respond(w, 200, result)
			return nil
		}
	} else if e != pgx.ErrNoRows {
		return e
	}
	if !prepared {
		epoch, e := strconv.ParseInt(actor.RestoreEpoch, 10, 64)
		if e != nil {
			return e
		}
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			return e
		}
		defer tx.Rollback(r.Context())
		// Actor/target are locked in stable order before consuming the bound proof.
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
		var state, recovery, deletion string
		var before, pending *string
		var version, security int64
		var ban *time.Time
		e = tx.QueryRow(r.Context(), `SELECT state,recovery_state,deletion_state,state_before_tombstone,row_version,security_version,ban_until,pending_operation_id FROM auth.users WHERE id=$1`, target).Scan(&state, &recovery, &deletion, &before, &version, &security, &ban, &pending)
		if e != nil {
			return fail(404, "USER_NOT_FOUND", "Target unavailable")
		}
		if pending != nil {
			return fail(409, "OPERATION_PENDING", "Target already has a pending safety operation")
		}
		if deletion == "processing" || deletion == "completed" {
			return fail(409, "DELETION_IRREVERSIBLE", "Physical deletion cannot be reversed")
		}
		if str(version) != q.ExpectedVersion {
			return fail(409, "VERSION_CONFLICT", "Reload the current account")
		}
		if state != "active" && state != "banned" && state != "tombstoned" {
			return fail(409, "ACCOUNT_NOT_SETTLED", "Account is not in a recoverable settled state")
		}
		desired := q.State
		nextBefore := before
		if state == "tombstoned" {
			if q.State != "active" || before == nil {
				return fail(409, "RESTORE_REQUIRED", "Use explicit restore before changing a tombstoned account")
			}
			desired = *before
			if desired != "active" && desired != "banned" {
				return fail(409, "ORIGINAL_STATE_UNVERIFIED", "Original punishment cannot be reconstructed")
			}
		}
		if q.State == "tombstoned" {
			copy := state
			nextBefore = &copy
		}
		if q.State == "banned" {
			ban = q.BanUntil
		}
		if q.State == "active" && state == "banned" {
			ban = nil
		}
		banText := ""
		if ban != nil {
			banText = ban.UTC().Format(time.RFC3339Nano)
		}
		beforeText := ""
		if nextBefore != nil {
			beforeText = *nextBefore
		}
		payload := map[string]any{"userId": target, "state": desired, "stateBefore": beforeText, "banUntil": banText, "securityVersion": str(security + 1), "version": str(version + 1)}
		payloadRaw, _ = json.Marshal(payload)
		if e = a.checkStepUpTx(r.Context(), tx, stepUpCheck{q.StepUpID, actor.ID, actor.DeviceID, "account.state", target, q.RequestID, raw}, epoch, true); e != nil {
			return e
		}
		preparedResult, _ := json.Marshal(map[string]any{"stateBefore": state, "stateAfter": desired, "reason": q.Reason, "actorId": actor.ID, "requestId": q.RequestID, "recoveryState": recovery})
		_, e = tx.Exec(r.Context(), `INSERT INTO auth.admin_operations(request_id,actor_id,target_id,action,request_digest,state,result,journal_payload,request_metadata) VALUES($1,$2,$3,'account.state',$4,'prepared',$5,$6,$5)`, q.RequestID, actor.ID, target, digest, preparedResult, payloadRaw)
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
	if e = a.protect(r.Context(), "account-state:"+q.RequestID, "intent", "auth.account_state", payload); e != nil {
		return e
	}
	if e = a.freezeUserExports(r.Context(), target, "account-state:"+q.RequestID); e != nil {
		return e
	}
	if payload["state"] == "tombstoned" {
		if e = a.rpc(r.Context(), "conversation", "/internal/owner-unavailable", map[string]string{"userId": target, "operationId": q.RequestID}, nil); e != nil {
			return e
		}
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var pending *string
	if e = tx.QueryRow(r.Context(), `SELECT pending_operation_id FROM auth.users WHERE id=$1 FOR UPDATE`, target).Scan(&pending); e != nil {
		return e
	}
	if pending == nil || *pending != q.RequestID {
		return fail(409, "OPERATION_CHANGED", "Original account operation no longer owns the target")
	}
	state, _ := payload["state"].(string)
	before, _ := payload["stateBefore"].(string)
	banText, _ := payload["banUntil"].(string)
	version, _ := payload["version"].(string)
	security, _ := payload["securityVersion"].(string)
	_, e = tx.Exec(r.Context(), `UPDATE auth.users SET state=$2,state_before_tombstone=NULLIF($3,''),ban_until=NULLIF($4,'')::timestamptz,row_version=$5,security_version=$6,pending_operation_id=NULL WHERE id=$1`, target, state, before, banText, version, security)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=now() WHERE user_id=$1`, target)
	if e != nil {
		return e
	}
	result := map[string]any{"id": target, "state": state, "version": version, "securityVersion": security, "requestId": q.RequestID}
	saved, _ = json.Marshal(result)
	_, e = tx.Exec(r.Context(), `UPDATE auth.admin_operations SET state='completed',result=$2 WHERE request_id=$1`, q.RequestID, saved)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if e = a.protect(r.Context(), "account-state:"+q.RequestID, "result", "auth.account_state", payload); e != nil {
		return e
	}
	respond(w, 200, result)
	return nil
}
