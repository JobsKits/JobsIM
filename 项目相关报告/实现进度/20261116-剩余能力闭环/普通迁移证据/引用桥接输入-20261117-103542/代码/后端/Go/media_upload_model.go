package jobsim

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const mediaUploadProtocol = "stable-upload-v1"
const mediaUploadFileLimit = 64 << 20
const mediaUploadRequestLimit = 65 << 20

type mediaUploadRequest struct {
	RequestID string `json:"requestId"`
	UserID    string `json:"userId"`
	DeviceID  string `json:"deviceId"`
	FamilyID  string `json:"familyId"`
	Epoch     string `json:"restoreEpoch"`
	Name      string `json:"name"`
	Size      string `json:"size"`
	Digest    string `json:"sha256"`
}

type mediaUploadPrincipal struct {
	UserID, DeviceID, FamilyID, Epoch, Security, SourceGeneration string
}

type mediaUploadRecord struct {
	RequestID, UserID, DeviceID, FamilyID, Epoch, Security, SourceGeneration string
	ObjectID, Name, ContentType, State                                       string
	Size                                                                     int64
	Digest, Binding                                                          []byte
}

type mediaUploadResult struct {
	Protocol    string `json:"protocol"`
	RequestID   string `json:"requestId"`
	State       string `json:"uploadState"`
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Digest      string `json:"sha256,omitempty"`
	URL         string `json:"url,omitempty"`
}

func init() {
	migrations["media"] += `CREATE TABLE IF NOT EXISTS media.upload_requests(
scope_id uuid NOT NULL,request_id text NOT NULL,user_id uuid NOT NULL,device_id uuid NOT NULL,family_id uuid NOT NULL,
restore_epoch bigint NOT NULL,security_version bigint NOT NULL,source_generation bigint NOT NULL,
object_id uuid NOT NULL,name text NOT NULL,content_type text NOT NULL,size_bytes bigint NOT NULL CHECK(size_bytes>=0 AND size_bytes<=67108864),
sha256 bytea NOT NULL,request_digest bytea NOT NULL,state text NOT NULL CHECK(state IN('prepared','committed','retired')),
created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,request_id),UNIQUE(scope_id,object_id));
CREATE INDEX IF NOT EXISTS media_upload_owner_pending ON media.upload_requests(user_id,object_id) WHERE state='prepared';`
}

