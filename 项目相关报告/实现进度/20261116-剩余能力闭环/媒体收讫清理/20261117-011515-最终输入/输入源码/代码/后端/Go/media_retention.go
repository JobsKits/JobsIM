package jobsim

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["media"] += `CREATE TABLE IF NOT EXISTS media.retention_floors(scope_id uuid NOT NULL,object_id uuid NOT NULL,message_id uuid NOT NULL,restore_epoch bigint NOT NULL,content_version bigint NOT NULL,generation bigint NOT NULL,policy_version bigint NOT NULL,manifest_digest text NOT NULL,due_at timestamptz NOT NULL,terminal boolean NOT NULL DEFAULT false,bytes_cleared boolean NOT NULL DEFAULT false,result_recorded boolean NOT NULL DEFAULT false,checked_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,object_id));
ALTER TABLE media.retention_floors ADD COLUMN IF NOT EXISTS result_recorded boolean NOT NULL DEFAULT false;
ALTER TABLE media.objects ADD COLUMN IF NOT EXISTS retention_checked_at timestamptz;
CREATE TABLE IF NOT EXISTS media.lifecycle_clock(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),last_database_time timestamptz NOT NULL,alarmed boolean NOT NULL);
CREATE INDEX IF NOT EXISTS media_retention_due ON media.retention_floors(bytes_cleared,due_at);`
}

func mediaRetentionJournalKinds() map[string][]string {
	fields := []string{"objectId", "messageId", "restoreEpoch", "contentVersion", "receiptGeneration", "policyVersion", "manifestDigest", "dueAt"}
	return map[string][]string{"media.retention_deadline": fields, "media.retention_delete": fields, "media.legacy_retention_delete": {"objectId", "messageId", "restoreEpoch", "sourceJobId", "sourceEventId", "dueAt"}}
}

func mediaPlanDeadline(p retentionMediaPlan, now time.Time) (time.Time, error) {
	if !syncValidationID(p.Message) || !syncValidationID(p.Object) || !syncValidationCounter(p.Content, false) || !syncValidationCounter(p.Generation, false) || !syncValidationCounter(p.Policy, false) || !syncValidationCounter(p.Epoch, false) || !syncValidationDigest(p.Digest) || p.ObjectDeadline.IsZero() || p.ObjectDeadline.Unix() <= 0 || p.DeleteScope != "" && p.DeleteScope != "cloud_only" && p.DeleteScope != "global" {
		return time.Time{}, fail(503, "MEDIA_RETENTION_PLAN_INVALID", "Original bounded object retention plan required")
	}
	due := p.ObjectDeadline.UTC()
	if p.AttachmentMode == "after_object_receipt" {
		if p.ObjectDelay == nil || *p.ObjectDelay < 0 || *p.ObjectDelay > 604800 {
			return time.Time{}, fail(503, "MEDIA_RETENTION_PLAN_INVALID", "Explicit independent object receipt delay required")
		}
		if p.ObjectCompleteAt != nil {
			if p.ObjectCompleteAt.IsZero() || p.ObjectCompleteAt.After(now) {
				return time.Time{}, fail(503, "MEDIA_RETENTION_PLAN_INVALID", "Object completion time cannot be in the future")
			}
			if receiptDue := p.ObjectCompleteAt.Add(time.Duration(*p.ObjectDelay) * time.Second); receiptDue.Before(due) {
				due = receiptDue.UTC()
			}
		}
	} else if p.AttachmentMode != "fixed_retention" || p.ObjectDelay != nil {
		return time.Time{}, fail(503, "MEDIA_RETENTION_PLAN_INVALID", "Supported independent object mode required")
	}
	if p.DeleteScope == "global" && now.Before(due) {
		due = now.UTC()
	}
	return due, nil
}

func mediaRetentionPayload(p retentionMediaPlan, due time.Time) map[string]any {
	return map[string]any{"objectId": p.Object, "messageId": p.Message, "restoreEpoch": p.Epoch, "contentVersion": p.Content, "receiptGeneration": p.Generation, "policyVersion": p.Policy, "manifestDigest": p.Digest, "dueAt": due.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)}
}

