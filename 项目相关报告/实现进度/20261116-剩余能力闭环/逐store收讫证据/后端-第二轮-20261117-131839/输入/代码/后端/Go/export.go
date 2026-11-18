package jobsim

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func init() {
	migrations["governance"] += `
CREATE TABLE IF NOT EXISTS governance.export_jobs(id uuid PRIMARY KEY,artifact_id uuid NOT NULL UNIQUE,user_id uuid NOT NULL,device_id uuid NOT NULL,request_id text NOT NULL,request_digest bytea NOT NULL,restore_epoch bigint NOT NULL,permission_generation bigint NOT NULL,build_generation bigint NOT NULL DEFAULT 1,state text NOT NULL DEFAULT 'requested',expires_at timestamptz NOT NULL,key_ciphertext bytea,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(user_id,request_id));
CREATE TABLE IF NOT EXISTS governance.export_artifacts(id uuid PRIMARY KEY,job_id uuid NOT NULL UNIQUE REFERENCES governance.export_jobs(id),restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,storage_prefix text NOT NULL UNIQUE,writer_token uuid,writer_lease_until timestamptz,writer_state text NOT NULL DEFAULT 'pending',state text NOT NULL DEFAULT 'prepared',size_bytes bigint,content_digest bytea,download_token_digest bytea,download_expires_at timestamptz,cleared_at timestamptz);
CREATE TABLE IF NOT EXISTS governance.export_items(job_id uuid NOT NULL REFERENCES governance.export_jobs(id),message_id uuid NOT NULL,conversation_id uuid NOT NULL,source_version bigint NOT NULL,attachment_id uuid,registration_id text,state text NOT NULL DEFAULT 'prepared',PRIMARY KEY(job_id,message_id));
CREATE TABLE IF NOT EXISTS governance.export_cancellations(job_id uuid NOT NULL,operation_id text NOT NULL,request_digest bytea NOT NULL,artifact_id uuid NOT NULL,restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,terminal_state text NOT NULL,state text NOT NULL DEFAULT 'pending',created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(job_id,operation_id));`
}

type exportJob struct {
	ID                   string    `json:"id"`
	ArtifactID           string    `json:"artifactId"`
	UserID               string    `json:"-"`
	DeviceID             string    `json:"-"`
	RestoreEpoch         string    `json:"restoreEpoch"`
	PermissionGeneration string    `json:"-"`
	BuildGeneration      string    `json:"buildGeneration"`
	State                string    `json:"state"`
	ExpiresAt            time.Time `json:"expiresAt"`
	CleanupState         string    `json:"cleanupState"`
	FailureCode          string    `json:"failureCode,omitempty"`
	Key                  []byte    `json:"-"`
	Prefix               string    `json:"-"`
}

