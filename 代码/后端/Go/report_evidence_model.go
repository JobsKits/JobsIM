package jobsim

import (
	"context"
	"net/http"
	"time"
)

const reportFileLimit = 10
const reportMessageLimit = 50

type reportCreateRequest struct {
	TargetType         string   `json:"targetType"`
	TargetID           string   `json:"targetId"`
	Reason             string   `json:"reason"`
	RequestID          string   `json:"requestId"`
	EvidenceIDs        []string `json:"evidenceIds,omitempty"`
	SelectedMessageIDs []string `json:"selectedMessageIds,omitempty"`
}

type ReportEvidence struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Digest      string `json:"sha256,omitempty"`
	URL         string `json:"url,omitempty"`
	Available   bool   `json:"available"`
}

type ReportMessageEvidence struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversationId,omitempty"`
	SenderID       string     `json:"senderId,omitempty"`
	Type           string     `json:"type,omitempty"`
	Text           string     `json:"text,omitempty"`
	ContentVersion string     `json:"contentVersion,omitempty"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
	AttachmentURL  string     `json:"attachmentURL,omitempty"`
	ObjectID       string     `json:"objectId,omitempty"`
	MediaMessageID string     `json:"mediaMessageId,omitempty"`
	Available      bool       `json:"available"`
}

func init() {
	migrations["governance"] += `
ALTER TABLE governance.reports ADD COLUMN IF NOT EXISTS target_user_id uuid;
UPDATE governance.reports SET target_user_id=target_id WHERE target_type='user' AND target_user_id IS NULL;
CREATE INDEX IF NOT EXISTS report_target_user ON governance.reports(target_user_id,created_at DESC);
CREATE TABLE IF NOT EXISTS governance.report_evidence(report_id uuid NOT NULL REFERENCES governance.reports(id),object_id uuid NOT NULL,PRIMARY KEY(report_id,object_id));
CREATE TABLE IF NOT EXISTS governance.report_messages(report_id uuid NOT NULL REFERENCES governance.reports(id),message_id uuid NOT NULL,content_version text NOT NULL,PRIMARY KEY(report_id,message_id));`
}

func validReportSelection(ids []string, limit int) bool {
	if len(ids) > limit {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !validAssetID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (a *App) reportMessageEvidence(ctx context.Context, user, id, version string) (ReportMessageEvidence, error) {
	var out ReportMessageEvidence
	err := a.rpc(ctx, "message", "/internal/report-message", map[string]string{"userId": user, "messageId": id, "contentVersion": version}, &out)
	return out, err
}

func (a *App) reportDetails(w http.ResponseWriter, r *http.Request, admin bool) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	if admin && !u.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	v, err := scanReport(a.DB.QueryRow(r.Context(), `SELECT `+reportColumns+` FROM governance.reports WHERE id=$1 AND($2 OR reporter_id=$3)`, r.PathValue("id"), admin, u.ID))
	if err != nil {
		return fail(404, "REPORT_NOT_FOUND", "Report unavailable")
	}
	files := []ReportEvidence{}
	rows, err := a.DB.Query(r.Context(), `SELECT object_id::text FROM governance.report_evidence WHERE report_id=$1 ORDER BY object_id`, v.ID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		var file ReportEvidence
		if err = a.rpc(r.Context(), "media", "/internal/report-evidence", map[string]string{"objectId": id, "userId": v.ReporterID}, &file); err != nil {
			files = append(files, ReportEvidence{ID: id})
			continue
		}
		file.URL += "?reportId=" + v.ID
		files = append(files, file)
	}
	rows, err = a.DB.Query(r.Context(), `SELECT message_id::text,content_version FROM governance.report_messages WHERE report_id=$1 ORDER BY message_id`, v.ID)
	if err != nil {
		return err
	}
	type selected struct{ id, version string }
	selectedIDs := []selected{}
	for rows.Next() {
		var value selected
		if err = rows.Scan(&value.id, &value.version); err != nil {
			rows.Close()
			return err
		}
		selectedIDs = append(selectedIDs, value)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	messages := []ReportMessageEvidence{}
	for _, item := range selectedIDs {
		m, lookup := a.reportMessageEvidence(r.Context(), v.ReporterID, item.id, item.version)
		if lookup != nil {
			messages = append(messages, ReportMessageEvidence{ID: item.id})
			continue
		}
		if m.ObjectID != "" {
			prefix := "/api/reports/"
			if admin {
				prefix = "/api/admin/reports/"
			}
			m.AttachmentURL = prefix + v.ID + "/messages/" + m.ID + "/attachment"
		}
		m.ObjectID = ""
		m.MediaMessageID = ""
		messages = append(messages, m)
	}
	respond(w, 200, map[string]any{"report": v, "evidence": files, "selectedMessages": messages, "includesWholeConversation": false})
	return nil
}
