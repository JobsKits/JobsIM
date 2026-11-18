package jobsim

import (
	"context"
	"net/http"
)

func init() {
	migrations["identity"] += `ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS export_pending_operation_id text;`
	migrations["conversation"] += `ALTER TABLE conversation.members ADD COLUMN IF NOT EXISTS export_pending_operation_id text;`
}
func (a *App) exportIdentityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.mediaUploadIdentityRoutes(h, internal)
	a.forwardIdentityRoutes(h, internal)
	a.forwardPendingIdentityRoutes(h, internal)
	h("POST /internal/exports/principal", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID   string `json:"userId"`
			DeviceID string `json:"deviceId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		epoch, e := a.currentIdentityEpoch(r.Context())
		if e != nil {
			return e
		}
		var security int64
		e = a.DB.QueryRow(r.Context(), `SELECT u.security_version FROM auth.users u JOIN auth.instance i ON i.singleton WHERE u.id=$1 AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL AND EXISTS(SELECT 1 FROM auth.sessions s WHERE s.user_id=u.id AND s.device_id=$2 AND s.family_id IS NOT NULL AND s.source_context_id='personal' AND s.source_generation=i.personal_source_generation AND s.revoked_at IS NULL AND s.export_pending_operation_id IS NULL AND s.expires_at>now() AND NOT s.web_pending AND s.security_version=u.security_version AND s.generation=u.credential_generation AND s.restore_epoch=$3)`, q.UserID, q.DeviceID, epoch).Scan(&security)
		if e != nil {
			return fail(403, "EXPORT_PRINCIPAL_INVALID", "Current source principal and original device required")
		}
		respond(w, 200, map[string]string{"securityVersion": str(security), "restoreEpoch": str(epoch)})
		return nil
	}))
}

func (a *App) invalidateUserExports(ctx context.Context, user, operation string) error {
	return a.rpc(ctx, "governance", "/internal/exports/invalidate", map[string]string{"userId": user, "operationId": operation}, nil)
}

// Invoke after the account owner's pending-operation row is committed and before
// the safety mutation completes. The pending row prevents new export admission.
func (a *App) freezeUserExports(ctx context.Context, user, operation string) error {
	var pending *string
	if e := a.DB.QueryRow(ctx, `SELECT pending_operation_id FROM auth.users WHERE id=$1`, user).Scan(&pending); e != nil {
		return e
	}
	if pending == nil {
		return fail(409, "PRINCIPAL_OPERATION_REQUIRED", "Durable original principal operation required")
	}
	q := forwardRevocation{OperationID: "principal:" + user + ":" + *pending, ForwarderID: user}
	if e := a.rpc(ctx, "message", "/internal/forwards/revoke-principal", q, nil); e != nil {
		return e
	}
	return a.invalidateUserExports(ctx, user, operation)
}
func (a *App) freezeDeviceExports(ctx context.Context, user, device, operation string) error {
	_, e := a.DB.Exec(ctx, `UPDATE auth.sessions SET export_pending_operation_id=$3 WHERE user_id=$1 AND device_id=$2 AND(export_pending_operation_id IS NULL OR export_pending_operation_id=$3)`, user, device, operation)
	if e != nil {
		return e
	}
	return a.rpc(ctx, "governance", "/internal/exports/invalidate", map[string]string{"userId": user, "deviceId": device, "operationId": operation}, nil)
}

func (a *App) freezeMembershipExports(ctx context.Context, conversation, user, actor string) (string, error) {
	return a.freezeMembershipExportsAt(ctx, conversation, user, actor, 0)
}

func (a *App) freezeMembershipExportsAt(ctx context.Context, conversation, user, actor string, expectedGeneration int64) (string, error) {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	var owner, kind string
	if e = tx.QueryRow(ctx, `SELECT owner_id::text,kind FROM conversation.conversations WHERE id=$1 FOR UPDATE`, conversation).Scan(&owner, &kind); e != nil {
		return "", e
	}
	if kind != "group" || user == owner || (actor != owner && actor != user) {
		return "", fail(403, "MEMBER_CHANGE_FORBIDDEN", "Current removal permission required")
	}
	var generation int64
	var state string
	var pending *string
	if e = tx.QueryRow(ctx, `SELECT membership_generation,state,export_pending_operation_id FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 FOR UPDATE`, conversation, user).Scan(&generation, &state, &pending); e != nil {
		return "", e
	}
	operation := "leave:" + conversation + ":" + user + ":" + str(generation)
	if pending != nil && *pending != operation {
		return "", fail(409, "MEMBERSHIP_OPERATION_PENDING", "Another membership operation owns the source fence")
	}
	if expectedGeneration > 0 && generation != expectedGeneration {
		return "", fail(409, "MEMBERSHIP_GENERATION_CHANGED", "Reload the current membership before removal")
	}
	if state != "active" && state != "left" {
		return "", fail(409, "MEMBERSHIP_CHANGED", "Membership is not settled")
	}
	if _, e = tx.Exec(ctx, `UPDATE conversation.members SET export_pending_operation_id=$3 WHERE conversation_id=$1 AND user_id=$2`, conversation, user, operation); e != nil {
		return "", e
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	q := forwardRevocation{OperationID: operation, ForwarderID: user, SourceConversationID: conversation, MembershipGeneration: str(generation)}
	if e = a.rpc(ctx, "message", "/internal/forwards/revoke-membership", q, nil); e != nil {
		return operation, e
	}
	if e = a.invalidateUserExports(ctx, user, operation); e != nil {
		return operation, e
	}
	return operation, nil
}