func (a *App) exportJob(ctx context.Context, id string) (exportJob, error) {
	var v exportJob
	var epoch, security, generation int64
	var sealed []byte
	e := a.DB.QueryRow(ctx, `SELECT j.id::text,j.artifact_id::text,j.user_id::text,j.device_id::text,j.restore_epoch,j.permission_generation,j.build_generation,j.state,j.expires_at,j.key_ciphertext,a.storage_prefix,CASE WHEN a.state='cleared' AND EXISTS(SELECT 1 FROM governance.export_cancellations c WHERE c.job_id=j.id AND c.state<>'cleared') THEN 'source_handoff_pending' ELSE a.state END,CASE WHEN EXISTS(SELECT 1 FROM governance.export_cancellations c WHERE c.job_id=j.id AND c.operation_id='builder-forward-unsupported') THEN 'FORWARD_EXPORT_UNSUPPORTED' ELSE COALESCE((SELECT split_part(c.operation_id,':',2) FROM governance.export_cancellations c WHERE c.job_id=j.id AND c.operation_id LIKE 'builder-failed:%' ORDER BY created_at DESC LIMIT 1),'') END FROM governance.export_jobs j JOIN governance.export_artifacts a ON a.id=j.artifact_id WHERE j.id=$1`, id).Scan(&v.ID, &v.ArtifactID, &v.UserID, &v.DeviceID, &epoch, &security, &generation, &v.State, &v.ExpiresAt, &sealed, &v.Prefix, &v.CleanupState, &v.FailureCode)
	if e != nil {
		return v, fail(404, "EXPORT_NOT_FOUND", "Export task unavailable")
	}
	v.RestoreEpoch = str(epoch)
	v.PermissionGeneration = str(security)
	v.BuildGeneration = str(generation)
	v.Key = sealed
	return v, nil
}
func (a *App) exportFenceFile(ctx context.Context, artifact string, exclusive bool) (*os.File, error) {
	if !validAssetID(artifact) {
		return nil, fmt.Errorf("invalid export lock identity")
	}
	dir := filepath.Join(a.Config.StateDir, "Exports", "Locks")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, artifact+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lockExportFile(ctx, f, exclusive); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func closeExportFence(f *os.File) { unlockExportFile(f); f.Close() }
func (a *App) exportPrefix(v exportJob) (string, error) {
	expected := filepath.Join(v.ID, v.ArtifactID, v.RestoreEpoch+"-1")
	if !validAssetID(v.ID) || !validAssetID(v.ArtifactID) || v.Prefix != filepath.ToSlash(expected) {
		return "", fmt.Errorf("invalid immutable export prefix")
	}
	return filepath.Join(a.Config.StateDir, "Exports", "Artifacts", expected), nil
}
func (a *App) checkExportJob(ctx context.Context, v exportJob) error {
	current, e := a.exportJob(ctx, v.ID)
	if e != nil {
		return e
	}
	if (current.State != "requested" && current.State != "building" && current.State != "ready") || current.BuildGeneration != v.BuildGeneration || current.RestoreEpoch != v.RestoreEpoch || !time.Now().Before(current.ExpiresAt) {
		return fail(409, "EXPORT_INVALIDATED", "Export task is no longer valid")
	}
	epoch, e := a.epoch(ctx)
	if e != nil {
		return e
	}
	if str(epoch) != v.RestoreEpoch {
		return fail(409, "RESTORE_EPOCH_CHANGED", "Export belongs to an earlier restore")
	}
	var principal struct {
		SecurityVersion string `json:"securityVersion"`
	}
	if e = a.rpc(ctx, "identity", "/internal/exports/principal", map[string]string{"userId": v.UserID, "deviceId": v.DeviceID}, &principal); e != nil {
		return e
	}
	if principal.SecurityVersion != v.PermissionGeneration {
		return fail(403, "EXPORT_PERMISSION_CHANGED", "Export principal changed")
	}
	return nil
}

func (a *App) exportRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("POST /api/me/exports", a.createExport)
	h("GET /api/me/exports/{id}", a.exportStatus)
	h("POST /api/me/exports/{id}/cancel", a.cancelOwnExport)
	h("POST /api/me/exports/{id}/download-ticket", a.exportDownloadTicket)
	h("GET /api/me/exports/{id}/download", a.downloadExport)
}
func (a *App) createExport(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q struct {
		RequestID string `json:"requestId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 {
		return fail(400, "REQUEST_ID_REQUIRED", "Stable export requestId required")
	}
	var existing string
	e = a.DB.QueryRow(r.Context(), `SELECT id::text FROM governance.export_jobs WHERE user_id=$1 AND request_id=$2`, u.ID, q.RequestID).Scan(&existing)
	if e == nil {
		var purpose string
		if e = a.DB.QueryRow(r.Context(), `SELECT purpose FROM governance.export_jobs WHERE id=$1`, existing).Scan(&purpose); e != nil {
			return e
		}
		if purpose != "experimental_chat" {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original export request has a different purpose")
		}
		v, e := a.exportJob(r.Context(), existing)
		if e != nil {
			return e
		}
		respond(w, 200, map[string]any{"job": v, "keyReturned": false})
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	var principal struct {
		SecurityVersion string `json:"securityVersion"`
	}
	if e = a.rpc(r.Context(), "identity", "/internal/exports/principal", map[string]string{"userId": u.ID, "deviceId": u.DeviceID}, &principal); e != nil {
		return e
	}
	id, artifact := newID(), newID()
	key := randomToken(32)
	sealed, e := a.seal(key, "export-key:"+id+":"+u.ID+":"+u.RestoreEpoch)
	if e != nil {
		return e
	}
	prefix := id + "/" + artifact + "/" + u.RestoreEpoch + "-1"
	expires := time.Now().UTC().Add(24 * time.Hour)
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "export-create:"+u.ID); e != nil {
		return e
	}
	var count int
	if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM governance.export_jobs WHERE user_id=$1 AND created_at>now()-interval '24 hours'`, u.ID).Scan(&count); e != nil {
		return e
	}
	if count >= 3 {
		return fail(429, "EXPORT_RATE_LIMITED", "At most three export tasks per 24 hours")
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO governance.export_jobs(id,artifact_id,user_id,device_id,request_id,request_digest,restore_epoch,permission_generation,expires_at,key_ciphertext) VALUES($1,$2,$3,$4,$5,$6,$7::bigint,$8::bigint,$9,$10)`, id, artifact, u.ID, u.DeviceID, q.RequestID, keyedDigest(a.Config.Key, "export-request", []byte(u.ID+":"+q.RequestID)), u.RestoreEpoch, principal.SecurityVersion, expires, sealed)
	if e != nil {
		return fail(409, "EXPORT_REQUEST_IN_PROGRESS", "Query the original export request")
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO governance.export_artifacts(id,job_id,restore_epoch,build_generation,storage_prefix) VALUES($1,$2,$3::bigint,1,$4)`, artifact, id, u.RestoreEpoch, prefix)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	v, e := a.exportJob(r.Context(), id)
	if e != nil {
		return e
	}
	respond(w, 202, map[string]any{"job": v, "decryptionKey": key, "keyReturned": true, "format": "jobsim-aesgcm-chunked-tar-1"})
	return nil
}
func (a *App) ownedExport(r *http.Request) (User, exportJob, error) {
	u, e := a.current(r)
	if e != nil {
		return u, exportJob{}, e
	}
	id := r.PathValue("id")
	if !validAssetID(id) {
		return u, exportJob{}, fail(404, "EXPORT_NOT_FOUND", "Export task unavailable")
	}
	v, e := a.exportJob(r.Context(), id)
	if e != nil {
		return u, v, e
	}
	if v.UserID != u.ID {
		return u, v, fail(404, "EXPORT_NOT_FOUND", "Export task unavailable")
	}
	var purpose string
	if e = a.DB.QueryRow(r.Context(), `SELECT purpose FROM governance.export_jobs WHERE id=$1`, v.ID).Scan(&purpose); e != nil {
		return u, v, e
	}
	if purpose != "experimental_chat" {
		return u, v, fail(404, "EXPORT_NOT_FOUND", "Experimental export task unavailable")
	}
	return u, v, nil
}
func (a *App) exportStatus(w http.ResponseWriter, r *http.Request) error {
	_, v, e := a.ownedExport(r)
	if e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}
