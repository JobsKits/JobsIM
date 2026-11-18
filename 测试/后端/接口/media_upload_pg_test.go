package jobsim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Real isolated PostgreSQL and production role handlers; role RPC remains
// in-process. No real HTTPS, browser, microphone or native-device claim.
func mediaUploadPGRequest(t *testing.T, f *deletionPGFixture, data []byte) (mediaUploadRequest, mediaUploadPrincipal) {
	t.Helper()
	var p mediaUploadPrincipal
	err := f.nodes["identity"].DB.QueryRow(f.ctx, `SELECT user_id::text,device_id::text,family_id::text,restore_epoch::text,security_version::text,source_generation::text FROM auth.sessions WHERE token_digest=$1`, tokenDigest(f.token)).Scan(&p.UserID, &p.DeviceID, &p.FamilyID, &p.Epoch, &p.Security, &p.SourceGeneration)
	if err != nil {
		t.Fatal("owned upload principal fixture unavailable")
	}
	digest := sha256.Sum256(data)
	return mediaUploadRequest{RequestID: newID(), UserID: p.UserID, DeviceID: p.DeviceID, FamilyID: p.FamilyID, Epoch: p.Epoch, Name: "original-fixture.txt", Size: str(int64(len(data))), Digest: hex.EncodeToString(digest[:])}, p
}

func mediaUploadPGCall(t *testing.T, a *App, token string, q *mediaUploadRequest, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var raw bytes.Buffer
	form := multipart.NewWriter(&raw)
	if q != nil {
		for key, value := range map[string]string{"requestId": q.RequestID, "userId": q.UserID, "deviceId": q.DeviceID, "familyId": q.FamilyID, "restoreEpoch": q.Epoch, "name": q.Name, "size": q.Size, "sha256": q.Digest} {
			if err := form.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	file, err := form.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/media", bytes.NewReader(raw.Bytes()))
	r.Header.Set("Content-Type", form.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func mediaUploadPGOriginal(t *testing.T, f *deletionPGFixture, q mediaUploadRequest) mediaUploadRecord {
	t.Helper()
	d, err := scanMediaUpload(f.nodes["media"].DB.QueryRow(f.ctx, `SELECT `+mediaUploadColumns+` FROM media.upload_requests WHERE scope_id=$1 AND request_id=$2`, f.nodes["media"].Config.DeploymentID, q.RequestID))
	if err != nil {
		t.Fatal("owned original upload responsibility unavailable")
	}
	return d
}

func TestMediaUploadPGOriginalResultAndStableRetry(t *testing.T) {
	f := newDeletionPG(t, 7)
	a := f.nodes["media"]
	data := []byte("non-secret original stable upload fixture")
	q, _ := mediaUploadPGRequest(t, f, data)
	deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 404)
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM media.upload_requests`) != 0 || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 0 {
		t.Fatal("missing original lookup created a request or object")
	}
	first := deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 200)
	if first["uploadState"] != "committed" || first["protocol"] != mediaUploadProtocol || first["sha256"] != q.Digest {
		t.Fatal("stable original upload was not acknowledged")
	}
	query := deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 200)
	retry := deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 200)
	if query["id"] != first["id"] || retry["id"] != first["id"] || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 1 || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.upload_requests`) != 1 {
		t.Fatal("original lookup or same-body retry changed object identity")
	}
	id := first["id"].(string)
	if _, err := os.Lstat(a.mediaUploadStagingPath(id)); !os.IsNotExist(err) {
		t.Fatal("committed upload retained a duplicate staging source")
	}
	changed := q
	changed.Name = "different-fixture.txt"
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &changed, changed.Name, data), 409)
	different := []byte("different same-nonce original fixture")
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, different), 409)
	changed = q
	digest := sha256.Sum256(different)
	changed.Size, changed.Digest = str(int64(len(different))), hex.EncodeToString(digest[:])
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &changed, changed.Name, different), 409)
	bytesNow, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", id))
	if err != nil || !bytes.Equal(bytesNow, data) || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 1 {
		t.Fatal("conflicting original overwrote committed bytes or created an object")
	}
}

