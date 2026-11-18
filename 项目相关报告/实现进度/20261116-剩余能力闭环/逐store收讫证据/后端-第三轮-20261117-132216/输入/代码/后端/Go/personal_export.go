package jobsim

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const personalExportPurpose = "personal_data"

// These migrations are wired explicitly after the existing export tables and
// owner capture tables; a source-only candidate never upgrades a running realm.
func migratePersonalExports(ctx context.Context, tx pgx.Tx, role string) error {
	if role == "governance" {
		_, err := tx.Exec(ctx, `ALTER TABLE governance.export_jobs ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT 'experimental_chat' CHECK(purpose IN('experimental_chat','personal_data'));
ALTER TABLE governance.export_jobs ADD COLUMN IF NOT EXISTS captured_at timestamptz;
CREATE TABLE IF NOT EXISTS governance.personal_export_refs(job_id uuid NOT NULL REFERENCES governance.export_jobs(id),owner text NOT NULL,kind text NOT NULL,object_id uuid NOT NULL,ref jsonb NOT NULL,registration_id text,journal_seq bigint,state text NOT NULL DEFAULT 'prepared',PRIMARY KEY(job_id,owner,kind,object_id));`)
		if err != nil {
			return err
		}
		return migratePersonalExportOwner(ctx, tx, role)
	}
	if role == "media" {
		return migratePersonalExportMedia(ctx, tx)
	}
	return migratePersonalExportOwner(ctx, tx, role)
}

type personalExportCreate struct {
	RequestID    string `json:"requestId"`
	RestoreEpoch string `json:"restoreEpoch"`
}

type personalExportView struct {
	Job        exportJob  `json:"job"`
	Format     string     `json:"format"`
	Scope      string     `json:"scope"`
	CapturedAt *time.Time `json:"capturedAt,omitempty"`
	Coverage   []string   `json:"coverage"`
	Exclusions []string   `json:"exclusions"`
}

func personalExportCoverage() []string {
	return []string{"account_public_fields", "device_public_metadata", "own_profile_and_static_avatar", "own_contacts_requests_and_blocks", "authorized_conversations_and_members", "announcement_references", "own_conversation_views", "own_read_state_and_observations", "own_applied_text_drafts", "own_source_preferences", "own_report_reasons_and_public_status", "current_authorized_messages_and_attachments", "forward_placeholders_only"}
}

func personalExportExclusions() []string {
	return []string{"credentials_and_recovery_secrets", "access_and_refresh_tokens", "client_outbox_and_original_requests", "pending_draft_recovery", "e2ee_plaintext_or_keys", "burn_or_once_open_content", "forward_source_bodies_and_proofs", "server_backups_and_client_migration_state", "unavailable_or_unreadable_content"}
}

func decodePersonalExportCreate(r *http.Request) (personalExportCreate, error) {
	var q personalExportCreate
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&q) != nil || d.Decode(new(any)) != io.EOF || !validPersonalExportNonce(q.RequestID) || !syncValidationCounter(q.RestoreEpoch, false) {
		return q, fail(400, "INVALID_EXPORT_REQUEST", "Stable requestId and exact restoreEpoch string required")
	}
	return q, nil
}

func validPersonalExportNonce(raw string) bool {
	return len(raw) >= 8 && len(raw) <= 128 && !strings.ContainsAny(raw, "\r\n\x00")
}

func (a *App) personalExportDigest(user string, q personalExportCreate) []byte {
	raw, _ := json.Marshal(q)
	return keyedDigest(a.Config.Key, "personal-data-export:"+a.Config.DeploymentID+":"+user, raw)
}

func (a *App) personalExportView(ctx context.Context, job exportJob) (personalExportView, error) {
	job.ExpiresAt = job.ExpiresAt.UTC()
	v := personalExportView{Job: job, Format: "zip", Scope: "personal_current", Coverage: personalExportCoverage(), Exclusions: personalExportExclusions()}
	var purpose string
	if err := a.DB.QueryRow(ctx, `SELECT purpose,captured_at FROM governance.export_jobs WHERE id=$1`, job.ID).Scan(&purpose, &v.CapturedAt); err != nil {
		return v, err
	}
	if purpose != personalExportPurpose {
		return v, fail(404, "EXPORT_NOT_FOUND", "Personal export task unavailable")
	}
	if v.CapturedAt != nil {
		utc := v.CapturedAt.UTC()
		v.CapturedAt = &utc
	}
	return v, nil
}

func (a *App) personalExportRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("POST /api/me/data-exports", a.createPersonalExport)
	h("POST /api/me/data-export-results", a.personalExportResult)
	h("GET /api/me/data-exports/{id}", a.personalExportStatus)
	h("POST /api/me/data-exports/{id}/cancel", a.cancelPersonalExport)
	h("POST /api/me/data-exports/{id}/download-ticket", a.personalExportTicket)
	h("GET /api/me/data-exports/{id}/download", a.downloadPersonalExport)
}

