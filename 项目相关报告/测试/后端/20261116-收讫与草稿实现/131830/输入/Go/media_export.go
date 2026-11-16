package jobsim

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func init() {
	migrations["media"] += `CREATE TABLE IF NOT EXISTS media.export_holders(job_id uuid NOT NULL,artifact_id uuid NOT NULL,object_id uuid NOT NULL REFERENCES media.objects(id),message_id uuid NOT NULL,user_id uuid NOT NULL,restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,state text NOT NULL,journal_seq bigint,expires_at timestamptz NOT NULL,PRIMARY KEY(job_id,object_id,message_id));`
}
func (a *App) exportMediaRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.exportSourceCleanupRoutes(h, internal)
	h("POST /internal/exports/register-media", internal(a.registerMediaExport))
	h("POST /internal/exports/read-media", internal(a.readMediaExport))
}
func (a *App) checkMediaExportSource(ctx context.Context, q exportHolderRequest) error {
	var proof struct {
		AttachmentID *string `json:"attachmentId"`
	}
	if e := a.rpc(ctx, "message", "/internal/exports/check-message", q, &proof); e != nil {
		return e
	}
	if proof.AttachmentID == nil || *proof.AttachmentID != q.ObjectID {
		return fail(403, "EXPORT_ATTACHMENT_CHANGED", "Attachment does not belong to the registered source")
	}
	return nil
}
func (a *App) registerMediaExport(w http.ResponseWriter, r *http.Request) error {
	var q exportHolderRequest
	if e := decode(r, &q); e != nil {
		return e
	}
	if e := a.checkMediaExportSource(r.Context(), q); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var owner, sourceMessage, purpose string
	var created time.Time
	e = tx.QueryRow(r.Context(), `SELECT owner_id::text,COALESCE(message_id::text,''),purpose,created_at FROM media.objects WHERE id=$1 FOR UPDATE`, q.ObjectID).Scan(&owner, &sourceMessage, &purpose, &created)
	if e != nil {
		return fail(404, "EXPORT_MEDIA_UNAVAILABLE", "Source media unavailable")
	}
	if purpose != "chat_attachment" || sourceMessage != q.MessageID {
		return fail(403, "EXPORT_ATTACHMENT_CHANGED", "Attachment source differs")
	}
	expires := created.Add(30 * 24 * time.Hour)
	if q.ExpiresAt.Before(expires) {
		expires = q.ExpiresAt
	}
	if !time.Now().Before(expires) {
		return fail(403, "EXPORT_MEDIA_EXPIRED", "Source media expired")
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO media.export_holders(job_id,artifact_id,object_id,message_id,user_id,restore_epoch,build_generation,state,expires_at) VALUES($1,$2,$3,$4,$5,$6::bigint,$7::bigint,'prepared',$8) ON CONFLICT DO NOTHING`, q.JobID, q.ArtifactID, q.ObjectID, q.MessageID, q.UserID, q.RestoreEpoch, q.BuildGeneration, expires)
	if e != nil {
		return e
	}
	var valid bool
	e = tx.QueryRow(r.Context(), `SELECT artifact_id=$5 AND user_id=$6 AND restore_epoch=$7::bigint AND build_generation=$8::bigint AND state IN('prepared','active'),expires_at FROM media.export_holders WHERE job_id=$1 AND object_id=$2 AND message_id=$3 AND user_id=$4`, q.JobID, q.ObjectID, q.MessageID, q.UserID, q.ArtifactID, q.UserID, q.RestoreEpoch, q.BuildGeneration).Scan(&valid, &expires)
	if e != nil {
		return e
	}
	if !valid {
		return fail(409, "EXPORT_HOLDER_CHANGED", "Media holder is no longer active")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	payload := map[string]any{"objectId": q.ObjectID, "messageId": q.MessageID, "userId": q.UserID, "jobId": q.JobID, "artifactId": q.ArtifactID, "buildGeneration": q.BuildGeneration, "restoreEpoch": q.RestoreEpoch, "expiresAt": expires.UTC().Format(time.RFC3339Nano)}
	operation := "export-media-holder:" + q.JobID + ":" + q.ObjectID
	var ack struct {
		JournalSeq string `json:"journalSeq"`
	}
	if e = a.rpc(r.Context(), "journal", "/internal/append", journalRecord{a.Role, operation, "intent", "media.export_holder", payload}, &ack); e != nil {
		return e
	}
	if _, e = a.DB.Exec(r.Context(), `UPDATE media.export_holders SET state='active',journal_seq=$4::bigint WHERE job_id=$1 AND object_id=$2 AND message_id=$3 AND state IN('prepared','active')`, q.JobID, q.ObjectID, q.MessageID, ack.JournalSeq); e != nil {
		return e
	}
	if e = a.protect(r.Context(), operation, "result", "media.export_holder", payload); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"registrationId": operation, "journalSeq": ack.JournalSeq, "expiresAt": expires})
	return nil
}
func (a *App) readMediaExport(w http.ResponseWriter, r *http.Request) error {
	var q exportHolderRequest
	if e := decode(r, &q); e != nil {
		return e
	}
	if e := a.checkMediaExportSource(r.Context(), q); e != nil {
		return e
	}
	var key string
	var size int64
	var hash []byte
	e := a.DB.QueryRow(r.Context(), `SELECT o.storage_key,o.size_bytes,o.sha256 FROM media.objects o JOIN media.export_holders h ON h.object_id=o.id WHERE h.job_id=$1 AND h.object_id=$2 AND h.message_id=$3 AND h.user_id=$4 AND h.artifact_id=$5 AND h.restore_epoch=$6::bigint AND h.build_generation=$7::bigint AND h.state='active' AND h.journal_seq IS NOT NULL AND h.expires_at>now() AND o.purpose='chat_attachment' AND o.message_id=h.message_id`, q.JobID, q.ObjectID, q.MessageID, q.UserID, q.ArtifactID, q.RestoreEpoch, q.BuildGeneration).Scan(&key, &size, &hash)
	if e != nil {
		return fail(403, "EXPORT_MEDIA_HOLDER_REQUIRED", "Current registered media holder required")
	}
	if key != filepath.Base(key) {
		return fail(500, "STORAGE_INTEGRITY", "Invalid media storage key")
	}
	f, e := os.Open(filepath.Join(a.Config.StateDir, "Media", key))
	if e != nil {
		return fail(404, "EXPORT_MEDIA_UNAVAILABLE", "Source media bytes unavailable")
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return e
	}
	if stat.Size() != size {
		return fail(500, "STORAGE_INTEGRITY", "Media source size differs")
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-JobsIM-Size", str(size))
	w.Header().Set("X-JobsIM-SHA256", hex.EncodeToString(hash))
	buffer := make([]byte, 65536)
	for {
		if e = a.checkMediaExportSource(r.Context(), q); e != nil {
			return nil
		}
		n, re := f.Read(buffer)
		if n > 0 {
			if _, e = w.Write(buffer[:n]); e != nil {
				return nil
			}
		}
		if re == io.EOF {
			return nil
		}
		if re != nil {
			return re
		}
	}
}
