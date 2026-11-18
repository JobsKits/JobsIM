package jobsim

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["message"] += `
CREATE TABLE IF NOT EXISTS message.forward_invalidation_jobs(operation_id text PRIMARY KEY,request_digest bytea NOT NULL,state text NOT NULL DEFAULT 'pending',created_at timestamptz NOT NULL DEFAULT now(),last_attempt_at timestamptz);
CREATE TABLE IF NOT EXISTS message.forward_invalidation_items(operation_id text NOT NULL REFERENCES message.forward_invalidation_jobs(operation_id),message_id uuid NOT NULL,state text NOT NULL DEFAULT 'pending',PRIMARY KEY(operation_id,message_id));
CREATE TABLE IF NOT EXISTS message.forward_worker_cursors(owner text PRIMARY KEY,cursor text NOT NULL);`
}

type forwardRevocation struct {
	OperationID          string `json:"operationId"`
	SourceMessageID      string `json:"sourceMessageId,omitempty"`
	ForwarderID          string `json:"forwarderId,omitempty"`
	SourceConversationID string `json:"sourceConversationId,omitempty"`
	MembershipGeneration string `json:"membershipGeneration,omitempty"`
}

type pendingForwardRevocations struct {
	Items  []forwardRevocation `json:"items"`
	Cursor string              `json:"cursor"`
}

func (a *App) beginForwardRevocation(ctx context.Context, q forwardRevocation) error {
	if len(q.OperationID) < 8 || len(q.OperationID) > 512 || q.SourceMessageID == "" && q.ForwarderID == "" {
		return fail(400, "INVALID_FORWARD_REVOCATION", "Original source operation required")
	}
	if err := a.requireDraftReplyRevocation(ctx, q); err != nil {
		return err
	}
	raw, _ := json.Marshal(q)
	digest := tokenDigest(string(raw))
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `INSERT INTO message.forward_invalidation_jobs(operation_id,request_digest) VALUES($1,$2) ON CONFLICT DO NOTHING`, q.OperationID, digest)
	if e != nil {
		return e
	}
	var original []byte
	var state string
	if e = tx.QueryRow(ctx, `SELECT request_digest,state FROM message.forward_invalidation_jobs WHERE operation_id=$1 FOR UPDATE`, q.OperationID).Scan(&original, &state); e != nil {
		return e
	}
	if !exportEqualDigest(original, digest) {
		return fail(409, "FORWARD_REVOCATION_CHANGED", "Original source revocation differs")
	}
	if state == "complete" {
		return tx.Commit(ctx)
	}
	_, e = tx.Exec(ctx, `WITH RECURSIVE affected(id) AS (
SELECT message_id FROM message.forward_refs WHERE state<>'unavailable' AND ((NULLIF($2,'')::uuid IS NOT NULL AND source_message_id=NULLIF($2,'')::uuid) OR(NULLIF($3,'')::uuid IS NOT NULL AND forwarder_id=NULLIF($3,'')::uuid AND($4='' OR source_conversation_id=NULLIF($4,'')::uuid) AND($5='' OR source_membership_generation=NULLIF($5,'')::bigint)))
UNION SELECT f.message_id FROM message.forward_refs f JOIN affected parent ON f.source_message_id=parent.id WHERE f.state<>'unavailable')
INSERT INTO message.forward_invalidation_items(operation_id,message_id) SELECT $1,id FROM affected ON CONFLICT DO NOTHING`, q.OperationID, q.SourceMessageID, q.ForwarderID, q.SourceConversationID, q.MembershipGeneration)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE message.forward_refs f SET state='invalidating' FROM message.forward_invalidation_items i WHERE i.operation_id=$1 AND i.message_id=f.message_id AND f.state='active'`, q.OperationID)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// References are already durably invalidating. Do not steal an unrelated
// original edit/retraction's pending row while draining derived holders.
func (a *App) freezeForwardHolders(ctx context.Context, message, operation string) error {
	draftOperation := "draft-derived:" + stableID(a.Config.Key, "draft-derived-revocation:"+operation+":"+message)
	if err := a.requireDraftReplyRevocation(ctx, forwardRevocation{OperationID: draftOperation, SourceMessageID: message}); err != nil {
		return err
	}
	rows, e := a.DB.Query(ctx, `UPDATE message.export_holders SET state='revoking' WHERE message_id=$1 AND state IN('prepared','active','revoking') RETURNING job_id::text`, message)
	if e != nil {
		return e
	}
	jobs := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		jobs = append(jobs, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	if len(jobs) > 0 {
		if e = a.rpc(ctx, "governance", "/internal/exports/invalidate", map[string]any{"jobIds": jobs, "operationId": operation}, nil); e != nil {
			return e
		}
	}
	_, e = a.DB.Exec(ctx, `UPDATE message.export_holders SET state='cleared' WHERE message_id=$1 AND state='revoking'`, message)
	return e
}

func (a *App) finishForwardRevocation(ctx context.Context, operation string) (bool, error) {
	if _, e := a.DB.Exec(ctx, `UPDATE message.forward_invalidation_jobs SET last_attempt_at=now() WHERE operation_id=$1`, operation); e != nil {
		return false, e
	}
	rows, e := a.DB.Query(ctx, `SELECT message_id::text FROM message.forward_invalidation_items WHERE operation_id=$1 AND state='pending' ORDER BY message_id LIMIT 100`, operation)
	if e != nil {
		return false, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return false, e
		}
		ids = append(ids, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return false, e
	}
	for _, id := range ids {
		if e = a.freezeForwardHolders(ctx, id, operation); e != nil {
			return false, e
		}
		if e = a.commitForwardPlaceholder(ctx, operation, id); e != nil {
			return false, e
		}
	}
	var complete bool
	e = a.DB.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM message.forward_invalidation_items WHERE operation_id=$1 AND state='pending')`, operation).Scan(&complete)
	if e != nil {
		return false, e
	}
	if complete {
		_, e = a.DB.Exec(ctx, `UPDATE message.forward_invalidation_jobs SET state='complete' WHERE operation_id=$1 AND NOT EXISTS(SELECT 1 FROM message.forward_invalidation_items WHERE operation_id=$1 AND state='pending')`, operation)
	}
	return complete, e
}