func mediaRetentionOperation(kind, object string, due time.Time) string {
	// PostgreSQL timestamps persist microseconds. Hash the same representation
	// before and after crash/restore so the typed result continues its intent.
	digest := sha256.Sum256([]byte(due.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)))
	return "media-" + kind + ":" + object + ":" + hex.EncodeToString(digest[:])
}

// The journal floor is committed before grants depend on it. Content edits and
// restored older business snapshots can only shorten this absolute deadline.
func (a *App) mediaRetentionPlan(ctx context.Context, message, object string) (retentionMediaPlan, time.Time, error) {
	var p retentionMediaPlan
	retired, err := a.legacyMediaRetired(ctx, message, object)
	if err != nil {
		return p, time.Time{}, err
	}
	if retired {
		return p, time.Time{}, fail(410, "OBJECT_RETIRED", "Original legacy object is permanently retired")
	}
	if err := a.rpc(ctx, "message", "/internal/retention/media-plan", map[string]string{"messageId": message, "objectId": object}, &p); err != nil {
		return p, time.Time{}, err
	}
	if p.Message != message || p.Object != object {
		return p, time.Time{}, fail(503, "MEDIA_RETENTION_SOURCE_CHANGED", "Original object binding differs")
	}
	if p.ErasureOnly {
		if err := validateLegacyMediaErasure(p); err != nil {
			return p, time.Time{}, err
		}
		return p, p.ObjectDeadline, nil
	}
	if p.Legacy && !p.CleanupEligible {
		var due time.Time
		var terminal bool
		err := a.DB.QueryRow(ctx, `SELECT due_at,terminal FROM media.retention_floors WHERE scope_id=$1 AND object_id=$2 AND message_id=$3`, a.Config.DeploymentID, object, message).Scan(&due, &terminal)
		if err == pgx.ErrNoRows {
			return p, time.Time{}, nil
		}
		if err != nil {
			return p, due, err
		}
		if terminal {
			return p, due, fail(410, "OBJECT_RETIRED", "A legacy label cannot override a protected retirement")
		}
		return p, due, nil
	}
	if p.Legacy || !p.CleanupEligible {
		return p, time.Time{}, fail(503, "MEDIA_RETENTION_PLAN_INVALID", "An explicit accepted object cleanup plan is required")
	}
	due, err := mediaPlanDeadline(p, time.Now().UTC())
	if err != nil {
		return p, due, err
	}
	var previous time.Time
	var terminal bool
	err = a.DB.QueryRow(ctx, `SELECT due_at,terminal FROM media.retention_floors WHERE scope_id=$1 AND object_id=$2 AND message_id=$3`, a.Config.DeploymentID, object, message).Scan(&previous, &terminal)
	if err != nil && err != pgx.ErrNoRows {
		return p, due, err
	}
	if err == nil && !due.Before(previous) {
		_, err = a.DB.Exec(ctx, `UPDATE media.retention_floors SET checked_at=now() WHERE scope_id=$1 AND object_id=$2`, a.Config.DeploymentID, object)
		if terminal {
			return p, previous, fail(410, "OBJECT_RETIRED", "Original object is permanently retired")
		}
		return p, previous, err
	}
	if err = a.protect(ctx, mediaRetentionOperation("deadline", object, due), "intent", "media.retention_deadline", mediaRetentionPayload(p, due)); err != nil {
		return p, due, err
	}
	_, err = a.DB.Exec(ctx, `INSERT INTO media.retention_floors(scope_id,object_id,message_id,restore_epoch,content_version,generation,policy_version,manifest_digest,due_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(scope_id,object_id) DO UPDATE SET due_at=LEAST(media.retention_floors.due_at,EXCLUDED.due_at),checked_at=now() WHERE media.retention_floors.message_id=EXCLUDED.message_id`, a.Config.DeploymentID, object, message, p.Epoch, p.Content, p.Generation, p.Policy, p.Digest, due)
	if err != nil {
		return p, due, err
	}
	if err = a.DB.QueryRow(ctx, `SELECT due_at,terminal FROM media.retention_floors WHERE scope_id=$1 AND object_id=$2 AND message_id=$3`, a.Config.DeploymentID, object, message).Scan(&due, &terminal); err != nil {
		return p, due, err
	}
	if terminal {
		return p, due, fail(410, "OBJECT_RETIRED", "Original object is permanently retired")
	}
	return p, due, nil
}

