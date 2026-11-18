package jobsim

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type retentionJob struct {
	ID, Message, Generation, Policy, Epoch, Reason, Scope, State, Token, Before, After string
	Due                                                                                time.Time
	Attempt                                                                            int
}

func (j retentionJob) safety() map[string]any {
	return map[string]any{"jobId": j.ID, "messageId": j.Message, "generation": j.Generation, "policyVersion": j.Policy, "restoreEpoch": j.Epoch, "reason": j.Reason, "deleteScope": j.Scope, "dueAt": j.Due.UTC().Format(time.RFC3339Nano), "beforeEventId": j.Before, "afterEventId": j.After}
}

const retentionJobColumns = `id::text,message_id::text,generation::text,policy_version::text,restore_epoch::text,reason,delete_scope,due_at,state,COALESCE(lease_token::text,''),before_event::text,after_event::text,attempt`

func scanRetentionJob(row pgx.Row) (retentionJob, error) {
	var j retentionJob
	err := row.Scan(&j.ID, &j.Message, &j.Generation, &j.Policy, &j.Epoch, &j.Reason, &j.Scope, &j.Due, &j.State, &j.Token, &j.Before, &j.After, &j.Attempt)
	return j, err
}

func (a *App) protectRetentionDeadlines(ctx context.Context, message string) error {
	rows, err := a.DB.Query(ctx, `SELECT `+retentionJobColumns+` FROM message.lifecycle_jobs WHERE scope_id=$1 AND message_id=$2 AND state='protection_pending' ORDER BY due_at,id`, a.Config.DeploymentID, message)
	if err != nil {
		return err
	}
	items := []retentionJob{}
	for rows.Next() {
		job, err := scanRetentionJob(rows)
		if err != nil {
			rows.Close()
			return err
		}
		items = append(items, job)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, job := range items {
		if err = a.protect(ctx, "retention-deadline:"+job.ID, "intent", "message.retention_deadline", job.safety()); err != nil {
			return err
		}
		if _, err = a.DB.Exec(ctx, `UPDATE message.lifecycle_jobs SET state='scheduled' WHERE scope_id=$1 AND id=$2 AND state='protection_pending'`, a.Config.DeploymentID, job.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) StartRetentionWorkers(ctx context.Context) {
	if a.Role != "message" {
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
				attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
				_ = a.resumeRetention(attempt, 16)
				cancel()
			}
		}
	}()
}

func (a *App) resumeRetention(ctx context.Context, limit int) error {
	rows, err := a.DB.Query(ctx, `SELECT DISTINCT message_id::text FROM message.lifecycle_jobs WHERE scope_id=$1 AND state='protection_pending' LIMIT $2`, a.Config.DeploymentID, limit)
	if err != nil {
		return err
	}
	messages := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		messages = append(messages, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range messages {
		if err = a.protectRetentionDeadlines(ctx, id); err != nil {
			return err
		}
	}
	for i := 0; i < limit; i++ {
		var worked bool
		fn := a.withAdmissionFence("DELETE /api/messages/{id}", func(_ http.ResponseWriter, r *http.Request) error {
			var err error
			worked, err = a.executeNextRetention(r.Context())
			return err
		})
		r, _ := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1/retention", nil)
		if err = fn(nil, r); err != nil {
			return err
		}
		if !worked {
			break
		}
	}
	return nil
}

func (a *App) executeNextRetention(ctx context.Context) (bool, error) {
	if err := a.retentionClockGuard(ctx); err != nil {
		return false, err
	}
	var currentEpoch string
	if err := a.DB.QueryRow(ctx, `SELECT restore_epoch::text FROM message.instance WHERE singleton`).Scan(&currentEpoch); err != nil {
		return false, err
	}
	epoch, err := a.epoch(ctx)
	if err != nil {
		return false, err
	}
	if currentEpoch != str(epoch) {
		return false, fail(503, "RESTORE_GATE_CLOSED", "Retention requires the independently verified current epoch")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	job, err := scanRetentionJob(tx.QueryRow(ctx, `SELECT `+retentionJobColumns+` FROM message.lifecycle_jobs WHERE scope_id=$1 AND due_at<=clock_timestamp() AND (state IN('scheduled','body_deleted') AND (lease_until IS NULL OR lease_until<clock_timestamp()) OR state='executing' AND lease_until<clock_timestamp()) ORDER BY CASE WHEN delete_scope='global' THEN 0 ELSE 1 END,due_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, a.Config.DeploymentID))
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	oldState := job.State
	job.Token = newID()
	job.Attempt++
	if _, err = tx.Exec(ctx, `UPDATE message.lifecycle_jobs SET lease_token=$3,lease_until=clock_timestamp()+interval '60 seconds',attempt=attempt+1,state=CASE WHEN state='body_deleted' THEN state ELSE 'executing' END WHERE scope_id=$1 AND id=$2`, a.Config.DeploymentID, job.ID, job.Token); err != nil {
		return false, err
	}
	if oldState != "body_deleted" {
		if err = a.retentionEvent(ctx, tx, job, job.Before, "before", ""); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	if oldState != "body_deleted" {
		if err = a.protect(ctx, "retention-delete:"+job.ID, "intent", "message.retention_delete", job.safety()); err != nil {
			return true, a.retentionFailure(ctx, job, err)
		}
		var version int64
		if err = a.DB.QueryRow(ctx, `SELECT version FROM message.messages WHERE id=$1`, job.Message).Scan(&version); err != nil {
			return true, a.retentionFailure(ctx, job, err)
		}
		// This existing synchronous source boundary drains exports, reference
		// forwards and draft holders before any cloud content is removed.
		operation := "retention:" + job.ID
		if err = a.freezeMessageContentExportsAt(ctx, job.Message, operation, version); err != nil {
			return true, a.retentionFailure(ctx, job, err)
		}
		if err = a.drainPersonalExportInvalidations(ctx); err != nil {
			return true, a.retentionFailure(ctx, job, err)
		}
		if err = a.retentionClockGuard(ctx); err != nil {
			return true, a.retentionFailure(ctx, job, err)
		}
		tx, err = a.DB.Begin(ctx)
		if err != nil {
			return true, err
		}
		defer tx.Rollback(ctx)
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT lease_token=$3::uuid AND lease_until>clock_timestamp() AND state='executing' FROM message.lifecycle_jobs WHERE scope_id=$1 AND id=$2 FOR UPDATE`, a.Config.DeploymentID, job.ID, job.Token).Scan(&valid); err != nil {
			return true, err
		}
		if !valid {
			return true, fail(409, "RETENTION_LEASE_CHANGED", "An old worker cannot commit after another lease")
		}
		var conversation, existingScope string
		var deleted bool
		if err = tx.QueryRow(ctx, `SELECT conversation_id::text,delete_scope,deleted FROM message.messages WHERE id=$1 FOR UPDATE`, job.Message).Scan(&conversation, &existingScope, &deleted); err != nil {
			return true, err
		}
		if job.Reason == "receipt_complete" {
			var generation string
			if err = tx.QueryRow(ctx, `SELECT generation::text FROM message.retention_assignments WHERE message_id=$1 FOR UPDATE`, job.Message).Scan(&generation); err != nil {
				return true, err
			}
			if generation != job.Generation {
				return true, fail(409, "RECEIPT_GENERATION_CHANGED", "An earlier completed range cannot erase an edited generation")
			}
		}
		global := job.Scope == "global" || deleted || existingScope == "global"
		seq, err := nextSequence(ctx, tx, conversation)
		if err != nil {
			return true, err
		}
		changed, err := tx.Exec(ctx, `UPDATE message.messages SET text='',content_unavailable=NOT $3,deleted=$3,delete_scope=CASE WHEN $3 THEN 'global' ELSE 'cloud_only' END,deletion_generation=deletion_generation+CASE WHEN delete_scope=CASE WHEN $3 THEN 'global' ELSE 'cloud_only' END THEN 0 ELSE 1 END,version=version+1,sequence=$4,updated_at=now(),export_pending_operation_id=NULL WHERE id=$1 AND export_pending_operation_id=$2`, job.Message, operation, global, seq)
		if err != nil {
			return true, err
		}
		if changed.RowsAffected() != 1 {
			return true, fail(409, "RETENTION_SOURCE_CHANGED", "The source freeze changed before erasure")
		}
		scope := "cloud_only"
		if global {
			scope = "global"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO message.retention_floors(scope_id,message_id,delete_scope,generation,due_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(scope_id,message_id) DO UPDATE SET delete_scope=CASE WHEN message.retention_floors.delete_scope='global' OR EXCLUDED.delete_scope='global' THEN 'global' ELSE 'cloud_only' END,generation=GREATEST(message.retention_floors.generation,EXCLUDED.generation),due_at=LEAST(message.retention_floors.due_at,EXCLUDED.due_at)`, a.Config.DeploymentID, job.Message, scope, job.Generation, job.Due); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO message.events(conversation_id,sequence,message_id) VALUES($1,$2,$3)`, conversation, seq, job.Message); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM message.pins WHERE message_id=$1`, job.Message); err != nil {
			return true, err
		}
		if err = a.retentionEvent(ctx, tx, job, job.After, "after", ""); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `UPDATE message.lifecycle_jobs SET state='body_deleted' WHERE scope_id=$1 AND id=$2 AND lease_token=$3`, a.Config.DeploymentID, job.ID, job.Token); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `UPDATE message.retention_assignments SET state=CASE WHEN $3 THEN 'global_deleted' ELSE 'cloud_deleted' END,updated_at=now() WHERE scope_id=$1 AND message_id=$2`, a.Config.DeploymentID, job.Message, global); err != nil {
			return true, err
		}
		if err = tx.Commit(ctx); err != nil {
			return true, err
		}
	}
	if err = a.protect(ctx, "retention-delete:"+job.ID, "result", "message.retention_delete", job.safety()); err != nil {
		return true, err
	}
	completed, err := a.DB.Exec(ctx, `UPDATE message.lifecycle_jobs SET state='complete',lease_token=NULL,lease_until=NULL,last_error=NULL WHERE scope_id=$1 AND id=$2 AND lease_token=$3 AND lease_until>clock_timestamp() AND state='body_deleted'`, a.Config.DeploymentID, job.ID, job.Token)
	if err == nil && completed.RowsAffected() != 1 {
		return true, fail(409, "RETENTION_LEASE_CHANGED", "An expired worker cannot report cleanup complete")
	}
	return true, err
}

func (a *App) retentionEvent(ctx context.Context, tx pgx.Tx, j retentionJob, id, phase, code string) error {
	payload := j.safety()
	payload["eventId"], payload["phase"], payload["attempt"], payload["errorCode"] = id, phase, j.Attempt, code
	raw, _ := json.Marshal(payload)
	_, err := tx.Exec(ctx, `INSERT INTO message.lifecycle_events(scope_id,event_id,job_id,phase,payload) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, id, j.ID, phase, raw)
	return err
}

func (a *App) retentionFailure(ctx context.Context, j retentionJob, cause error) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return cause
	}
	defer tx.Rollback(ctx)
	if err = a.retentionEvent(ctx, tx, j, stableID(a.Config.Key, j.ID+":failure:"+str(int64(j.Attempt))), "failure", "DEPENDENCY_OR_PROTECTION_PENDING"); err != nil {
		return cause
	}
	if _, err = tx.Exec(ctx, `UPDATE message.lifecycle_jobs SET last_error='DEPENDENCY_OR_PROTECTION_PENDING',lease_until=clock_timestamp()+interval '10 seconds' WHERE scope_id=$1 AND id=$2 AND lease_token=$3`, a.Config.DeploymentID, j.ID, j.Token); err != nil {
		return cause
	}
	_ = tx.Commit(ctx)
	return cause
}

// QueueRetentionErasure is the shared durable entry for future account erasure,
// group governance and ordinary expiry. The caller must already own the source
// EX fence and authorization; the job never recreates a deleted source.
// Legacy rows have no receipt eligibility: explicit erasure uses generation 1
// solely as a stable job identity, never as a fabricated delivery confirmation.
func (a *App) QueueRetentionErasure(ctx context.Context, message, operation, scope string, due time.Time) error {
	if a.Role != "message" || !syncValidationID(message) || !validReceiptNonce(operation) || scope != "global" && scope != "cloud_only" || due.IsZero() {
		return fail(400, "INVALID_RETENTION_ERASURE", "Authorized exact source, stable operation and scope required")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var generation, policy, epoch string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(a.generation,1)::text,COALESCE(a.policy_version,1)::text,i.restore_epoch::text FROM message.messages m LEFT JOIN message.retention_assignments a ON a.message_id=m.id CROSS JOIN message.instance i WHERE m.id=$1 AND i.singleton FOR UPDATE OF m`, message).Scan(&generation, &policy, &epoch); err != nil {
		return err
	}
	reason := "explicit_" + syncDigest([]byte(operation))
	if err = a.insertRetentionJob(ctx, tx, message, generation, policy, epoch, reason, scope, due); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return a.protectRetentionDeadlines(ctx, message)
}
