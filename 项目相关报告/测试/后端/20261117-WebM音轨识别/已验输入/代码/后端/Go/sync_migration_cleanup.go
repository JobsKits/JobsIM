package jobsim

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) retireMigration(ctx context.Context, id, reason string) error {
	// A held EX scope proves that no live builder or controlled download remains.
	if err := a.requireMigrationExclusive(ctx); err != nil {
		return err
	}
	_, err := a.DB.Exec(ctx, `UPDATE sync.migrations SET state='revoking',key_ciphertext=decode('','hex'),retired_reason=COALESCE(retired_reason,$3),updated_at=now() WHERE scope_id=$1 AND id=$2 AND state<>'retired'`, a.Config.DeploymentID, id, reason)
	if err != nil {
		return err
	}
	path, err := a.migrationPath(id)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fail(503, "MIGRATION_STORAGE_INTEGRITY", "Retirement refuses an unexpected private capsule path")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
		if err = syncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	_, err = a.DB.Exec(ctx, `UPDATE sync.migrations SET state='retired',updated_at=now() WHERE scope_id=$1 AND id=$2 AND state='revoking'`, a.Config.DeploymentID, id)
	return err
}
func (a *App) requireMigrationExclusive(ctx context.Context) error {
	held, ok := ctx.Value(heldAdmissionContextKey{}).(heldAdmissionContext)
	if !ok || held.Mode != "exclusive" || held.Identity != a.admissionFenceIdentity() || held.Scope == nil {
		return fail(503, "MIGRATION_DRAIN_FENCE_REQUIRED", "Retirement must drain every original builder and controlled download")
	}
	held.Scope.mu.RLock()
	defer held.Scope.mu.RUnlock()
	if !held.Scope.active || ctx.Err() != nil {
		return fail(503, "MIGRATION_DRAIN_FENCE_REQUIRED", "Retirement scope has expired")
	}
	return nil
}

func (a *App) drainMigrationRecords(ctx context.Context) error {
	if err := a.requireMigrationExclusive(ctx); err != nil {
		return err
	}
	if err := a.drainMigrationOwnerQueue(ctx); err != nil {
		return err
	}
	rows, err := a.DB.Query(ctx, `SELECT id::text FROM sync.migrations WHERE scope_id=$1 AND (state IN('building','revoking') OR state IN('ready','installed_reported') AND expires_at<=clock_timestamp()) ORDER BY expires_at,id LIMIT 128`, a.Config.DeploymentID)
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
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		rec, err := a.readMigration(ctx, id)
		if err != nil {
			return err
		}
		reason := "source_changed"
		if rec.State == "building" {
			reason = "interrupted_build"
		} else if rec.State == "revoking" {
			reason = "resume_retirement"
		} else {
			reason = "expired"
		}
		if err = a.retireMigration(ctx, id, reason); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) migrationDrain(w http.ResponseWriter, r *http.Request) error {
	caller := r.Header.Get("X-JobsIM-Caller")
	switch caller {
	case "identity", "directory", "conversation", "message", "media", "control", "governance", "sync":
	default:
		return fail(403, "MIGRATION_OWNER_REQUIRED", "Only an original source owner may drain controlled migration artifacts")
	}
	if !a.hasGatewayProjectionFence(r, true) {
		return fail(503, "MIGRATION_DRAIN_FENCE_REQUIRED", "A source mutation must retain its live outer exclusive scope")
	}
	ctx, expire := a.heldAdmissionScope(r.Context(), "exclusive")
	defer expire()
	if err := a.drainMigrationRecords(ctx); err != nil {
		return err
	}
	respond(w, 200, map[string]string{"state": "drained"})
	return nil
}
func (a *App) drainMigrations(ctx context.Context) error {
	if a.Config.RoleURLs["sync"] == "" {
		return nil
	}
	if a.Role == "sync" {
		return a.drainMigrationOwnerQueue(ctx)
	}
	owner := migrationOwner(a.Role)
	if owner == "" {
		return nil
	}
	var interested bool
	if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+owner+`.migration_interests WHERE scope_id=$1)`, a.Config.DeploymentID).Scan(&interested); err != nil {
		return err
	}
	if !interested {
		return nil
	}
	return a.drainMigrationOwnerQueue(ctx)
}
func (a *App) StartMigrationWorkers(ctx context.Context) {
	if a.Role != "sync" {
		return
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
				lock, err := a.admissionFence(attempt, true)
				if err == nil {
					held, expire := a.heldAdmissionScope(attempt, "exclusive")
					_ = a.drainMigrationRecords(held)
					expire()
					closeExportFence(lock)
				}
				cancel()
			}
		}
	}()
}

