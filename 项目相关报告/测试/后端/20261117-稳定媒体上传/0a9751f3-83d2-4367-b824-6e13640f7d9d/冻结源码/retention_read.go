package jobsim

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// A worker outage does not extend a known hard deadline. An unprotected local
// deadline denies new cloud content but does not claim global local deletion.
func (a *App) retentionReadState(ctx context.Context, m *Message) error {
	if m.retentionChecked {
		return nil
	}
	if a.DB == nil {
		return fail(503, "RETENTION_STATE_UNAVAILABLE", "Current lifecycle owner is unavailable")
	}
	var scope, state string
	// The account floor takes effect before bounded erasure reaches this row.
	err := a.DB.QueryRow(ctx, `SELECT delete_scope,state FROM (
SELECT 'global'::text AS delete_scope,'account_erasure'::text AS state,0 AS priority,'-infinity'::timestamptz AS due_at
WHERE EXISTS(SELECT 1 FROM message.messages m JOIN message.account_erasure_floors f ON f.user_id=m.sender_id WHERE m.id=$2)
UNION ALL
SELECT delete_scope,state,CASE WHEN delete_scope='global' THEN 1 ELSE 2 END,due_at FROM message.lifecycle_jobs WHERE scope_id=$1 AND message_id=$2 AND due_at<=clock_timestamp()
) denied ORDER BY priority,due_at LIMIT 1`, a.Config.DeploymentID, m.ID).Scan(&scope, &state)
	if err == pgx.ErrNoRows {
		m.retentionChecked = true
		return nil
	}
	if err != nil {
		return err
	}
	m.retentionChecked = true
	m.Text, m.AttachmentID = "", ""
	m.ContentUnavailable = true
	if state != "protection_pending" {
		m.DeleteScope = scope
		if scope == "global" {
			m.Deleted = true
			m.ContentUnavailable = false
		}
	}
	return nil
}

func (a *App) retentionRequireCloud(ctx context.Context, id string) error {
	m := Message{ID: id}
	if err := a.retentionReadState(ctx, &m); err != nil {
		return err
	}
	if m.Deleted || m.ContentUnavailable {
		return fail(409, "RETENTION_CLOUD_EXPIRED", "The original cloud deadline has elapsed")
	}
	return nil
}
