package jobsim

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var exportWorkerSlots = make(chan struct{}, 2)

func (a *App) StartExportWorkers(ctx context.Context) {
	a.startExportLoop(ctx, true)
}

func (a *App) StartExportCleanup(ctx context.Context) {
	a.startExportLoop(ctx, false)
}

func (a *App) startExportLoop(ctx context.Context, allowBuild bool) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pending, e := a.DB.Query(ctx, `SELECT job_id::text,artifact_id::text,restore_epoch::text,build_generation::text,operation_id,terminal_state FROM governance.export_cancellations WHERE state<>'cleared' ORDER BY created_at LIMIT 10`)
				if e != nil {
					return
				}
				continuations := []exportCancellation{}
				for pending.Next() {
					var v exportCancellation
					if e = pending.Scan(&v.JobID, &v.ArtifactID, &v.Epoch, &v.Generation, &v.Operation, &v.Terminal); e != nil {
						break
					}
					continuations = append(continuations, v)
				}
				pending.Close()
				for _, v := range continuations {
					select {
					case exportWorkerSlots <- struct{}{}:
						go func(c exportCancellation) {
							defer func() { <-exportWorkerSlots }()
							attempt, done := context.WithTimeout(ctx, 5*time.Second)
							defer done()
							_ = a.finishExportCancellation(attempt, c)
						}(v)
					default:
					}
				}
				rows, e := a.DB.Query(ctx, `SELECT id::text FROM governance.export_jobs WHERE state IN('requested','building','ready') ORDER BY created_at LIMIT 100`)
				if e != nil {
					return
				}
				ids := []string{}
				for rows.Next() {
					var id string
					if e = rows.Scan(&id); e != nil {
						break
					}
					ids = append(ids, id)
				}
				rows.Close()
				if e != nil {
					continue
				}
				for _, id := range ids {
					select {
					case exportWorkerSlots <- struct{}{}:
						go func(job string) {
							defer func() { <-exportWorkerSlots }()
							run, e := a.exportJob(ctx, job)
							if e != nil {
								return
							}
							if !allowBuild {
								_ = a.cancelExportTask(ctx, job, "feature-disabled", "cancelled")
								return
							}
							if !time.Now().Before(run.ExpiresAt) {
								_ = a.cancelExportTask(ctx, job, "expiry", "expired")
								return
							}
							if run.State == "ready" {
								if e = a.checkExportJob(ctx, run); e != nil {
									_ = a.cancelExportTask(ctx, job, "principal-invalid", "invalidated")
								}
								return
							}
							e = a.runExportBuild(ctx, job)
							if e != nil {
								cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
								defer cancel()
								operation := "builder-failed"
								var cause *apiError
								if errors.As(e, &cause) && cause.Code == "FORWARD_EXPORT_UNSUPPORTED" {
									operation = "builder-forward-unsupported"
								}
								_ = a.cancelExportTask(cleanup, job, operation, "failed")
							}
						}(id)
					default:
					}
				}
			}
		}
	}()
}

type guardedExportWriter struct {
	a       *App
	ctx     context.Context
	job     exportJob
	token   string
	file    *os.File
	written int64
}

