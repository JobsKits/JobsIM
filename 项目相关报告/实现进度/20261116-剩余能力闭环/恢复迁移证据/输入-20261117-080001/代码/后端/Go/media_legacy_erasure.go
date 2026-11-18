package jobsim

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["media"] += `CREATE TABLE IF NOT EXISTS media.legacy_erasure_tasks(scope_id uuid NOT NULL,object_id uuid NOT NULL,message_id uuid NOT NULL,restore_epoch bigint NOT NULL,source_job_id uuid NOT NULL,source_event_id uuid NOT NULL,due_at timestamptz NOT NULL,complete boolean NOT NULL DEFAULT false,PRIMARY KEY(scope_id,object_id));`
}

func legacyMediaErasurePayload(p retentionMediaPlan) map[string]any {
	return map[string]any{"objectId": p.Object, "messageId": p.Message, "restoreEpoch": p.Epoch, "sourceJobId": p.SourceJob, "sourceEventId": p.SourceEvent, "dueAt": p.ObjectDeadline.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)}
}

func validateLegacyMediaErasure(p retentionMediaPlan) error {
	if !p.Legacy || !p.CleanupEligible || !p.ErasureOnly || !syncValidationID(p.Message) || !syncValidationID(p.Object) || !syncValidationID(p.SourceJob) || !syncValidationID(p.SourceEvent) || !syncValidationCounter(p.Epoch, false) || p.ObjectDeadline.IsZero() || p.ObjectDeadline.Unix() <= 0 || p.ObjectDeadline.After(time.Now()) {
		return fail(503, "LEGACY_ERASURE_UNVERIFIED", "Original confirmed global erasure event required")
	}
	return nil
}

func (a *App) legacyMediaRetired(ctx context.Context, message, object string) (bool, error) {
	var retired bool
	err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.legacy_erasure_tasks WHERE scope_id=$1 AND object_id=$2 AND message_id=$3)`, a.Config.DeploymentID, object, message).Scan(&retired)
	return retired, err
}

func (a *App) clearLegacyRetainedMedia(ctx context.Context, p retentionMediaPlan, key string) error {
	if err := validateLegacyMediaErasure(p); err != nil {
		return err
	}
	if err := a.mediaRetentionClockGuard(ctx); err != nil {
		return err
	}
	var prior retentionMediaPlan
	err := a.DB.QueryRow(ctx, `SELECT object_id::text,message_id::text,restore_epoch::text,source_job_id::text,source_event_id::text,due_at FROM media.legacy_erasure_tasks WHERE scope_id=$1 AND object_id=$2 AND message_id=$3`, a.Config.DeploymentID, p.Object, p.Message).Scan(&prior.Object, &prior.Message, &prior.Epoch, &prior.SourceJob, &prior.SourceEvent, &prior.ObjectDeadline)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		p.Object, p.Message, p.Epoch, p.SourceJob, p.SourceEvent, p.ObjectDeadline = prior.Object, prior.Message, prior.Epoch, prior.SourceJob, prior.SourceEvent, prior.ObjectDeadline
	}
	operation := "media-legacy-delete:" + p.Object + ":" + p.SourceJob
	if err = a.protect(ctx, operation, "intent", "media.legacy_retention_delete", legacyMediaErasurePayload(p)); err != nil {
		return err
	}
	if _, err = a.DB.Exec(ctx, `INSERT INTO media.legacy_erasure_tasks(scope_id,object_id,message_id,restore_epoch,source_job_id,source_event_id,due_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(scope_id,object_id) DO NOTHING`, a.Config.DeploymentID, p.Object, p.Message, p.Epoch, p.SourceJob, p.SourceEvent, p.ObjectDeadline); err != nil {
		return err
	}
	if err = a.clearRetainedMediaBytes(ctx, p.Object, p.Message, key, operation); err != nil {
		return err
	}
	if err = a.protect(ctx, operation, "result", "media.legacy_retention_delete", legacyMediaErasurePayload(p)); err != nil {
		return err
	}
	_, err = a.DB.Exec(ctx, `UPDATE media.legacy_erasure_tasks SET complete=true WHERE scope_id=$1 AND object_id=$2 AND message_id=$3 AND source_job_id=$4`, a.Config.DeploymentID, p.Object, p.Message, p.SourceJob)
	return err
}

func (a *App) resumeLegacyMediaErasures(ctx context.Context) error {
	rows, err := a.DB.Query(ctx, `SELECT e.object_id::text,e.message_id::text,e.restore_epoch::text,e.source_job_id::text,e.source_event_id::text,e.due_at,o.storage_key FROM media.legacy_erasure_tasks e JOIN media.objects o ON o.id=e.object_id AND o.message_id=e.message_id AND o.purpose='chat_attachment' WHERE e.scope_id=$1 AND NOT e.complete AND e.due_at<=clock_timestamp() ORDER BY e.due_at,e.object_id LIMIT 16`, a.Config.DeploymentID)
	if err != nil {
		return err
	}
	type pending struct {
		plan retentionMediaPlan
		key  string
	}
	items := []pending{}
	for rows.Next() {
		item := pending{plan: retentionMediaPlan{Legacy: true, CleanupEligible: true, ErasureOnly: true}}
		if err = rows.Scan(&item.plan.Object, &item.plan.Message, &item.plan.Epoch, &item.plan.SourceJob, &item.plan.SourceEvent, &item.plan.ObjectDeadline, &item.key); err != nil {
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
		if err = a.clearLegacyRetainedMedia(ctx, item.plan, item.key); err != nil {
			return err
		}
	}
	return nil
}

func replayLegacyMediaErasure(ctx context.Context, business *pgx.Conn, scope string, q journalRecord) error {
	text := func(key string) string { value, _ := q.Payload[key].(string); return value }
	_, err := business.Exec(ctx, `INSERT INTO media.legacy_erasure_tasks(scope_id,object_id,message_id,restore_epoch,source_job_id,source_event_id,due_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(scope_id,object_id) DO UPDATE SET complete=false WHERE media.legacy_erasure_tasks.message_id=EXCLUDED.message_id`, scope, text("objectId"), text("messageId"), text("restoreEpoch"), text("sourceJobId"), text("sourceEventId"), text("dueAt"))
	return err
}
