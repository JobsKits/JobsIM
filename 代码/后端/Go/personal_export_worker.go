package jobsim

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) requirePersonalExportBuild(ctx context.Context, job, user string) error {
	return a.rpc(ctx, "governance", "/internal/personal-exports/build", map[string]string{"jobId": job, "userId": user}, nil)
}

func (a *App) personalExportGovernanceRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/personal-exports/build", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q personalExportHolder
		if err := decode(r, &q); err != nil {
			return err
		}
		job, err := a.exportJob(r.Context(), q.JobID)
		if err != nil {
			return err
		}
		if job.UserID != q.UserID || q.ArtifactID != "" && q.ArtifactID != job.ArtifactID || q.RestoreEpoch != "" && q.RestoreEpoch != job.RestoreEpoch || q.BuildGeneration != "" && q.BuildGeneration != job.BuildGeneration {
			return fail(403, "EXPORT_BUILD_MISMATCH", "Original personal export build required")
		}
		if _, err = a.personalExportView(r.Context(), job); err != nil {
			return err
		}
		if err = a.checkExportJob(r.Context(), job); err != nil {
			return err
		}
		respond(w, 200, map[string]bool{"valid": true})
		return nil
	}))
}

func (a *App) personalExportRefs(ctx context.Context, job exportJob) ([]syncRef, *time.Time, error) {
	rows, err := a.DB.Query(ctx, `SELECT ref FROM governance.personal_export_refs WHERE job_id=$1 ORDER BY owner,kind,object_id`, job.ID)
	if err != nil {
		return nil, nil, err
	}
	refs := []syncRef{}
	for rows.Next() {
		var raw []byte
		var ref syncRef
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if err = json.Unmarshal(raw, &ref); err != nil {
			rows.Close()
			return nil, nil, err
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	var at *time.Time
	if err = a.DB.QueryRow(ctx, `SELECT captured_at FROM governance.export_jobs WHERE id=$1`, job.ID).Scan(&at); err != nil {
		return nil, nil, err
	}
	if at != nil {
		utc := at.UTC()
		at = &utc
	}
	return refs, at, nil
}

func (a *App) capturePersonalExport(ctx context.Context, job exportJob) ([]syncRef, time.Time, error) {
	refs, at, err := a.personalExportRefs(ctx, job)
	if err != nil {
		return nil, time.Time{}, err
	}
	if at != nil {
		return refs, *at, nil
	}
	captured := time.Now().UTC()
	for _, owner := range personalExportOwners() {
		var page struct {
			Refs []syncRef `json:"refs"`
		}
		q := personalExportHolder{JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration, ExpiresAt: job.ExpiresAt}
		if err = a.rpc(ctx, personalExportRole(owner), "/internal/personal-exports/capture", q, &page); err != nil {
			return nil, time.Time{}, err
		}
		for _, ref := range page.Refs {
			if ref.Owner != owner || !validObjectID(ref.ID) || ref.RevisionEpoch != job.RestoreEpoch || !syncValidationCounter(ref.Revision, true) || personalExportRefOwner(ref.Kind) != owner {
				return nil, time.Time{}, fail(503, "EXPORT_MANIFEST_INVALID", "Owner returned an invalid personal export reference")
			}
			refs = append(refs, ref)
		}
	}
	if len(refs) > 40000 {
		return nil, time.Time{}, fail(413, "EXPORT_METADATA_CAPACITY", "Personal export reference capacity exceeded")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer tx.Rollback(ctx)
	var stored *time.Time
	if err = tx.QueryRow(ctx, `SELECT captured_at FROM governance.export_jobs WHERE id=$1 FOR UPDATE`, job.ID).Scan(&stored); err != nil {
		return nil, time.Time{}, err
	}
	if stored != nil {
		return nil, time.Time{}, fail(409, "EXPORT_MANIFEST_CHANGED", "Original export manifest already captured")
	}
	for _, ref := range refs {
		raw, _ := json.Marshal(ref)
		if _, err = tx.Exec(ctx, `INSERT INTO governance.personal_export_refs(job_id,owner,kind,object_id,ref) VALUES($1,$2,$3,$4,$5)`, job.ID, ref.Owner, ref.Kind, ref.ID, raw); err != nil {
			return nil, time.Time{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE governance.export_jobs SET captured_at=$2 WHERE id=$1`, job.ID, captured); err != nil {
		return nil, time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, time.Time{}, err
	}
	return refs, captured, nil
}

func (a *App) runPersonalExportBuild(parent context.Context, id string) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	source, err := a.admissionFence(ctx, false)
	if err != nil {
		return err
	}
	defer func() {
		if source != nil {
			closeExportFence(source)
		}
	}()
	if pending, err := a.admissionMarkers(); err != nil {
		return err
	} else if len(pending) > 0 {
		return fail(503, "ADMISSION_RECOVERY_PENDING", "Resolve prior send admission before capturing personal metadata")
	}
	job, err := a.exportJob(ctx, id)
	if err != nil {
		return err
	}
	if job.State != "requested" && job.State != "building" {
		return nil
	}
	probe, done := context.WithTimeout(ctx, 30*time.Millisecond)
	lock, err := a.exportFenceFile(probe, job.ArtifactID, true)
	done()
	if err != nil {
		return nil
	}
	defer closeExportFence(lock)
	job, err = a.exportJob(ctx, id)
	if err != nil {
		return err
	}
	if job.State != "requested" && job.State != "building" {
		return nil
	}
	if _, err = a.personalExportView(ctx, job); err != nil {
		return err
	}
	if err = a.checkExportJob(ctx, job); err != nil {
		return err
	}
	// Source->artifact is the only acquisition order. Once metadata is written
	// large media streams must not reacquire source SH under artifact EX.
	release := func() {
		if source != nil {
			closeExportFence(source)
			source = nil
		}
	}
	return a.buildPersonalZIPLocked(ctx, job, release)
}

func (a *App) buildPersonalZIPLocked(ctx context.Context, job exportJob, releaseSource func()) error {
	refs, captured, err := a.capturePersonalExport(ctx, job)
	if err != nil {
		return err
	}
	prefix, err := a.exportPrefix(job)
	if err != nil {
		return err
	}
	if err = clearExportPrefix(prefix); err != nil {
		return err
	}
	if err = os.MkdirAll(prefix, 0700); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(prefix)); err != nil {
		return err
	}
	keyText, err := a.unseal(job.Key, "export-key:"+job.ID+":"+job.UserID+":"+job.RestoreEpoch)
	if err != nil {
		return err
	}
	key, err := base64.RawURLEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("invalid wrapped personal export key")
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	token := newID()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE governance.export_jobs SET state='building' WHERE id=$1 AND purpose=$2 AND state IN('requested','building') AND build_generation=$3::bigint`, job.ID, personalExportPurpose, job.BuildGeneration); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE governance.export_artifacts SET state='writing',writer_state='open',writer_token=$2,writer_lease_until=now()+interval '15 minutes' WHERE id=$1 AND state IN('prepared','writing')`, job.ArtifactID, token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "EXPORT_WRITER_FENCED", "Artifact is no longer open")
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(prefix, "package.partial"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	guard := &guardedExportWriter{a: a, ctx: ctx, job: job, token: token, file: f}
	encrypted, err := newChunkWriter(guard, key)
	if err != nil {
		return err
	}
	out := zip.NewWriter(encrypted)
	writeJSON := func(name string, value any) error {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		return writePersonalZIPEntry(out, name, bytes.NewReader(raw), int64(len(raw)))
	}
	if err = writeJSON("account-export.json", map[string]any{"format": "jobsim-personal-data-zip-1", "deploymentId": a.Config.DeploymentID, "sourceContextId": "personal", "userId": job.UserID, "jobId": job.ID, "artifactId": job.ArtifactID, "restoreEpoch": job.RestoreEpoch, "capturedAt": captured, "coverage": personalExportCoverage(), "exclusions": personalExportExclusions(), "includesCredentials": false, "migrationCapable": false}); err != nil {
		return err
	}
	conversationIDs := []string{}
	for _, ref := range refs {
		q := personalExportHolder{JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration, ExpiresAt: job.ExpiresAt, Ref: ref}
		var registration struct {
			RegistrationID string `json:"registrationId"`
			JournalSeq     string `json:"journalSeq"`
		}
		if err = a.rpc(ctx, personalExportRole(ref.Owner), "/internal/personal-exports/register", q, &registration); err != nil {
			return err
		}
		if registration.RegistrationID == "" || !syncValidationCounter(registration.JournalSeq, false) {
			return fail(503, "EXPORT_JOURNAL_REQUIRED", "Personal source holder has no durability proof")
		}
		if _, err = a.DB.Exec(ctx, `UPDATE governance.personal_export_refs SET state='registered',registration_id=$5,journal_seq=$6::bigint WHERE job_id=$1 AND owner=$2 AND kind=$3 AND object_id=$4`, job.ID, ref.Owner, ref.Kind, ref.ID, registration.RegistrationID, registration.JournalSeq); err != nil {
			return err
		}
		var body struct {
			Ref          syncRef `json:"ref"`
			BodyEnvelope string  `json:"bodyEnvelope"`
			BodyDigest   string  `json:"bodyDigest"`
		}
		if err = a.rpc(ctx, personalExportRole(ref.Owner), "/internal/personal-exports/read", q, &body); err != nil {
			return err
		}
		raw, err := base64.RawURLEncoding.DecodeString(body.BodyEnvelope)
		if err != nil || syncDigest(raw) != body.BodyDigest || !personalExportRefEqual(ref, body.Ref) {
			return fail(503, "EXPORT_SOURCE_INTEGRITY", "Personal source body does not match its authenticated reference")
		}
		var value any
		if err = json.Unmarshal(raw, &value); err != nil {
			return err
		}
		stripPersonalExportAuthority(value)
		if ref.Kind == "profile" {
			if err = a.writePersonalAvatar(ctx, out, job, ref, value); err != nil {
				return err
			}
		}
		if err = writeJSON("Data/"+ref.Owner+"/"+ref.Kind+"/"+ref.ID+".json", value); err != nil {
			return err
		}
		if ref.Kind == "conversation" {
			conversationIDs = append(conversationIDs, ref.ID)
		}
	}
	// The metadata bytes already have durable owner grants. Any later metadata
	// mutation closes the whole job before committing its source mutation.
	releaseSource()
	items := []exportSource{}
	cursor := ""
	for {
		var page struct {
			Items    []exportSource `json:"items"`
			Cursor   string         `json:"cursor"`
			Complete bool           `json:"complete"`
		}
		if err = a.rpc(ctx, "message", "/internal/exports/manifest", map[string]any{"jobId": job.ID, "userId": job.UserID, "conversationIds": conversationIDs, "before": captured, "after": cursor, "personal": true}, &page); err != nil {
			return err
		}
		items = append(items, page.Items...)
		if page.Complete {
			break
		}
		if page.Cursor == cursor {
			return fmt.Errorf("personal export message manifest did not advance")
		}
		cursor = page.Cursor
	}
	for _, item := range items {
		if _, err = a.DB.Exec(ctx, `INSERT INTO governance.export_items(job_id,message_id,conversation_id,source_version,attachment_id) VALUES($1,$2,$3,$4::bigint,NULLIF($5,'')::uuid) ON CONFLICT DO NOTHING`, job.ID, item.MessageID, item.ConversationID, item.SourceVersion, item.AttachmentID); err != nil {
			return err
		}
		q := exportHolderRequest{Personal: true, JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, MessageID: item.MessageID, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration, SourceVersion: item.SourceVersion, ExpiresAt: job.ExpiresAt}
		var registration struct {
			RegistrationID string `json:"registrationId"`
			JournalSeq     string `json:"journalSeq"`
		}
		if err = a.rpc(ctx, "message", "/internal/exports/register-message", q, &registration); err != nil {
			return err
		}
		if registration.RegistrationID == "" || registration.JournalSeq == "" {
			return fmt.Errorf("message source durability proof absent")
		}
		if _, err = a.DB.Exec(ctx, `UPDATE governance.export_items SET state='registered',registration_id=$3 WHERE job_id=$1 AND message_id=$2`, job.ID, item.MessageID, registration.RegistrationID); err != nil {
			return err
		}
		var message map[string]any
		if err = a.rpc(ctx, "message", "/internal/exports/read-message", q, &message); err != nil {
			return err
		}
		if message["id"] != item.MessageID || message["version"] != item.SourceVersion {
			return fail(409, "EXPORT_SOURCE_CHANGED", "Frozen message source differs")
		}
		stripPersonalExportAuthority(message)
		if item.AttachmentID != "" {
			message["attachmentPath"] = "Media/" + item.AttachmentID
		}
		if err = writeJSON("Messages/"+item.ConversationID+"/"+item.MessageID+".json", message); err != nil {
			return err
		}
		if item.AttachmentID != "" {
			q.ObjectID = item.AttachmentID
			var media struct {
				ExpiresAt  time.Time `json:"expiresAt"`
				JournalSeq string    `json:"journalSeq"`
			}
			if err = a.rpc(ctx, "media", "/internal/exports/register-media", q, &media); err != nil {
				return err
			}
			if !syncValidationCounter(media.JournalSeq, false) {
				return fmt.Errorf("media durability proof absent")
			}
			if _, err = a.DB.Exec(ctx, `UPDATE governance.export_jobs SET expires_at=LEAST(expires_at,$2) WHERE id=$1`, job.ID, media.ExpiresAt); err != nil {
				return err
			}
			if err = a.copyPersonalExportMedia(ctx, out, guard, q); err != nil {
				return err
			}
		}
	}
	if err = writeJSON("manifest.json", map[string]any{"formatVersion": 1, "capturedAt": captured, "metadataRefs": refs, "messages": items, "includesCredentials": false, "forwardPolicy": "placeholder_only"}); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = encrypted.Close(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = a.validatePersonalExportSources(ctx, job); err != nil {
		return err
	}
	if err = a.checkExportJob(ctx, job); err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(prefix, "package.partial"), filepath.Join(prefix, "package.jimexport")); err != nil {
		return err
	}
	if err = syncDirectory(prefix); err != nil {
		return err
	}
	digest, err := fileDigest(filepath.Join(prefix, "package.jimexport"))
	if err != nil {
		return err
	}
	hash, err := hex.DecodeString(digest.Digest)
	if err != nil {
		return err
	}
	tx, err = a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err = tx.Exec(ctx, `UPDATE governance.export_jobs SET state='ready' WHERE id=$1 AND state='building' AND purpose=$2 AND build_generation=$3::bigint AND restore_epoch=$4::bigint AND expires_at>now() AND key_ciphertext IS NOT NULL`, job.ID, personalExportPurpose, job.BuildGeneration, job.RestoreEpoch)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "EXPORT_WRITER_FENCED", "Personal export was invalidated before publication")
	}
	if _, err = tx.Exec(ctx, `UPDATE governance.export_artifacts SET state='published',writer_state='closed',writer_token=NULL,writer_lease_until=NULL,size_bytes=$2,content_digest=$3 WHERE id=$1 AND writer_token=$4 AND state='writing'`, job.ArtifactID, digest.Size, hash, token); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func personalExportRefEqual(first, second syncRef) bool {
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	return bytes.Equal(a, b)
}

func writePersonalZIPEntry(out *zip.Writer, name string, src io.Reader, size int64) error {
	if size < 0 || size > 64<<20 {
		return fail(413, "EXPORT_ENTRY_CAPACITY", "Personal export entry limit exceeded")
	}
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0600)
	entry, err := out.CreateHeader(header)
	if err != nil {
		return err
	}
	n, err := io.Copy(entry, io.LimitReader(src, size+1))
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("personal ZIP entry size differs")
	}
	return nil
}

func (a *App) copyPersonalExportMedia(ctx context.Context, out *zip.Writer, guard *guardedExportWriter, q exportHolderRequest) error {
	body, _ := json.Marshal(q)
	request, err := http.NewRequestWithContext(ctx, "POST", a.Config.RoleURLs["media"]+"/internal/exports/read-media", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-JobsIM-Service-Key", base64.RawURLEncoding.EncodeToString(a.Config.Key))
	request.Header.Set("X-JobsIM-Caller", a.Role)
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return fail(503, "EXPORT_MEDIA_STREAM_UNAVAILABLE", "Media stream unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fail(409, "EXPORT_MEDIA_SOURCE_CHANGED", "Media stream was not authorized")
	}
	if !hmac.Equal([]byte(response.Header.Get("X-JobsIM-Admission-Fence")), []byte(a.admissionFenceIdentity())) {
		return fail(503, "ADMISSION_FENCE_MISMATCH", "Media export source must share the canonical local instance")
	}
	size, err := strconv.ParseInt(response.Header.Get("X-JobsIM-Size"), 10, 64)
	if err != nil || size < 0 || size > 64<<20 {
		return fmt.Errorf("invalid personal export media size")
	}
	expected := response.Header.Get("X-JobsIM-SHA256")
	if len(expected) != 64 {
		return fmt.Errorf("personal export media digest absent")
	}
	nameBytes, err := base64.RawURLEncoding.DecodeString(response.Header.Get("X-JobsIM-Name"))
	if err != nil || len(nameBytes) > 4096 {
		return fail(503, "EXPORT_SOURCE_INTEGRITY", "Media name metadata invalid")
	}
	contentType := response.Header.Get("X-JobsIM-Content-Type")
	if contentType == "" || len(contentType) > 256 {
		return fail(503, "EXPORT_SOURCE_INTEGRITY", "Media content type metadata invalid")
	}
	metadata, _ := json.Marshal(map[string]any{"id": q.ObjectID, "name": string(nameBytes), "contentType": contentType, "size": str(size), "sha256": expected, "path": "Media/" + q.ObjectID})
	if err = writePersonalZIPEntry(out, "Media/"+q.ObjectID+".json", bytes.NewReader(metadata), int64(len(metadata))); err != nil {
		return err
	}
	header := &zip.FileHeader{Name: "Media/" + q.ObjectID, Method: zip.Store}
	header.SetMode(0600)
	entry, err := out.CreateHeader(header)
	if err != nil {
		return err
	}
	hash := sha256.New()
	reader := io.LimitReader(response.Body, size+1)
	buffer := make([]byte, 65536)
	var total int64
	for {
		if err = a.checkExportJob(ctx, guard.job); err != nil {
			return err
		}
		n, re := reader.Read(buffer)
		if n > 0 {
			if total+int64(n) > size {
				return fmt.Errorf("personal export media exceeds size")
			}
			if _, err = entry.Write(buffer[:n]); err != nil {
				return err
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
		return fmt.Errorf("personal export media integrity differs")
	}
	return nil
}

func (a *App) validatePersonalExportSources(ctx context.Context, job exportJob) error {
	refs, at, err := a.personalExportRefs(ctx, job)
	if err != nil {
		return err
	}
	if at == nil {
		return fail(409, "EXPORT_NOT_READY", "Personal export manifest not captured")
	}
	for _, ref := range refs {
		q := personalExportHolder{JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration, ExpiresAt: job.ExpiresAt, Ref: ref}
		if err = a.rpc(ctx, personalExportRole(ref.Owner), "/internal/personal-exports/check", q, nil); err != nil {
			return err
		}
	}
	rows, err := a.DB.Query(ctx, `SELECT message_id::text,source_version::text FROM governance.export_items WHERE job_id=$1 ORDER BY message_id`, job.ID)
	if err != nil {
		return err
	}
	sources := []exportSource{}
	for rows.Next() {
		var source exportSource
		if err = rows.Scan(&source.MessageID, &source.SourceVersion); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, source)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, source := range sources {
		q := exportHolderRequest{Personal: true, JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, MessageID: source.MessageID, SourceVersion: source.SourceVersion, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration}
		if err = a.rpc(ctx, "message", "/internal/exports/check-message", q, nil); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) clearPersonalExportHolders(ctx context.Context, job exportJob) error {
	var purpose string
	if err := a.DB.QueryRow(ctx, `SELECT purpose FROM governance.export_jobs WHERE id=$1`, job.ID).Scan(&purpose); err != nil {
		return err
	}
	if purpose != personalExportPurpose {
		return nil
	}
	for _, owner := range personalExportOwners() {
		q := personalExportHolder{JobID: job.ID, ArtifactID: job.ArtifactID, UserID: job.UserID, RestoreEpoch: job.RestoreEpoch, BuildGeneration: job.BuildGeneration}
		if err := a.rpc(ctx, personalExportRole(owner), "/internal/personal-exports/clear-holders", q, nil); err != nil {
			return err
		}
	}
	if err := a.rpc(ctx, "media", "/internal/personal-exports/clear-avatar-holders", map[string]string{"jobId": job.ID, "artifactId": job.ArtifactID}, nil); err != nil {
		return err
	}
	_, err := a.DB.Exec(ctx, `UPDATE governance.personal_export_refs SET state='cleared' WHERE job_id=$1`, job.ID)
	return err
}

// The target epoch never regains a source grant from a restored metadata row.
func CancelRestoredPersonalExportHolders(ctx context.Context, business *pgx.Conn) error {
	for _, owner := range personalExportOwners() {
		var exists bool
		if err := business.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, owner+".personal_export_holders").Scan(&exists); err != nil {
			return err
		}
		if exists {
			if _, err := business.Exec(ctx, "UPDATE "+owner+".personal_export_holders SET state='cleared' WHERE state<>'cleared'"); err != nil {
				return err
			}
			var cleanupExists bool
			if err := business.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, owner+".personal_export_cleanups").Scan(&cleanupExists); err != nil {
				return err
			}
			if cleanupExists {
				if _, err := business.Exec(ctx, "UPDATE "+owner+".personal_export_cleanups SET state='cleared',updated_at=now() WHERE state<>'cleared'"); err != nil {
					return err
				}
			}
		}
	}
	var avatarExists bool
	if err := business.QueryRow(ctx, `SELECT to_regclass('media.personal_export_avatar_holders') IS NOT NULL`).Scan(&avatarExists); err != nil {
		return err
	}
	if avatarExists {
		if _, err := business.Exec(ctx, `UPDATE media.personal_export_avatar_holders SET state='cleared' WHERE state<>'cleared'`); err != nil {
			return err
		}
	}
	var refsExist bool
	if err := business.QueryRow(ctx, `SELECT to_regclass('governance.personal_export_refs') IS NOT NULL`).Scan(&refsExist); err != nil {
		return err
	}
	if refsExist {
		if _, err := business.Exec(ctx, `UPDATE governance.personal_export_refs SET state='cleared' WHERE state<>'cleared'`); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) drainExportCancellations(ctx context.Context, jobs []string) error {
	for batch := 0; batch < 50; batch++ {
		rows, err := a.DB.Query(ctx, `SELECT job_id::text,artifact_id::text,restore_epoch::text,build_generation::text,operation_id,terminal_state FROM governance.export_cancellations WHERE job_id=ANY($1::uuid[]) AND state<>'cleared' ORDER BY created_at LIMIT 20`, jobs)
		if err != nil {
			return err
		}
		pending := []exportCancellation{}
		for rows.Next() {
			var v exportCancellation
			if err = rows.Scan(&v.JobID, &v.ArtifactID, &v.Epoch, &v.Generation, &v.Operation, &v.Terminal); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		for _, v := range pending {
			if err = a.finishExportCancellation(ctx, v); err != nil {
				return err
			}
		}
	}
	return fail(503, "EXPORT_CLEANUP_PENDING", "Original export cancellation continuations remain pending")
}
