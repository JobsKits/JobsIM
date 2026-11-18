package jobsim

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func accountDeletionJournalKinds() map[string][]string {
	kinds := map[string][]string{
		"auth.deletion":         {"userId", "deletionId", "deviceId", "requestKey", "restoreEpoch", "requestVersion", "deletionState", "securityVersion", "cancelUntil"},
		"auth.data_rights_code": {"userId", "codeId"},
	}
	for _, owner := range []string{"directory", "conversation", "message", "media", "sync", "governance"} {
		kinds[owner+".account_erasure"] = []string{"userId", "deletionId", "requestVersion"}
	}
	return kinds
}
func validateAccountDeletionJournalField(field string, raw any) (bool, error) {
	value, ok := raw.(string)
	valid := false
	switch field {
	case "deletionState":
		valid = ok && (value == "requested" || value == "cancelled" || value == "processing" || value == "completed")
	case "requestVersion":
		valid = ok && syncValidationCounter(value, false)
	case "cancelUntil":
		t, err := time.Parse(time.RFC3339Nano, value)
		valid = ok && err == nil && t.Unix() > 0
	default:
		return false, nil
	}
	if !valid {
		return true, fail(400, "INVALID_JOURNAL_PAYLOAD", "Invalid minimal account-deletion safety field")
	}
	return true, nil
}

