package jobsim

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type migrationInterestReference struct {
	Message      string `json:"messageId"`
	Conversation string `json:"conversationId"`
	Object       string `json:"objectId,omitempty"`
}
type migrationInterest struct {
	ID           string                       `json:"id"`
	Epoch        string                       `json:"restoreEpoch"`
	User         string                       `json:"userId"`
	SourceDevice string                       `json:"sourceDeviceId"`
	TargetDevice string                       `json:"targetDeviceId"`
	Refs         []migrationInterestReference `json:"refs"`
}

func migrateMigrationInvalidations(ctx context.Context, tx pgx.Tx, role string) error {
	o := migrationOwner(role)
	if o == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE `+o+`.migration_interests ADD COLUMN IF NOT EXISTS user_id uuid;ALTER TABLE `+o+`.migration_interests ADD COLUMN IF NOT EXISTS source_device uuid;ALTER TABLE `+o+`.migration_interests ADD COLUMN IF NOT EXISTS target_device uuid;ALTER TABLE `+o+`.migration_interests ADD COLUMN IF NOT EXISTS refs jsonb;CREATE TABLE IF NOT EXISTS `+o+`.migration_invalidations(scope_id uuid NOT NULL,migration_id uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,migration_id));ALTER TABLE `+o+`.migration_invalidations ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz;`); err != nil {
		return err
	}
	type watched struct{ table, fields, predicate string }
	var items []watched
	byMessage := `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r WHERE r->>'messageId'=rowdata->>'message_id')`
	switch role {
	case "identity":
		items = []watched{
			{"users", "state recovery_state deletion_state pending_operation_id security_version credential_generation", `i.user_id::text=rowdata->>'id'`},
			{"sessions", "user_id device_id family_id revoked_at web_pending restore_epoch source_context_id source_generation security_version generation export_pending_operation_id", `i.user_id::text=rowdata->>'user_id' AND (i.source_device::text=rowdata->>'device_id' OR i.target_device::text=rowdata->>'device_id')`},
			{"instance", "restore_epoch personal_source_generation", "true"}}
	case "conversation":
		items = []watched{
			{"members", "state membership_generation role_generation role joined_at export_pending_operation_id", `i.user_id::text=rowdata->>'user_id' AND EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r WHERE r->>'conversationId'=rowdata->>'conversation_id')`},
			{"conversations", "kind owner_id state export_pending_operation_id", `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r WHERE r->>'conversationId'=rowdata->>'id')`}}
	case "message":
		items = []watched{
			{"messages", "type text reply_to_id attachment_id content_version deleted content_unavailable export_pending_operation_id delete_scope", `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r WHERE r->>'messageId'=rowdata->>'id')`},
			{"forward_refs", "state source_content_version source_membership_generation security_version restore_epoch", byMessage},
			{"retention_assignments", "receipt_complete_at object_complete_at policy generation", byMessage},
			{"lifecycle_jobs", "due_at delete_scope state", byMessage},
			{"retention_floors", "delete_scope generation due_at", byMessage},
			{"account_erasure_floors", "request_version state", `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r JOIN message.messages m ON m.id::text=r->>'messageId' WHERE m.sender_id::text=rowdata->>'user_id')`}}
	case "media":
		items = []watched{
			{"objects", "owner_id purpose message_id size_bytes sha256 storage_key content_type", `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r WHERE r->>'objectId'=rowdata->>'id')`},
			{"retention_floors", "due_at terminal", `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r WHERE r->>'objectId'=rowdata->>'object_id')`},
			{"legacy_erasure_tasks", "due_at source_event_id", `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r WHERE r->>'objectId'=rowdata->>'object_id')`},
			{"account_erasure_floors", "request_version state", `EXISTS(SELECT 1 FROM jsonb_array_elements(i.refs) r JOIN media.objects m ON m.id::text=r->>'objectId' WHERE m.owner_id::text=rowdata->>'user_id')`}}
	case "sync":
		items = []watched{{"device_stores", "store_generation", `i.user_id::text=rowdata->>'user_id' AND(i.source_device::text=rowdata->>'device_id' OR i.target_device::text=rowdata->>'device_id')`}, {"instance", "restore_epoch", "true"}}
	}
	for _, item := range items {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, o+"."+item.table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			continue
		}
		var oldFields, newFields []string
		for _, field := range strings.Fields(item.fields) {
			oldFields = append(oldFields, "olddata->'"+field+"'")
			newFields = append(newFields, "newdata->'"+field+"'")
		}
		function := o + ".migration_invalidate_" + item.table
		statement := `CREATE OR REPLACE FUNCTION ` + function + `() RETURNS trigger LANGUAGE plpgsql AS $migration$ DECLARE olddata jsonb;newdata jsonb;rowdata jsonb;BEGIN IF TG_OP<>'INSERT' THEN olddata:=to_jsonb(OLD);END IF;IF TG_OP<>'DELETE' THEN newdata:=to_jsonb(NEW);END IF;IF TG_OP='UPDATE' AND jsonb_build_array(` + strings.Join(oldFields, ",") + `) IS NOT DISTINCT FROM jsonb_build_array(` + strings.Join(newFields, ",") + `) THEN RETURN NEW;END IF;rowdata:=COALESCE(newdata,olddata);INSERT INTO ` + o + `.migration_invalidations(scope_id,migration_id) SELECT i.scope_id,i.migration_id FROM ` + o + `.migration_interests i WHERE i.refs IS NULL OR (` + item.predicate + `) ON CONFLICT DO NOTHING;IF TG_OP='DELETE' THEN RETURN OLD;END IF;RETURN NEW;END $migration$;DROP TRIGGER IF EXISTS migration_invalidate ON ` + o + `.` + item.table + `;CREATE TRIGGER migration_invalidate AFTER INSERT OR UPDATE OR DELETE ON ` + o + `.` + item.table + ` FOR EACH ROW EXECUTE FUNCTION ` + function + `();`
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) drainMigrationOwnerQueue(ctx context.Context) error {
	if err := a.requireMigrationExclusive(ctx); err != nil {
		return err
	}
	owner := migrationOwner(a.Role)
	if owner == "" {
		return nil
	}
	for batch := 0; batch < 16; batch++ {
		rows, err := a.DB.Query(ctx, `SELECT migration_id::text FROM `+owner+`.migration_invalidations WHERE scope_id=$1 ORDER BY last_attempt_at NULLS FIRST,created_at,migration_id LIMIT 128`, a.Config.DeploymentID)
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
		if _, err = a.DB.Exec(ctx, `UPDATE `+owner+`.migration_invalidations SET last_attempt_at=clock_timestamp() WHERE scope_id=$1 AND migration_id=ANY($2::uuid[])`, a.Config.DeploymentID, ids); err != nil {
			return err
		}
		if a.Role == "sync" {
			var firstError error
			for _, id := range ids {
				if err = a.retireMigration(ctx, id, "source_changed"); err != nil {
					if firstError == nil {
						firstError = err
					}
				}
			}
			if firstError != nil {
				return firstError
			}
		} else {
			if err = a.rpc(ctx, "sync", "/internal/migrations/retire", map[string]any{"ids": ids}, nil); err != nil {
				return err
			}
		}
		if _, err = a.DB.Exec(ctx, `DELETE FROM `+owner+`.migration_invalidations WHERE scope_id=$1 AND migration_id=ANY($2::uuid[]);`, a.Config.DeploymentID, ids); err != nil {
			return err
		}
		if _, err = a.DB.Exec(ctx, `DELETE FROM `+owner+`.migration_interests WHERE scope_id=$1 AND migration_id=ANY($2::uuid[])`, a.Config.DeploymentID, ids); err != nil {
			return err
		}
	}
	return fail(503, "MIGRATION_CLEANUP_PENDING", "More original affected migration holders remain")
}

func (a *App) migrationRetire(w http.ResponseWriter, r *http.Request) error {
	if migrationOwner(r.Header.Get("X-JobsIM-Caller")) == "" || !a.hasGatewayProjectionFence(r, true) {
		return fail(403, "MIGRATION_OWNER_REQUIRED", "An original source owner with its live exclusive drain is required")
	}
	var q struct {
		IDs []string `json:"ids"`
	}
	if err := decodeMigrationRequest(r, &q, 32<<10); err != nil {
		return err
	}
	if len(q.IDs) == 0 || len(q.IDs) > 128 {
		return fail(400, "INVALID_MIGRATION_RETIREMENT", "A bounded exact source holder set required")
	}
	ctx, expire := a.heldAdmissionScope(r.Context(), "exclusive")
	defer expire()
	for _, id := range q.IDs {
		if !syncValidationID(id) {
			return fail(400, "INVALID_MIGRATION_ID", "Original transfer identities required")
		}
	}
	var firstError error
	for _, id := range q.IDs {
		if err := a.retireMigration(ctx, id, "source_changed"); err != nil {
			if firstError == nil {
				firstError = err
			}
		}
	}
	if firstError != nil {
		return firstError
	}
	respond(w, 200, map[string]string{"state": "retired"})
	return nil
}