func (a *App) originalPersonalExport(ctx context.Context, u User, q personalExportCreate) (exportJob, error) {
	var id string
	var original []byte
	err := a.DB.QueryRow(ctx, `SELECT id::text,request_digest FROM governance.export_jobs WHERE user_id=$1 AND request_id=$2 AND purpose=$3`, u.ID, q.RequestID, personalExportPurpose).Scan(&id, &original)
	if err == pgx.ErrNoRows {
		return exportJob{}, fail(404, "EXPORT_OPERATION_NOT_FOUND", "Original export request has no committed task")
	}
	if err != nil {
		return exportJob{}, err
	}
	if !exportEqualDigest(original, a.personalExportDigest(u.ID, q)) {
		return exportJob{}, fail(409, "IDEMPOTENCY_CONFLICT", "Original personal export body differs")
	}
	// An old task may be inspected after restore, but cannot regain a key or
	// download grant. Delivery always checks the task's original epoch.
	return a.exportJob(ctx, id)
}

func (a *App) personalExportResult(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	q, err := decodePersonalExportCreate(r)
	if err != nil {
		return err
	}
	job, err := a.originalPersonalExport(r.Context(), u, q)
	if err != nil {
		return err
	}
	view, err := a.personalExportView(r.Context(), job)
	if err == nil {
		respond(w, 200, view)
	}
	return err
}

func (a *App) createPersonalExport(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	q, err := decodePersonalExportCreate(r)
	if err != nil {
		return err
	}
	var principal struct {
		SecurityVersion string `json:"securityVersion"`
	}
	if err = a.rpc(r.Context(), "identity", "/internal/exports/principal", map[string]string{"userId": u.ID, "deviceId": u.DeviceID}, &principal); err != nil {
		return err
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "export-create:"+u.ID); err != nil {
		return err
	}
	var id, purpose string
	var digest []byte
	err = tx.QueryRow(r.Context(), `SELECT id::text,purpose,request_digest FROM governance.export_jobs WHERE user_id=$1 AND request_id=$2`, u.ID, q.RequestID).Scan(&id, &purpose, &digest)
	if err == nil {
		if purpose != personalExportPurpose || !exportEqualDigest(digest, a.personalExportDigest(u.ID, q)) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original export request differs")
		}
		if err = tx.Rollback(r.Context()); err != nil {
			return err
		}
		job, err := a.exportJob(r.Context(), id)
		if err != nil {
			return err
		}
		view, err := a.personalExportView(r.Context(), job)
		if err == nil {
			respond(w, 200, view)
		}
		return err
	}
	if err != pgx.ErrNoRows {
		return err
	}
	if q.RestoreEpoch != u.RestoreEpoch {
		return fail(409, "RESTORE_EPOCH_CHANGED", "New export intent belongs to an earlier restore")
	}
	var count int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM governance.export_jobs WHERE user_id=$1 AND created_at>now()-interval '24 hours'`, u.ID).Scan(&count); err != nil {
		return err
	}
	if count >= 3 {
		return fail(429, "EXPORT_RATE_LIMITED", "At most three export tasks per 24 hours")
	}
	id, artifact := newID(), newID()
	sealed, err := a.seal(randomToken(32), "export-key:"+id+":"+u.ID+":"+u.RestoreEpoch)
	if err != nil {
		return err
	}
	prefix := id + "/" + artifact + "/" + u.RestoreEpoch + "-1"
	expires := time.Now().UTC().Add(24 * time.Hour)
	_, err = tx.Exec(r.Context(), `INSERT INTO governance.export_jobs(id,artifact_id,user_id,device_id,request_id,request_digest,restore_epoch,permission_generation,expires_at,key_ciphertext,purpose) VALUES($1,$2,$3,$4,$5,$6,$7::bigint,$8::bigint,$9,$10,$11)`, id, artifact, u.ID, u.DeviceID, q.RequestID, a.personalExportDigest(u.ID, q), u.RestoreEpoch, principal.SecurityVersion, expires, sealed, personalExportPurpose)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO governance.export_artifacts(id,job_id,restore_epoch,build_generation,storage_prefix) VALUES($1,$2,$3::bigint,1,$4)`, artifact, id, u.RestoreEpoch, prefix); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	job, err := a.exportJob(r.Context(), id)
	if err != nil {
		return err
	}
	view, err := a.personalExportView(r.Context(), job)
	if err == nil {
		respond(w, 202, view)
	}
	return err
}

func (a *App) ownedPersonalExport(r *http.Request) (User, exportJob, error) {
	u, err := a.current(r)
	if err != nil {
		return u, exportJob{}, err
	}
	if !validAssetID(r.PathValue("id")) {
		return u, exportJob{}, fail(404, "EXPORT_NOT_FOUND", "Personal export task unavailable")
	}
	job, err := a.exportJob(r.Context(), r.PathValue("id"))
	if err != nil {
		return u, job, err
	}
	if job.UserID != u.ID {
		return u, job, fail(404, "EXPORT_NOT_FOUND", "Personal export task unavailable")
	}
	_, err = a.personalExportView(r.Context(), job)
	return u, job, err
}