func TestMediaUploadPGPreparedJournalUnknownAndPublishedCrashResume(t *testing.T) {
	f := newDeletionPG(t, 7)
	a := f.nodes["media"]
	data := []byte("non-secret crash recovery upload fixture")
	q, _ := mediaUploadPGRequest(t, f, data)
	f.wire.failKind, f.wire.failPhase = "media.upload", "intent"
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 503)
	d := mediaUploadPGOriginal(t, f, q)
	if d.State != "prepared" || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 0 {
		t.Fatal("unknown safety acknowledgement published a registered object")
	}
	if err := checkMediaUploadFile(a.mediaUploadStagingPath(d.ObjectID), d); err != nil {
		t.Fatal("prepared responsibility lost its complete private staging bytes")
	}
	prepared := deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 200)
	if prepared["uploadState"] != "prepared" || prepared["url"] != nil {
		t.Fatal("pure lookup falsely delivered prepared bytes")
	}
	if _, err := os.Stat(filepath.Join(a.Config.StateDir, "Media", d.ObjectID)); !os.IsNotExist(err) {
		t.Fatal("lookup published a prepared final UUID")
	}
	f.wire.failKind = ""
	deletionFence(t, a, func(ctx context.Context) error {
		if err := a.protect(ctx, mediaUploadOperation(d.RequestID), "intent", "media.upload", mediaUploadSafety(d)); err != nil {
			return err
		}
		return a.publishMediaUpload(a.mediaUploadStagingPath(d.ObjectID), d)
	})
	before, err := os.Stat(filepath.Join(a.Config.StateDir, "Media", d.ObjectID))
	if err != nil {
		t.Fatal("controlled crash boundary final bytes unavailable")
	}
	deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 200)
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 0 {
		t.Fatal("pure lookup repaired a file-published original")
	}
	result := deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 200)
	after, err := os.Stat(filepath.Join(a.Config.StateDir, "Media", d.ObjectID))
	if err != nil || result["id"] != d.ObjectID || !os.SameFile(before, after) {
		t.Fatal("explicit recovery replaced the original final file or object ID")
	}
}

func TestMediaUploadPGCommittedMissingOrFlooredNeverRepublishes(t *testing.T) {
	for _, mode := range []string{"missing", "retention", "owner"} {
		t.Run(mode, func(t *testing.T) {
			f := newDeletionPG(t, 7)
			a := f.nodes["media"]
			data := []byte("non-secret retired source fixture")
			q, _ := mediaUploadPGRequest(t, f, data)
			result := deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 200)
			id := result["id"].(string)
			if err := os.Remove(filepath.Join(a.Config.StateDir, "Media", id)); err != nil {
				t.Fatal(err)
			}
			if mode == "owner" {
				deletionExec(t, f.ctx, a, `INSERT INTO media.account_erasure_floors(user_id,deletion_id,request_version) VALUES($1,$2,2)`, q.UserID, newID())
			}
			if mode == "retention" {
				deletionExec(t, f.ctx, a, `INSERT INTO media.retention_floors(scope_id,object_id,message_id,restore_epoch,content_version,generation,policy_version,manifest_digest,due_at,terminal) VALUES($1,$2,$3,1,1,1,1,$4,clock_timestamp(),true)`, a.Config.DeploymentID, id, newID(), q.Digest)
			}
			deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 410)
			deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 410)
			if _, err := os.Lstat(filepath.Join(a.Config.StateDir, "Media", id)); !os.IsNotExist(err) || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 1 {
				t.Fatal("retired committed original was re-created")
			}
		})
	}
}

func TestMediaUploadPGSuccessorFamilyEpochAndBanCannotResumeOriginal(t *testing.T) {
	f := newDeletionPG(t, 7)
	a := f.nodes["media"]
	data := []byte("non-secret immutable authentication context fixture")
	q, _ := mediaUploadPGRequest(t, f, data)
	f.wire.failKind, f.wire.failPhase = "media.upload", "intent"
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 503)
	f.wire.failKind = ""
	tx, err := f.nodes["identity"].DB.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.nodes["identity"].issueSession(f.ctx, tx, f.user, "successor-native", 1, 1, 1, false)
	if err != nil || tx.Commit(f.ctx) != nil {
		t.Fatal("owned successor fixture unavailable")
	}
	deletionBody(t, deletionCall(a, "/api/media/upload-results", next.Result.Token, q), 409)
	deletionBody(t, mediaUploadPGCall(t, a, next.Result.Token, &q, q.Name, data), 409)
	rebased := q
	rebased.DeviceID, rebased.FamilyID = next.Result.DeviceID, next.Result.FamilyID
	deletionBody(t, mediaUploadPGCall(t, a, next.Result.Token, &rebased, rebased.Name, data), 409)
	deletionExec(t, f.ctx, a, `UPDATE auth.users SET state='banned' WHERE id=$1`, q.UserID)
	deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 401)
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 401)
	deletionExec(t, f.ctx, a, `UPDATE auth.users SET state='active' WHERE id=$1`, q.UserID)
	deletionExec(t, f.ctx, a, `UPDATE journal.restore_epochs SET restore_epoch=2 WHERE singleton`)
	deletionExec(t, f.ctx, a, `UPDATE auth.instance SET restore_epoch=2 WHERE singleton`)
	deletionExec(t, f.ctx, a, `UPDATE media.instance SET restore_epoch=2 WHERE singleton`)
	deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 401)
	tx, err = f.nodes["identity"].DB.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	newEpoch, err := f.nodes["identity"].issueSession(f.ctx, tx, f.user, "restored-native", 1, 1, 2, false)
	if err != nil || tx.Commit(f.ctx) != nil {
		t.Fatal("owned restored-epoch fixture unavailable")
	}
	deletionBody(t, deletionCall(a, "/api/media/upload-results", newEpoch.Result.Token, q), 409)
	deletionBody(t, mediaUploadPGCall(t, a, newEpoch.Result.Token, &q, q.Name, data), 409)
	rebased.DeviceID, rebased.FamilyID, rebased.Epoch = newEpoch.Result.DeviceID, newEpoch.Result.FamilyID, "2"
	deletionBody(t, mediaUploadPGCall(t, a, newEpoch.Result.Token, &rebased, rebased.Name, data), 409)
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 0 || mediaUploadPGOriginal(t, f, q).State != "prepared" {
		t.Fatal("a changed principal published or rebased the original upload")
	}
}

