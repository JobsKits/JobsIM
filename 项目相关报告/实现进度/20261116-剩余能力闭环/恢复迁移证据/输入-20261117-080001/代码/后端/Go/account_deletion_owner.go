package jobsim

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

func (a *App) eraseAccountOwner(w http.ResponseWriter, r *http.Request) error {
	if err := deletionInternalCaller(r, "governance"); err != nil {
		return err
	}
	var d accountDeletion
	if err := decodeReceiptRequest(r, &d); err != nil {
		return err
	}
	if !validDeletionBinding(d) || !syncValidationCounter(d.Version, false) {
		return fail(400, "INVALID_ACCOUNT_DELETION", "Exact original owner and cleanup version required")
	}
	var qualified accountDeletion
	if err := a.rpc(r.Context(), "governance", "/internal/deletion/qualify", d, &qualified); err != nil {
		return err
	}
	if qualified.ID != d.ID || qualified.User != d.User || qualified.Version != d.Version {
		return fail(409, "DELETION_NOT_QUALIFIED", "Original irreversible version changed")
	}
	if err := a.eraseOwnerData(r.Context(), d); err != nil {
		return err
	}
	respond(w, 200, map[string]bool{"cleared": true})
	return nil
}
func (a *App) eraseOwnerData(ctx context.Context, d accountDeletion) error {
	schema := schemaFor[a.Role]
	if schema == "" || a.Role == "identity" || a.Role == "journal" || a.Role == "control" {
		return fail(400, "INVALID_DELETION_OWNER", "A content owner is required")
	}
	epoch, err := a.epoch(ctx)
	if err != nil {
		return err
	}
	var local int64
	if err = a.DB.QueryRow(ctx, "SELECT restore_epoch FROM "+schema+".instance WHERE singleton").Scan(&local); err != nil {
		return err
	}
	if local != epoch {
		return fail(503, "RESTORE_GATE_CLOSED", "Owner cleanup requires the verified current epoch")
	}
	var complete bool
	if err = a.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+schema+".account_erasure_floors WHERE user_id=$1 AND deletion_id=$2 AND request_version=$3 AND state='complete')", d.User, d.ID, d.Version).Scan(&complete); err != nil {
		return err
	}
	if complete {
		return nil
	}
	payload := map[string]any{"userId": d.User, "deletionId": d.ID, "requestVersion": d.Version}
	operation := deletionOperation(d, "erase:"+a.Role)
	if err = a.protect(ctx, operation, "intent", a.Role+".account_erasure", payload); err != nil {
		return err
	}
	if _, err = a.DB.Exec(ctx, "INSERT INTO "+schema+`.account_erasure_floors(user_id,deletion_id,request_version) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET request_version=GREATEST(`+schema+`.account_erasure_floors.request_version,EXCLUDED.request_version) WHERE `+schema+`.account_erasure_floors.deletion_id=EXCLUDED.deletion_id`, d.User, d.ID, d.Version); err != nil {
		return err
	}
	if a.Role == "message" {
		if err = a.eraseAccountMessages(ctx, d); err != nil {
			return err
		}
	}
	if a.Role == "media" {
		if err = a.eraseAccountMedia(ctx, d); err != nil {
			return err
		}
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = applyAccountOwnerErasure(ctx, tx, a.Role, d.User); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if err = a.drainPersonalExportInvalidations(ctx); err != nil {
		return err
	}
	if err = a.protect(ctx, operation, "result", a.Role+".account_erasure", payload); err != nil {
		return err
	}
	_, err = a.DB.Exec(ctx, "UPDATE "+schema+`.account_erasure_floors SET state='complete',updated_at=now() WHERE user_id=$1 AND deletion_id=$2 AND request_version=$3`, d.User, d.ID, d.Version)
	return err
}
func (a *App) eraseAccountMessages(ctx context.Context, d accountDeletion) error {
	rows, err := a.DB.Query(ctx, `SELECT id::text FROM message.messages WHERE sender_id=$1 AND (SELECT last_object IS NULL OR id>last_object FROM message.account_erasure_floors WHERE user_id=$1) ORDER BY id LIMIT 32`, d.User)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// The durable lifecycle, rather than a direct body UPDATE, drains exports,
	// reference forwards, draft holders and emits global deletion references.
	for _, id := range ids {
		if err = a.QueueRetentionErasure(ctx, id, "account-erasure:"+d.ID+":"+id, "global", d.Until); err != nil {
			return err
		}
	}
	for i := 0; i < 64; i++ {
		did, e := a.executeNextRetention(ctx)
		if e != nil {
			return e
		}
		if !did {
			break
		}
	}
	for _, id := range ids {
		var complete bool
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message.lifecycle_jobs WHERE message_id=$1 AND reason=$2 AND state='complete')`, id, "explicit_"+syncDigest([]byte("account-erasure:"+d.ID+":"+id))).Scan(&complete); err != nil {
			return err
		}
		if !complete {
			return fail(503, "DELETION_CLEANUP_PENDING", "Original durable message cleanup still has work")
		}
	}
	if len(ids) > 0 {
		if _, err = a.DB.Exec(ctx, `UPDATE message.account_erasure_floors SET last_object=$2 WHERE user_id=$1 AND deletion_id=$3`, d.User, ids[len(ids)-1], d.ID); err != nil {
			return err
		}
	}
	if len(ids) == 32 {
		return fail(503, "DELETION_CLEANUP_PENDING", "Original message erasure has another bounded page")
	}

	return nil
}
func (a *App) eraseAccountMedia(ctx context.Context, d accountDeletion) error {
	rows, err := a.DB.Query(ctx, `SELECT id::text,storage_key FROM media.objects WHERE owner_id=$1 AND (SELECT last_object IS NULL OR id>last_object FROM media.account_erasure_floors WHERE user_id=$1) ORDER BY id LIMIT 32`, d.User)
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
			return fail(503, "MEDIA_STORAGE_INTEGRITY", "Only original owned UUID storage keys may be erased")
		}
		if err = os.Remove(filepath.Join(a.Config.StateDir, "Media", o.key)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if len(items) > 0 {
		dir, err := os.Open(filepath.Join(a.Config.StateDir, "Media"))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		err = dir.Sync()
		closeErr := dir.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if len(items) > 0 {
		if _, err = a.DB.Exec(ctx, `UPDATE media.account_erasure_floors SET last_object=$2 WHERE user_id=$1 AND deletion_id=$3`, d.User, items[len(items)-1].id, d.ID); err != nil {
			return err
		}
	}
	if len(items) == 32 {
		return fail(503, "DELETION_CLEANUP_PENDING", "Original media erasure has another bounded page")
	}
	return nil
}

// Owner-local metadata only. These exact statements are also used by restore
// replay, after the independently protected irreversible owner intent.
func applyAccountOwnerErasure(ctx context.Context, tx pgx.Tx, role, user string) error {
	var statements []string
	switch role {
	case "directory":
		statements = []string{
			`UPDATE directory.profiles SET display_name='',avatar_object_id=NULL,avatar_thumbnail_id=NULL,pending_avatar_operation_id=NULL,row_version=row_version+1,updated_at=now() WHERE user_id=$1 AND (display_name<>'' OR avatar_object_id IS NOT NULL OR avatar_thumbnail_id IS NOT NULL OR pending_avatar_operation_id IS NOT NULL)`,
			`UPDATE directory.contacts SET state='removed',request_gate='denied',request_generation=request_generation+1 WHERE (owner_id=$1 OR peer_id=$1) AND state<>'removed'`,
			`UPDATE directory.contact_requests SET state='removed' WHERE requester_id=$1 OR recipient_id=$1`,
			`DELETE FROM directory.source_preferences WHERE user_id=$1`, `DELETE FROM directory.source_preference_operations WHERE user_id=$1`,
			`UPDATE directory.avatar_operations SET result='{}',state='retired' WHERE user_id=$1`,
		}
	case "conversation":
		statements = []string{
			`UPDATE conversation.group_invites SET revoked_at=COALESCE(revoked_at,now()) WHERE creator_id=$1`,
			`UPDATE conversation.conversations SET state='owner_unavailable',version=version+1,updated_at=now() WHERE owner_id=$1 AND kind='group' AND state='active'`,
			`UPDATE conversation.members SET state='left',export_pending_operation_id=NULL WHERE user_id=$1 AND state='active'`,
			`UPDATE conversation.membership_periods SET left_at=COALESCE(left_at,now()) WHERE user_id=$1`,
			`UPDATE conversation.conversations SET title='',state='closed',version=version+1,updated_at=now() WHERE owner_id=$1 AND kind='self' AND state<>'closed'`,
		}
	case "message":
		statements = []string{`DELETE FROM message.reactions WHERE user_id=$1`, `DELETE FROM message.favorites WHERE user_id=$1`}
	case "media":
		statements = []string{
			`UPDATE media.objects SET name='',content_type='application/octet-stream',size_bytes=0,sha256=decode(repeat('00',32),'hex') WHERE owner_id=$1`,
			`UPDATE media.avatar_jobs SET state='retired' WHERE owner_id=$1`, `UPDATE media.avatar_processing_items SET state='retired' WHERE owner_id=$1`,
		}
	case "sync":
		statements = []string{
			`DELETE FROM sync.snapshot_pages WHERE snapshot_id IN(SELECT id FROM sync.snapshots WHERE user_id=$1)`, `DELETE FROM sync.snapshots WHERE user_id=$1`,
			`DELETE FROM sync.events WHERE stream_id IN(SELECT id FROM sync.streams WHERE user_id=$1)`, `DELETE FROM sync.stream_import WHERE stream_id IN(SELECT id FROM sync.streams WHERE user_id=$1)`,
			`DELETE FROM sync.registrations WHERE user_id=$1`, `DELETE FROM sync.streams WHERE user_id=$1`,
			`DELETE FROM sync.storage_qualifications WHERE user_id=$1`, `DELETE FROM sync.device_stores WHERE user_id=$1`, `DELETE FROM sync.storage_declarations WHERE user_id=$1`,
			`DELETE FROM sync.conversation_view_operations WHERE user_id=$1`, `DELETE FROM sync.read_operations WHERE user_id=$1`, `DELETE FROM sync.read_observations WHERE user_id=$1`, `DELETE FROM sync.read_cursors WHERE user_id=$1`,
			`DELETE FROM sync.draft_operations WHERE user_id=$1`, `DELETE FROM sync.draft_pending WHERE user_id=$1`, `DELETE FROM sync.conversation_views WHERE user_id=$1`,
		}
	case "governance":
		statements = []string{`UPDATE governance.reports SET reason='[deleted account]' WHERE reporter_id=$1`, `UPDATE governance.report_operations SET reason='[deleted account]',result='{}' WHERE actor_id=$1`}
	}
	for _, sql := range statements {
		if _, err := tx.Exec(ctx, sql, user); err != nil {
			return err
		}
	}
	return nil
}
