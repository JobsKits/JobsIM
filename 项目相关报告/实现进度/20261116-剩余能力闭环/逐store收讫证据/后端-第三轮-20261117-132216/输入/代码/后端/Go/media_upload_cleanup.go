package jobsim

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

func clearMediaUploadPaths(stateDir, object string, final bool) error {
	if !validObjectID(object) {
		return fmt.Errorf("canonical media upload cleanup identity required")
	}
	dir := filepath.Join(stateDir, "Media")
	names := []string{".upload-" + object}
	if final {
		names = append(names, object)
	}
	for _, name := range names {
		file := filepath.Join(dir, name)
		info, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("prepared media upload cleanup refuses non-regular source")
		}
		if err = os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return syncDirectory(dir)
}

func (a *App) erasePreparedMediaUploads(ctx context.Context, user string) error {
	var floor bool
	if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.account_erasure_floors WHERE user_id=$1)`, user).Scan(&floor); err != nil {
		return err
	}
	if !floor {
		return fail(409, "MEDIA_OWNER_FLOOR_REQUIRED", "Prepared source cleanup requires the original durable owner floor")
	}
	rows, err := a.DB.Query(ctx, `SELECT object_id::text FROM media.upload_requests WHERE user_id=$1 AND state<>'committed' AND NOT bytes_cleared ORDER BY object_id LIMIT 32`, user)
	if err != nil {
		return err
	}
	objects := []string{}
	for rows.Next() {
		var object string
		if err = rows.Scan(&object); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, object)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, object := range objects {
		var registered, otherOwner bool
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.objects WHERE id=$1),EXISTS(SELECT 1 FROM media.objects WHERE id=$1 AND owner_id<>$2)`, object, user).Scan(&registered, &otherOwner); err != nil {
			return err
		}
		if otherOwner {
			return fmt.Errorf("prepared media upload owner differs")
		}
		// Registered objects retain the normal export-holder drain and bounded
		// owner cleanup. Only an unregistered prepared UUID is removed here.
		if err = clearMediaUploadPaths(a.Config.StateDir, object, !registered); err != nil {
			return err
		}
		if _, err = a.DB.Exec(ctx, `UPDATE media.upload_requests SET state='retired',name='',content_type='application/octet-stream',size_bytes=0,sha256=decode('','hex'),bytes_cleared=true,updated_at=now() WHERE user_id=$1 AND object_id=$2 AND state<>'committed'`, user, object); err != nil {
			return err
		}
	}
	var pending bool
	if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.upload_requests WHERE user_id=$1 AND state<>'committed' AND NOT bytes_cleared)`, user).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return fail(503, "DELETION_CLEANUP_PENDING", "Original owner has another bounded prepared upload page")
	}
	return nil
}

func retireMediaUploadsForOwner(ctx context.Context, tx pgx.Tx, user string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('media.upload_requests') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE media.upload_requests SET state='retired',name='',content_type='application/octet-stream',size_bytes=0,sha256=decode('','hex'),bytes_cleared=true,updated_at=now() WHERE user_id=$1`, user)
	return err
}

// The restore is still offline. Retire pending old-epoch responsibilities and
// remove every exact restored staging/final UUID covered by a denial record.
func ClearRestoredMediaUploads(ctx context.Context, business *pgx.Conn, stateDir string) error {
	if _, err := business.Exec(ctx, mediaUploadMigration); err != nil {
		return err
	}
	if _, err := business.Exec(ctx, `UPDATE media.upload_requests SET state='retired',name='',content_type='application/octet-stream',size_bytes=0,sha256=decode('','hex'),updated_at=now() WHERE state='prepared'`); err != nil {
		return err
	}
	rows, err := business.Query(ctx, `SELECT u.object_id::text,u.user_id::text FROM media.upload_requests u WHERE u.state='retired' OR EXISTS(SELECT 1 FROM media.account_erasure_floors f WHERE f.user_id=u.user_id)`)
	if err != nil {
		return err
	}
	type item struct{ object, user string }
	items := []item{}
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.object, &value.user); err != nil {
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
	for _, value := range items {
		var otherOwner bool
		if err = business.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.objects WHERE id=$1 AND owner_id<>$2)`, value.object, value.user).Scan(&otherOwner); err != nil {
			return err
		}
		if otherOwner {
			return fmt.Errorf("restored media upload owner differs")
		}
		if err = clearMediaUploadPaths(stateDir, value.object, true); err != nil {
			return err
		}
	}
	_, err = business.Exec(ctx, `UPDATE media.upload_requests u SET bytes_cleared=true WHERE u.state='retired' OR EXISTS(SELECT 1 FROM media.account_erasure_floors f WHERE f.user_id=u.user_id)`)
	if err != nil {
		return err
	}
	return nil
}