func (a *App) StartMigrationOwnerWorkers(ctx context.Context) {
	if migrationOwner(a.Role) == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
				var pending bool
				if err := a.DB.QueryRow(attempt, `SELECT EXISTS(SELECT 1 FROM `+migrationOwner(a.Role)+`.migration_invalidations WHERE scope_id=$1)`, a.Config.DeploymentID).Scan(&pending); err == nil && pending {
					lock, err := a.admissionFence(attempt, true)
					if err == nil {
						held, expire := a.heldAdmissionScope(attempt, "exclusive")
						_ = a.drainMigrationOwnerQueue(held)
						expire()
						closeExportFence(lock)
					}
				}
				cancel()
			}
		}
	}()
}

func (a *App) eraseAccountMigrationArtifacts(ctx context.Context, user string) error {
	if a.Role != "sync" {
		return nil
	}
	if err := a.requireMigrationExclusive(ctx); err != nil {
		return err
	}
	for {
		rows, err := a.DB.Query(ctx, `SELECT id::text FROM sync.migrations WHERE scope_id=$1 AND user_id=$2 ORDER BY id LIMIT 32`, a.Config.DeploymentID, user)
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
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			if err = a.retireMigration(ctx, id, "account_erasure"); err != nil {
				return err
			}
		}
		if _, err = a.DB.Exec(ctx, `DELETE FROM sync.migrations WHERE scope_id=$1 AND user_id=$2 AND id=ANY($3::uuid[]) AND state='retired'`, a.Config.DeploymentID, user, ids); err != nil {
			return err
		}
	}
}

// Migration packages are derived delivery material, never a restoration source.
func CancelRestoredMigrations(ctx context.Context, business *pgx.Conn, stateDir string) error {
	var exists bool
	if err := business.QueryRow(ctx, `SELECT to_regclass('sync.migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := business.Exec(ctx, `UPDATE sync.migrations SET state='revoking',key_ciphertext=decode('','hex'),retired_reason='restore',updated_at=now() WHERE state<>'retired';UPDATE sync.migration_pairings SET expires_at=LEAST(expires_at,now())`); err != nil {
		return err
	}
	root := filepath.Join(stateDir, "Migrations")
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		id := name
		if len(name) > 8 && name[len(name)-8:] == ".capsule" {
			id = name[:len(name)-8]
		} else {
			return fail(503, "MIGRATION_RESTORE_STORAGE_INTEGRITY", "Unexpected restored migration file")
		}
		if !syncValidationID(id) {
			return fail(503, "MIGRATION_RESTORE_STORAGE_INTEGRITY", "Unexpected restored migration identity")
		}
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil || !info.Mode().IsRegular() {
			return fail(503, "MIGRATION_RESTORE_STORAGE_INTEGRITY", "Restored migration paths must be regular files")
		}
		if err = os.Remove(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	if len(entries) > 0 {
		if err = syncDirectory(root); err != nil {
			return err
		}
	}
	_, err = business.Exec(ctx, `UPDATE sync.migrations SET state='retired',updated_at=now() WHERE state='revoking'`)
	return err
}
