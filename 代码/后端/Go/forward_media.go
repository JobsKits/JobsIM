package jobsim

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type forwardMediaRequest struct {
	ObjectID        string `json:"objectId"`
	SourceMessageID string `json:"sourceMessageId"`
}

func (a *App) gatewayProjectionProof(r *http.Request, mode string) string {
	return base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "gateway-projection-fence-v1", []byte(a.admissionFenceIdentity()+"\x00"+mode+"\x00"+r.Method+"\x00"+r.URL.Path)))
}
func (a *App) hasGatewayProjectionFence(r *http.Request, exclusive bool) bool {
	mode := "shared"
	if exclusive {
		mode = "exclusive"
	}
	return r.Header.Get("X-JobsIM-Projection-Fence-Mode") == mode && hmac.Equal([]byte(r.Header.Get("X-JobsIM-Projection-Fence")), []byte(a.gatewayProjectionProof(r, mode)))
}

// These are public responses carrying Message projections, not an ACL bypass.
func gatewayProjectionMode(r *http.Request) string {
	if mode := personalExportGatewayMode(r); mode != "" {
		return mode
	}
	method := r.Method
	if method == "HEAD" {
		method = "GET"
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	if len(parts) == 2 && parts[0] == "media" && method == "GET" {
		return "shared"
	}
	if len(parts) >= 2 && parts[0] == "sync" {
		return "shared"
	}
	if selfStatePublicPath(r.URL.Path) {
		return "shared"
	}
	if r.URL.Path == "/api/me/preferences" || r.URL.Path == "/api/me/preferences-results" || strings.HasPrefix(r.URL.Path, "/api/me/preferences/operations/") {
		return "shared"
	}
	if len(parts) == 1 && method == "GET" && (parts[0] == "search" || parts[0] == "favorites") {
		return "shared"
	}
	if len(parts) >= 2 && parts[0] == "messages" {
		if len(parts) == 2 {
			if method == "GET" {
				return "shared"
			}
			if method == "PATCH" || method == "DELETE" {
				return "exclusive"
			}
		}
		if len(parts) == 3 {
			if parts[2] == "attachment" && method == "GET" {
				return "shared"
			}
			if parts[2] == "reactions" && method == "POST" {
				return "shared"
			}
			if (parts[2] == "pin" || parts[2] == "favorite") && (method == "PUT" || method == "DELETE") {
				return "shared"
			}
		}
		if len(parts) == 4 && parts[2] == "reactions" && method == "DELETE" {
			return "shared"
		}
	}
	if len(parts) == 3 && parts[0] == "conversations" {
		if parts[2] == "messages" && (method == "GET" || method == "POST") {
			return "shared"
		}
		if parts[2] == "message-results" && method == "POST" {
			return "shared"
		}
		if parts[2] == "announcement" && method == "GET" {
			return "shared"
		}
	}
	return ""
}

func (a *App) forwardAttachmentRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/messages/{id}/attachment", a.forwardAttachment)
}

