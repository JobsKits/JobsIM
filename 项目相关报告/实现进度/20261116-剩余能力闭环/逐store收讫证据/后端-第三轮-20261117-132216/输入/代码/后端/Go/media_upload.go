package jobsim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) mediaUploadRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/media/capabilities", a.mediaUploadCapabilities)
	h("POST /api/media/upload-results", a.mediaUploadResults)
}

func (a *App) mediaUploadCapabilities(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.mediaUploadPrincipal(r); err != nil {
		return err
	}
	if err := a.requireFeature(r.Context(), "media"); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"protocol": mediaUploadProtocol, "supported": true, "maxBytes": str(mediaUploadFileLimit), "maxRequestBytes": str(mediaUploadRequestLimit), "resultPath": "/api/media/upload-results"})
	return nil
}

func (a *App) mediaUploadQualified(r *http.Request, q mediaUploadRequest) (mediaUploadPrincipal, error) {
	p, err := a.mediaUploadPrincipal(r)
	if err == nil {
		err = a.validateMediaUploadContext(r.Context(), q, p)
	}
	return p, err
}

func (a *App) mediaUploadResults(w http.ResponseWriter, r *http.Request) error {
	var q mediaUploadRequest
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if _, _, err := validateMediaUploadRequest(q); err != nil {
		return err
	}
	p, err := a.mediaUploadQualified(r, q)
	if err != nil {
		return err
	}
	d, err := scanMediaUpload(a.DB.QueryRow(r.Context(), `SELECT `+mediaUploadColumns+` FROM media.upload_requests WHERE scope_id=$1 AND request_id=$2`, a.Config.DeploymentID, q.RequestID))
	if err == pgx.ErrNoRows {
		return fail(404, "UPLOAD_NOT_FOUND", "Original upload responsibility was not found; lookup never creates it")
	}
	if err != nil {
		return err
	}
	if err = a.matchMediaUpload(q, p, d); err != nil {
		return err
	}
	if err = a.mediaUploadGate(r.Context(), a.DB, d); err != nil {
		return err
	}
	if d.State == "committed" {
		if err = a.checkCommittedMediaUpload(r.Context(), d); err != nil {
			return err
		}
	}
	respond(w, 200, mediaUploadPublic(d))
	return nil
}

type mediaUploadQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (a *App) mediaUploadGate(ctx context.Context, db mediaUploadQueryer, d mediaUploadRecord) error {
	var retired bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.account_erasure_floors WHERE user_id=$1)
OR EXISTS(SELECT 1 FROM media.retention_floors WHERE object_id=$2 AND (terminal OR due_at<=clock_timestamp()))
OR EXISTS(SELECT 1 FROM media.legacy_erasure_tasks WHERE object_id=$2)`, d.UserID, d.ObjectID).Scan(&retired)
	if err != nil {
		return err
	}
	if retired {
		return fail(410, "UPLOAD_RETIRED", "Original media is covered by an irreversible owner or object floor")
	}
	return nil
}

func (a *App) prepareMediaUpload(ctx context.Context, q mediaUploadRequest, p mediaUploadPrincipal, contentType string, size int64, digest []byte) (mediaUploadRecord, error) {
	d := mediaUploadRecord{RequestID: q.RequestID, UserID: p.UserID, DeviceID: p.DeviceID, FamilyID: p.FamilyID, Epoch: p.Epoch, Security: p.Security, SourceGeneration: p.SourceGeneration, ObjectID: a.avatarObjectID(q.UserID, q.RequestID, "stable-upload"), Name: q.Name, ContentType: contentType, Size: size, Digest: digest, Binding: a.mediaUploadBinding(q, p), State: "prepared"}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)
	if err = a.mediaUploadGate(ctx, tx, d); err != nil {
		return d, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media.upload_requests(scope_id,request_id,user_id,device_id,family_id,restore_epoch,security_version,source_generation,object_id,name,content_type,size_bytes,sha256,request_digest,state)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'prepared') ON CONFLICT DO NOTHING`, a.Config.DeploymentID, d.RequestID, d.UserID, d.DeviceID, d.FamilyID, d.Epoch, d.Security, d.SourceGeneration, d.ObjectID, d.Name, d.ContentType, d.Size, d.Digest, d.Binding)
	if err != nil {
		return d, err
	}
	d, err = scanMediaUpload(tx.QueryRow(ctx, `SELECT `+mediaUploadColumns+` FROM media.upload_requests WHERE scope_id=$1 AND request_id=$2 FOR UPDATE`, a.Config.DeploymentID, q.RequestID))
	if err != nil {
		return d, err
	}
	if err = a.matchMediaUpload(q, p, d); err != nil {
		return d, err
	}
	if d.ContentType != contentType {
		return d, fail(409, "IDEMPOTENCY_CONFLICT", "Original actual media classification differs")
	}
	return d, tx.Commit(ctx)
}

