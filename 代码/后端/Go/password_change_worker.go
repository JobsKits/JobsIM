package jobsim

import (
	"context"
	"errors"
	"strings"
)

func (a *App) finishPasswordChange(ctx context.Context, original passwordChangeRecord) error {
	value, err := scanPasswordChange(a.DB.QueryRow(ctx, `SELECT `+passwordChangeColumns+` FROM auth.password_changes WHERE user_id=$1 AND request_id=$2`, original.User, original.Request))
	if err != nil {
		return err
	}
	if value.State == "completed" || value.State == "retired" {
		return nil
	}
	epoch, err := a.currentIdentityEpoch(ctx)
	if err != nil {
		return err
	}
	if value.Epoch != str(epoch) {
		return a.retirePasswordChange(ctx, value)
	}
	payload := map[string]any{"userId": value.User, "generation": str(value.Generation), "securityVersion": str(value.Security)}
	if value.State == "prepared" {
		var owns bool
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.users WHERE id=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none'
AND pending_operation_id=$2 AND credential_generation=$3 AND security_version=$4)`, value.User, value.Operation, value.ExpectedGeneration, value.ExpectedSecurity).Scan(&owns); err != nil {
			return err
		}
		if !owns {
			return a.retirePasswordChange(ctx, value)
		}
		if err = a.protect(ctx, value.Operation, "intent", "auth.invalidate_user", payload); err != nil {
			return err
		}
		if err = a.freezeUserExports(ctx, value.User, value.Operation); err != nil {
			return err
		}
		replacement, err := a.unseal(value.Ciphertext, a.passwordChangeAAD(value))
		if err != nil || len(replacement) > 1024 || !strings.HasPrefix(replacement, "$argon2id$v=19$m=65536,t=3,p=1$") {
			return fail(503, "PASSWORD_CHANGE_PROTECTION_PENDING", "The protected replacement credential cannot be recovered")
		}
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		var state, recovery, deletion string
		var pending *string
		var generation, security int64
		err = tx.QueryRow(ctx, `SELECT state,recovery_state,deletion_state,pending_operation_id,credential_generation,security_version FROM auth.users WHERE id=$1 FOR UPDATE`, value.User).Scan(&state, &recovery, &deletion, &pending, &generation, &security)
		if err != nil {
			return err
		}
		locked, err := scanPasswordChange(tx.QueryRow(ctx, `SELECT `+passwordChangeColumns+` FROM auth.password_changes WHERE user_id=$1 AND request_id=$2 FOR UPDATE`, value.User, value.Request))
		if err != nil {
			return err
		}
		if locked.State != "prepared" {
			return fail(409, "PASSWORD_CHANGE_CONTEXT_CHANGED", "The original credential operation changed")
		}
		var currentEpoch string
		if err = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM auth.instance WHERE singleton FOR SHARE`).Scan(&currentEpoch); err != nil {
			return err
		}
		if state != "active" || recovery != "normal" || deletion != "none" || pending == nil || *pending != value.Operation || generation != value.ExpectedGeneration || security != value.ExpectedSecurity || currentEpoch != value.Epoch {
			return fail(409, "PASSWORD_CHANGE_CONTEXT_CHANGED", "This original operation no longer owns the active account")
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.users SET password_hash=$2,credential_generation=$3,security_version=$4,pending_operation_id=NULL,row_version=row_version+1 WHERE id=$1`, value.User, replacement, value.Generation, value.Security); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, value.User); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1 AND active`, value.User); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.reset_grants SET revoked_at=now(),state='revoked' WHERE user_id=$1 AND state='active'`, value.User); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth.password_changes SET state='committed',replacement_hash_ciphertext=decode('','hex'),updated_at=now() WHERE user_id=$1 AND request_id=$2 AND state='prepared'`, value.User, value.Request); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	if err = a.protect(ctx, value.Operation, "result", "auth.invalidate_user", payload); err != nil {
		return err
	}
	if err = a.drainPersonalExportInvalidations(ctx); err != nil {
		return err
	}
	if err = a.drainMigrations(ctx); err != nil {
		return err
	}
	_, err = a.DB.Exec(ctx, `UPDATE auth.password_changes SET state='completed',updated_at=now() WHERE user_id=$1 AND request_id=$2 AND state='committed' AND restore_epoch=$3`, value.User, value.Request, value.Epoch)
	return err
}

func (a *App) retirePasswordChange(ctx context.Context, value passwordChangeRecord) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Retiring an old epoch never installs its hash, unbans an account or lowers
	// a denial generation already replayed from the independent journal.
	if _, err = tx.Exec(ctx, `UPDATE auth.users SET pending_operation_id=NULL WHERE id=$1 AND pending_operation_id=$2`, value.User, value.Operation); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.password_changes SET state='retired',replacement_hash_ciphertext=decode('','hex'),updated_at=now() WHERE user_id=$1 AND request_id=$2 AND state IN('prepared','committed')`, value.User, value.Request); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// The existing identity worker calls this under its live outer EX scope.
// Committed operations retry journal/result cleanup without a second password.
func (a *App) resumePasswordChanges(ctx context.Context, limit int) error {
	if limit < 1 || limit > 10 {
		return fail(400, "INVALID_PASSWORD_CHANGE_BATCH", "Bounded credential recovery batch required")
	}
	rows, err := a.DB.Query(ctx, `SELECT `+passwordChangeColumns+` FROM auth.password_changes WHERE state IN('prepared','committed') ORDER BY last_attempt_at NULLS FIRST,updated_at,user_id,request_id LIMIT $1`, limit)
	if err != nil {
		return err
	}
	items := []passwordChangeRecord{}
	for rows.Next() {
		value, err := scanPasswordChange(rows)
		if err != nil {
			rows.Close()
			return err
		}
		items = append(items, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var pending error
	for _, value := range items {
		if _, err = a.DB.Exec(ctx, `UPDATE auth.password_changes SET last_attempt_at=clock_timestamp() WHERE user_id=$1 AND request_id=$2 AND state IN('prepared','committed')`, value.User, value.Request); err != nil {
			return errors.Join(pending, err)
		}
		pending = errors.Join(pending, a.finishPasswordChange(ctx, value))
		if ctx.Err() != nil {
			break
		}
	}
	return pending
}