func (a *App) cancelOwnExport(w http.ResponseWriter, r *http.Request) error {
	u, v, e := a.ownedExport(r)
	if e != nil {
		return e
	}
	var q struct {
		RequestID string `json:"requestId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.RequestID) < 8 {
		return fail(400, "REQUEST_ID_REQUIRED", "Stable cancellation requestId required")
	}
	if e = a.cancelExportTask(r.Context(), v.ID, "user:"+u.ID+":"+q.RequestID, "cancelled"); e != nil {
		return e
	}
	v, e = a.exportJob(r.Context(), v.ID)
	if e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}

type exportCancellation struct{ JobID, ArtifactID, Epoch, Generation, Operation, Terminal string }

func (a *App) prepareExportCancellation(ctx context.Context, id, operation, terminal string) (exportCancellation, error) {
	v := exportCancellation{JobID: id, Operation: operation, Terminal: terminal}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer tx.Rollback(ctx)
	var artifact string
	var epoch, generation int64
	if e = tx.QueryRow(ctx, `SELECT artifact_id::text,restore_epoch,build_generation FROM governance.export_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&artifact, &epoch, &generation); e != nil {
		return v, e
	}
	digest := keyedDigest(a.Config.Key, "export-cancel", []byte(id+":"+operation+":"+terminal))
	var original []byte
	e = tx.QueryRow(ctx, `SELECT artifact_id::text,restore_epoch,build_generation,request_digest,terminal_state FROM governance.export_cancellations WHERE job_id=$1 AND operation_id=$2`, id, operation).Scan(&v.ArtifactID, &epoch, &generation, &original, &v.Terminal)
	if e == nil {
		if !bytes.Equal(original, digest) {
			return v, fail(409, "IDEMPOTENCY_CONFLICT", "Original cancellation differs")
		}
	} else if e == pgx.ErrNoRows {
		generation++
		v.ArtifactID = artifact
		_, e = tx.Exec(ctx, `INSERT INTO governance.export_cancellations(job_id,operation_id,request_digest,artifact_id,restore_epoch,build_generation,terminal_state) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, operation, digest, artifact, epoch, generation, terminal)
		if e != nil {
			return v, e
		}
		_, e = tx.Exec(ctx, `UPDATE governance.export_jobs SET state=$2,build_generation=$3,key_ciphertext=NULL WHERE id=$1`, id, terminal, generation)
		if e != nil {
			return v, e
		}
		_, e = tx.Exec(ctx, `UPDATE governance.export_artifacts SET state='revoked',writer_state='closing',download_token_digest=NULL,download_expires_at=NULL WHERE job_id=$1 AND state<>'cleared'`, id)
		if e != nil {
			return v, e
		}
	} else {
		return v, e
	}
	v.Epoch = str(epoch)
	v.Generation = str(generation)
	return v, tx.Commit(ctx)
}
func (a *App) finishExportCancellation(ctx context.Context, v exportCancellation) error {
	payload := map[string]any{"jobId": v.JobID, "artifactId": v.ArtifactID, "restoreEpoch": v.Epoch, "buildGeneration": v.Generation, "state": v.Terminal}
	operation := "export-fence:" + v.JobID + ":" + v.Operation
	if len(operation) > 200 {
		operation = "export-fence:" + stringBase64(keyedDigest(a.Config.Key, "export-fence-operation", []byte(operation)))
	}
	if e := a.protect(ctx, operation, "intent", "governance.export_fence", payload); e != nil {
		return e
	}
	lock, e := a.exportFenceFile(ctx, v.ArtifactID, true)
	if e != nil {
		return fail(503, "EXPORT_CLEANUP_PENDING", "Export writer or reader has not drained; retry the original operation")
	}
	defer closeExportFence(lock)
	job, e := a.exportJob(ctx, v.JobID)
	if e != nil {
		return e
	}
	prefix, e := a.exportPrefix(job)
	if e != nil {
		return e
	}
	if e = clearExportPrefix(prefix); e != nil {
		return e
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE governance.export_artifacts SET state='cleared',writer_state='closed',writer_token=NULL,writer_lease_until=NULL,cleared_at=now() WHERE job_id=$1`, v.JobID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE governance.export_cancellations SET state='drained' WHERE job_id=$1 AND operation_id=$2 AND state<>'cleared'`, v.JobID, v.Operation); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	for _, role := range []string{"message", "media"} {
		if e = a.rpc(ctx, role, "/internal/exports/clear-holders", map[string]string{"jobId": v.JobID, "artifactId": v.ArtifactID}, nil); e != nil {
			return e
		}
	}
	if e = a.clearPersonalExportHolders(ctx, job); e != nil {
		return e
	}
	payload["state"] = "cleared"
	if e = a.protect(ctx, operation, "result", "governance.export_fence", payload); e != nil {
		return e
	}
	_, e = a.DB.Exec(ctx, `UPDATE governance.export_cancellations SET state='cleared' WHERE job_id=$1 AND operation_id=$2`, v.JobID, v.Operation)
	return e
}
func (a *App) cancelExportTask(ctx context.Context, id, operation, terminal string) error {
	v, e := a.prepareExportCancellation(ctx, id, operation, terminal)
	if e != nil {
		return e
	}
	return a.finishExportCancellation(ctx, v)
}
func clearExportPrefix(prefix string) error {
	entries, e := os.ReadDir(prefix)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unrecognized export prefix entry; cleanup remains pending")
		}
		if e = os.Remove(filepath.Join(prefix, entry.Name())); e != nil {
			return e
		}
	}
	entries, e = os.ReadDir(prefix)
	if e != nil {
		return e
	}
	if len(entries) != 0 {
		return fmt.Errorf("export prefix still has writers")
	}
	if e = syncDirectory(prefix); e != nil {
		return e
	}
	if e = os.Remove(prefix); e != nil {
		return e
	}
	return syncDirectory(filepath.Dir(prefix))
}

func (a *App) exportDownloadTicket(w http.ResponseWriter, r *http.Request) error {
	_, v, e := a.ownedExport(r)
	if e != nil {
		return e
	}
	if v.State != "ready" {
		return fail(409, "EXPORT_NOT_READY", "Export is not downloadable")
	}
	if e = a.checkExportJob(r.Context(), v); e != nil {
		return e
	}
	if e = a.validateExportSources(r.Context(), v); e != nil {
		return e
	}
	token := randomToken(32)
	expires := time.Now().UTC().Add(60 * time.Second)
	if v.ExpiresAt.Before(expires) {
		expires = v.ExpiresAt
	}
	_, e = a.DB.Exec(r.Context(), `UPDATE governance.export_artifacts SET download_token_digest=$2,download_expires_at=$3 WHERE job_id=$1 AND state='published'`, v.ID, tokenDigest(token), expires)
	if e != nil {
		return e
	}
	respond(w, 200, map[string]any{"downloadToken": token, "expiresAt": expires})
	return nil
}
func (a *App) downloadExport(w http.ResponseWriter, r *http.Request) error {
	_, v, e := a.ownedExport(r)
	if e != nil {
		return e
	}
	if v.State != "ready" {
		return fail(409, "EXPORT_NOT_READY", "Export is not downloadable")
	}
	lock, e := a.exportFenceFile(r.Context(), v.ArtifactID, false)
	if e != nil {
		return e
	}
	defer closeExportFence(lock)
	var valid bool
	var expiry time.Time
	if e = a.DB.QueryRow(r.Context(), `SELECT download_token_digest=$2 AND download_expires_at>now() AND state='published',download_expires_at FROM governance.export_artifacts WHERE job_id=$1`, v.ID, tokenDigest(r.Header.Get("X-JobsIM-Download-Token"))).Scan(&valid, &expiry); e != nil || !valid {
		return fail(403, "EXPORT_DOWNLOAD_AUTH_REQUIRED", "Current short download authorization required")
	}
	if e = a.checkExportJob(r.Context(), v); e != nil {
		return e
	}
	if e = a.validateExportSources(r.Context(), v); e != nil {
		return e
	}
	prefix, e := a.exportPrefix(v)
	if e != nil {
		return e
	}
	f, e := os.Open(filepath.Join(prefix, "package.jimexport"))
	if e != nil {
		return fail(404, "EXPORT_BYTES_UNAVAILABLE", "Export bytes unavailable")
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=jobsim-encrypted-export.jimexport")
	w.Header().Set("Cache-Control", "no-store")
	buffer := make([]byte, 65536)
	for {
		if !time.Now().Before(expiry) {
			return nil
		}
		if e = a.checkExportJob(r.Context(), v); e != nil {
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

func (a *App) exportInternalRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/exports/build", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			JobID           string `json:"jobId"`
			UserID          string `json:"userId"`
			ArtifactID      string `json:"artifactId"`
			RestoreEpoch    string `json:"restoreEpoch"`
			BuildGeneration string `json:"buildGeneration"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if !validAssetID(q.JobID) {
			return fail(404, "EXPORT_NOT_FOUND", "Export task unavailable")
		}
		v, e := a.exportJob(r.Context(), q.JobID)
		if e != nil {
			return e
		}
		if v.UserID != q.UserID || (q.ArtifactID != "" && q.ArtifactID != v.ArtifactID) || (q.RestoreEpoch != "" && q.RestoreEpoch != v.RestoreEpoch) || (q.BuildGeneration != "" && q.BuildGeneration != v.BuildGeneration) {
			return fail(403, "EXPORT_BUILD_MISMATCH", "Export build differs")
		}
		if e = a.checkExportJob(r.Context(), v); e != nil {
			return e
		}
		respond(w, 200, map[string]bool{"valid": true})
		return nil
	}))
	h("POST /internal/exports/cleared", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			JobID      string `json:"jobId"`
			ArtifactID string `json:"artifactId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		var cleared bool
		if e := a.DB.QueryRow(r.Context(), `SELECT a.state='cleared' AND a.writer_state='closed' AND a.cleared_at IS NOT NULL AND j.state NOT IN('requested','building','ready') FROM governance.export_artifacts a JOIN governance.export_jobs j ON j.id=a.job_id WHERE a.job_id=$1 AND a.id=$2`, q.JobID, q.ArtifactID).Scan(&cleared); e != nil {
			return e
		}
		if !cleared {
			return fail(409, "EXPORT_CLEANUP_PENDING", "Export build has not drained")
		}
		respond(w, 200, map[string]bool{"cleared": true})
		return nil
	}))
	h("POST /internal/exports/invalidate", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID      string   `json:"userId"`
			DeviceID    string   `json:"deviceId,omitempty"`
			JobIDs      []string `json:"jobIds"`
			OperationID string   `json:"operationId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if len(q.OperationID) < 1 || len(q.OperationID) > 180 || len(q.JobIDs) > 1000 {
			return fail(400, "INVALID_EXPORT_INVALIDATION", "Bound operation and bounded targets required")
		}
		if q.DeviceID != "" && (q.UserID == "" || !validObjectID(q.DeviceID)) {
			return fail(400, "INVALID_EXPORT_INVALIDATION", "Device cleanup requires its original user")
		}
		if q.UserID != "" {
			rows, e := a.DB.Query(r.Context(), `SELECT j.id::text FROM governance.export_jobs j JOIN governance.export_artifacts a ON a.job_id=j.id WHERE j.user_id=$1 AND ($2='' OR j.device_id=NULLIF($2,'')::uuid) AND(j.state IN('requested','building','ready') OR a.state<>'cleared' OR EXISTS(SELECT 1 FROM governance.export_cancellations c WHERE c.job_id=j.id AND c.state<>'cleared'))`, q.UserID, q.DeviceID)
			if e != nil {
				return e
			}
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return e
				}
				q.JobIDs = append(q.JobIDs, id)
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
		}
		pending := []exportCancellation{}
		seen := map[string]bool{}
		for _, id := range q.JobIDs {
			if !validAssetID(id) {
				return fail(400, "INVALID_EXPORT_TARGET", "Invalid export task")
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			job, lookupError := a.exportJob(r.Context(), id)
			if lookupError != nil {
				return lookupError
			}
			if job.CleanupState == "cleared" && job.State != "requested" && job.State != "building" && job.State != "ready" {
				continue
			}
			v, e := a.prepareExportCancellation(r.Context(), id, q.OperationID, "invalidated")
			if e != nil {
				return e
			}
			pending = append(pending, v)
		}
		for _, v := range pending {
			if e := a.finishExportCancellation(r.Context(), v); e != nil {
				return e
			}
		}
		if e := a.drainExportCancellations(r.Context(), q.JobIDs); e != nil {
			return e
		}
		respond(w, 200, map[string]any{"state": "cleared", "jobs": len(pending)})
		return nil
	}))
}
