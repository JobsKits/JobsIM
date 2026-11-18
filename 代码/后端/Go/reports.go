package jobsim

import (
	"bytes"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Report struct {
	ID           string    `json:"id"`
	ReporterID   string    `json:"reporterId"`
	TargetType   string    `json:"targetType"`
	TargetID     string    `json:"targetId"`
	TargetUserID string    `json:"targetUserId,omitempty"`
	Reason       string    `json:"reason"`
	State        string    `json:"state"`
	Version      string    `json:"version"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

const reportColumns = `id::text,reporter_id::text,target_type,target_id::text,reason,state,row_version,created_at,updated_at,COALESCE(target_user_id::text,'')`

func scanReport(row pgx.Row) (Report, error) {
	var v Report
	var version int64
	e := row.Scan(&v.ID, &v.ReporterID, &v.TargetType, &v.TargetID, &v.Reason, &v.State, &version, &v.CreatedAt, &v.UpdatedAt, &v.TargetUserID)
	v.Version = str(version)
	return v, e
}
func validReportState(state string) bool {
	return state == "open" || state == "reviewed" || state == "dismissed" || state == "action_required"
}

func (a *App) governanceRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.reportEvidenceGovernanceRoutes(h, internal)
	h("POST /api/reports", a.createReport)
	h("GET /api/reports", func(w http.ResponseWriter, r *http.Request) error { return a.listReports(w, r, false) })
	h("GET /api/admin/reports", func(w http.ResponseWriter, r *http.Request) error { return a.listReports(w, r, true) })
	h("PATCH /api/admin/reports/{id}", a.resolveReport)
	h("GET /api/admin/report-operations/{id}", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !u.Admin {
			return fail(403, "ADMIN_REQUIRED", "Administrator required")
		}
		var state string
		var raw []byte
		if e = a.DB.QueryRow(r.Context(), `SELECT state,result FROM governance.report_operations WHERE request_id=$1 AND actor_id=$2`, r.PathValue("id"), u.ID).Scan(&state, &raw); e != nil {
			return fail(404, "OPERATION_NOT_FOUND", "Report operation unavailable")
		}
		var result any
		if e = json.Unmarshal(raw, &result); e != nil {
			return e
		}
		respond(w, 200, map[string]any{"state": state, "result": result})
		return nil
	})
}
func (a *App) createReport(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q reportCreateRequest
	if e = decode(r, &q); e != nil {
		return e
	}
	if (q.TargetType != "message" && q.TargetType != "user" && q.TargetType != "conversation") || !validAssetID(q.TargetID) || len(q.RequestID) < 8 || len(q.RequestID) > 128 || !utf8.ValidString(q.Reason) || utf8.RuneCountInString(strings.TrimSpace(q.Reason)) < 1 || utf8.RuneCountInString(q.Reason) > 1000 {
		return fail(400, "INVALID_REPORT", "Report target, 1–1000 character reason and stable requestId required")
	}
	if !validReportSelection(q.EvidenceIDs, reportFileLimit) || !validReportSelection(q.SelectedMessageIDs, reportMessageLimit) {
		return fail(400, "INVALID_REPORT_SELECTION", "At most10 distinct evidence files and50 distinct current messages may be selected")
	}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "report:"+u.ID, raw)
	var prior []byte
	e = a.DB.QueryRow(r.Context(), `SELECT request_digest FROM governance.reports WHERE reporter_id=$1 AND request_id=$2`, u.ID, q.RequestID).Scan(&prior)
	if e == nil {
		if !bytes.Equal(prior, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original report differs")
		}
		v, e := scanReport(a.DB.QueryRow(r.Context(), `SELECT `+reportColumns+` FROM governance.reports WHERE reporter_id=$1 AND request_id=$2`, u.ID, q.RequestID))
		if e != nil {
			return e
		}
		respond(w, 200, v)
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	var count int
	if e = a.DB.QueryRow(r.Context(), `INSERT INTO governance.rate_windows(subject,window_start,count) VALUES($1,date_trunc('minute',now()),1) ON CONFLICT(subject,window_start) DO UPDATE SET count=governance.rate_windows.count+1 RETURNING count`, u.ID).Scan(&count); e != nil {
		return e
	}
	if count > 5 {
		return fail(429, "REPORT_RATE_LIMITED", "Report limit reached; try later")
	}
	targetUser := ""
	switch q.TargetType {
	case "message":
		var target struct {
			SenderID string `json:"senderId"`
		}
		e = a.rpc(r.Context(), "message", "/internal/report-target", map[string]string{"userId": u.ID, "messageId": q.TargetID}, &target)
		targetUser = target.SenderID
		if e == nil && !validAssetID(targetUser) {
			e = fail(503, "REPORT_TARGET_UNAVAILABLE", "Current message target identity could not be qualified")
		}
	case "user":
		targetUser = q.TargetID
		e = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": []string{q.TargetID}}, nil)
	case "conversation":
		_, e = a.member(r.Context(), u.ID, q.TargetID)
	}
	if e != nil {
		return e
	}
	for _, object := range q.EvidenceIDs {
		if e = a.rpc(r.Context(), "media", "/internal/report-evidence", map[string]string{"objectId": object, "userId": u.ID}, nil); e != nil {
			return e
		}
	}
	selected := map[string]string{}
	for _, message := range q.SelectedMessageIDs {
		view, lookup := a.reportMessageEvidence(r.Context(), u.ID, message, "")
		if lookup != nil {
			return lookup
		}
		selected[message] = view.ContentVersion
	}
	id := newID()
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `INSERT INTO governance.reports(id,reporter_id,target_type,target_id,reason,request_id,request_digest,target_user_id) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid) ON CONFLICT(reporter_id,request_id) DO NOTHING`, id, u.ID, q.TargetType, q.TargetID, q.Reason, q.RequestID, digest, targetUser)
	if e != nil {
		return e
	}
	if e = tx.QueryRow(r.Context(), `SELECT id::text,request_digest FROM governance.reports WHERE reporter_id=$1 AND request_id=$2 FOR UPDATE`, u.ID, q.RequestID).Scan(&id, &prior); e != nil {
		return e
	}
	if !bytes.Equal(prior, digest) {
		return fail(409, "IDEMPOTENCY_CONFLICT", "Concurrent report differs")
	}
	for _, object := range q.EvidenceIDs {
		if _, e = tx.Exec(r.Context(), `INSERT INTO governance.report_evidence(report_id,object_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, object); e != nil {
			return e
		}
	}
	for message, version := range selected {
		if _, e = tx.Exec(r.Context(), `INSERT INTO governance.report_messages(report_id,message_id,content_version) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, message, version); e != nil {
			return e
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	v, e := scanReport(a.DB.QueryRow(r.Context(), `SELECT `+reportColumns+` FROM governance.reports WHERE reporter_id=$1 AND request_id=$2`, u.ID, q.RequestID))
	if e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}
func (a *App) listReports(w http.ResponseWriter, r *http.Request, admin bool) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	if admin && !u.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	state := r.URL.Query().Get("state")
	if state != "" && !validReportState(state) {
		return fail(400, "INVALID_REPORT_STATE", "Unknown report state")
	}
	reporter, target := r.URL.Query().Get("reporterId"), r.URL.Query().Get("targetUserId")
	if reporter != "" && !validAssetID(reporter) || target != "" && !validAssetID(target) {
		return fail(400, "INVALID_REPORT_FILTER", "Stable reporter or target user ID required")
	}
	rows, e := a.DB.Query(r.Context(), `SELECT `+reportColumns+` FROM governance.reports WHERE($1 OR reporter_id=$2)AND($3='' OR state=$3)AND($4='' OR reporter_id=NULLIF($4,'')::uuid)AND($5='' OR target_user_id=NULLIF($5,'')::uuid) ORDER BY created_at DESC,id LIMIT 100`, admin, u.ID, state, reporter, target)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []Report{}
	for rows.Next() {
		v, e := scanReport(rows)
		if e != nil {
			return e
		}
		items = append(items, v)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"items": items, "includesChatBodies": false})
	return nil
}
func (a *App) resolveReport(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	if !u.Admin {
		return fail(403, "ADMIN_REQUIRED", "Administrator required")
	}
	id := r.PathValue("id")
	if !validAssetID(id) {
		return fail(404, "REPORT_NOT_FOUND", "Report unavailable")
	}
	var q struct {
		State           string `json:"state"`
		Reason          string `json:"reason"`
		ExpectedVersion string `json:"expectedVersion"`
		RequestID       string `json:"requestId"`
		StepUpID        string `json:"stepUpId"`
	}
	raw, e := readAdminBody(r, &q)
	if e != nil {
		return e
	}
	if !validReportState(q.State) || q.State == "open" || len(q.RequestID) < 8 || len(q.RequestID) > 128 || q.ExpectedVersion == "" || len(strings.TrimSpace(q.Reason)) == 0 || utf8.RuneCountInString(q.Reason) > 1000 {
		return fail(400, "INVALID_REPORT_OPERATION", "Settled state, reason, expectedVersion and stable requestId required")
	}
	canonical, e := canonicalAdminPayload(raw)
	if e != nil {
		return e
	}
	digest := keyedDigest(a.Config.Key, "report-resolve:"+u.ID+":"+id, canonical)
	var original, result []byte
	var actor, reportID, state string
	e = a.DB.QueryRow(r.Context(), `SELECT actor_id::text,report_id::text,request_digest,state,result FROM governance.report_operations WHERE request_id=$1`, q.RequestID).Scan(&actor, &reportID, &original, &state, &result)
	if e == nil {
		if actor != u.ID || reportID != id || !bytes.Equal(original, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original report operation differs")
		}
		if state == "completed" {
			var body any
			if e = json.Unmarshal(result, &body); e != nil {
				return e
			}
			respond(w, 200, body)
			return nil
		}
	} else if e != pgx.ErrNoRows {
		return e
	}
	if e = a.previewAdminStepUp(r, u, q.StepUpID, "report.resolve", id, q.RequestID, raw); e != nil {
		return e
	}
	// The proof is checked before reserving the operation so invalid payloads cannot poison retries.
	_, e = a.DB.Exec(r.Context(), `INSERT INTO governance.report_operations(request_id,report_id,actor_id,request_digest,state,reason) SELECT $1,id,$3,$4,'prepared',$5 FROM governance.reports WHERE id=$2 ON CONFLICT DO NOTHING`, q.RequestID, id, u.ID, digest, q.Reason)
	if e != nil {
		return e
	}
	if e = a.DB.QueryRow(r.Context(), `SELECT actor_id::text,report_id::text,request_digest FROM governance.report_operations WHERE request_id=$1`, q.RequestID).Scan(&actor, &reportID, &original); e != nil {
		return fail(404, "REPORT_NOT_FOUND", "Report unavailable")
	}
	if actor != u.ID || reportID != id || !bytes.Equal(original, digest) {
		return fail(409, "IDEMPOTENCY_CONFLICT", "Original report operation differs")
	}
	if e = a.adminStepUp(r, u, q.StepUpID, "report.resolve", id, q.RequestID, raw); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = tx.QueryRow(r.Context(), `SELECT state,result FROM governance.report_operations WHERE request_id=$1 FOR UPDATE`, q.RequestID).Scan(&state, &result); e != nil {
		return e
	}
	if state == "completed" {
		var body any
		if e = json.Unmarshal(result, &body); e != nil {
			return e
		}
		respond(w, 200, body)
		return nil
	}
	var version int64
	if e = tx.QueryRow(r.Context(), `SELECT row_version FROM governance.reports WHERE id=$1 FOR UPDATE`, id).Scan(&version); e != nil {
		return fail(404, "REPORT_NOT_FOUND", "Report unavailable")
	}
	if str(version) != q.ExpectedVersion {
		return fail(409, "VERSION_CONFLICT", "Reload the current report")
	}
	_, e = tx.Exec(r.Context(), `UPDATE governance.reports SET state=$2,row_version=row_version+1,updated_at=now() WHERE id=$1`, id, q.State)
	if e != nil {
		return e
	}
	v, e := scanReport(tx.QueryRow(r.Context(), `SELECT `+reportColumns+` FROM governance.reports WHERE id=$1`, id))
	if e != nil {
		return e
	}
	result, _ = json.Marshal(v)
	_, e = tx.Exec(r.Context(), `UPDATE governance.report_operations SET state='completed',result=$2 WHERE request_id=$1`, q.RequestID, result)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}

func (a *App) reportMessageRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/report-target", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID    string `json:"userId"`
			MessageID string `json:"messageId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		m, e := a.loadMessage(r.Context(), q.MessageID, q.UserID)
		if e != nil {
			return e
		}
		if m.Deleted || m.ContentUnavailable || m.ForwardUnavailable || m.sourcePending {
			return fail(404, "MESSAGE_NOT_FOUND", "Deleted message cannot supply report evidence")
		}
		respond(w, 200, map[string]string{"messageId": m.ID, "conversationId": m.ConversationID, "senderId": m.SenderID})
		return nil
	}))
	h("POST /internal/report-message", internal(a.reportMessageSource))
}