func (a *App) commitForwardPlaceholder(ctx context.Context, operation, id string) error {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var conversation, state string
	e = tx.QueryRow(ctx, `SELECT m.conversation_id::text,f.state FROM message.forward_refs f JOIN message.messages m ON m.id=f.message_id WHERE f.message_id=$1 FOR UPDATE OF f`, id).Scan(&conversation, &state)
	if e != nil {
		return e
	}
	if state != "unavailable" {
		sequence, e := nextSequence(ctx, tx, conversation)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE message.messages SET text='',attachment_id=NULL,version=version+1,content_version=content_version+1,sequence=$2,updated_at=now() WHERE id=$1`, id, sequence); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE message.forward_refs SET state='unavailable' WHERE message_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO message.events(conversation_id,sequence,message_id) VALUES($1,$2,$3)`, conversation, sequence, id); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE message.forward_invalidation_items SET state='complete' WHERE operation_id=$1 AND message_id=$2`, operation, id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func (a *App) requireForwardRevocation(ctx context.Context, q forwardRevocation) error {
	if e := a.beginForwardRevocation(ctx, q); e != nil {
		return e
	}
	complete, e := a.finishForwardRevocation(ctx, q.OperationID)
	if e != nil || !complete {
		return fail(503, "FORWARD_CLEANUP_PENDING", "Source revocation is durable and still draining; continue the original operation")
	}
	return nil
}

func (a *App) forwardRevocationRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	for _, path := range []string{"/internal/forwards/revoke-principal", "/internal/forwards/revoke-membership", "/internal/forwards/revoke-source"} {
		h("POST "+path, internal(func(w http.ResponseWriter, r *http.Request) error {
			var q forwardRevocation
			if e := decode(r, &q); e != nil {
				return e
			}
			if e := a.requireForwardRevocation(r.Context(), q); e != nil {
				return e
			}
			respond(w, 200, map[string]string{"state": "complete"})
			return nil
		}))
	}
}

func (a *App) forwardPendingIdentityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/forwards/pending-principals", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			After string `json:"after"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		rows, e := a.DB.Query(r.Context(), `SELECT id::text,pending_operation_id FROM auth.users WHERE pending_operation_id IS NOT NULL AND id::text>$1 ORDER BY id LIMIT 100`, q.After)
		if e != nil {
			return e
		}
		defer rows.Close()
		page := pendingForwardRevocations{Items: []forwardRevocation{}}
		for rows.Next() {
			var id, pending string
			if e = rows.Scan(&id, &pending); e != nil {
				return e
			}
			page.Items = append(page.Items, forwardRevocation{OperationID: "principal:" + id + ":" + pending, ForwarderID: id})
			page.Cursor = id
		}
		if e = rows.Err(); e != nil {
			return e
		}
		respond(w, 200, page)
		return nil
	}))
}