func (a *App) personalExportStatus(w http.ResponseWriter, r *http.Request) error {
	_, job, err := a.ownedPersonalExport(r)
	if err != nil {
		return err
	}
	view, err := a.personalExportView(r.Context(), job)
	if err == nil {
		respond(w, 200, view)
	}
	return err
}

func (a *App) cancelPersonalExport(w http.ResponseWriter, r *http.Request) error {
	u, job, err := a.ownedPersonalExport(r)
	if err != nil {
		return err
	}
	var q struct {
		RequestID string `json:"requestId"`
	}
	if err = decode(r, &q); err != nil {
		return err
	}
	if !validPersonalExportNonce(q.RequestID) {
		return fail(400, "REQUEST_ID_REQUIRED", "Stable cancellation requestId required")
	}
	if err = a.cancelExportTask(r.Context(), job.ID, "user:"+u.ID+":"+q.RequestID, "cancelled"); err != nil {
		return err
	}
	return a.personalExportStatus(w, r)
}

func (a *App) personalExportTicket(w http.ResponseWriter, r *http.Request) error {
	_, job, err := a.ownedPersonalExport(r)
	if err != nil {
		return err
	}
	if job.State != "ready" {
		return fail(409, "EXPORT_NOT_READY", "Export is not downloadable")
	}
	if err = a.checkExportJob(r.Context(), job); err != nil {
		return err
	}
	if err = a.validatePersonalExportSources(r.Context(), job); err != nil {
		return err
	}
	token := randomToken(32)
	expires := time.Now().UTC().Add(60 * time.Second)
	if job.ExpiresAt.Before(expires) {
		expires = job.ExpiresAt
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE governance.export_artifacts SET download_token_digest=$2,download_expires_at=$3 WHERE job_id=$1 AND state='published'`, job.ID, tokenDigest(token), expires)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "EXPORT_INVALIDATED", "Export artifact no longer published")
	}
	respond(w, 200, map[string]any{"downloadToken": token, "expiresAt": expires, "format": "zip"})
	return nil
}

func (a *App) downloadPersonalExport(w http.ResponseWriter, r *http.Request) error {
	_, job, err := a.ownedPersonalExport(r)
	if err != nil {
		return err
	}
	if job.State != "ready" {
		return fail(409, "EXPORT_NOT_READY", "Export is not downloadable")
	}
	// Public gateway holds the outer source SH fence. A direct owner request is
	// deliberately fail-closed instead of taking artifact->source locks.
	if !a.hasGatewayProjectionFence(r, false) {
		return fail(503, "ADMISSION_FENCE_REQUIRED", "Controlled export gateway required")
	}
	lock, err := a.exportFenceFile(r.Context(), job.ArtifactID, false)
	if err != nil {
		return err
	}
	defer closeExportFence(lock)
	var valid bool
	var expiry time.Time
	var size int64
	var digest []byte
	err = a.DB.QueryRow(r.Context(), `SELECT download_token_digest=$2 AND download_expires_at>now() AND state='published',download_expires_at,size_bytes,content_digest FROM governance.export_artifacts WHERE job_id=$1`, job.ID, tokenDigest(r.Header.Get("X-JobsIM-Download-Token"))).Scan(&valid, &expiry, &size, &digest)
	if err != nil || !valid {
		return fail(403, "EXPORT_DOWNLOAD_AUTH_REQUIRED", "Current short download authorization required")
	}
	if err = a.checkExportJob(r.Context(), job); err != nil {
		return err
	}
	if err = a.validatePersonalExportSources(r.Context(), job); err != nil {
		return err
	}
	keyText, err := a.unseal(job.Key, "export-key:"+job.ID+":"+job.UserID+":"+job.RestoreEpoch)
	if err != nil {
		return fail(409, "EXPORT_KEY_UNAVAILABLE", "Export key no longer deliverable")
	}
	key, err := base64.RawURLEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("invalid sealed personal export key")
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	prefix, err := a.exportPrefix(job)
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(prefix, "package.jimexport"))
	if err != nil {
		return fail(404, "EXPORT_BYTES_UNAVAILABLE", "Export bytes unavailable")
	}
	defer f.Close()
	check := func() error {
		if !time.Now().Before(expiry) {
			return fail(403, "EXPORT_DOWNLOAD_EXPIRED", "Short download authorization expired")
		}
		return a.checkExportJob(r.Context(), job)
	}
	// Headers are delayed until the complete authenticated first pass succeeds.
	out := &personalZIPResponse{ResponseWriter: w}
	err = authenticatedPersonalZIP(r.Context(), f, key, digest, size, out, check)
	if out.started && err != nil {
		// A failed stream remains a truncated ZIP; never append a JSON error to it.
		return nil
	}
	return err
}

type personalZIPResponse struct {
	http.ResponseWriter
	started bool
}

func (w *personalZIPResponse) Write(p []byte) (int, error) {
	if !w.started {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=jobsim-personal-data.zip")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.started = true
	}
	return w.ResponseWriter.Write(p)
}
