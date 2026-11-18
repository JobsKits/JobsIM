package jobsim

import (
	"crypto/sha256"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) mediaRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.avatarMediaRoutes(h, internal)
	a.exportMediaRoutes(h, internal)
	a.forwardMediaRoutes(h, internal)
	h("POST /internal/receipts/object", internal(a.receiptMediaObject))
	h("POST /api/media", a.upload)
	h("GET /api/media/{id}", a.download)
	h("POST /internal/bind", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			ObjectID       string `json:"objectId"`
			UserID         string `json:"userId"`
			MessageID      string `json:"messageId"`
			ConversationID string `json:"conversationId"`
			Type           string `json:"type"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if _, e := a.member(r.Context(), q.UserID, q.ConversationID); e != nil {
			return e
		}
		var owner, ct string
		var bound *string
		e := a.DB.QueryRow(r.Context(), `SELECT owner_id::text,content_type,message_id::text FROM media.objects WHERE id=$1 AND purpose='chat_attachment'`, q.ObjectID).Scan(&owner, &ct, &bound)
		if e != nil || owner != q.UserID {
			return fail(403, "OBJECT_NOT_OWNED", "Private object not owned")
		}
		if q.Type != "file" && !strings.HasPrefix(ct, q.Type+"/") {
			return fail(400, "TYPE_MISMATCH", "Attachment type differs")
		}
		if bound != nil && *bound != q.MessageID {
			return fail(409, "OBJECT_ALREADY_BOUND", "Separate upload required")
		}
		tag, e := a.DB.Exec(r.Context(), `UPDATE media.objects SET conversation_id=$2,message_id=$3 WHERE id=$1 AND purpose='chat_attachment' AND(message_id IS NULL OR message_id=$3)`, q.ObjectID, q.ConversationID, q.MessageID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fail(409, "OBJECT_ALREADY_BOUND", "Attachment already reserved by another stable message")
		}
		respond(w, 200, map[string]bool{"bound": true})
		return nil
	}))
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	if e = a.requireFeature(r.Context(), "media"); e != nil {
		return e
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if e = r.ParseMultipartForm(1 << 20); e != nil {
		return fail(413, "UPLOAD_LIMIT", "Upload limit is 64 MiB")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	src, header, e := r.FormFile("file")
	if e != nil {
		return fail(400, "FILE_REQUIRED", "Multipart file field required")
	}
	defer src.Close()
	prefix := make([]byte, 512)
	n, e := io.ReadFull(src, prefix)
	if e != nil && e != io.EOF && e != io.ErrUnexpectedEOF {
		return e
	}
	prefix = prefix[:n]
	ct := http.DetectContentType(prefix)
	dir := filepath.Join(a.Config.StateDir, "Media")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	id := newID()
	temp, e := os.CreateTemp(dir, ".upload-")
	if e != nil {
		return e
	}
	tmp := temp.Name()
	defer os.Remove(tmp)
	hash := sha256.New()
	dest := io.MultiWriter(temp, hash)
	size, e := dest.Write(prefix)
	if e != nil {
		temp.Close()
		return e
	}
	rest, e := io.Copy(dest, io.LimitReader(src, (64<<20)+1))
	if e != nil {
		temp.Close()
		return e
	}
	total := int64(size) + rest
	if total > 64<<20 {
		temp.Close()
		return fail(413, "UPLOAD_LIMIT", "Upload exceeds 64 MiB")
	}
	if strings.HasPrefix(ct, "image/") && total > 20<<20 {
		temp.Close()
		return fail(413, "IMAGE_LIMIT", "Image limit is 20 MiB")
	}
	if e = temp.Sync(); e != nil {
		temp.Close()
		return e
	}
	if e = temp.Close(); e != nil {
		return e
	}
	ct, e = classifyStoredMedia(tmp, ct)
	if e != nil {
		return e
	}
	if e = os.Rename(tmp, filepath.Join(dir, id)); e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	e = d.Sync()
	d.Close()
	if e != nil {
		return e
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	_, e = a.DB.Exec(r.Context(), `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, u.ID, name, ct, total, hash.Sum(nil), id)
	if e != nil {
		return e
	}
	respond(w, 200, map[string]any{"id": id, "name": name, "contentType": ct, "size": total, "url": "/api/media/" + id})
	return nil
}
func (a *App) download(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var owner, name, ct, key string
	var message *string
	e = a.DB.QueryRow(r.Context(), `SELECT owner_id::text,name,content_type,storage_key,message_id::text FROM media.objects WHERE id=$1 AND purpose='chat_attachment'`, r.PathValue("id")).Scan(&owner, &name, &ct, &key, &message)
	if e != nil {
		return fail(404, "OBJECT_NOT_FOUND", "Object unavailable")
	}
	if e = a.requireMediaOwnerReadable(r.Context(), r.PathValue("id")); e != nil {
		return e
	}
	if message != nil {
		if e = a.mediaAttachmentAccess(r.Context(), *message, r.PathValue("id"), u.ID, u.DeviceID, r.Header.Get("X-JobsIM-Store-Generation"), u.RestoreEpoch, ""); e != nil {
			return e
		}
	} else if owner != u.ID {
		return fail(403, "FORBIDDEN", "Private upload")
	}
	if key == "." || key != r.PathValue("id") || key != filepath.Base(key) {
		return fail(500, "STORAGE_INTEGRITY", "Invalid storage identity")
	}
	path := filepath.Join(a.Config.StateDir, "Media", key)
	before, e := os.Lstat(path)
	if e != nil || !before.Mode().IsRegular() {
		return fail(404, "OBJECT_MISSING", "Original regular object bytes unavailable")
	}
	f, e := os.Open(path)
	if e != nil {
		return fail(404, "OBJECT_MISSING", "Object bytes unavailable")
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil || !stat.Mode().IsRegular() || !os.SameFile(before, stat) {
		return fail(503, "STORAGE_INTEGRITY", "Original object changed while opening")
	}
	var source io.ReadSeeker = f
	if message != nil {
		due, deadlineError := a.mediaReadDeadline(r.Context(), *message, r.PathValue("id"))
		if deadlineError != nil {
			return deadlineError
		}
		source = &mediaRetainedReader{File: f, ctx: r.Context(), due: due}
	}
	disposition := "attachment"
	if strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private,no-store")
	http.ServeContent(w, r, name, stat.ModTime(), source)
	return nil
}
