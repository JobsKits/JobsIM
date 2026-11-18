package jobsim

import (
	"net/http"
)

func (a *App) reportEvidenceGovernanceRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/reports/{id}", func(w http.ResponseWriter, r *http.Request) error { return a.reportDetails(w, r, false) })
	h("GET /api/admin/reports/{id}", func(w http.ResponseWriter, r *http.Request) error { return a.reportDetails(w, r, true) })
	h("POST /internal/report-evidence-access", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "media" {
			return fail(403, "REPORT_MEDIA_REQUIRED", "Only the media owner may qualify evidence access")
		}
		var q struct {
			ReportID string `json:"reportId"`
			ObjectID string `json:"objectId"`
			UserID   string `json:"userId"`
			Admin    bool   `json:"admin"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		if !validAssetID(q.ReportID) || !validAssetID(q.ObjectID) || !validAssetID(q.UserID) {
			return fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Selected report evidence unavailable")
		}
		var allowed bool
		if err := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM governance.reports r JOIN governance.report_evidence e ON e.report_id=r.id WHERE r.id=$1 AND e.object_id=$2 AND($3 OR r.reporter_id=$4))`, q.ReportID, q.ObjectID, q.Admin, q.UserID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return fail(404, "REPORT_EVIDENCE_UNAVAILABLE", "Selected report evidence unavailable")
		}
		respond(w, 200, map[string]bool{"allowed": true})
		return nil
	}))
	h("POST /internal/report-message-access", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "media" {
			return fail(403, "REPORT_MEDIA_REQUIRED", "Only the media owner may qualify evidence access")
		}
		var q struct {
			ReportID  string `json:"reportId"`
			MessageID string `json:"messageId"`
			UserID    string `json:"userId"`
			Admin     bool   `json:"admin"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		if !validAssetID(q.ReportID) || !validAssetID(q.MessageID) || !validAssetID(q.UserID) {
			return fail(404, "REPORT_MESSAGE_UNAVAILABLE", "Selected report message unavailable")
		}
		var reporter, version string
		if err := a.DB.QueryRow(r.Context(), `SELECT r.reporter_id::text,m.content_version FROM governance.reports r JOIN governance.report_messages m ON m.report_id=r.id WHERE r.id=$1 AND m.message_id=$2 AND($3 OR r.reporter_id=$4)`, q.ReportID, q.MessageID, q.Admin, q.UserID).Scan(&reporter, &version); err != nil {
			return fail(404, "REPORT_MESSAGE_UNAVAILABLE", "Selected report message unavailable")
		}
		m, err := a.reportMessageEvidence(r.Context(), reporter, q.MessageID, version)
		if err != nil {
			return err
		}
		respond(w, 200, m)
		return nil
	}))
}

// Only references selected by the reporting user reach this read. A fresh source
// authorization is required on every view; no historical body is copied back.
func (a *App) reportMessageSource(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "governance" {
		return fail(403, "REPORT_OWNER_REQUIRED", "Only the report owner may qualify a selected message")
	}
	var q struct {
		UserID    string `json:"userId"`
		MessageID string `json:"messageId"`
		Version   string `json:"contentVersion"`
	}
	if err := decode(r, &q); err != nil {
		return err
	}
	if !validAssetID(q.UserID) || !validAssetID(q.MessageID) {
		return fail(404, "REPORT_MESSAGE_UNAVAILABLE", "Selected report message unavailable")
	}
	if _, err := a.readForwardPrincipal(r.Context(), q.UserID); err != nil {
		return err
	}
	m, err := a.loadMessage(r.Context(), q.MessageID, q.UserID)
	if err != nil {
		return err
	}
	if m.Deleted || m.ContentUnavailable || m.ForwardUnavailable || m.sourcePending || q.Version != "" && q.Version != m.ContentVersion {
		return fail(410, "REPORT_MESSAGE_UNAVAILABLE", "Selected source has changed or is no longer readable")
	}
	object := m.AttachmentID
	mediaMessage := m.ID
	if m.Forwarded {
		object, mediaMessage = m.forwardLeafAttachment, m.forwardLeafMessage
	}
	respond(w, 200, ReportMessageEvidence{ID: m.ID, ConversationID: m.ConversationID, SenderID: m.SenderID, Type: m.Type, Text: m.Text, ContentVersion: m.ContentVersion, CreatedAt: &m.CreatedAt, ObjectID: object, MediaMessageID: mediaMessage, Available: true})
	return nil
}