func (a *App) requireMediaRetention(ctx context.Context, message, object string) error {
	p, due, err := a.mediaRetentionPlan(ctx, message, object)
	if err == nil && due.IsZero() && p.Legacy && !p.CleanupEligible {
		return nil
	}
	if err == nil && !time.Now().Before(due) {
		err = fail(410, "OBJECT_RETENTION_EXPIRED", "Independent object retention has expired")
	}
	return err
}

type mediaRetainedReader struct {
	*os.File
	ctx context.Context
	due time.Time
}

func (r *mediaRetainedReader) Read(body []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if !r.due.IsZero() && !time.Now().Before(r.due) {
		return 0, io.ErrClosedPipe
	}
	return r.File.Read(body)
}

func (a *App) mediaReadDeadline(ctx context.Context, message, object string) (time.Time, error) {
	var due time.Time
	err := a.DB.QueryRow(ctx, `SELECT due_at FROM media.retention_floors WHERE scope_id=$1 AND object_id=$2 AND message_id=$3 AND NOT terminal AND due_at>clock_timestamp()`, a.Config.DeploymentID, object, message).Scan(&due)
	if err == pgx.ErrNoRows {
		p, legacyDue, planErr := a.mediaRetentionPlan(ctx, message, object)
		if planErr == nil && p.Legacy && !p.CleanupEligible && legacyDue.IsZero() {
			return time.Time{}, nil
		}
	}
	if err != nil {
		return due, fail(410, "OBJECT_RETENTION_EXPIRED", "Original object grant has expired")
	}
	return due, nil
}

func (a *App) mediaAttachmentAccess(ctx context.Context, message, object, user, device, store, epoch, content string) error {
	if !syncValidationID(user) || !syncValidationID(device) || !syncValidationCounter(epoch, false) || store != "" && !syncValidationID(store) {
		return fail(400, "INVALID_ATTACHMENT_DEVICE", "Current physical device and optional registered store required")
	}
	var result struct {
		Allowed bool   `json:"allowed"`
		Object  string `json:"attachmentId"`
		Content string `json:"contentVersion"`
	}
	if err := a.rpc(ctx, "message", "/internal/retention/attachment-access", map[string]string{"messageId": message, "objectId": object, "userId": user, "deviceId": device, "storeGeneration": store, "restoreEpoch": epoch}, &result); err != nil {
		return err
	}
	if !result.Allowed || result.Object != object || content != "" && result.Content != content {
		return fail(403, "ATTACHMENT_UNAVAILABLE", "Current source permission required")
	}
	return a.requireMediaRetention(ctx, message, object)
}

func (a *App) StartMediaRetentionWorkers(ctx context.Context) {
	if a.Role != "media" {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			child, cancel := context.WithTimeout(ctx, 10*time.Second)
			_ = a.resumeMediaRetention(child)
			cancel()
		}
	}()
}

