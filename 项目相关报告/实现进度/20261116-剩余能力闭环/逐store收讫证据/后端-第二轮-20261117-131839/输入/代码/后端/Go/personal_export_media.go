package jobsim

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

func migratePersonalExportMedia(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS media.personal_export_avatar_holders(job_id uuid NOT NULL,artifact_id uuid NOT NULL,user_id uuid NOT NULL,object_id uuid NOT NULL,restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,profile_ref jsonb NOT NULL,state text NOT NULL CHECK(state IN('prepared','active','cleared')),journal_seq bigint,expires_at timestamptz NOT NULL,PRIMARY KEY(job_id,object_id));`)
	return err
}

func (a *App) personalExportMediaRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/personal-exports/read-avatar", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "governance" {
			return fail(403, "EXPORT_SERVICE_REQUIRED", "Personal export service required")
		}
		var q personalExportHolder
		if err := decode(r, &q); err != nil {
			return err
		}
		if !validAssetID(q.ObjectID) || !validAssetID(q.JobID) || !validAssetID(q.ArtifactID) || !validAssetID(q.UserID) || q.Ref.Kind != "profile" || q.Ref.Owner != "directory" || q.Ref.ID != q.UserID {
			return fail(400, "INVALID_EXPORT_HOLDER", "Bound own avatar source required")
		}
		if err := a.rpc(r.Context(), "directory", "/internal/personal-exports/check-avatar", q, nil); err != nil {
			return err
		}
		var key, contentType string
		var size int64
		var digest []byte
		err := a.DB.QueryRow(r.Context(), `SELECT storage_key,content_type,size_bytes,sha256 FROM media.objects WHERE id=$1 AND owner_id=$2 AND owner_reference_id=$2 AND purpose='profile_avatar'`, q.ObjectID, q.UserID).Scan(&key, &contentType, &size, &digest)
		if err != nil {
			return fail(404, "EXPORT_AVATAR_UNAVAILABLE", "Own avatar asset unavailable")
		}
		if key != filepath.Base(key) || size < 0 || size > 2<<20 || len(digest) != sha256.Size {
			return fail(409, "EXPORT_SOURCE_INTEGRITY", "Own avatar asset metadata invalid")
		}
		raw, _ := json.Marshal(q.Ref)
		_, err = a.DB.Exec(r.Context(), `INSERT INTO media.personal_export_avatar_holders(job_id,artifact_id,user_id,object_id,restore_epoch,build_generation,profile_ref,state,expires_at) VALUES($1,$2,$3,$4,$5::bigint,$6::bigint,$7,'prepared',$8) ON CONFLICT DO NOTHING`, q.JobID, q.ArtifactID, q.UserID, q.ObjectID, q.RestoreEpoch, q.BuildGeneration, raw, q.ExpiresAt)
		if err != nil {
			return err
		}
		var valid bool
		if err = a.DB.QueryRow(r.Context(), `SELECT artifact_id=$3 AND user_id=$4 AND restore_epoch=$5::bigint AND build_generation=$6::bigint AND profile_ref=$7::jsonb AND state IN('prepared','active') AND expires_at>now() FROM media.personal_export_avatar_holders WHERE job_id=$1 AND object_id=$2`, q.JobID, q.ObjectID, q.ArtifactID, q.UserID, q.RestoreEpoch, q.BuildGeneration, raw).Scan(&valid); err != nil || !valid {
			return fail(409, "EXPORT_HOLDER_CHANGED", "Own avatar holder changed")
		}
		payload := map[string]any{"jobId": q.JobID, "artifactId": q.ArtifactID, "userId": q.UserID, "objectId": q.ObjectID, "restoreEpoch": q.RestoreEpoch, "buildGeneration": q.BuildGeneration, "expiresAt": q.ExpiresAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}
		operation := "personal-export-avatar:" + q.JobID + ":" + q.ObjectID
		var ack struct {
			JournalSeq string `json:"journalSeq"`
		}
		if err = a.rpc(r.Context(), "journal", "/internal/append", journalRecord{a.Role, operation, "intent", "media.personal_export_avatar_holder", payload}, &ack); err != nil {
			return err
		}
		if !syncValidationCounter(ack.JournalSeq, false) {
			return fail(503, "EXPORT_JOURNAL_REQUIRED", "Own avatar holder durability result required")
		}
		if _, err = a.DB.Exec(r.Context(), `UPDATE media.personal_export_avatar_holders SET state='active',journal_seq=$3::bigint WHERE job_id=$1 AND object_id=$2 AND state IN('prepared','active')`, q.JobID, q.ObjectID, ack.JournalSeq); err != nil {
			return err
		}
		if err = a.protect(r.Context(), operation, "result", "media.personal_export_avatar_holder", payload); err != nil {
			return err
		}
		if err = a.rpc(r.Context(), "directory", "/internal/personal-exports/check-avatar", q, nil); err != nil {
			return err
		}
		content, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", key))
		if err != nil {
			return fail(404, "EXPORT_AVATAR_UNAVAILABLE", "Own avatar bytes unavailable")
		}
		hash := sha256.Sum256(content)
		if int64(len(content)) != size || !exportEqualDigest(hash[:], digest) {
			return fail(409, "EXPORT_SOURCE_INTEGRITY", "Own avatar bytes differ")
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-JobsIM-Size", str(size))
		w.Header().Set("X-JobsIM-SHA256", syncDigest(content))
		_, err = w.Write(content)
		return err
	}))
	h("POST /internal/personal-exports/clear-avatar-holders", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			JobID      string `json:"jobId"`
			ArtifactID string `json:"artifactId"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		if err := a.rpc(r.Context(), "governance", "/internal/exports/cleared", q, nil); err != nil {
			return err
		}
		_, err := a.DB.Exec(r.Context(), `UPDATE media.personal_export_avatar_holders SET state='cleared' WHERE job_id=$1 AND artifact_id=$2`, q.JobID, q.ArtifactID)
		if err == nil {
			respond(w, 200, map[string]bool{"cleared": true})
		}
		return err
	}))
}