// Cancellation lowers only its own deletion gate, and only a durable result may
// do so. Owner erasure and processing are denial facts at the intent boundary.
func ReplayAccountDeletionSafety(ctx context.Context, business *pgx.Conn, q journalRecord) (bool, error) {
	if _, ok := accountDeletionJournalKinds()[q.Kind]; !ok {
		return false, nil
	}
	text := func(k string) string { v, _ := q.Payload[k].(string); return v }
	if q.Kind == "auth.data_rights_code" {
		if q.Phase != "intent" {
			return true, nil
		}
		_, err := business.Exec(ctx, `UPDATE auth.recovery_codes SET consumed_at=COALESCE(consumed_at,now()) WHERE id=$1 AND batch_id IN(SELECT id FROM auth.recovery_code_batches WHERE user_id=$2)`, text("codeId"), text("userId"))
		return true, err
	}
	if q.Kind != "auth.deletion" {
		if q.Phase != "intent" {
			return true, nil
		}
		role := strings.TrimSuffix(q.Kind, ".account_erasure")
		if q.Owner != role {
			return true, fmt.Errorf("account erasure owner differs")
		}
		tx, err := business.Begin(ctx)
		if err != nil {
			return true, err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "INSERT INTO "+role+`.account_erasure_floors(user_id,deletion_id,request_version) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET request_version=GREATEST(`+role+`.account_erasure_floors.request_version,EXCLUDED.request_version),state='pending',last_object=NULL`, text("userId"), text("deletionId"), text("requestVersion")); err != nil {
			return true, err
		}
		if role == "message" {
			if _, err = tx.Exec(ctx, `UPDATE message.messages SET text='',deleted=true,content_unavailable=false,delete_scope='global',deletion_generation=GREATEST(deletion_generation,1),export_pending_operation_id=NULL WHERE sender_id=$1 AND (NOT deleted OR text<>'' OR delete_scope<>'global')`, text("userId")); err != nil {
				return true, err
			}
		}
		if err = applyAccountOwnerErasure(ctx, tx, role, text("userId")); err != nil {
			return true, err
		}
		return true, tx.Commit(ctx)
	}
	state := text("deletionState")
	if q.Owner != "identity" {
		return true, fmt.Errorf("deletion safety owner differs")
	}
	if state == "cancelled" && q.Phase != "result" || state != "cancelled" && q.Phase != "intent" {
		return true, nil
	}
	tx, err := business.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(ctx)
	// An empty restored request digest can never verify a creating password
	// retry. Current credential re-verification remains a separate restricted path.
	_, err = tx.Exec(ctx, `INSERT INTO auth.account_deletions(id,user_id,request_id,device_id,restore_epoch,request_digest,state,expected_security,security_version,request_version,cancel_until)
SELECT $1,id,$3,$4,$5,decode('','hex'),$6,GREATEST($7::bigint-1,1),$7,$8,$9 FROM auth.users WHERE id=$2
ON CONFLICT(id) DO UPDATE SET state=CASE WHEN auth.account_deletions.state='completed' OR auth.account_deletions.state='processing' AND EXCLUDED.state='requested' THEN auth.account_deletions.state ELSE EXCLUDED.state END,request_version=GREATEST(auth.account_deletions.request_version,EXCLUDED.request_version),cancel_until=LEAST(auth.account_deletions.cancel_until,EXCLUDED.cancel_until)
WHERE auth.account_deletions.request_version<=EXCLUDED.request_version`, text("deletionId"), text("userId"), text("requestKey"), text("deviceId"), text("restoreEpoch"), state, text("securityVersion"), text("requestVersion"), text("cancelUntil"))
	if err != nil {
		return true, err
	}
	if state == "cancelled" {
		_, err = tx.Exec(ctx, `UPDATE auth.users SET deletion_state='none',deletion_request_id=NULL,deletion_version=$3,security_version=GREATEST(security_version,$4::bigint),pending_operation_id=NULL WHERE id=$1 AND deletion_request_id=$2 AND deletion_state='requested' AND deletion_version<=$3::bigint`, text("userId"), text("deletionId"), text("requestVersion"), text("securityVersion"))
	} else {
		_, err = tx.Exec(ctx, `UPDATE auth.users SET deletion_state=$3,deletion_request_id=$2,deletion_version=$4,security_version=GREATEST(security_version,$5::bigint),pending_operation_id=NULL WHERE id=$1 AND deletion_state<>'completed' AND ((deletion_request_id=$2 AND deletion_version<=$4::bigint AND (deletion_state<>'processing' OR $3<>'requested')) OR (deletion_request_id IS NULL AND security_version<=$5::bigint AND $3='requested'))`, text("userId"), text("deletionId"), state, text("requestVersion"), text("securityVersion"))
	}
	if err != nil {
		return true, err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, text("userId")); err != nil {
		return true, err
	}
	if state == "completed" {
		for _, sql := range []string{`UPDATE auth.users SET password_hash='',display_name='',admin=false,state='tombstoned',recovery_state='recovery_required' WHERE id=$1 AND deletion_state='completed'`, `UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1`, `UPDATE auth.mfa_factors SET secret_ciphertext=decode('','hex'),revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, `UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now()) WHERE user_id=$1`} {
			if _, err = tx.Exec(ctx, sql, text("userId")); err != nil {
				return true, err
			}
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO governance.account_deletions(id,user_id,request_id,device_id,restore_epoch,state,row_version,cancel_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(id) DO UPDATE SET state=CASE WHEN governance.account_deletions.state='completed' OR governance.account_deletions.state='processing' AND EXCLUDED.state='requested' THEN governance.account_deletions.state ELSE EXCLUDED.state END,row_version=GREATEST(governance.account_deletions.row_version,EXCLUDED.row_version),cancel_until=LEAST(governance.account_deletions.cancel_until,EXCLUDED.cancel_until)
WHERE governance.account_deletions.row_version<=EXCLUDED.row_version`, text("deletionId"), text("userId"), text("requestKey"), text("deviceId"), text("restoreEpoch"), state, text("requestVersion"), text("cancelUntil"))
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

// Called while the isolated restore remains closed, after safety replay and
// after copying backup files. A journal-protected owner floor is required for
// every exact UUID path; an ordinary filename match is never sufficient.
func ClearRestoredAccountMedia(ctx context.Context, business *pgx.Conn, stateDir string) error {
	rows, err := business.Query(ctx, `SELECT o.id::text,o.storage_key FROM media.objects o JOIN media.account_erasure_floors f ON f.user_id=o.owner_id`)
	if err != nil {
		return err
	}
	type object struct{ id, key string }
	items := []object{}
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.id, &o.key); err != nil {
			rows.Close()
			return err
		}
		items = append(items, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, o := range items {
		if o.id != o.key || !validObjectID(o.key) {
			return fmt.Errorf("restored deletion media has an invalid exact storage binding")
		}
		if err = os.Remove(filepath.Join(stateDir, "Media", o.key)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if len(items) > 0 {
		if _, err = os.Stat(filepath.Join(stateDir, "Media")); os.IsNotExist(err) {
			return nil
		}
		return syncDirectory(filepath.Join(stateDir, "Media"))
	}
	return nil
}
