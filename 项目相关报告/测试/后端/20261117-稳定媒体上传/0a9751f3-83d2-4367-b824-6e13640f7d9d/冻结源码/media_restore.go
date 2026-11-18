package jobsim

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

// Called only after the live independent journal/snapshot has been verified.
// Missing bytes alone never authorize their omission from a primary backup.
func MediaObjectNeedsBackup(ctx context.Context, business *pgx.Conn, id string) (bool, error) {
	if !syncValidationID(id) {
		return false, fmt.Errorf("canonical original media identity required")
	}
	var retired bool
	err := business.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.account_erasure_floors a WHERE a.user_id=o.owner_id) OR EXISTS(SELECT 1 FROM media.retention_floors f CROSS JOIN media.instance i WHERE i.singleton AND f.scope_id=i.deployment_id::uuid AND f.object_id=o.id AND f.message_id=o.message_id AND f.terminal) OR EXISTS(SELECT 1 FROM media.legacy_erasure_tasks e CROSS JOIN media.instance i WHERE i.singleton AND e.scope_id=i.deployment_id::uuid AND e.object_id=o.id AND e.message_id=o.message_id) FROM media.objects o WHERE o.id=$1`, id).Scan(&retired)
	if err != nil {
		return false, err
	}
	return !retired, nil
}

// Offline target only, after authenticated file restore and per-owner safety
// replay. Deadline-only facts wait for the runtime clock guard; delete intents
// already authorize re-clearing exact bytes restored from an older snapshot.
func ClearRestoredRetainedMedia(ctx context.Context, business *pgx.Conn, stateDir string) error {
	rows, err := business.Query(ctx, `SELECT o.id::text,o.storage_key FROM media.objects o WHERE o.purpose='chat_attachment' AND (EXISTS(SELECT 1 FROM media.retention_floors f CROSS JOIN media.instance i WHERE i.singleton AND f.scope_id=i.deployment_id::uuid AND f.object_id=o.id AND f.message_id=o.message_id AND f.terminal) OR EXISTS(SELECT 1 FROM media.legacy_erasure_tasks e CROSS JOIN media.instance i WHERE i.singleton AND e.scope_id=i.deployment_id::uuid AND e.object_id=o.id AND e.message_id=o.message_id))`)
	if err != nil {
		return err
	}
	type object struct{ id, key string }
	items := []object{}
	for rows.Next() {
		var item object
		if err = rows.Scan(&item.id, &item.key); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.id != item.key || !syncValidationID(item.key) {
			return fmt.Errorf("retired media original UUID binding differs")
		}
		path := filepath.Join(stateDir, "Media", item.key)
		info, statErr := os.Lstat(path)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("retired restored media is not a regular private object")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	if len(items) == 0 {
		return nil
	}
	dir := filepath.Join(stateDir, "Media")
	if _, err = os.Lstat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return syncDirectory(dir)
}