func validMediaUploadName(name string) bool {
	if name == "" || len(name) > 255 || !utf8.ValidString(name) || name == "." || name == ".." || strings.TrimSpace(name) != name || filepath.Base(strings.ReplaceAll(name, "\\", "/")) != name {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validateMediaUploadRequest(q mediaUploadRequest) (int64, []byte, error) {
	size, err := strconv.ParseInt(q.Size, 10, 64)
	digest, decodeErr := hex.DecodeString(q.Digest)
	if !validReceiptNonce(q.RequestID) || !validObjectID(q.UserID) || !validObjectID(q.DeviceID) || !validObjectID(q.FamilyID) || !syncValidationCounter(q.Epoch, false) || !validMediaUploadName(q.Name) || err != nil || size < 0 || size > mediaUploadFileLimit || str(size) != q.Size || decodeErr != nil || len(digest) != 32 || hex.EncodeToString(digest) != q.Digest {
		return 0, nil, fail(400, "INVALID_MEDIA_UPLOAD", "Exact original upload identity, metadata and digest required")
	}
	return size, digest, nil
}

func (a *App) mediaUploadBinding(q mediaUploadRequest, p mediaUploadPrincipal) []byte {
	raw, _ := json.Marshal(struct {
		Request  mediaUploadRequest
		Security string
		Source   string
	}{q, p.Security, p.SourceGeneration})
	return keyedDigest(a.Config.Key, "stable-media-upload:"+a.Config.DeploymentID, raw)
}

func (a *App) validateMediaUploadContext(q mediaUploadRequest, p mediaUploadPrincipal) error {
	if q.UserID != p.UserID || q.DeviceID != p.DeviceID || q.FamilyID != p.FamilyID || q.Epoch != p.Epoch {
		return fail(409, "UPLOAD_CONTEXT_CHANGED", "The original user, device, family and epoch must remain current")
	}
	var epoch string
	if err := a.DB.QueryRow(context.Background(), `SELECT restore_epoch::text FROM media.instance WHERE singleton`).Scan(&epoch); err != nil {
		return err
	}
	if epoch != p.Epoch {
		return fail(503, "RESTORE_GATE_CLOSED", "Media has not activated the current original epoch")
	}
	return nil
}

const mediaUploadColumns = `request_id,user_id::text,device_id::text,family_id::text,restore_epoch::text,security_version::text,source_generation::text,object_id::text,name,content_type,state,size_bytes,sha256,request_digest`

func scanMediaUpload(row pgx.Row) (mediaUploadRecord, error) {
	var d mediaUploadRecord
	err := row.Scan(&d.RequestID, &d.UserID, &d.DeviceID, &d.FamilyID, &d.Epoch, &d.Security, &d.SourceGeneration, &d.ObjectID, &d.Name, &d.ContentType, &d.State, &d.Size, &d.Digest, &d.Binding)
	return d, err
}

func (a *App) matchMediaUpload(q mediaUploadRequest, p mediaUploadPrincipal, d mediaUploadRecord) error {
	if d.UserID != p.UserID || d.DeviceID != p.DeviceID || d.FamilyID != p.FamilyID || d.Epoch != p.Epoch || d.Security != p.Security || d.SourceGeneration != p.SourceGeneration || !bytes.Equal(d.Binding, a.mediaUploadBinding(q, p)) {
		return fail(409, "IDEMPOTENCY_CONFLICT", "Original upload metadata or authentication context differs")
	}
	if d.State == "retired" {
		return fail(410, "UPLOAD_RETIRED", "Original upload responsibility is permanently retired")
	}
	return nil
}

func mediaUploadPublic(d mediaUploadRecord) mediaUploadResult {
	out := mediaUploadResult{Protocol: mediaUploadProtocol, RequestID: d.RequestID, State: d.State, ID: d.ObjectID}
	if d.State == "committed" {
		out.Name, out.ContentType, out.Size, out.Digest, out.URL = d.Name, d.ContentType, d.Size, hex.EncodeToString(d.Digest), "/api/media/"+d.ObjectID
	}
	return out
}

func (a *App) multipartMediaUpload(r *http.Request, name string, size int64, digest []byte) (mediaUploadRequest, bool, error) {
	var q mediaUploadRequest
	if r.MultipartForm == nil || len(r.MultipartForm.Value["requestId"]) == 0 {
		return q, false, nil
	}
	values := map[string]*string{"requestId": &q.RequestID, "userId": &q.UserID, "deviceId": &q.DeviceID, "familyId": &q.FamilyID, "restoreEpoch": &q.Epoch, "name": &q.Name, "size": &q.Size, "sha256": &q.Digest}
	if len(r.MultipartForm.Value) != len(values) || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		return q, true, fail(400, "INVALID_MEDIA_UPLOAD", "One original file and the exact stable metadata fields required")
	}
	for field, target := range values {
		items := r.MultipartForm.Value[field]
		if len(items) != 1 {
			return q, true, fail(400, "INVALID_MEDIA_UPLOAD", "Each stable field must appear exactly once")
		}
		*target = items[0]
	}
	wantedSize, wantedDigest, err := validateMediaUploadRequest(q)
	if err != nil {
		return q, true, err
	}
	if q.Name != name || wantedSize != size || !bytes.Equal(wantedDigest, digest) {
		return q, true, fail(409, "UPLOAD_BODY_MISMATCH", "Actual original file name, length or digest differs")
	}
	return q, true, nil
}
