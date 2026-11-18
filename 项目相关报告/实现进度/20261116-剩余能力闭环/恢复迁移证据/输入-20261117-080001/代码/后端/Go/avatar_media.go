package jobsim

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
)

type avatarProcessRequest struct {
	UserID       string `json:"userId"`
	OperationID  string `json:"operationId"`
	Generation   string `json:"generation"`
	RestoreEpoch string `json:"restoreEpoch"`
	SourceDigest []byte `json:"sourceDigest"`
	Content      []byte `json:"content,omitempty"`
}

var avatarProcessingSlots = make(chan struct{}, 2)

func (a *App) avatarMediaRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/avatar/process", internal(a.processAvatar))
	h("GET /api/avatars/{id}", a.downloadAvatar)
}
func (a *App) avatarObjectID(user, operation, kind string) string {
	b := keyedDigest(a.Config.Key, "avatar-object", []byte(user+":"+operation+":"+kind))[:16]
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func (a *App) processAvatar(w http.ResponseWriter, r *http.Request) error {
	var q avatarProcessRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 3<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(&q); e != nil {
		return fail(400, "INVALID_AVATAR_PROCESS", "Invalid avatar processing request")
	}
	if len(q.Content) > 2<<20 || len(q.Content) == 0 {
		return fail(413, "AVATAR_UPLOAD_LIMIT", "Avatar upload limit is 2 MiB")
	}
	digest := sha256.Sum256(q.Content)
	if !bytes.Equal(digest[:], q.SourceDigest) {
		return fail(400, "AVATAR_DIGEST_MISMATCH", "Avatar source digest differs")
	}
	proof := q
	proof.Content = nil
	if e := a.rpc(r.Context(), "directory", "/internal/avatar/prepared", proof, nil); e != nil {
		return e
	}
	root, thumb := a.avatarObjectID(q.UserID, q.OperationID, "root"), a.avatarObjectID(q.UserID, q.OperationID, "thumbnail")
	_, e := a.DB.Exec(r.Context(), `INSERT INTO media.avatar_jobs(owner_id,operation_id,source_digest,restore_epoch,generation,root_id,thumbnail_id,state) VALUES($1,$2,$3,$4::bigint,$5::bigint,$6,$7,'prepared') ON CONFLICT DO NOTHING`, q.UserID, q.OperationID, digest[:], q.RestoreEpoch, q.Generation, root, thumb)
	if e != nil {
		return e
	}
	var original []byte
	var epoch, generation int64
	var state string
	e = a.DB.QueryRow(r.Context(), `SELECT source_digest,restore_epoch,generation,state FROM media.avatar_jobs WHERE owner_id=$1 AND operation_id=$2`, q.UserID, q.OperationID).Scan(&original, &epoch, &generation, &state)
	if e != nil {
		return e
	}
	if !bytes.Equal(original, digest[:]) || str(epoch) != q.RestoreEpoch || str(generation) != q.Generation {
		return fail(409, "IDEMPOTENCY_CONFLICT", "Original avatar process differs")
	}
	for kind, id := range map[string]string{"crop_512": root, "thumbnail_128": thumb} {
		// Each output has its own stable processing identity; neither consumes the publication operation as a job ID.
		job := a.avatarObjectID(q.UserID, q.OperationID, "job:"+kind+":static-jpeg-1")
		if _, e = a.DB.Exec(r.Context(), `INSERT INTO media.avatar_processing_items(job_id,owner_id,publication_operation_id,kind,profile_version,output_id,state) VALUES($1,$2,$3,$4,'static-jpeg-1',$5,'prepared') ON CONFLICT DO NOTHING`, job, q.UserID, q.OperationID, kind, id); e != nil {
			return e
		}
	}
	if state == "completed" {
		complete := true
		for _, id := range []string{root, thumb} {
			var expected []byte
			if e = a.DB.QueryRow(r.Context(), `SELECT sha256 FROM media.objects WHERE id=$1 AND owner_id=$2 AND purpose='profile_avatar'`, id, q.UserID).Scan(&expected); e != nil {
				return e
			}
			data, re := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", id))
			if re != nil {
				if os.IsNotExist(re) {
					complete = false
					continue
				}
				return re
			}
			actual := sha256.Sum256(data)
			if !bytes.Equal(expected, actual[:]) {
				return fail(500, "AVATAR_STORAGE_INTEGRITY", "Avatar output bytes do not match their stored digest")
			}
		}
		if complete {
			if _, e = a.DB.Exec(r.Context(), `UPDATE media.avatar_processing_items SET state='completed' WHERE owner_id=$1 AND publication_operation_id=$2 AND profile_version='static-jpeg-1'`, q.UserID, q.OperationID); e != nil {
				return e
			}
			respond(w, 200, map[string]string{"rootId": root, "thumbnailId": thumb})
			return nil
		}
	}
	select {
	case avatarProcessingSlots <- struct{}{}:
		defer func() { <-avatarProcessingSlots }()
	case <-r.Context().Done():
		return r.Context().Err()
	}
	src, e := decodeStaticAvatar(q.Content)
	if e != nil {
		return e
	}
	rootBytes, e := encodeAvatarSquare(src, 512)
	if e != nil {
		return e
	}
	thumbBytes, e := encodeAvatarSquare(src, 128)
	if e != nil {
		return e
	}
	for id, data := range map[string][]byte{root: rootBytes, thumb: thumbBytes} {
		if e = a.writeAvatarFile(id, data); e != nil {
			return e
		}
	}
	// A prepared operation may be superseded while CPU work runs. Its outputs stay private.
	if e = a.rpc(r.Context(), "directory", "/internal/avatar/prepared", proof, nil); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	for id, data := range map[string][]byte{root: rootBytes, thumb: thumbBytes} {
		hash := sha256.Sum256(data)
		var parent *string
		if id == thumb {
			parent = &root
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,purpose,owner_reference_id,owner_generation,parent_object_id) VALUES($1,$2,'avatar.jpg','image/jpeg',$3,$4,$7,'profile_avatar',$2,$5::bigint,$6) ON CONFLICT(id) DO NOTHING`, id, q.UserID, len(data), hash[:], q.Generation, parent, id)
		if e != nil {
			return e
		}
		var valid bool
		if e = tx.QueryRow(r.Context(), `SELECT owner_id=$2 AND purpose='profile_avatar' AND owner_reference_id=$2 AND owner_generation=$3::bigint AND sha256=$4 FROM media.objects WHERE id=$1`, id, q.UserID, q.Generation, hash[:]).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return fail(409, "AVATAR_OUTPUT_CONFLICT", "Avatar output identity conflict")
		}
	}
	_, e = tx.Exec(r.Context(), `UPDATE media.avatar_jobs SET state='completed' WHERE owner_id=$1 AND operation_id=$2`, q.UserID, q.OperationID)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `UPDATE media.avatar_processing_items SET state='completed' WHERE owner_id=$1 AND publication_operation_id=$2 AND profile_version='static-jpeg-1' AND output_id=ANY($3::uuid[])`, q.UserID, q.OperationID, []string{root, thumb}); e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"rootId": root, "thumbnailId": thumb})
	return nil
}
func decodeStaticAvatar(content []byte) (image.Image, error) {
	cfg, format, e := image.DecodeConfig(bytes.NewReader(content))
	if e != nil || (format != "jpeg" && format != "png") {
		return nil, fail(400, "AVATAR_FORMAT", "Only static JPEG and PNG avatars are supported")
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return nil, fail(413, "AVATAR_PIXELS", "Avatar pixel limit is 16 megapixels")
	}
	if format == "png" {
		for p := 8; p+12 <= len(content); {
			n := uint64(binary.BigEndian.Uint32(content[p : p+4]))
			if n > uint64(len(content)-p-12) {
				return nil, fail(400, "AVATAR_FORMAT", "Malformed PNG avatar")
			}
			if string(content[p+4:p+8]) == "acTL" {
				return nil, fail(400, "AVATAR_FORMAT", "Animated avatars are not enabled")
			}
			p += int(n) + 12
		}
	}
	img, _, e := image.Decode(bytes.NewReader(content))
	if e != nil {
		return nil, fail(400, "AVATAR_DECODE", "Avatar image cannot be decoded")
	}
	return img, nil
}
func encodeAvatarSquare(src image.Image, size int) ([]byte, error) {
	b := src.Bounds()
	edge := b.Dx()
	if b.Dy() < edge {
		edge = b.Dy()
	}
	ox, oy := b.Min.X+(b.Dx()-edge)/2, b.Min.Y+(b.Dy()-edge)/2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// Center crop is resampled into a bounded canonical image; re-encoding strips source metadata.
			sx, sy := ox+(2*x+1)*edge/(2*size), oy+(2*y+1)*edge/(2*size)
			r, g, blue, alpha := src.At(sx, sy).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8((r + 65535 - alpha) >> 8), uint8((g + 65535 - alpha) >> 8), uint8((blue + 65535 - alpha) >> 8), 255})
		}
	}
	var out bytes.Buffer
	e := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88})
	return out.Bytes(), e
}
func (a *App) writeAvatarFile(id string, data []byte) error {
	dir := filepath.Join(a.Config.StateDir, "Media")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	path := filepath.Join(dir, id)
	if existing, e := os.ReadFile(path); e == nil {
		if !bytes.Equal(existing, data) {
			return fail(409, "AVATAR_STORAGE_CONFLICT", "Avatar output bytes differ")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	f, e := os.CreateTemp(dir, ".avatar-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	// Link publishes a complete file without ever overwriting a prior successful writer.
	if e = os.Link(name, path); e != nil {
		if !os.IsExist(e) {
			return e
		}
		existing, re := os.ReadFile(path)
		if re != nil {
			return re
		}
		if !bytes.Equal(existing, data) {
			return fail(409, "AVATAR_STORAGE_CONFLICT", "Avatar output bytes differ")
		}
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	e = d.Sync()
	d.Close()
	return e
}
func (a *App) downloadAvatar(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	id := r.PathValue("id")
	if e = a.rpc(r.Context(), "directory", "/internal/avatar/access", map[string]string{"userId": u.ID, "objectId": id}, nil); e != nil {
		return e
	}
	var key, ct string
	if e = a.DB.QueryRow(r.Context(), `SELECT storage_key,content_type FROM media.objects WHERE id=$1 AND purpose='profile_avatar'`, id).Scan(&key, &ct); e != nil {
		return fail(404, "AVATAR_UNAVAILABLE", "Avatar bytes unavailable")
	}
	if key != filepath.Base(key) {
		return fail(500, "STORAGE_INTEGRITY", "Invalid avatar storage identity")
	}
	f, e := os.Open(filepath.Join(a.Config.StateDir, "Media", key))
	if e != nil {
		return fail(404, "AVATAR_UNAVAILABLE", "Avatar bytes unavailable")
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return e
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private,no-store")
	w.Header().Set("Content-Disposition", "inline; filename=avatar.jpg")
	http.ServeContent(w, r, "avatar.jpg", stat.ModTime(), f)
	return nil
}