func (g *guardedExportWriter) Write(p []byte) (int, error) {
	if g.written+int64(len(p)) > 1<<30 {
		return 0, fail(413, "EXPORT_BYTE_CAPACITY", "Export slice limit is one GiB; no partial package is published")
	}
	if e := g.a.checkExportJob(g.ctx, g.job); e != nil {
		return 0, e
	}
	var valid bool
	e := g.a.DB.QueryRow(g.ctx, `SELECT state='writing' AND writer_state='open' AND writer_token=$2 AND writer_lease_until>now() AND restore_epoch=$3::bigint AND build_generation=$4::bigint FROM governance.export_artifacts WHERE id=$1`, g.job.ArtifactID, g.token, g.job.RestoreEpoch, g.job.BuildGeneration).Scan(&valid)
	if e != nil {
		return 0, e
	}
	if !valid {
		return 0, fail(409, "EXPORT_WRITER_FENCED", "Export writer no longer owns this build")
	}
	n, e := g.file.Write(p)
	g.written += int64(n)
	return n, e
}
func (a *App) runExportBuild(parent context.Context, id string) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	job, e := a.exportJob(ctx, id)
	if e != nil {
		return e
	}
	if job.State != "requested" && job.State != "building" {
		return nil
	}
	lockCtx, done := context.WithTimeout(ctx, 30*time.Millisecond)
	lock, e := a.exportFenceFile(lockCtx, job.ArtifactID, true)
	done()
	if e != nil {
		return nil
	}
	defer closeExportFence(lock)
	job, e = a.exportJob(ctx, id)
	if e != nil {
		return e
	}
	if job.State != "requested" && job.State != "building" {
		return nil
	}
	if e = a.checkExportJob(ctx, job); e != nil {
		return e
	}
	return a.buildChatExportLocked(ctx, job)
}
func (a *App) buildChatExportLocked(ctx context.Context, job exportJob) error {
	prefix, e := a.exportPrefix(job)
	if e != nil {
		return e
	}
	if e = clearExportPrefix(prefix); e != nil {
		return e
	}
	if e = os.MkdirAll(prefix, 0700); e != nil {
		return e
	}
	if e = syncDirectory(filepath.Dir(prefix)); e != nil {
		return e
	}
	keyText, e := a.unseal(job.Key, "export-key:"+job.ID+":"+job.UserID+":"+job.RestoreEpoch)
	if e != nil {
		return e
	}
	key, e := base64.RawURLEncoding.DecodeString(keyText)
	if e != nil || len(key) != 32 {
		return fmt.Errorf("invalid wrapped export key")
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	token := newID()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE governance.export_jobs SET state='building' WHERE id=$1 AND state IN('requested','building') AND build_generation=$2::bigint`, job.ID, job.BuildGeneration); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE governance.export_artifacts SET state='writing',writer_state='open',writer_token=$2,writer_lease_until=now()+interval '15 minutes' WHERE id=$1 AND state IN('prepared','writing')`, job.ArtifactID, token)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "EXPORT_WRITER_FENCED", "Artifact is no longer open")
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(prefix, "package.partial"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	guard := &guardedExportWriter{a: a, ctx: ctx, job: job, token: token, file: f}
	encrypted, e := newChunkWriter(guard, key)
	if e != nil {
		return e
	}
	tarOut := tar.NewWriter(encrypted)
	writeJSON := func(name string, value any) error {
		data, e := json.Marshal(value)
		if e != nil {
			return e
		}
		if e = tarOut.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), ModTime: time.Now().UTC()}); e != nil {
			return e
		}
		_, e = tarOut.Write(data)
		return e
	}
	if e = writeJSON("account.json", map[string]any{"formatVersion": 1, "deploymentId": a.Config.DeploymentID, "sourceContextId": "personal", "userId": job.UserID, "jobId": job.ID, "artifactId": job.ArtifactID, "restoreEpoch": job.RestoreEpoch, "buildGeneration": job.BuildGeneration, "scope": "current_authorized_chat_messages_and_attachments", "includesCredentials": false}); e != nil {
		return e
	}
	var convs struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if e = a.rpc(ctx, "conversation", "/internal/export", map[string]string{"userId": job.UserID}, &convs); e != nil {
		return e
	}
	ids := []string{}
	for _, c := range convs.Items {
		ids = append(ids, c.ID)
	}
	var before time.Time
	if e = a.DB.QueryRow(ctx, `SELECT created_at FROM governance.export_jobs WHERE id=$1`, job.ID).Scan(&before); e != nil {
		return e
	}
	items := []exportSource{}
	cursor := ""
	for {
		var page struct {
			Items    []exportSource `json:"items"`
			Cursor   string         `json:"cursor"`
			Complete bool           `json:"complete"`
		}
		if e = a.rpc(ctx, "message", "/internal/exports/manifest", map[string]any{"jobId": job.ID, "userId": job.UserID, "conversationIds": ids, "before": before, "after": cursor}, &page); e != nil {
			return e
		}
		items = append(items, page.Items...)
		if page.Complete {
			break
		}
		if page.Cursor == cursor {
			return fmt.Errorf("export manifest did not advance")
		}
		cursor = page.Cursor
	}
	for _, item := range items {
		_, e = a.DB.Exec(ctx, `INSERT INTO governance.export_items(job_id,message_id,conversation_id,source_version,attachment_id) VALUES($1,$2,$3,$4::bigint,NULLIF($5,'')::uuid) ON CONFLICT DO NOTHING`, job.ID, item.MessageID, item.ConversationID, item.SourceVersion, item.AttachmentID)
		if e != nil {
			return e
		}
		q := exportHolderRequest{JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, MessageID: item.MessageID, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration, SourceVersion: item.SourceVersion, ExpiresAt: job.ExpiresAt}
		var registration struct {
			RegistrationID string `json:"registrationId"`
			JournalSeq     string `json:"journalSeq"`
		}
		if e = a.rpc(ctx, "message", "/internal/exports/register-message", q, &registration); e != nil {
			return e
		}
		if registration.RegistrationID == "" || registration.JournalSeq == "" {
			return fmt.Errorf("source holder durability proof absent")
		}
		if _, e = a.DB.Exec(ctx, `UPDATE governance.export_items SET state='registered',registration_id=$3 WHERE job_id=$1 AND message_id=$2`, job.ID, item.MessageID, registration.RegistrationID); e != nil {
			return e
		}
		var message Message
		if e = a.rpc(ctx, "message", "/internal/exports/read-message", q, &message); e != nil {
			return e
		}
		if message.ID != item.MessageID || message.Version != item.SourceVersion || message.Deleted {
			return fail(409, "EXPORT_SOURCE_CHANGED", "Frozen message source differs")
		}
		if e = writeJSON("Messages/"+item.ConversationID+"/"+item.MessageID+".json", message); e != nil {
			return e
		}
		if item.AttachmentID != "" {
			q.ObjectID = item.AttachmentID
			var media struct {
				ExpiresAt  time.Time `json:"expiresAt"`
				JournalSeq string    `json:"journalSeq"`
			}
			if e = a.rpc(ctx, "media", "/internal/exports/register-media", q, &media); e != nil {
				return e
			}
			if media.JournalSeq == "" {
				return fmt.Errorf("media holder durability proof absent")
			}
			if _, e = a.DB.Exec(ctx, `UPDATE governance.export_jobs SET expires_at=LEAST(expires_at,$2) WHERE id=$1`, job.ID, media.ExpiresAt); e != nil {
				return e
			}
			if e = a.copyExportMedia(ctx, tarOut, guard, q); e != nil {
				return e
			}
		}
	}
	if e = writeJSON("manifest.json", map[string]any{"messages": items, "formatVersion": 1, "includesCredentials": false, "scope": "current_authorized_chat_messages_and_attachments"}); e != nil {
		return e
	}
	if e = tarOut.Close(); e != nil {
		return e
	}
	if e = encrypted.Close(); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = a.validateExportSources(ctx, job); e != nil {
		return e
	}
	if e = a.checkExportJob(ctx, job); e != nil {
		return e
	}
	if e = os.Rename(filepath.Join(prefix, "package.partial"), filepath.Join(prefix, "package.jimexport")); e != nil {
		return e
	}
	if e = syncDirectory(prefix); e != nil {
		return e
	}
	file, e := fileDigest(filepath.Join(prefix, "package.jimexport"))
	if e != nil {
		return e
	}
	hash, e := hex.DecodeString(file.Digest)
	if e != nil {
		return e
	}
	tx, e = a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e = tx.Exec(ctx, `UPDATE governance.export_jobs SET state='ready',key_ciphertext=NULL WHERE id=$1 AND state='building' AND build_generation=$2::bigint AND restore_epoch=$3::bigint AND expires_at>now()`, job.ID, job.BuildGeneration, job.RestoreEpoch)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "EXPORT_WRITER_FENCED", "Export was invalidated before publication")
	}
	if _, e = tx.Exec(ctx, `UPDATE governance.export_artifacts SET state='published',writer_state='closed',writer_token=NULL,writer_lease_until=NULL,size_bytes=$2,content_digest=$3 WHERE id=$1 AND writer_token=$4`, job.ArtifactID, file.Size, hash, token); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (a *App) copyExportMedia(ctx context.Context, out *tar.Writer, guard *guardedExportWriter, q exportHolderRequest) error {
	body, _ := json.Marshal(q)
	request, e := http.NewRequestWithContext(ctx, "POST", a.Config.RoleURLs["media"]+"/internal/exports/read-media", bytes.NewReader(body))
	if e != nil {
		return e
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-JobsIM-Service-Key", base64.RawURLEncoding.EncodeToString(a.Config.Key))
	request.Header.Set("X-JobsIM-Caller", a.Role)
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(request)
	if e != nil {
		return fail(503, "EXPORT_MEDIA_STREAM_UNAVAILABLE", "Media stream unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fail(409, "EXPORT_MEDIA_SOURCE_CHANGED", "Media stream was not authorized")
	}
	size, e := strconv.ParseInt(response.Header.Get("X-JobsIM-Size"), 10, 64)
	if e != nil || size < 0 || size > 64<<20 {
		return fmt.Errorf("invalid export media size")
	}
	expected := response.Header.Get("X-JobsIM-SHA256")
	if len(expected) != 64 {
		return fmt.Errorf("media stream digest absent")
	}
	if e = out.WriteHeader(&tar.Header{Name: "Media/" + q.ObjectID, Mode: 0600, Size: size}); e != nil {
		return e
	}
	hash := sha256.New()
	limited := io.LimitReader(response.Body, size+1)
	buffer := make([]byte, 65536)
	var total int64
	for {
		if e = a.checkExportJob(ctx, guard.job); e != nil {
			return e
		}
		n, re := limited.Read(buffer)
		if n > 0 {
			if total+int64(n) > size {
				return fmt.Errorf("export media exceeded its declared size")
			}
			if _, e = out.Write(buffer[:n]); e != nil {
				return e
			}
			hash.Write(buffer[:n])
			total += int64(n)
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return re
		}
	}
	if total != size || hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("media stream integrity mismatch")
	}
	return nil
}
func (a *App) validateExportSources(ctx context.Context, v exportJob) error {
	rows, e := a.DB.Query(ctx, `SELECT message_id::text,source_version FROM governance.export_items WHERE job_id=$1 ORDER BY message_id`, v.ID)
	if e != nil {
		return e
	}
	type source struct{ id, version string }
	sources := []source{}
	for rows.Next() {
		var id string
		var version int64
		if e = rows.Scan(&id, &version); e != nil {
			rows.Close()
			return e
		}
		sources = append(sources, source{id, str(version)})
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, item := range sources {
		q := exportHolderRequest{JobID: v.ID, ArtifactID: v.ArtifactID, UserID: v.UserID, MessageID: item.id, RestoreEpoch: v.RestoreEpoch, BuildGeneration: v.BuildGeneration, SourceVersion: item.version}
		if e = a.rpc(ctx, "message", "/internal/exports/check-message", q, nil); e != nil {
			return e
		}
	}
	return nil
}

func (a *App) exportSourceCleanupRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	schema := map[string]string{"message": "message", "media": "media"}[a.Role]
	if schema == "" {
		return
	}
	h("POST /internal/exports/clear-holders", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			JobID      string `json:"jobId"`
			ArtifactID string `json:"artifactId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if e := a.rpc(r.Context(), "governance", "/internal/exports/cleared", q, nil); e != nil {
			return e
		}
		if _, e := a.DB.Exec(r.Context(), fmt.Sprintf(`UPDATE %s.export_holders SET state='cleared' WHERE job_id=$1 AND artifact_id=$2`, schema), q.JobID, q.ArtifactID); e != nil {
			return e
		}
		respond(w, 200, map[string]string{"state": "cleared"})
		return nil
	}))
}