func (a *App) forwardAttachment(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	m, e := a.loadMessage(r.Context(), r.PathValue("id"), u.ID)
	if e != nil {
		return e
	}
	if m.forwardUncertain {
		return fail(503, "FORWARD_QUALIFICATION_UNAVAILABLE", "Current forward chain cannot be proved")
	}
	if !m.Forwarded || m.ForwardUnavailable || m.Deleted || m.ContentUnavailable || m.forwardLeafAttachment == "" {
		return fail(404, "FORWARD_ATTACHMENT_UNAVAILABLE", "Forward attachment unavailable")
	}
	raw, _ := json.Marshal(forwardMediaRequest{m.forwardLeafAttachment, m.forwardLeafMessage})
	request, e := http.NewRequestWithContext(r.Context(), "POST", a.Config.RoleURLs["media"]+"/internal/forwards/media", bytes.NewReader(raw))
	if e != nil {
		return e
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-JobsIM-Service-Key", base64.RawURLEncoding.EncodeToString(a.Config.Key))
	request.Header.Set("X-JobsIM-Caller", a.Role)
	if r.Method == "HEAD" {
		request.Header.Set("X-JobsIM-Forward-Head", "1")
	}
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := r.Header.Get(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	// Streaming has no five-second JSON-RPC body timeout. Client cancellation is
	// propagated immediately, while the outer public gateway keeps the SH lock.
	client := &http.Client{Transport: a.Client.Transport}
	response, e := client.Do(request)
	if e != nil {
		return fail(503, "DEPENDENCY_UNAVAILABLE", "Forward media owner unavailable")
	}
	defer response.Body.Close()
	if !hmac.Equal([]byte(response.Header.Get("X-JobsIM-Admission-Fence")), []byte(a.admissionFenceIdentity())) {
		return fail(503, "ADMISSION_FENCE_MISMATCH", "Media owner must share the same canonical local instance directory")
	}
	if response.StatusCode != 200 && response.StatusCode != 206 && response.StatusCode != 304 && response.StatusCode != 416 {
		return fail(response.StatusCode, "FORWARD_ATTACHMENT_UNAVAILABLE", "Original attachment cannot be served")
	}
	for _, name := range []string{"Content-Type", "Content-Disposition", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag", "Cache-Control", "X-Content-SHA256", "X-Content-Size"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
	return nil
}

func (a *App) forwardMediaRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/forwards/media", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q forwardMediaRequest
		if e := decode(r, &q); e != nil {
			return e
		}
		if r.Header.Get("X-JobsIM-Caller") != "message" || !validObjectID(q.ObjectID) || !validObjectID(q.SourceMessageID) {
			return fail(403, "FORWARD_MEDIA_SOURCE_REQUIRED", "Verified source message owner required")
		}
		var name, contentType, key string
		var size int64
		var digest []byte
		e := a.DB.QueryRow(r.Context(), `SELECT name,content_type,storage_key,size_bytes,sha256 FROM media.objects WHERE id=$1 AND message_id=$2 AND purpose='chat_attachment'`, q.ObjectID, q.SourceMessageID).Scan(&name, &contentType, &key, &size, &digest)
		if e != nil {
			return fail(404, "OBJECT_NOT_FOUND", "Original bound chat attachment unavailable")
		}
		if key != filepath.Base(key) || strings.ContainsAny(key, "/\\") {
			return fail(500, "STORAGE_INTEGRITY", "Original storage identity invalid")
		}
		file, e := os.Open(filepath.Join(a.Config.StateDir, "Media", key))
		if e != nil {
			return fail(404, "OBJECT_MISSING", "Original attachment bytes unavailable")
		}
		defer file.Close()
		info, e := file.Stat()
		if e != nil || !info.Mode().IsRegular() || info.Size() != size {
			return fail(500, "STORAGE_INTEGRITY", "Original attachment size differs")
		}
		hash := sha256.New()
		if _, e = io.Copy(hash, file); e != nil || !hmac.Equal(hash.Sum(nil), digest) {
			return fail(500, "STORAGE_INTEGRITY", "Original attachment digest differs")
		}
		if _, e = file.Seek(0, io.SeekStart); e != nil {
			return e
		}
		disposition := "attachment"
		if strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "audio/") || strings.HasPrefix(contentType, "video/") {
			disposition = "inline"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
		w.Header().Set("Cache-Control", "private,no-store")
		w.Header().Set("ETag", `"`+hex.EncodeToString(digest)+`"`)
		w.Header().Set("X-Content-SHA256", hex.EncodeToString(digest))
		w.Header().Set("X-Content-Size", strconv.FormatInt(size, 10))
		get := r.Clone(r.Context())
		get.Method = http.MethodGet
		if r.Header.Get("X-JobsIM-Forward-Head") == "1" {
			get.Method = http.MethodHead
		}
		http.ServeContent(w, get, name, info.ModTime(), file)
		return nil
	}))
}
