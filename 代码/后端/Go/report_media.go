package jobsim

import (
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func reportMediaPath(path string) bool {
	return path == "/api/reports/evidence" || strings.HasPrefix(path, "/api/reports/evidence/") || (strings.HasPrefix(path, "/api/reports/") || strings.HasPrefix(path, "/api/admin/reports/")) && strings.HasSuffix(path, "/attachment")
}

func (a *App) reportMediaRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /api/reports/evidence", func(w http.ResponseWriter, r *http.Request) error { return a.uploadForPurpose(w, r, "report_evidence") })
	h("GET /api/reports/evidence/capabilities", a.mediaUploadCapabilities)
	h("POST /api/reports/evidence/upload-results", a.mediaUploadResults)
	h("GET /api/reports/evidence/{id}", a.downloadReportEvidence)
	h("GET /api/reports/{reportId}/messages/{messageId}/attachment", a.downloadReportMessageAttachment)
	h("GET /api/admin/reports/{reportId}/messages/{messageId}/attachment", a.downloadReportMessageAttachment)
	h("POST /internal/report-evidence", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "governance" {
			return fail(403, "REPORT_OWNER_REQUIRED", "Only the report owner may qualify evidence")
		}
		var q struct {
			UserID   string `json:"userId"`
			ObjectID string `json:"objectId"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		v, err := a.reportEvidenceObject(r, q.ObjectID, q.UserID)
		if err != nil {
			return err
		}
		respond(w, 200, v)
		return nil
	}))
}

func (a *App) reportEvidenceObject(r *http.Request, id, owner string) (ReportEvidence, error) {
	var v ReportEvidence
	var digest []byte
	var key string
	if !validAssetID(id) || !validAssetID(owner) {
		return v, fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Owned report evidence unavailable")
	}
	err := a.DB.QueryRow(r.Context(), `SELECT id::text,name,content_type,size_bytes,sha256,storage_key FROM media.objects WHERE id=$1 AND owner_id=$2 AND purpose='report_evidence' AND message_id IS NULL`, id, owner).Scan(&v.ID, &v.Name, &v.ContentType, &v.Size, &digest, &key)
	if err != nil {
		return v, fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Owned report evidence unavailable")
	}
	if err = a.requireMediaOwnerReadable(r.Context(), id); err != nil {
		return ReportEvidence{}, err
	}
	if key != id {
		return ReportEvidence{}, fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Original report evidence storage unavailable")
	}
	stat, err := os.Lstat(filepath.Join(a.Config.StateDir, "Media", key))
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != v.Size {
		return ReportEvidence{}, fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Original report evidence bytes unavailable")
	}
	v.Digest, v.URL, v.Available = hex.EncodeToString(digest), "/api/reports/evidence/"+id, true
	return v, nil
}

func (a *App) downloadReportEvidence(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if !validAssetID(id) {
		return fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Report evidence unavailable")
	}
	var owner, name, ct, key string
	if err = a.DB.QueryRow(r.Context(), `SELECT owner_id::text,name,content_type,storage_key FROM media.objects WHERE id=$1 AND purpose='report_evidence' AND message_id IS NULL`, id).Scan(&owner, &name, &ct, &key); err != nil {
		return fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Report evidence unavailable")
	}
	if owner != u.ID {
		if !u.Admin {
			return fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Report evidence unavailable")
		}
		if err = a.rpc(r.Context(), "governance", "/internal/report-evidence-access", map[string]any{"reportId": r.URL.Query().Get("reportId"), "objectId": id, "userId": u.ID, "admin": true}, nil); err != nil {
			return err
		}
	}
	if err = a.requireMediaOwnerReadable(r.Context(), id); err != nil {
		return err
	}
	return a.servePrivateMedia(w, r, id, name, ct, key, "")
}

func (a *App) downloadReportMessageAttachment(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	admin := strings.HasPrefix(r.URL.Path, "/api/admin/")
	if admin && !u.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	var source ReportMessageEvidence
	if err = a.rpc(r.Context(), "governance", "/internal/report-message-access", map[string]any{"reportId": r.PathValue("reportId"), "messageId": r.PathValue("messageId"), "userId": u.ID, "admin": admin}, &source); err != nil {
		return err
	}
	if !source.Available || !validAssetID(source.ObjectID) {
		return fail(410, "REPORT_MESSAGE_UNAVAILABLE", "Selected source has no current readable attachment")
	}
	var name, ct, key string
	if err = a.DB.QueryRow(r.Context(), `SELECT name,content_type,storage_key FROM media.objects WHERE id=$1 AND purpose='chat_attachment'`, source.ObjectID).Scan(&name, &ct, &key); err != nil {
		return fail(410, "REPORT_MESSAGE_UNAVAILABLE", "Selected attachment unavailable")
	}
	if err = a.requireMediaOwnerReadable(r.Context(), source.ObjectID); err != nil {
		return err
	}
	if !validAssetID(source.MediaMessageID) {
		return fail(410, "REPORT_MESSAGE_UNAVAILABLE", "Selected attachment source unavailable")
	}
	if err = a.requireMediaRetention(r.Context(), source.MediaMessageID, source.ObjectID); err != nil {
		return err
	}
	return a.servePrivateMedia(w, r, source.ObjectID, name, ct, key, source.MediaMessageID)
}