func TestMediaUploadPGPreparedCannotOverwriteRegisteredOrDifferentBytes(t *testing.T) {
	for _, mode := range []string{"registered", "different-bytes"} {
		t.Run(mode, func(t *testing.T) {
			f := newDeletionPG(t, 7)
			a := f.nodes["media"]
			data := []byte("non-secret protected prepared fixture")
			q, _ := mediaUploadPGRequest(t, f, data)
			f.wire.failKind, f.wire.failPhase = "media.upload", "intent"
			deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 503)
			f.wire.failKind = ""
			d := mediaUploadPGOriginal(t, f, q)
			file := filepath.Join(a.Config.StateDir, "Media", d.ObjectID)
			foreign := []byte("already existing original bytes must stay untouched")
			if err := os.WriteFile(file, foreign, 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "registered" {
				deletionExec(t, f.ctx, a, `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,message_id) VALUES($1,$2,'bound-original','application/octet-stream',$3,$4,$5,$6)`, d.ObjectID, d.UserID, len(foreign), tokenDigest(string(foreign)), d.ObjectID, newID())
			}
			w := mediaUploadPGCall(t, a, f.token, &q, q.Name, data)
			if w.Code != 409 && w.Code != 410 {
				t.Fatal("prepared upload did not reject an existing incompatible original")
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(after, foreign) || mediaUploadPGOriginal(t, f, q).State != "prepared" {
				t.Fatal("prepared retry overwrote an existing UUID or falsely completed")
			}
		})
	}
}

func TestMediaUploadPGReplayRetiresMissingRequestAndClearsExactUUIDs(t *testing.T) {
	f := newDeletionPG(t, 7)
	a := f.nodes["media"]
	data := []byte("non-secret restored prepared source fixture")
	q, _ := mediaUploadPGRequest(t, f, data)
	f.wire.failKind, f.wire.failPhase = "media.upload", "intent"
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 503)
	f.wire.failKind = ""
	d := mediaUploadPGOriginal(t, f, q)
	deletionFence(t, a, func(ctx context.Context) error {
		if err := a.protect(ctx, mediaUploadOperation(d.RequestID), "intent", "media.upload", mediaUploadSafety(d)); err != nil {
			return err
		}
		return a.publishMediaUpload(a.mediaUploadStagingPath(d.ObjectID), d)
	})
	neighbor := filepath.Join(a.Config.StateDir, "Media", newID())
	if err := os.WriteFile(neighbor, []byte("other original bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	deletionExec(t, f.ctx, a, `DELETE FROM media.upload_requests WHERE scope_id=$1 AND request_id=$2`, a.Config.DeploymentID, q.RequestID)
	conn, err := a.DB.Acquire(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	record := journalRecord{Owner: "media", OperationKey: mediaUploadOperation(d.RequestID), Phase: "intent", Kind: "media.upload", Payload: mediaUploadSafety(d)}
	if handled, err := ReplayMediaUploadSafety(f.ctx, conn.Conn(), record); !handled || err != nil {
		t.Fatal("original upload safety replay unavailable", err)
	}
	if err = ClearRestoredMediaUploads(f.ctx, conn.Conn(), a.Config.StateDir); err != nil {
		t.Fatal(err)
	}
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM media.objects`) != 0 || mediaUploadPGOriginal(t, f, q).State != "retired" {
		t.Fatal("safety replay created an object or lost its denial responsibility")
	}
	for _, file := range []string{a.mediaUploadStagingPath(d.ObjectID), filepath.Join(a.Config.StateDir, "Media", d.ObjectID)} {
		if _, err = os.Lstat(file); !os.IsNotExist(err) {
			t.Fatal("restored original upload bytes survived their exact floor")
		}
	}
	if _, err = os.Stat(neighbor); err != nil {
		t.Fatal("upload cleanup touched a neighboring UUID")
	}
	deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 410)
	deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 410)
}

func TestMediaUploadPGOwnerErasureWaitsForEveryPreparedPage(t *testing.T) {
	f := newDeletionPG(t, 7)
	a := f.nodes["media"]
	data := []byte("non-secret paged prepared source fixture")
	q, p := mediaUploadPGRequest(t, f, data)
	root := filepath.Join(a.Config.StateDir, "Media")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, ".owned-fixture-body")
	if err := os.WriteFile(temp, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	var objects []string
	for range 33 {
		q.RequestID = newID()
		d, err := a.prepareMediaUpload(f.ctx, q, p, http.DetectContentType(data), int64(len(data)), digest[:])
		if err != nil {
			t.Fatal(err)
		}
		if err = a.retainPreparedMediaUpload(f.ctx, temp, d); err != nil {
			t.Fatal(err)
		}
		if err = a.publishMediaUpload(a.mediaUploadStagingPath(d.ObjectID), d); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, d.ObjectID)
	}
	deletion := accountDeletion{ID: newID(), User: q.UserID, Version: "2"}
	deletionFence(t, a, func(ctx context.Context) error {
		if err := a.eraseOwnerData(ctx, deletion); err == nil {
			t.Fatal("owner acknowledged before the second prepared-source page")
		}
		return nil
	})
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM media.upload_requests WHERE user_id=$1 AND NOT bytes_cleared`, q.UserID) != 1 || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.account_erasure_floors WHERE user_id=$1 AND state='complete'`, q.UserID) != 0 {
		t.Fatal("bounded first page lost pending responsibilities or falsely acknowledged completion")
	}
	deletionFence(t, a, func(ctx context.Context) error { return a.eraseOwnerData(ctx, deletion) })
	if pcCount(t, f.ctx, a, `SELECT count(*) FROM media.upload_requests WHERE user_id=$1 AND (state<>'retired' OR NOT bytes_cleared OR name<>'' OR length(sha256)<>0)`, q.UserID) != 0 || pcCount(t, f.ctx, a, `SELECT count(*) FROM media.account_erasure_floors WHERE user_id=$1 AND state='complete'`, q.UserID) != 1 {
		t.Fatal("original owner did not finish every prepared responsibility")
	}
	for _, object := range objects {
		for _, file := range []string{a.mediaUploadStagingPath(object), filepath.Join(root, object)} {
			if _, err := os.Lstat(file); !os.IsNotExist(err) {
				t.Fatal("owner completion left an exact prepared source")
			}
		}
	}
}

func TestMediaUploadManifestAndMinimalSafetyFields(t *testing.T) {
	data := []byte("non-secret manifest fixture")
	digest := sha256.Sum256(data)
	q := mediaUploadRequest{RequestID: newID(), UserID: newID(), DeviceID: newID(), FamilyID: newID(), Epoch: "1", Name: "original.txt", Size: str(int64(len(data))), Digest: hex.EncodeToString(digest[:])}
	if _, _, err := validateMediaUploadRequest(q); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"-1", "01", "67108865", "9223372036854775808"} {
		bad := q
		bad.Size = value
		if _, _, err := validateMediaUploadRequest(bad); err == nil {
			t.Fatal("invalid or excessive declared byte count accepted")
		}
	}
	d := mediaUploadRecord{RequestID: q.RequestID, UserID: q.UserID, DeviceID: q.DeviceID, FamilyID: q.FamilyID, Epoch: "1", Security: "1", SourceGeneration: "1", ObjectID: newID(), Name: q.Name, Binding: digest[:]}
	payload := mediaUploadSafety(d)
	for key, value := range payload {
		if err := validateJournalField(key, value); err != nil {
			t.Fatal("minimal upload safety field rejected", key, err)
		}
	}
	raw, _ := json.Marshal(payload)
	if len(payload) != 9 || bytes.Contains(raw, []byte(q.Name)) || bytes.Contains(raw, data) {
		t.Fatal("upload safety fact contains source metadata or content")
	}
}
