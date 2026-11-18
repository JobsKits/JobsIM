package jobsim

import (
	"context"
	"math"
	"net/http"
)

func (a *App) finishDeletionRequest(ctx context.Context, original accountDeletion) error {
	d, err := scanIdentityDeletion(a.DB.QueryRow(ctx, `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE id=$1`, original.ID))
	if err != nil {
		return err
	}
	if d.State != "prepared" && d.State != "requested" {
		return nil
	}
	epoch, err := a.currentIdentityEpoch(ctx)
	if err != nil {
		return err
	}
	if d.State == "prepared" && d.Epoch != str(epoch) {
		_, err = a.DB.Exec(ctx, `UPDATE auth.account_deletions SET state='retired',updated_at=now() WHERE id=$1 AND state='prepared'`, d.ID)
		return err
	}
	var g accountDeletion
	if err = a.rpc(ctx, "governance", "/internal/deletion/prepare", d, &g); err != nil {
		return err
	}
	if g.ID != d.ID || g.User != d.User || g.Request != d.Request || g.Version != "1" || g.Until.IsZero() {
		return fail(503, "DELETION_BINDING_CHANGED", "Original governance responsibility differs")
	}
	d.Until = g.Until
	if _, err = a.DB.Exec(ctx, `UPDATE auth.account_deletions SET cancel_until=$2 WHERE id=$1 AND (cancel_until IS NULL OR cancel_until=$2)`, d.ID, d.Until); err != nil {
		return err
	}
	payload := a.deletionSafety(d, "requested")
	if err = a.protect(ctx, deletionOperation(d, "request"), "intent", "auth.deletion", payload); err != nil {
		return err
	}
	if d.State == "prepared" {
		if err = a.freezeUserExports(ctx, d.User, deletionOperation(d, "request")); err != nil {
			return err
		}
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		var security int64
		var state string
		var pending *string
		if err = tx.QueryRow(ctx, `SELECT security_version,deletion_state,pending_operation_id FROM auth.users WHERE id=$1 FOR UPDATE`, d.User).Scan(&security, &state, &pending); err != nil {
			return err
		}
		locked, err := scanIdentityDeletion(tx.QueryRow(ctx, `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE id=$1 FOR UPDATE`, d.ID))
		if err != nil {
			return err
		}
		if locked.State == "prepared" {
			if security != d.Expected || state != "none" || pending == nil || *pending != deletionOperation(d, "request") {
				return fail(409, "ACCOUNT_DELETION_CONTEXT_CHANGED", "Original deletion no longer owns the account")
			}
			if _, err = tx.Exec(ctx, `UPDATE auth.users SET deletion_state='requested',deletion_request_id=$2,deletion_version=1,security_version=$3,row_version=row_version+1,pending_operation_id=NULL WHERE id=$1`, d.User, d.ID, d.Security); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, d.User); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE auth.reset_grants SET revoked_at=COALESCE(revoked_at,now()),state='revoked' WHERE user_id=$1 AND state='active'`, d.User); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE auth.account_deletions SET state='requested',updated_at=now() WHERE id=$1`, d.ID); err != nil {
				return err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	if err = a.protect(ctx, deletionOperation(d, "request"), "result", "auth.deletion", payload); err != nil {
		return err
	}
	if err = a.drainPersonalExportInvalidations(ctx); err != nil {
		return err
	}
	if err = a.drainMigrations(ctx); err != nil {
		return err
	}
	return a.rpc(ctx, "governance", "/internal/deletion/activate-governance", d, nil)
}
func (a *App) activateDeletionInternal(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "governance" {
		return fail(403, "DELETION_OWNER_REQUIRED", "Governance qualification required")
	}
	var d accountDeletion
	if err := decodeReceiptRequest(r, &d); err != nil {
		return err
	}
	if !validObjectID(d.ID) {
		return fail(400, "INVALID_ACCOUNT_DELETION", "Original deletion required")
	}
	if err := a.finishDeletionRequest(r.Context(), d); err != nil {
		return err
	}
	var state string
	if err := a.DB.QueryRow(r.Context(), `SELECT state FROM auth.account_deletions WHERE id=$1`, d.ID).Scan(&state); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"confirmed": true, "status": state})
	return nil
}
func (a *App) finishDeletionCancellation(ctx context.Context, g accountDeletion) error {
	if g.State != "cancel_pending" && g.State != "cancelled" || !validReceiptNonce(g.Cancel) {
		return fail(409, "DELETION_CANCEL_NOT_QUALIFIED", "Original cancellation qualification required")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deletion string
	var currentID *string
	var security int64
	var pending *string
	if err = tx.QueryRow(ctx, `SELECT deletion_state,deletion_request_id::text,security_version,pending_operation_id FROM auth.users WHERE id=$1 FOR UPDATE`, g.User).Scan(&deletion, &currentID, &security, &pending); err != nil {
		return err
	}
	d, err := scanIdentityDeletion(tx.QueryRow(ctx, `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE id=$1 AND user_id=$2 FOR UPDATE`, g.ID, g.User))
	if err != nil {
		return err
	}
	op := deletionOperation(d, "cancel:"+g.Cancel)
	if d.State == "requested" {
		if deletion != "requested" || currentID == nil || *currentID != d.ID || pending != nil || security == math.MaxInt64 {
			return fail(409, "DELETION_CANCEL_CONTEXT_CHANGED", "Only the original requested deletion can be cancelled")
		}
		d.Cancel, d.Version, d.CancelSecurity = g.Cancel, g.Version, security+1
		if _, err = tx.Exec(ctx, `UPDATE auth.account_deletions SET state='cancel_pending',cancel_request_id=$2,request_version=$3,cancel_security=$4,updated_at=now() WHERE id=$1`, d.ID, d.Cancel, d.Version, d.CancelSecurity); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.users SET pending_operation_id=$2 WHERE id=$1`, d.User, op); err != nil {
			return err
		}
	} else if d.State != "cancel_pending" && d.State != "cancelled" || d.Cancel != g.Cancel || d.Version != g.Version {
		return fail(409, "DELETION_CANCEL_CONTEXT_CHANGED", "Original cancellation changed")
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	payload := a.deletionSafety(d, "cancelled")
	if err = a.protect(ctx, op, "intent", "auth.deletion", payload); err != nil {
		return err
	}
	if d.State != "cancelled" {
		if err = a.freezeUserExports(ctx, d.User, op); err != nil {
			return err
		}
		tx, err = a.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		changed, err := tx.Exec(ctx, `UPDATE auth.users SET deletion_state='none',deletion_request_id=NULL,deletion_version=$3,security_version=GREATEST(security_version,$4),row_version=row_version+1 WHERE id=$1 AND deletion_request_id=$2 AND deletion_state='requested' AND pending_operation_id=$5`, d.User, d.ID, d.Version, d.CancelSecurity, op)
		if err != nil {
			return err
		}
		if changed.RowsAffected() != 1 {
			return fail(409, "DELETION_CANCEL_CONTEXT_CHANGED", "Original requested identity was superseded")
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, d.User); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.account_deletions SET state='cancelled',updated_at=now() WHERE id=$1 AND state='cancel_pending'`, d.ID); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	if err = a.drainPersonalExportInvalidations(ctx); err != nil {
		return err
	}
	if err = a.drainMigrations(ctx); err != nil {
		return err
	}
	if err = a.protect(ctx, op, "result", "auth.deletion", payload); err != nil {
		return err
	}
	if err = a.rpc(ctx, "governance", "/internal/deletion/confirm-cancel", g, nil); err != nil {
		return err
	}
	_, err = a.DB.Exec(ctx, `UPDATE auth.users SET pending_operation_id=NULL WHERE id=$1 AND pending_operation_id=$2 AND deletion_state='none' AND deletion_version=$3`, d.User, op, d.Version)
	return err
}
func (a *App) cancelDeletionInternal(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "governance" {
		return fail(403, "DELETION_OWNER_REQUIRED", "Governance cancellation qualification required")
	}
	var d accountDeletion
	if err := decodeReceiptRequest(r, &d); err != nil {
		return err
	}
	if err := a.finishDeletionCancellation(r.Context(), d); err != nil {
		return err
	}
	respond(w, 200, map[string]bool{"confirmed": true})
	return nil
}
func (a *App) identityDeletionStageInternal(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "governance" {
		return fail(403, "DELETION_OWNER_REQUIRED", "Governance cleanup qualification required")
	}
	var g accountDeletion
	if err := decodeReceiptRequest(r, &g); err != nil {
		return err
	}
	if g.State != "processing" && g.State != "completed" {
		return fail(400, "INVALID_ACCOUNT_DELETION", "Qualified irreversible stage required")
	}
	var qualified accountDeletion
	if err := a.rpc(r.Context(), "governance", "/internal/deletion/qualify", g, &qualified); err != nil {
		return err
	}
	d, err := scanIdentityDeletion(a.DB.QueryRow(r.Context(), `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE id=$1 AND user_id=$2`, g.ID, g.User))
	if err != nil {
		return err
	}
	if qualified.State != "processing" || qualified.Version != g.Version || d.State == "cancelled" || d.State == "cancel_pending" {
		return fail(409, "DELETION_NOT_QUALIFIED", "Original cleanup stage differs")
	}
	d.Version = g.Version
	payload := a.deletionSafety(d, g.State)
	op := deletionOperation(d, g.State)
	if err = a.protect(r.Context(), op, "intent", "auth.deletion", payload); err != nil {
		return err
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	changed, err := tx.Exec(r.Context(), `UPDATE auth.users SET deletion_state=$3,deletion_version=$4,security_version=GREATEST(security_version,$5),pending_operation_id=NULL,row_version=row_version+1 WHERE id=$1 AND deletion_request_id=$2 AND deletion_state IN('requested','processing','completed')`, d.User, d.ID, g.State, g.Version, d.Security)
	if err != nil {
		return err
	}
	if changed.RowsAffected() != 1 {
		return fail(409, "DELETION_NOT_QUALIFIED", "Identity no longer owns this deletion")
	}
	if g.State == "completed" {
		for _, sql := range []string{
			`UPDATE auth.users SET password_hash='',display_name='',admin=false,state='tombstoned',recovery_state='recovery_required' WHERE id=$1`,
			`UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1`,
			`UPDATE auth.mfa_factors SET secret_ciphertext=decode('','hex'),revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`,
			`UPDATE auth.password_changes SET replacement_hash_ciphertext=decode('','hex'),state=CASE WHEN state IN('prepared','committed') THEN 'retired' ELSE state END WHERE user_id=$1`,
			`UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now()) WHERE user_id=$1`,
			`DELETE FROM auth.data_rights_proofs WHERE user_id=$1 AND purpose='cancel'`,
		} {
			if _, err = tx.Exec(r.Context(), sql, d.User); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE auth.account_deletions SET state=$2,request_version=$3,updated_at=now() WHERE id=$1`, d.ID, g.State, g.Version); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	if err = a.protect(r.Context(), op, "result", "auth.deletion", payload); err != nil {
		return err
	}
	respond(w, 200, map[string]bool{"confirmed": true})
	return nil
}