func (a *App) resumeMediaRetention(ctx context.Context) error {
	// The same exclusive local fence drains public downloads, forward projections
	// and export reads before any physical byte or grant is retired.
	lock, err := a.admissionFence(ctx, true)
	if err != nil {
		return err
	}
	defer closeExportFence(lock)
	ctx, expire := a.heldAdmissionScope(ctx, "exclusive")
	defer expire()
	markers, err := a.admissionMarkers()
	if err != nil {
		return err
	}
	if len(markers) > 0 {
		if err = a.rpc(ctx, "message", "/internal/admission/recover", map[string]string{}, nil); err != nil {
			return err
		}
	}
	if err = a.resumeLegacyMediaErasures(ctx); err != nil {
		return err
	}
	rows, err := a.DB.Query(ctx, `SELECT o.id::text,o.message_id::text,o.storage_key FROM media.objects o LEFT JOIN media.retention_floors f ON f.scope_id=$1 AND f.object_id=o.id WHERE o.purpose='chat_attachment' AND o.message_id IS NOT NULL AND NOT COALESCE(f.result_recorded,false) AND NOT EXISTS(SELECT 1 FROM media.legacy_erasure_tasks e WHERE e.scope_id=$1 AND e.object_id=o.id AND e.complete) ORDER BY COALESCE(o.retention_checked_at,'epoch'::timestamptz),o.created_at LIMIT 16`, a.Config.DeploymentID)
	if err != nil {
		return err
	}
	type candidate struct{ object, message, key string }
	items := []candidate{}
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.object, &item.message, &item.key); err != nil {
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
	var first error
	for _, item := range items {
		if _, err = a.DB.Exec(ctx, `UPDATE media.objects SET retention_checked_at=now() WHERE id=$1`, item.object); err != nil {
			return err
		}
		p, due, planErr := a.mediaRetentionPlan(ctx, item.message, item.object)
		if planErr == nil && p.ErasureOnly {
			if err = a.clearLegacyRetainedMedia(ctx, p, item.key); err != nil && first == nil {
				first = err
			}
			continue
		}
		if planErr != nil {
			// A persisted retirement can resume even when the message owner no
			// longer exposes its old source. It can never grant bytes again.
			var terminal bool
			if e := a.DB.QueryRow(ctx, `SELECT terminal,due_at FROM media.retention_floors WHERE scope_id=$1 AND object_id=$2 AND message_id=$3`, a.Config.DeploymentID, item.object, item.message).Scan(&terminal, &due); e != nil || !terminal && time.Now().Before(due) {
				if first == nil {
					first = planErr
				}
				continue
			}
		} else if due.IsZero() || time.Now().Before(due) {
			continue
		}
		if err = a.clearRetainedMedia(ctx, p, item.object, item.message, item.key, due); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (a *App) clearRetainedMedia(ctx context.Context, p retentionMediaPlan, object, message, key string, due time.Time) error {
	if err := a.mediaRetentionClockGuard(ctx); err != nil {
		return err
	}
	if key != object || !syncValidationID(key) {
		return fail(503, "MEDIA_STORAGE_INTEGRITY", "Only the original private UUID storage key may be removed")
	}
	var terminal bool
	if err := a.DB.QueryRow(ctx, `SELECT terminal,object_id::text,message_id::text,restore_epoch::text,content_version::text,generation::text,policy_version::text,manifest_digest,due_at FROM media.retention_floors WHERE scope_id=$1 AND object_id=$2 AND message_id=$3`, a.Config.DeploymentID, object, message).Scan(&terminal, &p.Object, &p.Message, &p.Epoch, &p.Content, &p.Generation, &p.Policy, &p.Digest, &due); err != nil {
		return err
	}
	if !terminal {
		if err := a.protect(ctx, mediaRetentionOperation("delete", object, due), "intent", "media.retention_delete", mediaRetentionPayload(p, due)); err != nil {
			return err
		}
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `UPDATE media.retention_floors SET terminal=true WHERE scope_id=$1 AND object_id=$2 AND message_id=$3;`, a.Config.DeploymentID, object, message); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	if err := a.clearRetainedMediaBytes(ctx, object, message, key, mediaRetentionOperation("delete", object, due)); err != nil {
		return err
	}
	_, err := a.DB.Exec(ctx, `UPDATE media.retention_floors SET bytes_cleared=true,checked_at=now() WHERE scope_id=$1 AND object_id=$2 AND message_id=$3 AND terminal`, a.Config.DeploymentID, object, message)
	if err != nil {
		return err
	}
	if err = a.protect(ctx, mediaRetentionOperation("delete", object, due), "result", "media.retention_delete", mediaRetentionPayload(p, due)); err != nil {
		return err
	}
	_, err = a.DB.Exec(ctx, `UPDATE media.retention_floors SET result_recorded=true WHERE scope_id=$1 AND object_id=$2 AND message_id=$3 AND terminal AND bytes_cleared`, a.Config.DeploymentID, object, message)
	return err
}

func (a *App) clearRetainedMediaBytes(ctx context.Context, object, message, key, operation string) error {
	if key != object || !syncValidationID(key) {
		return fail(503, "MEDIA_STORAGE_INTEGRITY", "Only the original private UUID storage key may be removed")
	}
	// Derived archives must acknowledge exact job cancellation before unlinking
	// their source. A failed acknowledgement leaves original bytes quarantined.
	rows, err := a.DB.Query(ctx, `SELECT DISTINCT job_id::text FROM media.export_holders WHERE object_id=$1 AND message_id=$2 AND state<>'cleared' LIMIT 1000`, object, message)
	if err != nil {
		return err
	}
	jobs := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(jobs) > 0 {
		if err = a.rpc(ctx, "governance", "/internal/exports/invalidate", map[string]any{"jobIds": jobs, "operationId": operation}, nil); err != nil {
			return err
		}
		if _, err = a.DB.Exec(ctx, `UPDATE media.export_holders SET state='cleared' WHERE object_id=$1 AND message_id=$2 AND job_id=ANY($3::uuid[])`, object, message, jobs); err != nil {
			return err
		}
		var pending bool
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.export_holders WHERE object_id=$1 AND message_id=$2 AND state<>'cleared')`, object, message).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return fail(503, "MEDIA_EXPORT_CLEANUP_PENDING", "Additional exact source holders still require cancellation")
		}
	}
	path := filepath.Join(a.Config.StateDir, "Media", key)
	if err = a.mediaRetentionClockGuard(ctx); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fail(503, "MEDIA_STORAGE_INTEGRITY", "Cleanup refuses a nonregular private object")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = syncAdmissionDirectory(filepath.Join(a.Config.StateDir, "Media")); err != nil {
		return err
	}
	return nil
}

// Media can use its own database. Its irreversible file cleanup therefore has
// its own durable clock alarm, independent of the message owner's schema.
func (a *App) mediaRetentionClockGuard(ctx context.Context) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO media.lifecycle_clock(singleton,last_database_time,alarmed) VALUES(true,clock_timestamp(),false) ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	var previous, current time.Time
	var alarm bool
	if err = tx.QueryRow(ctx, `SELECT last_database_time,clock_timestamp(),alarmed FROM media.lifecycle_clock WHERE singleton FOR UPDATE`).Scan(&previous, &current, &alarm); err != nil {
		return err
	}
	sampled := time.Now()
	bad := alarm || current.Before(previous.Add(-time.Second)) || current.Sub(sampled) > 5*time.Second || sampled.Sub(current) > 5*time.Second
	if value, ok := retentionClockObservations.Load(a); ok {
		last := value.(retentionClockObservation)
		drift := current.Sub(last.Database) - sampled.Sub(last.Monotonic)
		bad = bad || drift > 5*time.Second || drift < -5*time.Second
	}
	if bad {
		if _, err = tx.Exec(ctx, `UPDATE media.lifecycle_clock SET alarmed=true WHERE singleton`); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return fail(503, "MEDIA_RETENTION_CLOCK_UNVERIFIED", "Independent clock audit required before irreversible media cleanup")
	}
	if _, err = tx.Exec(ctx, `UPDATE media.lifecycle_clock SET last_database_time=$1 WHERE singleton`, current); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	retentionClockObservations.Store(a, retentionClockObservation{current, sampled})
	return nil
}

func ReplayMediaRetentionSafety(ctx context.Context, business *pgx.Conn, q journalRecord) (bool, error) {
	if q.Kind != "media.retention_deadline" && q.Kind != "media.retention_delete" && q.Kind != "media.legacy_retention_delete" {
		return false, nil
	}
	if q.Owner != "media" {
		return true, fmt.Errorf("media retention owner differs")
	}
	text := func(k string) string { v, _ := q.Payload[k].(string); return v }
	var scope string
	if err := business.QueryRow(ctx, `SELECT deployment_id FROM media.instance WHERE singleton`).Scan(&scope); err != nil {
		return true, err
	}
	if q.Kind == "media.legacy_retention_delete" {
		return true, replayLegacyMediaErasure(ctx, business, scope, q)
	}
	_, err := business.Exec(ctx, `INSERT INTO media.retention_floors(scope_id,object_id,message_id,restore_epoch,content_version,generation,policy_version,manifest_digest,due_at,terminal) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(scope_id,object_id) DO UPDATE SET due_at=LEAST(media.retention_floors.due_at,EXCLUDED.due_at),terminal=media.retention_floors.terminal OR EXCLUDED.terminal,bytes_cleared=false WHERE media.retention_floors.message_id=EXCLUDED.message_id`, scope, text("objectId"), text("messageId"), text("restoreEpoch"), text("contentVersion"), text("receiptGeneration"), text("policyVersion"), text("manifestDigest"), text("dueAt"), q.Kind == "media.retention_delete")
	return true, err
}
