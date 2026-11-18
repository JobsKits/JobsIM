package jobsim

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// The media owner describes the original bound file, never its preview or URL.
func (a *App) receiptMediaObject(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "sync" {
		return fail(403, "RECEIPT_OWNER_REQUIRED", "Only the receipt owner may request this private descriptor")
	}
	if !a.hasGatewayProjectionFence(r, false) && !a.hasGatewayProjectionFence(r, true) {
		return fail(503, "ADMISSION_FENCE_REQUIRED", "The original object descriptor requires its caller's live source fence")
	}
	var q struct {
		User    string `json:"userId"`
		Message string `json:"messageId"`
		Object  string `json:"objectId"`
		Device  string `json:"deviceId"`
		Store   string `json:"storeGeneration"`
		Epoch   string `json:"restoreEpoch"`
		Content string `json:"contentVersion"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.User) || !syncValidationID(q.Message) || !syncValidationID(q.Object) || !syncValidationCounter(q.Content, false) {
		return fail(400, "INVALID_RECEIPT_OBJECT", "Canonical source, object and recipient identities required")
	}
	if err := a.mediaAttachmentAccess(r.Context(), q.Message, q.Object, q.User, q.Device, q.Store, q.Epoch, q.Content); err != nil {
		return err
	}
	var key, contentType string
	var size int64
	var digest []byte
	err := a.DB.QueryRow(r.Context(), `SELECT storage_key,size_bytes,sha256,content_type FROM media.objects
WHERE id=$1 AND message_id=$2 AND purpose='chat_attachment'`, q.Object, q.Message).Scan(&key, &size, &digest, &contentType)
	if err != nil {
		return fail(404, "RECEIPT_OBJECT_UNAVAILABLE", "Bound original media object is unavailable")
	}
	if key == "." || key != filepath.Base(key) || size < 0 || size > 64<<20 || len(digest) != sha256.Size {
		return fail(503, "RECEIPT_OBJECT_INTEGRITY", "Original media descriptor is invalid")
	}
	path := filepath.Join(a.Config.StateDir, "Media", key)
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() != size {
		return fail(503, "RECEIPT_OBJECT_INTEGRITY", "Original complete media file is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return fail(503, "RECEIPT_OBJECT_INTEGRITY", "Original media file cannot be opened")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() || opened.Size() != size {
		return fail(503, "RECEIPT_OBJECT_INTEGRITY", "Original media file changed while opening")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, size+1))
	if err != nil || n != size || !bytes.Equal(hash.Sum(nil), digest) {
		return fail(503, "RECEIPT_OBJECT_INTEGRITY", "Original media byte count or digest differs")
	}
	respond(w, 200, receiptObject{ID: q.Object, Size: str(size), Digest: hex.EncodeToString(digest), ContentType: contentType})
	return nil
}
