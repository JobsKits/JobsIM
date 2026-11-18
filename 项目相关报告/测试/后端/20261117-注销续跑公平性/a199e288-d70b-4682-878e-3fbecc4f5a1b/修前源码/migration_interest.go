package jobsim

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func migrationOwner(role string) string {
	switch role {
	case "identity":
		return "auth"
	case "directory", "conversation", "message", "media", "control", "governance", "sync":
		return role
	}
	return ""
}
func migrateMigrationInterests(ctx context.Context, tx pgx.Tx, role string) error {
	owner := migrationOwner(role)
	if owner == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+owner+`.migration_interests(scope_id uuid NOT NULL,migration_id uuid NOT NULL,restore_epoch bigint NOT NULL,PRIMARY KEY(scope_id,migration_id))`)
	return err
}
func (a *App) migrationInterestRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	if migrationOwner(a.Role) == "" {
		return
	}
	h("POST /internal/migrations/register-interest", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "sync" || (!a.hasGatewayProjectionFence(r, false) && !a.hasGatewayProjectionFence(r, true)) {
			return fail(403, "MIGRATION_OWNER_REQUIRED", "A live migration builder must register its separate source interest")
		}
		var q struct {
			ID    string `json:"id"`
			Epoch string `json:"restoreEpoch"`
		}
		if err := decodeMigrationRequest(r, &q, 16<<10); err != nil {
			return err
		}
		if !syncValidationID(q.ID) || !syncValidationCounter(q.Epoch, false) {
			return fail(400, "INVALID_MIGRATION_INTEREST", "Original transfer and epoch required")
		}
		var epoch string
		if err := a.DB.QueryRow(r.Context(), `SELECT restore_epoch::text FROM `+migrationOwner(a.Role)+`.instance WHERE singleton`).Scan(&epoch); err != nil {
			return err
		}
		if epoch != q.Epoch {
			return fail(409, "RESTORE_EPOCH_CHANGED", "A restored owner rejects old migration holders")
		}
		_, err := a.DB.Exec(r.Context(), `INSERT INTO `+migrationOwner(a.Role)+`.migration_interests(scope_id,migration_id,restore_epoch) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, q.ID, q.Epoch)
		if err != nil {
			return err
		}
		respond(w, 200, map[string]string{"state": "registered"})
		return nil
	}))
}
func (a *App) registerMigrationInterests(ctx context.Context, id, epoch string) error {
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "governance"} {
		if err := a.rpc(ctx, role, "/internal/migrations/register-interest", map[string]string{"id": id, "restoreEpoch": epoch}, nil); err != nil {
			return err
		}
	}
	return nil
}
