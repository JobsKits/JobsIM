package jobsim

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func migrateRetentionLegacy(ctx context.Context, tx pgx.Tx, role string) error {
	if role != "message" {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO message.retention_legacy_objects(scope_id,message_id,object_id) SELECT i.deployment_id::uuid,m.id,m.attachment_id FROM message.messages m CROSS JOIN message.instance i CROSS JOIN message.retention_activation activation WHERE i.singleton AND activation.singleton AND m.created_at<activation.activated_at AND m.attachment_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM message.retention_assignments a WHERE a.message_id=m.id) ON CONFLICT DO NOTHING;`)
	return err
}
func (a *App) retentionLegacyMediaPlan(w http.ResponseWriter, r *http.Request, message, object string) error {
	var p retentionMediaPlan
	err := a.DB.QueryRow(r.Context(), `SELECT m.id::text,m.attachment_id::text,m.content_version::text,i.restore_epoch::text FROM message.retention_legacy_objects l JOIN message.messages m ON m.id=l.message_id AND m.attachment_id=l.object_id CROSS JOIN message.instance i WHERE i.singleton AND l.scope_id=$1 AND l.message_id=$2 AND l.object_id=$3 AND NOT m.deleted AND NOT m.content_unavailable AND m.delete_scope='' AND m.export_pending_operation_id IS NULL AND NOT EXISTS(SELECT 1 FROM message.retention_floors f WHERE f.scope_id=l.scope_id AND f.message_id=l.message_id) AND NOT EXISTS(SELECT 1 FROM message.lifecycle_jobs j WHERE j.scope_id=l.scope_id AND j.message_id=l.message_id AND j.due_at<=clock_timestamp())`, a.Config.DeploymentID, message, object).Scan(&p.Message, &p.Object, &p.Content, &p.Epoch)
	if err == pgx.ErrNoRows {
		err = a.DB.QueryRow(r.Context(), `SELECT m.id::text,m.attachment_id::text,m.content_version::text,i.restore_epoch::text,j.id::text,j.after_event::text,j.due_at FROM message.retention_legacy_objects l JOIN message.messages m ON m.id=l.message_id AND m.attachment_id=l.object_id JOIN message.retention_floors f ON f.scope_id=l.scope_id AND f.message_id=l.message_id AND f.delete_scope='global' JOIN message.lifecycle_jobs j ON j.scope_id=l.scope_id AND j.message_id=l.message_id AND j.delete_scope='global' AND j.state='complete' CROSS JOIN message.instance i WHERE i.singleton AND l.scope_id=$1 AND l.message_id=$2 AND l.object_id=$3 AND m.deleted AND m.delete_scope='global' ORDER BY j.due_at,j.id LIMIT 1`, a.Config.DeploymentID, message, object).Scan(&p.Message, &p.Object, &p.Content, &p.Epoch, &p.SourceJob, &p.SourceEvent, &p.ObjectDeadline)
		if err == nil {
			p.Legacy, p.CleanupEligible, p.ErasureOnly = true, true, true
			p.DeleteScope = "global"
			respond(w, 200, p)
			return nil
		}
	}
	if err != nil {
		return fail(409, "RETENTION_PLAN_PENDING", "Only a registered original legacy source can preserve its earlier read contract")
	}
	p.Legacy = true
	p.CleanupEligible = false
	respond(w, 200, p)
	return nil
}
