package jobsim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func retentionJournalKinds() map[string][]string {
	fields := []string{"jobId", "messageId", "generation", "policyVersion", "restoreEpoch", "reason", "deleteScope", "dueAt", "beforeEventId", "afterEventId"}
	return map[string][]string{"message.retention_deadline": fields, "message.retention_delete": fields}
}

func validateRetentionJournalField(field string, raw any) (bool, error) {
	value, ok := raw.(string)
	valid := false
	switch field {
	case "reason":
		valid = ok && (value == "receipt_complete" || value == "undelivered_expired" || value == "history_expired" || strings.HasPrefix(value, "explicit_") && syncValidationDigest(strings.TrimPrefix(value, "explicit_")))
	case "deleteScope":
		valid = ok && (value == "cloud_only" || value == "global")
	case "dueAt":
		t, err := time.Parse(time.RFC3339Nano, value)
		valid = ok && err == nil && t.Unix() > 0
	case "policyVersion":
		valid = ok && syncValidationCounter(value, false)
	default:
		return false, nil
	}
	if !valid {
		return true, fail(400, "INVALID_JOURNAL_PAYLOAD", "Invalid minimal retention safety field")
	}
	return true, nil
}

func (a *App) retentionSendFloor(ctx context.Context, message string) error {
	var denied bool
	err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message.retention_floors WHERE scope_id=$1 AND message_id=$2)`, a.Config.DeploymentID, message).Scan(&denied)
	if err != nil {
		return err
	}
	if denied {
		return fail(409, "HISTORICAL_RESULT_UNAVAILABLE", "A protected deleted original identity cannot be created again")
	}
	return nil
}

// Deadline replay preserves the original absolute time. Delete intents retain a
// monotone floor even when the old business snapshot predates the source row.
func ReplayRetentionSafety(ctx context.Context, business *pgx.Conn, q journalRecord) (bool, error) {
	if q.Kind != "message.retention_deadline" && q.Kind != "message.retention_delete" {
		return false, nil
	}
	text := func(key string) string { v, _ := q.Payload[key].(string); return v }
	if q.Owner != "message" {
		return true, fmt.Errorf("retention safety owner differs")
	}
	tx, err := business.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(ctx)
	var scope string
	if err = tx.QueryRow(ctx, `SELECT deployment_id FROM message.instance WHERE singleton`).Scan(&scope); err != nil {
		return true, err
	}
	if q.Kind == "message.retention_delete" {
		_, err = tx.Exec(ctx, `INSERT INTO message.retention_floors(scope_id,message_id,delete_scope,generation,due_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(scope_id,message_id) DO UPDATE SET delete_scope=CASE WHEN message.retention_floors.delete_scope='global' OR EXCLUDED.delete_scope='global' THEN 'global' ELSE 'cloud_only' END,generation=GREATEST(message.retention_floors.generation,EXCLUDED.generation),due_at=LEAST(message.retention_floors.due_at,EXCLUDED.due_at)`, scope, text("messageId"), text("deleteScope"), text("generation"), text("dueAt"))
		if err != nil {
			return true, err
		}
		_, err = tx.Exec(ctx, `UPDATE message.messages SET text='',content_unavailable=NOT (delete_scope='global' OR $2='global'),deleted=deleted OR $2='global',delete_scope=CASE WHEN delete_scope='global' OR $2='global' THEN 'global' ELSE 'cloud_only' END,export_pending_operation_id=NULL WHERE id=$1`, text("messageId"), text("deleteScope"))
		if err != nil {
			return true, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO message.lifecycle_jobs(scope_id,id,message_id,generation,policy_version,restore_epoch,reason,delete_scope,due_at,state,before_event,after_event) SELECT $1,$2,id,$4,$5,$6,$7,$8,$9,'scheduled',$10,$11 FROM message.messages WHERE id=$3 ON CONFLICT(id) DO UPDATE SET due_at=LEAST(message.lifecycle_jobs.due_at,EXCLUDED.due_at),lease_token=NULL,lease_until=NULL,state=CASE WHEN message.lifecycle_jobs.state='complete' THEN 'complete' ELSE 'scheduled' END`, scope, text("jobId"), text("messageId"), text("generation"), text("policyVersion"), text("restoreEpoch"), text("reason"), text("deleteScope"), text("dueAt"), text("beforeEventId"), text("afterEventId"))
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}
