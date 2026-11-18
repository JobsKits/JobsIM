package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type draftReplyRevocationRequest struct {
	Holder      draftReplyHolder `json:"holder"`
	OperationID string           `json:"operationId"`
}

func migrateDraftReplyRevocations(ctx context.Context, tx pgx.Tx, scope string) error {
	if !syncValidationID(scope) {
		return fail(400, "INVALID_DRAFT_SCOPE", "Canonical draft-reply scope required")
	}
	var deployment string
	if err := tx.QueryRow(ctx, `SELECT deployment_id FROM message.instance WHERE singleton FOR SHARE`).Scan(&deployment); err != nil {
		return err
	}
	if deployment != scope {
		return fail(409, "DRAFT_SCOPE_CHANGED", "Draft-reply revocations belong to another deployment")
	}
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS message.draft_reply_revocations(
scope_id uuid NOT NULL,operation_id text NOT NULL CHECK(octet_length(operation_id) BETWEEN 8 AND 512),
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','complete')),
created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,operation_id));`)
	return err
}

func validDraftReplyRevocation(q forwardRevocation) bool {
	if len(q.OperationID) < 8 || len(q.OperationID) > 512 || q.SourceMessageID == "" && q.ForwarderID == "" {
		return false
	}
	for _, id := range []string{q.SourceMessageID, q.ForwarderID, q.SourceConversationID} {
		if id != "" && !syncValidationID(id) {
			return false
		}
	}
	if q.SourceConversationID != "" && q.ForwarderID == "" || q.MembershipGeneration != "" && (q.ForwarderID == "" || q.SourceConversationID == "" || !syncValidationCounter(q.MembershipGeneration, false)) {
		return false
	}
	return true
}

const draftReplyRevocationSelector = `scope_id=$5 AND source_context_id='personal' AND state IN('prepared','active','revoking') AND (
(NULLIF($1,'')::uuid IS NOT NULL AND source_message_id=NULLIF($1,'')::uuid)
OR(NULLIF($2,'')::uuid IS NOT NULL AND user_id=NULLIF($2,'')::uuid AND($3='' OR conversation_id=NULLIF($3,'')::uuid) AND($4='' OR membership_generation=NULLIF($4,'')::bigint)))`

// The caller retains the source's EX host fence. Every PG transaction commits
// before a callback; the sync callback must not reacquire that host fence.
func (a *App) beginDraftReplyRevocation(ctx context.Context, q forwardRevocation) (bool, error) {
	if a.Role != "message" || a.DB == nil || !validDraftReplyRevocation(q) {
		return false, fail(400, "INVALID_DRAFT_REVOCATION", "Original message-owner draft revocation required")
	}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "draft-reply-revocation-v1:"+a.Config.DeploymentID, raw)
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO message.draft_reply_revocations(scope_id,operation_id,request_digest) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, q.OperationID, digest); err != nil {
		return false, err
	}
	var original []byte
	var state string
	if err = tx.QueryRow(ctx, `SELECT request_digest,state FROM message.draft_reply_revocations WHERE scope_id=$1 AND operation_id=$2 FOR UPDATE`, a.Config.DeploymentID, q.OperationID).Scan(&original, &state); err != nil {
		return false, err
	}
	if !hmac.Equal(original, digest) {
		return false, fail(409, "DRAFT_REPLY_REVOCATION_CONFLICT", "Original draft revocation selector differs")
	}
	if state == "complete" {
		return true, tx.Commit(ctx)
	}
	arguments := []any{q.SourceMessageID, q.ForwarderID, q.SourceConversationID, q.MembershipGeneration, a.Config.DeploymentID}
	rows, err := tx.Query(ctx, `SELECT revocation_operation_id FROM message.draft_reply_holders WHERE `+draftReplyRevocationSelector+` ORDER BY id FOR UPDATE`, arguments...)
	if err != nil {
		return false, err
	}
	conflict := false
	for rows.Next() {
		var pending *string
		if err = rows.Scan(&pending); err != nil {
			rows.Close()
			return false, err
		}
		conflict = conflict || pending != nil && *pending != q.OperationID
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	if conflict {
		return false, fail(409, "DRAFT_REPLY_REVOCATION_CONFLICT", "Another original operation owns an unresolved draft holder")
	}
	arguments = append(arguments, q.OperationID)
	if _, err = tx.Exec(ctx, `UPDATE message.draft_reply_holders SET state='revoking',revocation_operation_id=$6,updated_at=now() WHERE `+draftReplyRevocationSelector, arguments...); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

func scanDraftReplyRevocation(row pgx.Row) (draftReplyRevocationRequest, error) {
	var request draftReplyRevocationRequest
	holder := &request.Holder
	err := row.Scan(&holder.HolderID, &holder.UserID, &holder.ConversationID, &holder.DraftID, &holder.OperationID, &holder.SourceID, &holder.SourceVersion, &holder.MembershipGeneration, &holder.SecurityVersion, &holder.RestoreEpoch, &holder.State, &request.OperationID)
	return request, err
}

// A worker invocation processes at most one batch. No holder, job or deadline
// substitutes for the callback's durable sync-owner acknowledgement.
func (a *App) drainDraftReplyRevocations(ctx context.Context, operation string) (int, error) {
	rows, err := a.DB.Query(ctx, `SELECT `+draftReplyHolderColumns+`,revocation_operation_id FROM message.draft_reply_holders WHERE scope_id=$1 AND source_context_id='personal' AND state='revoking' AND($2='' OR revocation_operation_id=$2) ORDER BY updated_at,id LIMIT 100`, a.Config.DeploymentID, operation)
	if err != nil {
		return 0, err
	}
	batch := []draftReplyRevocationRequest{}
	for rows.Next() {
		request, err := scanDraftReplyRevocation(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, request)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	completed := 0
	for _, request := range batch {
		if err = a.rpc(ctx, "sync", "/internal/drafts/revoke-reply", request, nil); err != nil {
			return completed, err
		}
		result, err := a.DB.Exec(ctx, `UPDATE message.draft_reply_holders SET state='cleared',revocation_operation_id=NULL,updated_at=now() WHERE scope_id=$1 AND source_context_id='personal' AND id=$2 AND user_id=$3 AND state='revoking' AND revocation_operation_id=$4`, a.Config.DeploymentID, request.Holder.HolderID, request.Holder.UserID, request.OperationID)
		if err != nil {
			return completed, err
		}
		if result.RowsAffected() != 1 {
			var cleared bool
			if err = a.DB.QueryRow(ctx, `SELECT state='cleared' FROM message.draft_reply_holders WHERE scope_id=$1 AND id=$2 AND user_id=$3`, a.Config.DeploymentID, request.Holder.HolderID, request.Holder.UserID).Scan(&cleared); err != nil || !cleared {
				return completed, fail(409, "DRAFT_REPLY_REVOCATION_CONFLICT", "Draft holder ownership changed before completion")
			}
		}
		completed++
	}
	_, err = a.DB.Exec(ctx, `UPDATE message.draft_reply_revocations j SET state='complete',updated_at=now() WHERE j.scope_id=$1 AND j.state='pending' AND NOT EXISTS(SELECT 1 FROM message.draft_reply_holders h WHERE h.scope_id=j.scope_id AND h.source_context_id='personal' AND h.state='revoking' AND h.revocation_operation_id=j.operation_id)`, a.Config.DeploymentID)
	return completed, err
}

func (a *App) requireDraftReplyRevocation(ctx context.Context, q forwardRevocation) error {
	complete, err := a.beginDraftReplyRevocation(ctx, q)
	if err != nil || complete {
		return err
	}
	for ctx.Err() == nil {
		processed, err := a.drainDraftReplyRevocations(ctx, q.OperationID)
		if err != nil {
			return fail(http.StatusServiceUnavailable, "DRAFT_REPLY_REVOCATION_PENDING", "Draft reply revocation is durable and still draining; continue the original operation")
		}
		var done bool
		if a.DB.QueryRow(ctx, `SELECT state='complete' FROM message.draft_reply_revocations WHERE scope_id=$1 AND operation_id=$2`, a.Config.DeploymentID, q.OperationID).Scan(&done) != nil {
			break
		}
		if done {
			return nil
		}
		if processed == 0 {
			break
		}
	}
	return fail(http.StatusServiceUnavailable, "DRAFT_REPLY_REVOCATION_PENDING", "Draft reply revocation is durable and still draining; continue the original operation")
}

func (a *App) StartDraftReplyWorkers(ctx context.Context) {
	if a.Role != "message" || a.DB == nil {
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
				attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
				lock, err := a.admissionFence(attempt, true)
				if err == nil {
					if a.recoverAdmissionMarkers(attempt) == nil {
						_, _ = a.drainDraftReplyRevocations(attempt, "")
					}
					closeExportFence(lock)
				}
				cancel()
				ticker.Reset(time.Second)
			}
		}
	}()
}