func (a *App) finishMediaUpload(w http.ResponseWriter, r *http.Request, q mediaUploadRequest, temp, contentType string, size int64, digest []byte) error {
	p, err := a.mediaUploadQualified(r, q)
	if err != nil {
		return err
	}
	d, err := a.prepareMediaUpload(r.Context(), q, p, contentType, size, digest)
	if err != nil {
		return err
	}
	if d.State == "committed" {
		if err = a.checkCommittedMediaUpload(r.Context(), d); err != nil {
			return err
		}
		respond(w, 200, mediaUploadPublic(d))
		return nil
	}
	if err = a.retainPreparedMediaUpload(r.Context(), temp, d); err != nil {
		return err
	}
	// Durable minimal intent precedes any final UUID publication. An unknown
	// journal acknowledgement leaves only the original prepared responsibility.
	if err = a.protect(r.Context(), mediaUploadOperation(d.RequestID), "intent", "media.upload", mediaUploadSafety(d)); err != nil {
		return err
	}
	p, err = a.mediaUploadQualified(r, q)
	if err != nil {
		return err
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	d, err = scanMediaUpload(tx.QueryRow(r.Context(), `SELECT `+mediaUploadColumns+` FROM media.upload_requests WHERE scope_id=$1 AND request_id=$2 FOR UPDATE`, a.Config.DeploymentID, q.RequestID))
	if err != nil {
		return err
	}
	if err = a.matchMediaUpload(q, p, d); err != nil {
		return err
	}
	if d.State == "committed" {
		if err = tx.Rollback(r.Context()); err != nil {
			return err
		}
		if err = a.checkCommittedMediaUpload(r.Context(), d); err != nil {
			return err
		}
		respond(w, 200, mediaUploadPublic(d))
		return nil
	}
	if err = a.mediaUploadGate(r.Context(), tx, d); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM media.objects WHERE id=$1)`, d.ObjectID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fail(409, "UPLOAD_OBJECT_CONFLICT", "A prepared upload never republishes an already registered object")
	}
	if err = a.publishMediaUpload(a.mediaUploadStagingPath(d.ObjectID), d); err != nil {
		return err
	}
	if err = os.Remove(a.mediaUploadStagingPath(d.ObjectID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = syncDirectory(filepath.Join(a.Config.StateDir, "Media")); err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,purpose) VALUES($1,$2,$3,$4,$5,$6,$7,'chat_attachment')`, d.ObjectID, d.UserID, d.Name, d.ContentType, d.Size, d.Digest, d.ObjectID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE media.upload_requests SET state='committed',updated_at=now() WHERE scope_id=$1 AND request_id=$2 AND state='prepared'`, a.Config.DeploymentID, d.RequestID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	d.State = "committed"
	respond(w, 200, mediaUploadPublic(d))
	return nil
}

func checkMediaUploadFile(path string, d mediaUploadRecord) error {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() != d.Size {
		return fail(410, "UPLOAD_RETIRED", "Original regular complete upload bytes are unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(410, "UPLOAD_RETIRED", "Original upload bytes are unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || opened.Size() != d.Size {
		return fail(503, "UPLOAD_STORAGE_INTEGRITY", "Original upload changed while opening")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, mediaUploadFileLimit+1))
	if err != nil || n != d.Size || !bytes.Equal(hash.Sum(nil), d.Digest) {
		return fail(409, "UPLOAD_STORAGE_CONFLICT", "Original upload bytes differ; existing files are never overwritten")
	}
	return nil
}

func (a *App) publishMediaUpload(temp string, d mediaUploadRecord) error {
	if !validObjectID(d.ObjectID) || d.State != "prepared" {
		return fail(409, "UPLOAD_NOT_PREPARED", "Original prepared upload required")
	}
	if err := checkMediaUploadFile(temp, d); err != nil {
		return err
	}
	dir := filepath.Join(a.Config.StateDir, "Media")
	final := filepath.Join(dir, d.ObjectID)
	if err := os.Link(temp, final); err != nil {
		if !os.IsExist(err) {
			return err
		}
		if err = checkMediaUploadFile(final, d); err != nil {
			return err
		}
	}
	return syncDirectory(dir)
}

func (a *App) mediaUploadStagingPath(object string) string {
	return filepath.Join(a.Config.StateDir, "Media", ".upload-"+object)
}

func (a *App) retainPreparedMediaUpload(ctx context.Context, temp string, d mediaUploadRecord) error {
	if !validObjectID(d.ObjectID) || d.State != "prepared" {
		return fail(409, "UPLOAD_NOT_PREPARED", "Original prepared upload required")
	}
	if err := a.mediaUploadGate(ctx, a.DB, d); err != nil {
		return err
	}
	if err := checkMediaUploadFile(temp, d); err != nil {
		return err
	}
	stage := a.mediaUploadStagingPath(d.ObjectID)
	if err := os.Link(temp, stage); err != nil {
		if !os.IsExist(err) {
			return err
		}
		if err = checkMediaUploadFile(stage, d); err != nil {
			return err
		}
	}
	return syncDirectory(filepath.Join(a.Config.StateDir, "Media"))
}

func (a *App) checkCommittedMediaUpload(ctx context.Context, d mediaUploadRecord) error {
	if err := a.mediaUploadGate(ctx, a.DB, d); err != nil {
		return err
	}
	var key, name, contentType string
	var size int64
	var digest []byte
	var message *string
	err := a.DB.QueryRow(ctx, `SELECT storage_key,name,content_type,size_bytes,sha256,message_id::text FROM media.objects WHERE id=$1 AND owner_id=$2 AND purpose='chat_attachment'`, d.ObjectID, d.UserID).Scan(&key, &name, &contentType, &size, &digest, &message)
	if err != nil || key != d.ObjectID || name != d.Name || contentType != d.ContentType || size != d.Size || !bytes.Equal(digest, d.Digest) {
		return fail(410, "UPLOAD_RETIRED", "Original registered object is missing or no longer matches its upload")
	}
	if message != nil {
		var plan retentionMediaPlan
		if err = a.rpc(ctx, "message", "/internal/retention/media-plan", map[string]string{"messageId": *message, "objectId": d.ObjectID}, &plan); err != nil {
			return err
		}
		if plan.DeleteScope == "global" {
			return fail(410, "UPLOAD_RETIRED", "Global source erasure rejects the original upload")
		}
		if plan.CleanupEligible {
			due, planErr := mediaPlanDeadline(plan, time.Now().UTC())
			if planErr != nil {
				return planErr
			}
			if !time.Now().Before(due) {
				return fail(410, "UPLOAD_RETIRED", "Original attachment deadline has elapsed")
			}
		}
	}
	return checkMediaUploadFile(filepath.Join(a.Config.StateDir, "Media", d.ObjectID), d)
}
