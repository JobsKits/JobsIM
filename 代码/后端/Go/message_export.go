package jobsim

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func init() {
	migrations["message"] += `
ALTER TABLE message.messages ADD COLUMN IF NOT EXISTS export_pending_operation_id text;
CREATE TABLE IF NOT EXISTS message.export_manifests(job_id uuid PRIMARY KEY,user_id uuid NOT NULL,request_digest bytea NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS message.export_manifest_items(job_id uuid NOT NULL,message_id uuid NOT NULL,conversation_id uuid NOT NULL,source_version bigint NOT NULL,attachment_id uuid,PRIMARY KEY(job_id,message_id));
CREATE TABLE IF NOT EXISTS message.export_holders(job_id uuid NOT NULL,artifact_id uuid NOT NULL,message_id uuid NOT NULL REFERENCES message.messages(id),user_id uuid NOT NULL,restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,source_version bigint NOT NULL,state text NOT NULL,journal_seq bigint,expires_at timestamptz NOT NULL,PRIMARY KEY(job_id,message_id));`
}

type exportSource struct {
	MessageID          string `json:"messageId"`
	ConversationID     string `json:"conversationId"`
	SourceVersion      string `json:"sourceVersion"`
	AttachmentID       string `json:"attachmentId,omitempty"`
	ForwardPlaceholder bool   `json:"forwardPlaceholder,omitempty"`
}
type exportHolderRequest struct {
	Personal        bool      `json:"personal,omitempty"`
	JobID           string    `json:"jobId"`
	ArtifactID      string    `json:"artifactId"`
	UserID          string    `json:"userId"`
	MessageID       string    `json:"messageId"`
	ObjectID        string    `json:"objectId,omitempty"`
	RestoreEpoch    string    `json:"restoreEpoch"`
	BuildGeneration string    `json:"buildGeneration"`
	SourceVersion   string    `json:"sourceVersion"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

func (a *App) exportMessageRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.exportSourceCleanupRoutes(h, internal)
	h("POST /internal/exports/manifest", internal(a.messageExportManifest))
	h("POST /internal/exports/register-message", internal(a.registerMessageExport))
	for _, path := range []string{"/internal/exports/read-message", "/internal/exports/check-message"} {
		endpoint := path
		h("POST "+endpoint, internal(func(w http.ResponseWriter, r *http.Request) error {
			return a.readMessageExport(w, r, endpoint == "/internal/exports/read-message")
		}))
	}
}
func (a *App) messageExportManifest(w http.ResponseWriter, r *http.Request) error {
	var q struct {
		JobID           string    `json:"jobId"`
		UserID          string    `json:"userId"`
		ConversationIDs []string  `json:"conversationIds"`
		Before          time.Time `json:"before"`
		After           string    `json:"after"`
		Personal        bool      `json:"personal,omitempty"`
	}
	if e := decode(r, &q); e != nil {
		return e
	}
	if !validAssetID(q.JobID) || !validAssetID(q.UserID) || len(q.ConversationIDs) > 1000 {
		return fail(400, "INVALID_EXPORT_SCOPE", "Bound export scope required")
	}
	if e := a.rpc(r.Context(), "governance", "/internal/exports/build", map[string]string{"jobId": q.JobID, "userId": q.UserID}, nil); e != nil {
		return e
	}
	if q.Personal {
		q.Before = q.Before.UTC()
		if e := a.requirePersonalExportBuild(r.Context(), q.JobID, q.UserID); e != nil {
			return e
		}
	}
	groupIDs := []string{}
	groupJoined := []time.Time{}
	for _, id := range q.ConversationIDs {
		meta, e := a.member(r.Context(), q.UserID, id)
		if e != nil {
			return e
		}
		if meta["kind"] == "group" {
			raw, ok := meta["joinedAt"].(string)
			if !ok {
				return fail(403, "HISTORY_NOT_ALLOWED", "Current membership history boundary required")
			}
			joined, e := time.Parse(time.RFC3339Nano, raw)
			if e != nil {
				return fail(403, "HISTORY_NOT_ALLOWED", "Membership history boundary invalid")
			}
			groupIDs = append(groupIDs, id)
			groupJoined = append(groupJoined, joined)
		}
	}
	manifestScope := map[string]any{"userId": q.UserID, "conversations": q.ConversationIDs, "before": q.Before}
	if q.Personal {
		manifestScope["personal"] = true
	}
	raw, _ := json.Marshal(manifestScope)
	digest := keyedDigest(a.Config.Key, "export-manifest", raw)
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "export-manifest:"+q.JobID); e != nil {
		return e
	}
	var original []byte
	e = tx.QueryRow(r.Context(), `SELECT request_digest FROM message.export_manifests WHERE job_id=$1`, q.JobID).Scan(&original)
	if e == pgx.ErrNoRows {
		if _, e = tx.Exec(r.Context(), `INSERT INTO message.export_manifests(job_id,user_id,request_digest) VALUES($1,$2,$3)`, q.JobID, q.UserID, digest); e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO message.export_manifest_items(job_id,message_id,conversation_id,source_version,attachment_id) SELECT $1,id,conversation_id,version,attachment_id FROM message.messages WHERE conversation_id=ANY($2::uuid[]) AND NOT deleted AND NOT content_unavailable AND export_pending_operation_id IS NULL AND created_at<=$3 AND(NOT(conversation_id=ANY($4::uuid[])) OR created_at>=(SELECT visibility.joined_at FROM unnest($4::uuid[],$5::timestamptz[]) AS visibility(conversation_id,joined_at) WHERE visibility.conversation_id=message.messages.conversation_id)) ORDER BY id LIMIT 10001`, q.JobID, q.ConversationIDs, q.Before, groupIDs, groupJoined)
		if e != nil {
			return e
		}
		var count int
		if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM message.export_manifest_items WHERE job_id=$1`, q.JobID).Scan(&count); e != nil {
			return e
		}
		if count > 10000 {
			return fail(413, "EXPORT_MESSAGE_CAPACITY", "Export slice limit is 10000 messages; no partial package is published")
		}
	} else if e != nil {
		return e
	} else if !exportEqualDigest(original, digest) {
		return fail(409, "EXPORT_SCOPE_CHANGED", "Original export scope differs")
	}
	var containsForward bool
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM message.export_manifest_items i JOIN message.messages m ON m.id=i.message_id WHERE i.job_id=$1 AND m.type='forward')`, q.JobID).Scan(&containsForward); e != nil {
		return e
	}
	if containsForward && !q.Personal {
		return fail(409, "FORWARD_EXPORT_UNSUPPORTED", "This export contains forward references; chain-level export holders are not implemented")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	rows, e := a.DB.Query(r.Context(), `SELECT i.message_id::text,i.conversation_id::text,i.source_version,CASE WHEN m.type='forward' THEN '' ELSE COALESCE(i.attachment_id::text,'') END,m.type='forward' FROM message.export_manifest_items i JOIN message.messages m ON m.id=i.message_id WHERE i.job_id=$1 AND($2='' OR i.message_id>NULLIF($2,'')::uuid) ORDER BY i.message_id LIMIT 200`, q.JobID, q.After)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []exportSource{}
	after := q.After
	for rows.Next() {
		var item exportSource
		var version int64
		if e = rows.Scan(&item.MessageID, &item.ConversationID, &version, &item.AttachmentID, &item.ForwardPlaceholder); e != nil {
			return e
		}
		item.SourceVersion = str(version)
		items = append(items, item)
		after = item.MessageID
	}
	if e = rows.Err(); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"items": items, "cursor": after, "complete": len(items) < 200})
	return nil
}
func exportEqualDigest(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
func (a *App) registerMessageExport(w http.ResponseWriter, r *http.Request) error {
	var q exportHolderRequest
	if e := decode(r, &q); e != nil {
		return e
	}
	if !validAssetID(q.JobID) || !validAssetID(q.ArtifactID) || !validAssetID(q.MessageID) || !validAssetID(q.UserID) {
		return fail(400, "INVALID_EXPORT_HOLDER", "Bound holder identity required")
	}
	if e := a.rpc(r.Context(), "governance", "/internal/exports/build", map[string]string{"jobId": q.JobID, "userId": q.UserID, "artifactId": q.ArtifactID, "restoreEpoch": q.RestoreEpoch, "buildGeneration": q.BuildGeneration}, nil); e != nil {
		return e
	}
	if q.Personal {
		if e := a.requirePersonalExportBuild(r.Context(), q.JobID, q.UserID); e != nil {
			return e
		}
	}
	var conversation, kind string
	var created time.Time
	if e := a.DB.QueryRow(r.Context(), `SELECT conversation_id::text,created_at,type FROM message.messages WHERE id=$1`, q.MessageID).Scan(&conversation, &created, &kind); e != nil {
		return fail(404, "MESSAGE_NOT_FOUND", "Export source unavailable")
	}
	if kind == "forward" && !q.Personal {
		return fail(409, "FORWARD_EXPORT_UNSUPPORTED", "Forward chain export holders are not implemented")
	}
	meta, e := a.member(r.Context(), q.UserID, conversation)
	if e != nil {
		return e
	}
	if meta["kind"] == "group" {
		joined, _ := time.Parse(time.RFC3339Nano, meta["joinedAt"].(string))
		if created.Before(joined) {
			return fail(403, "HISTORY_NOT_ALLOWED", "Export source predates membership")
		}
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var version int64
	var deleted, unavailable bool
	var pending *string
	if e = tx.QueryRow(r.Context(), `SELECT version,deleted,content_unavailable,export_pending_operation_id FROM message.messages WHERE id=$1 FOR UPDATE`, q.MessageID).Scan(&version, &deleted, &unavailable, &pending); e != nil {
		return e
	}
	if deleted || unavailable || pending != nil || str(version) != q.SourceVersion {
		return fail(409, "EXPORT_SOURCE_CHANGED", "Source was changed or frozen")
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO message.export_holders(job_id,artifact_id,message_id,user_id,restore_epoch,build_generation,source_version,state,expires_at) VALUES($1,$2,$3,$4,$5::bigint,$6::bigint,$7,'prepared',$8) ON CONFLICT DO NOTHING`, q.JobID, q.ArtifactID, q.MessageID, q.UserID, q.RestoreEpoch, q.BuildGeneration, version, q.ExpiresAt)
	if e != nil {
		return e
	}
	var valid bool
	var expires time.Time
	if e = tx.QueryRow(r.Context(), `SELECT artifact_id=$3 AND user_id=$4 AND restore_epoch=$5::bigint AND build_generation=$6::bigint AND source_version=$7 AND state IN('prepared','active'),expires_at FROM message.export_holders WHERE job_id=$1 AND message_id=$2`, q.JobID, q.MessageID, q.ArtifactID, q.UserID, q.RestoreEpoch, q.BuildGeneration, version).Scan(&valid, &expires); e != nil {
		return e
	}
	if !valid {
		return fail(409, "EXPORT_HOLDER_CHANGED", "Original holder is no longer active")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	payload := map[string]any{"messageId": q.MessageID, "userId": q.UserID, "jobId": q.JobID, "artifactId": q.ArtifactID, "buildGeneration": q.BuildGeneration, "restoreEpoch": q.RestoreEpoch, "sourceVersion": q.SourceVersion, "expiresAt": expires.UTC().Format(time.RFC3339Nano)}
	operation := "export-holder:" + q.JobID + ":" + q.MessageID
	var ack struct {
		JournalSeq string `json:"journalSeq"`
	}
	if e = a.rpc(r.Context(), "journal", "/internal/append", journalRecord{a.Role, operation, "intent", "message.export_holder", payload}, &ack); e != nil {
		return e
	}
	_, e = a.DB.Exec(r.Context(), `UPDATE message.export_holders h SET state='active',journal_seq=$3::bigint FROM message.messages m WHERE h.job_id=$1 AND h.message_id=$2 AND m.id=h.message_id AND m.export_pending_operation_id IS NULL AND NOT m.deleted AND NOT m.content_unavailable AND m.version=h.source_version AND h.state IN('prepared','active')`, q.JobID, q.MessageID, ack.JournalSeq)
	if e != nil {
		return e
	}
	if e = a.protect(r.Context(), operation, "result", "message.export_holder", payload); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"registrationId": operation, "journalSeq": ack.JournalSeq})
	return nil
}
func (a *App) readMessageExport(w http.ResponseWriter, r *http.Request, body bool) error {
	var q exportHolderRequest
	if e := decode(r, &q); e != nil {
		return e
	}
	if e := a.rpc(r.Context(), "governance", "/internal/exports/build", map[string]string{"jobId": q.JobID, "userId": q.UserID, "artifactId": q.ArtifactID, "restoreEpoch": q.RestoreEpoch, "buildGeneration": q.BuildGeneration}, nil); e != nil {
		return e
	}
	if q.Personal {
		if e := a.requirePersonalExportBuild(r.Context(), q.JobID, q.UserID); e != nil {
			return e
		}
	}
	var conversation, kind string
	var created time.Time
	if e := a.DB.QueryRow(r.Context(), `SELECT conversation_id::text,created_at,type FROM message.messages WHERE id=$1`, q.MessageID).Scan(&conversation, &created, &kind); e != nil {
		return fail(404, "MESSAGE_NOT_FOUND", "Export source unavailable")
	}
	if kind == "forward" && !q.Personal {
		return fail(409, "FORWARD_EXPORT_UNSUPPORTED", "Forward chain export holders are not implemented")
	}
	meta, e := a.member(r.Context(), q.UserID, conversation)
	if e != nil {
		return e
	}
	if meta["kind"] == "group" {
		joined, _ := time.Parse(time.RFC3339Nano, meta["joinedAt"].(string))
		if created.Before(joined) {
			return fail(403, "HISTORY_NOT_ALLOWED", "Export source predates membership")
		}
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var valid bool
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM message.export_holders h WHERE h.job_id=$2 AND h.message_id=m.id AND h.user_id=$3 AND h.artifact_id=$4 AND h.restore_epoch=$5::bigint AND h.build_generation=$6::bigint AND h.source_version=m.version AND h.state='active' AND h.journal_seq IS NOT NULL AND h.expires_at>now()) AND NOT m.deleted AND NOT m.content_unavailable AND m.export_pending_operation_id IS NULL FROM message.messages m WHERE m.id=$1 FOR SHARE`, q.MessageID, q.JobID, q.UserID, q.ArtifactID, q.RestoreEpoch, q.BuildGeneration).Scan(&valid); e != nil {
		return e
	}
	if !valid {
		return fail(409, "EXPORT_SOURCE_CHANGED", "Source holder is no longer valid")
	}
	if body {
		m, e := scanMessage(tx.QueryRow(r.Context(), `SELECT `+messageColumns+` FROM message.messages WHERE id=$1`, q.MessageID))
		if e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
		if kind == "forward" {
			// A target placeholder has no source body, source identity, URL or
			// temporary admission. No chain-level export grant is implied.
			respond(w, 200, map[string]any{"id": m.ID, "conversationId": m.ConversationID, "senderId": m.SenderID, "type": "forward", "forwarded": true, "forwardUnavailable": true, "exportState": "forward_placeholder", "createdAt": m.CreatedAt, "updatedAt": m.UpdatedAt, "version": m.Version, "contentVersion": m.ContentVersion})
			return nil
		}
		if e = a.enrich(r.Context(), &m, q.UserID); e != nil {
			return e
		}
		m.ForwardAdmission = ""
		m.ReplyAdmission = ""
		m.ClientMsgID = ""
		respond(w, 200, m)
	} else {
		var attachment *string
		if e = tx.QueryRow(r.Context(), `SELECT attachment_id::text FROM message.messages WHERE id=$1`, q.MessageID).Scan(&attachment); e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
		respond(w, 200, map[string]any{"valid": true, "attachmentId": attachment})
	}
	return nil
}

// Call before an edit/retraction changes the source. The caller must clear the
// matching pending operation in the same transaction that commits that change.
func (a *App) freezeMessageExports(ctx context.Context, message, operation string) error {
	return a.freezeMessageExportsAt(ctx, message, operation, 0)
}

func (a *App) freezeMessageExportsAt(ctx context.Context, message, operation string, expectedVersion int64) error {
	return a.freezeMessageSourcesAt(ctx, message, operation, expectedVersion, false)
}
func (a *App) freezeMessageContentExportsAt(ctx context.Context, message, operation string, expectedVersion int64) error {
	return a.freezeMessageSourcesAt(ctx, message, operation, expectedVersion, true)
}
func (a *App) freezeMessageSourcesAt(ctx context.Context, message, operation string, expectedVersion int64, content bool) error {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var pending *string
	var version int64
	if e = tx.QueryRow(ctx, `SELECT export_pending_operation_id,version FROM message.messages WHERE id=$1 FOR UPDATE`, message).Scan(&pending, &version); e != nil {
		return e
	}
	if pending != nil && *pending != operation {
		return fail(409, "SOURCE_OPERATION_PENDING", "Another source operation owns the export fence")
	}
	if expectedVersion > 0 && version != expectedVersion {
		return fail(409, "VERSION_CONFLICT", "Source changed before export freeze")
	}
	if _, e = tx.Exec(ctx, `UPDATE message.messages SET export_pending_operation_id=$2 WHERE id=$1`, message, operation); e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `UPDATE message.export_holders SET state='revoking' WHERE message_id=$1 AND state IN('prepared','active','revoking') RETURNING job_id::text`, message)
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
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	if content {
		if e = a.beginForwardRevocation(ctx, forwardRevocation{OperationID: operation, SourceMessageID: message}); e != nil {
			return e
		}
	}
	if len(jobs) > 0 {
		if e = a.rpc(ctx, "governance", "/internal/exports/invalidate", map[string]any{"jobIds": jobs, "operationId": operation}, nil); e != nil {
			return e
		}
	}
	_, e = a.DB.Exec(ctx, `UPDATE message.export_holders SET state='cleared' WHERE message_id=$1 AND state='revoking'`, message)
	if e != nil {
		return e
	}
	if content {
		return a.requireForwardRevocation(ctx, forwardRevocation{OperationID: operation, SourceMessageID: message})
	}
	return nil
}