func (a *App) writePersonalAvatar(ctx context.Context, out *zip.Writer, job exportJob, ref syncRef, value any) error {
	profile, ok := value.(map[string]any)
	if !ok {
		return fail(503, "EXPORT_SOURCE_INTEGRITY", "Own profile projection unavailable")
	}
	for _, field := range []string{"avatarURL", "avatarThumbnailURL"} {
		url, _ := profile[field].(string)
		delete(profile, field)
		if url == "" {
			continue
		}
		id := strings.TrimPrefix(url, "/api/avatars/")
		if url != "/api/avatars/"+id || !validAssetID(id) {
			return fail(503, "EXPORT_SOURCE_INTEGRITY", "Own avatar reference invalid")
		}
		q := personalExportHolder{JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration, ExpiresAt: job.ExpiresAt, Ref: ref, ObjectID: id}
		body, _ := json.Marshal(q)
		request, err := http.NewRequestWithContext(ctx, "POST", a.Config.RoleURLs["media"]+"/internal/personal-exports/read-avatar", bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-JobsIM-Service-Key", base64.RawURLEncoding.EncodeToString(a.Config.Key))
		request.Header.Set("X-JobsIM-Caller", a.Role)
		response, err := a.Client.Do(request)
		if err != nil {
			return fail(503, "EXPORT_AVATAR_UNAVAILABLE", "Own avatar source unavailable")
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			return fail(503, "EXPORT_AVATAR_UNAVAILABLE", "Own avatar body not delivered")
		}
		if !hmac.Equal([]byte(response.Header.Get("X-JobsIM-Admission-Fence")), []byte(a.admissionFenceIdentity())) {
			return fail(503, "ADMISSION_FENCE_MISMATCH", "Avatar owner must share the canonical local instance")
		}
		if len(raw) > 2<<20 || response.Header.Get("Content-Type") != "image/jpeg" || syncDigest(raw) != response.Header.Get("X-JobsIM-SHA256") || str(int64(len(raw))) != response.Header.Get("X-JobsIM-Size") {
			return fail(503, "EXPORT_SOURCE_INTEGRITY", "Own avatar body does not match reference")
		}
		name := "Profile/" + id + ".jpg"
		if err = writePersonalZIPEntry(out, name, bytes.NewReader(raw), int64(len(raw))); err != nil {
			return err
		}
		profile[strings.TrimSuffix(field, "URL")+"Path"] = name
	}
	return nil
}