func (a *App) forwardPendingConversationRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/forwards/pending-memberships", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			After string `json:"after"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		rows, e := a.DB.Query(r.Context(), `SELECT conversation_id::text,user_id::text,membership_generation,export_pending_operation_id FROM conversation.members WHERE export_pending_operation_id IS NOT NULL AND(conversation_id::text||':'||user_id::text)>$1 ORDER BY conversation_id,user_id LIMIT 100`, q.After)
		if e != nil {
			return e
		}
		defer rows.Close()
		page := pendingForwardRevocations{Items: []forwardRevocation{}}
		for rows.Next() {
			var conversation, user, pending string
			var generation int64
			if e = rows.Scan(&conversation, &user, &generation, &pending); e != nil {
				return e
			}
			page.Items = append(page.Items, forwardRevocation{OperationID: pending, ForwarderID: user, SourceConversationID: conversation, MembershipGeneration: str(generation)})
			page.Cursor = conversation + ":" + user
		}
		if e = rows.Err(); e != nil {
			return e
		}
		respond(w, 200, page)
		return nil
	}))
}

func (a *App) collectForwardOwnerPending(ctx context.Context, owner, path string) error {
	var after string
	e := a.DB.QueryRow(ctx, `SELECT cursor FROM message.forward_worker_cursors WHERE owner=$1`, owner).Scan(&after)
	if e != nil && e != pgx.ErrNoRows {
		return e
	}
	var page pendingForwardRevocations
	if e = a.rpc(ctx, owner, path, map[string]string{"after": after}, &page); e != nil {
		return e
	}
	for _, item := range page.Items {
		if e = a.beginForwardRevocation(ctx, item); e != nil {
			return e
		}
	}
	_, e = a.DB.Exec(ctx, `INSERT INTO message.forward_worker_cursors(owner,cursor) VALUES($1,$2) ON CONFLICT(owner) DO UPDATE SET cursor=excluded.cursor`, owner, page.Cursor)
	return e
}

func (a *App) resumeForwardSources(ctx context.Context) error {
	rows, e := a.DB.Query(ctx, `SELECT id::text,export_pending_operation_id FROM message.messages WHERE export_pending_operation_id LIKE 'edit:%' OR export_pending_operation_id LIKE 'retract:%' ORDER BY id LIMIT 100`)
	if e != nil {
		return e
	}
	pending := []forwardRevocation{}
	for rows.Next() {
		var id, operation string
		if e = rows.Scan(&id, &operation); e != nil {
			rows.Close()
			return e
		}
		pending = append(pending, forwardRevocation{OperationID: operation, SourceMessageID: id})
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, item := range pending {
		if e = a.beginForwardRevocation(ctx, item); e != nil {
			return e
		}
	}
	rows, e = a.DB.Query(ctx, `SELECT operation_id FROM message.forward_invalidation_jobs WHERE state='pending' ORDER BY last_attempt_at NULLS FIRST,created_at LIMIT 10`)
	if e != nil {
		return e
	}
	operations := []string{}
	for rows.Next() {
		var operation string
		if e = rows.Scan(&operation); e != nil {
			rows.Close()
			return e
		}
		operations = append(operations, operation)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, operation := range operations {
		_, _ = a.finishForwardRevocation(ctx, operation)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	for _, owner := range []string{"identity", "conversation"} {
		path := "/internal/forwards/pending-principals"
		if owner == "conversation" {
			path = "/internal/forwards/pending-memberships"
		}
		_ = a.collectForwardOwnerPending(ctx, owner, path)
	}
	return nil
}

func (a *App) StartForwardWorkers(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
				lock, e := a.admissionFence(attempt, true)
				if e == nil {
					if e = a.recoverAdmissionMarkers(attempt); e == nil {
						_ = a.resumeForwardSources(attempt)
					}
					closeExportFence(lock)
				}
				cancel()
				ticker.Reset(time.Second)
			}
		}
	}()
}
