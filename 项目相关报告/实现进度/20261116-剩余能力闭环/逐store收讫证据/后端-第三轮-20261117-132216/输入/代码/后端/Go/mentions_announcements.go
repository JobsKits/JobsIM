package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["message"] += `
ALTER TABLE message.messages ADD COLUMN IF NOT EXISTS content_version bigint NOT NULL DEFAULT 1 CHECK(content_version>0);
CREATE TABLE IF NOT EXISTS message.mention_envelopes(message_id uuid PRIMARY KEY REFERENCES message.messages(id),mention_all boolean NOT NULL);
CREATE TABLE IF NOT EXISTS message.mentions(message_id uuid NOT NULL REFERENCES message.messages(id),conversation_id uuid NOT NULL,mentioned_user_id uuid NOT NULL,mention_kind text NOT NULL CHECK(mention_kind IN('member','all')),membership_generation bigint NOT NULL CHECK(membership_generation>0),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(message_id,mentioned_user_id));
CREATE TABLE IF NOT EXISTS message.mention_rate_windows(sender_id uuid NOT NULL,conversation_id uuid NOT NULL,kind text NOT NULL,window_started bigint NOT NULL,count bigint NOT NULL,PRIMARY KEY(sender_id,conversation_id,kind,window_started));`
	migrations["conversation"] += `
ALTER TABLE conversation.members ADD COLUMN IF NOT EXISTS role_generation bigint NOT NULL DEFAULT 1 CHECK(role_generation>0);
ALTER TABLE conversation.conversations ADD COLUMN IF NOT EXISTS announcement_message_id uuid;
ALTER TABLE conversation.conversations ADD COLUMN IF NOT EXISTS announcement_source_version bigint;
ALTER TABLE conversation.conversations ADD COLUMN IF NOT EXISTS announcement_expires_at timestamptz;
ALTER TABLE conversation.conversations ADD COLUMN IF NOT EXISTS announcement_version bigint NOT NULL DEFAULT 0;
ALTER TABLE conversation.conversations ADD COLUMN IF NOT EXISTS announcement_updated_at timestamptz;
CREATE TABLE IF NOT EXISTS conversation.announcement_operations(actor_id uuid NOT NULL,request_id text NOT NULL,conversation_id uuid NOT NULL,restore_epoch bigint NOT NULL,request_digest bytea NOT NULL,result jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(actor_id,request_id));`
}

type mentionTarget struct {
	UserID               string `json:"userId"`
	MembershipGeneration string `json:"membershipGeneration"`
}

type mentionPolicy struct {
	WindowSeconds int64
	MemberLimit   int64
	AllLimit      int64
	MaxTargets    int64
}

// These are engineering defaults. Each role receives the same deployment env.
func currentMentionPolicy() (mentionPolicy, error) {
	p := mentionPolicy{60, 20, 2, 20}
	for _, setting := range []struct {
		Name string
		To   *int64
		Max  int64
	}{
		{"JOBS_IM_MENTION_WINDOW_SECONDS", &p.WindowSeconds, 86400},
		{"JOBS_IM_MENTION_MEMBER_LIMIT", &p.MemberLimit, 10000},
		{"JOBS_IM_MENTION_ALL_LIMIT", &p.AllLimit, 10000},
		{"JOBS_IM_MENTION_MAX_TARGETS", &p.MaxTargets, 200},
	} {
		if raw, exists := os.LookupEnv(setting.Name); exists {
			v, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || v < 1 || v > setting.Max || str(v) != raw {
				return p, fail(503, "MENTION_POLICY_INVALID", "Mention policy configuration invalid")
			}
			*setting.To = v
		}
	}
	return p, nil
}

func (a *App) prepareMentions(ctx context.Context, sender, conversation string, ids []string, all bool) ([]mentionTarget, error) {
	if len(ids) == 0 && !all {
		return nil, nil
	}
	if e := a.requireFeature(ctx, "groups"); e != nil {
		return nil, e
	}
	p, e := currentMentionPolicy()
	if e != nil {
		return nil, e
	}
	if all && len(ids) > 0 || int64(len(ids)) > p.MaxTargets {
		return nil, fail(400, "INVALID_MENTIONS", "Choose a bounded member list or mentionAll")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !validObjectID(id) || seen[id] {
			return nil, fail(400, "INVALID_MENTIONS", "Unique member identities required")
		}
		seen[id] = true
	}
	var result struct {
		Items []mentionTarget `json:"items"`
	}
	e = a.rpc(ctx, "conversation", "/internal/mentions/snapshot", map[string]any{"userId": sender, "conversationId": conversation, "mentionedUserIds": ids, "mentionAll": all}, &result)
	return result.Items, e
}

func (a *App) persistMentions(ctx context.Context, tx pgx.Tx, id, sender, conversation string, targets []mentionTarget, all bool) error {
	if len(targets) == 0 && !all {
		return nil
	}
	p, e := currentMentionPolicy()
	if e != nil {
		return e
	}
	kind, limit := "member", p.MemberLimit
	if all {
		kind, limit = "all", p.AllLimit
	}
	var count int64
	window := time.Now().Unix() / p.WindowSeconds * p.WindowSeconds
	e = tx.QueryRow(ctx, `INSERT INTO message.mention_rate_windows(sender_id,conversation_id,kind,window_started,count) VALUES($1,$2,$3,$4,1) ON CONFLICT(sender_id,conversation_id,kind,window_started) DO UPDATE SET count=message.mention_rate_windows.count+1 RETURNING count`, sender, conversation, kind, window).Scan(&count)
	if e != nil {
		return e
	}
	if count > limit {
		return fail(429, "MENTION_RATE_LIMIT", "Mention rate limit reached; retry the original message later")
	}
	if _, e = tx.Exec(ctx, `INSERT INTO message.mention_envelopes(message_id,mention_all) VALUES($1,$2)`, id, all); e != nil {
		return e
	}
	for _, target := range targets {
		_, e = tx.Exec(ctx, `INSERT INTO message.mentions(message_id,conversation_id,mentioned_user_id,mention_kind,membership_generation) VALUES($1,$2,$3,$4,$5::bigint)`, id, conversation, target.UserID, kind, target.MembershipGeneration)
		if e != nil {
			return e
		}
	}
	return nil
}

func (a *App) enrichMentions(ctx context.Context, m *Message, user string) error {
	if m.Deleted || m.ContentUnavailable {
		return nil
	}
	var all bool
	e := a.DB.QueryRow(ctx, `SELECT mention_all FROM message.mention_envelopes WHERE message_id=$1`, m.ID).Scan(&all)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	m.MentionAll = all
	rows, e := a.DB.Query(ctx, `SELECT mentioned_user_id::text,membership_generation,mention_kind FROM message.mentions WHERE message_id=$1 ORDER BY mentioned_user_id`, m.ID)
	if e != nil {
		return e
	}
	var mine string
	for rows.Next() {
		var id, kind string
		var generation int64
		if e = rows.Scan(&id, &generation, &kind); e != nil {
			rows.Close()
			return e
		}
		if kind == "member" {
			m.MentionedUserIDs = append(m.MentionedUserIDs, id)
		}
		if id == user {
			mine = str(generation)
		}
	}
	rows.Close()
	if e = rows.Err(); e != nil || mine == "" || user == m.SenderID {
		return e
	}
	meta, e := a.member(ctx, user, m.ConversationID)
	if e != nil {
		return e
	}
	if meta["membershipGeneration"] != mine {
		return nil
	}
	blocked, _, _, e := a.relationshipAdmission(ctx, user, m.SenderID)
	if e != nil {
		return e
	}
	m.MentionedMe = !blocked
	return nil
}

func (a *App) mentionAnnouncementConversationRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/mentions/snapshot", internal(a.mentionSnapshot))
	h("GET /api/conversations/{id}/announcement", a.getAnnouncement)
	h("PUT /api/conversations/{id}/announcement", func(w http.ResponseWriter, r *http.Request) error { return a.writeAnnouncement(w, r, false) })
	h("DELETE /api/conversations/{id}/announcement", func(w http.ResponseWriter, r *http.Request) error { return a.writeAnnouncement(w, r, true) })
	h("GET /api/conversations/{id}/announcement/operations/{requestId}", a.getAnnouncementOperation)
}

func (a *App) mentionSnapshot(w http.ResponseWriter, r *http.Request) error {
	var q struct {
		UserID           string   `json:"userId"`
		ConversationID   string   `json:"conversationId"`
		MentionedUserIDs []string `json:"mentionedUserIds"`
		MentionAll       bool     `json:"mentionAll"`
	}
	if e := decode(r, &q); e != nil {
		return e
	}
	var owner, kind, state string
	e := a.DB.QueryRow(r.Context(), `SELECT c.owner_id::text,c.kind,c.state FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id WHERE c.id=$1 AND m.user_id=$2 AND m.state='active' AND m.export_pending_operation_id IS NULL`, q.ConversationID, q.UserID).Scan(&owner, &kind, &state)
	if e != nil || kind != "group" || state != "active" {
		return fail(403, "MENTION_FORBIDDEN", "An active group membership is required")
	}
	if q.MentionAll && owner != q.UserID {
		return fail(403, "MENTION_ALL_FORBIDDEN", "Only the current group owner may mention all")
	}
	rows, e := a.DB.Query(r.Context(), `SELECT user_id::text,membership_generation FROM conversation.members WHERE conversation_id=$1 AND state='active' AND export_pending_operation_id IS NULL AND ($2 OR user_id=ANY($3::uuid[])) ORDER BY user_id`, q.ConversationID, q.MentionAll, q.MentionedUserIDs)
	if e != nil {
		return e
	}
	items := []mentionTarget{}
	for rows.Next() {
		var target mentionTarget
		var generation int64
		if e = rows.Scan(&target.UserID, &generation); e != nil {
			rows.Close()
			return e
		}
		target.MembershipGeneration = str(generation)
		items = append(items, target)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	if !q.MentionAll && len(items) != len(q.MentionedUserIDs) {
		return fail(400, "MENTION_MEMBER_REQUIRED", "Every selected mention must be a current group member")
	}
	respond(w, 200, map[string]any{"items": items})
	return nil
}

type announcement struct {
	ConversationID string     `json:"conversationId"`
	Version        string     `json:"version"`
	State          string     `json:"state"`
	MessageID      string     `json:"messageId,omitempty"`
	SourceVersion  string     `json:"sourceVersion,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
	CanManage      bool       `json:"canManage"`
	Message        *Message   `json:"message,omitempty"`
}

type announcementMutation struct {
	MessageID            string     `json:"messageId,omitempty"`
	SourceVersion        string     `json:"sourceVersion,omitempty"`
	ExpiresAt            *time.Time `json:"expiresAt,omitempty"`
	ExpectedVersion      string     `json:"expectedVersion"`
	MembershipGeneration string     `json:"membershipGeneration"`
	RequestID            string     `json:"requestId"`
	RoleGeneration       string     `json:"roleGeneration,omitempty"`
}

type announcementOperation struct {
	OperationID string       `json:"operationId"`
	Status      string       `json:"status"`
	Result      announcement `json:"result"`
	Current     bool         `json:"current"`
}

func canonicalCounter(raw string, allowZero bool) (int64, bool) {
	v, e := strconv.ParseInt(raw, 10, 64)
	return v, e == nil && str(v) == raw && (v > 0 || allowZero && v == 0)
}

func scanAnnouncement(row pgx.Row) (announcement, error) {
	var v announcement
	var version int64
	var source *int64
	e := row.Scan(&v.ConversationID, &version, &v.MessageID, &source, &v.ExpiresAt, &v.UpdatedAt)
	v.Version, v.State = str(version), "empty"
	if source != nil {
		v.SourceVersion = str(*source)
	}
	if v.MessageID != "" {
		v.State = "unavailable"
	}
	return v, e
}

const announcementColumns = `id::text,announcement_version,COALESCE(announcement_message_id::text,''),announcement_source_version,announcement_expires_at,announcement_updated_at`

func (a *App) announcementAccess(r *http.Request, user string) (string, string, time.Time, error) {
	var owner, state string
	var joined time.Time
	e := a.DB.QueryRow(r.Context(), `SELECT c.owner_id::text,c.state,m.joined_at FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id WHERE c.id=$1 AND c.kind='group' AND c.state IN('active','owner_unavailable') AND m.user_id=$2 AND m.state='active' AND m.export_pending_operation_id IS NULL`, r.PathValue("id"), user).Scan(&owner, &state, &joined)
	if e != nil {
		return "", "", joined, fail(403, "NOT_A_MEMBER", "Current group membership required")
	}
	return owner, state, joined, nil
}

func (a *App) getAnnouncement(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	v, e := a.projectAnnouncement(r, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}

func (a *App) projectAnnouncement(r *http.Request, user string) (announcement, error) {
	owner, state, joined, e := a.announcementAccess(r, user)
	if e != nil {
		return announcement{}, e
	}
	v, e := scanAnnouncement(a.DB.QueryRow(r.Context(), `SELECT `+announcementColumns+` FROM conversation.conversations WHERE id=$1`, r.PathValue("id")))
	if e != nil {
		return announcement{}, e
	}
	v.CanManage = owner == user && state == "active"
	if v.MessageID != "" {
		if v.ExpiresAt != nil && !v.ExpiresAt.After(time.Now()) {
			v.State = "expired"
		} else {
			var result struct {
				Available bool     `json:"available"`
				Message   *Message `json:"message"`
			}
			e = a.rpc(r.Context(), "message", "/internal/announcement-source", map[string]any{"userId": user, "conversationId": v.ConversationID, "messageId": v.MessageID, "sourceVersion": v.SourceVersion, "joinedAt": joined}, &result)
			if e != nil {
				return announcement{}, e
			}
			if result.Available && result.Message != nil {
				v.State, v.Message = "active", result.Message
			}
		}
	}
	return v, nil
}

func (a *App) announcementMessageRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/announcement-source", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID         string    `json:"userId"`
			ConversationID string    `json:"conversationId"`
			MessageID      string    `json:"messageId"`
			SourceVersion  string    `json:"sourceVersion"`
			JoinedAt       time.Time `json:"joinedAt"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		// Conversation owner already checked membership. Do not call it back while
		// an announcement writer holds the parent group row and the shared-host fence.
		m, e := scanMessage(a.DB.QueryRow(r.Context(), `SELECT `+messageColumns+` FROM message.messages WHERE id=$1 AND conversation_id=$2 AND export_pending_operation_id IS NULL`, q.MessageID, q.ConversationID))
		if e == pgx.ErrNoRows || e == nil && (m.ContentVersion != q.SourceVersion || m.Deleted || m.ContentUnavailable || m.CreatedAt.Before(q.JoinedAt)) {
			respond(w, 200, map[string]bool{"available": false})
			return nil
		}
		if e != nil {
			return e
		}
		a.projectMessage(r.Context(), &m, q.UserID, nil)
		if m.ForwardUnavailable {
			respond(w, 200, map[string]bool{"available": false})
			return nil
		}
		// Projection for announcement reads includes no new reminder event.
		respond(w, 200, map[string]any{"available": true, "message": m})
		return nil
	}))
}

func (a *App) originalAnnouncement(ctx context.Context, user, request, conversation, epoch string) (announcementOperation, []byte, error) {
	var op announcementOperation
	var raw, digest []byte
	e := a.DB.QueryRow(ctx, `SELECT result,request_digest FROM conversation.announcement_operations WHERE actor_id=$1 AND request_id=$2 AND conversation_id=$3 AND restore_epoch=$4::bigint`, user, request, conversation, epoch).Scan(&raw, &digest)
	if e != nil {
		return op, nil, e
	}
	if e = json.Unmarshal(raw, &op.Result); e != nil {
		return op, nil, e
	}
	op.OperationID, op.Status = request, "applied"
	e = a.DB.QueryRow(ctx, `SELECT announcement_version=$2::bigint FROM conversation.conversations WHERE id=$1`, conversation, op.Result.Version).Scan(&op.Current)
	return op, digest, e
}

func (a *App) getAnnouncementOperation(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	if _, _, _, e = a.announcementAccess(r, u.ID); e != nil {
		return e
	}
	op, _, e := a.originalAnnouncement(r.Context(), u.ID, r.PathValue("requestId"), r.PathValue("id"), u.RestoreEpoch)
	if e == pgx.ErrNoRows {
		return fail(404, "ANNOUNCEMENT_OPERATION_NOT_FOUND", "No committed original operation; keep its original payload")
	}
	if e != nil {
		return e
	}
	respond(w, 200, op)
	return nil
}

func (a *App) writeAnnouncement(w http.ResponseWriter, r *http.Request, clear bool) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	_, _, joined, e := a.announcementAccess(r, u.ID)
	if e != nil {
		return e
	}
	var q announcementMutation
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 {
		return fail(400, "INVALID_REQUEST_ID", "A stable original requestId is required")
	}
	if _, ok := canonicalCounter(q.ExpectedVersion, true); !ok {
		return fail(400, "INVALID_VERSION", "Canonical announcement version required")
	}
	if _, ok := canonicalCounter(q.MembershipGeneration, false); !ok {
		return fail(400, "INVALID_GENERATION", "Original membership generation required")
	}
	if clear && (q.MessageID != "" || q.SourceVersion != "" || q.ExpiresAt != nil) {
		return fail(400, "INVALID_ANNOUNCEMENT", "Clear operation has no source")
	}
	if !clear {
		if _, ok := canonicalCounter(q.SourceVersion, false); !ok || !validObjectID(q.MessageID) {
			return fail(400, "INVALID_ANNOUNCEMENT", "Same-group message and exact sourceVersion required")
		}
	}
	raw, _ := json.Marshal(map[string]any{"conversationId": r.PathValue("id"), "clear": clear, "payload": q})
	digest := tokenDigest(string(raw))
	op, original, e := a.originalAnnouncement(r.Context(), u.ID, q.RequestID, r.PathValue("id"), u.RestoreEpoch)
	if e == nil {
		if !bytes.Equal(original, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original announcement request has different content")
		}
		respond(w, 200, op)
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	if _, ok := canonicalCounter(q.RoleGeneration, false); !ok {
		return fail(409, "QUALIFICATION_CHANGED", "Original role generation is absent or no longer current; do not patch the old request")
	}
	if q.ExpiresAt != nil && !q.ExpiresAt.After(time.Now()) {
		return fail(400, "INVALID_EXPIRY", "Announcement expiry must be in the future")
	}
	if !clear {
		for _, feature := range []string{"groups", "messages"} {
			if e = a.requireFeature(r.Context(), feature); e != nil {
				return e
			}
		}
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var actualOwner, actualState, kind string
	var generation, roleGeneration int64
	e = tx.QueryRow(r.Context(), `SELECT c.owner_id::text,c.state,c.kind,m.membership_generation,m.role_generation FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id WHERE c.id=$1 AND m.user_id=$2 AND m.state='active' AND m.export_pending_operation_id IS NULL FOR UPDATE OF c`, r.PathValue("id"), u.ID).Scan(&actualOwner, &actualState, &kind, &generation, &roleGeneration)
	if e != nil || kind != "group" {
		return fail(403, "ANNOUNCEMENT_FORBIDDEN", "Current active group membership required")
	}
	if str(roleGeneration) != q.RoleGeneration {
		return fail(409, "QUALIFICATION_CHANGED", "Original role generation changed; keep the original request frozen")
	}
	if actualState != "active" {
		return fail(409, "GROUP_OWNER_UNAVAILABLE", "Group is read-only")
	}
	if actualOwner != u.ID {
		return fail(403, "ANNOUNCEMENT_FORBIDDEN", "Only the current group owner may change the announcement")
	}
	if str(generation) != q.MembershipGeneration {
		return fail(409, "MEMBERSHIP_GENERATION_CHANGED", "Original membership generation changed")
	}
	current, e := scanAnnouncement(tx.QueryRow(r.Context(), `SELECT `+announcementColumns+` FROM conversation.conversations WHERE id=$1`, r.PathValue("id")))
	if e != nil {
		return e
	}
	if current.Version != q.ExpectedVersion {
		return fail(409, "ANNOUNCEMENT_VERSION_CONFLICT", "Reload the current announcement before a new operation")
	}
	if !clear {
		var result struct {
			Available bool `json:"available"`
		}
		if e = a.rpc(r.Context(), "message", "/internal/announcement-source", map[string]any{"userId": u.ID, "conversationId": r.PathValue("id"), "messageId": q.MessageID, "sourceVersion": q.SourceVersion, "joinedAt": joined}, &result); e != nil {
			return e
		}
		if !result.Available {
			return fail(409, "ANNOUNCEMENT_SOURCE_CHANGED", "The referenced source is no longer current and visible")
		}
	}
	_, e = tx.Exec(r.Context(), `UPDATE conversation.conversations SET announcement_message_id=NULLIF($2,'')::uuid,announcement_source_version=NULLIF($3,'')::bigint,announcement_expires_at=$4,announcement_version=announcement_version+1,announcement_updated_at=now() WHERE id=$1`, r.PathValue("id"), q.MessageID, q.SourceVersion, q.ExpiresAt)
	if e != nil {
		return e
	}
	result, e := scanAnnouncement(tx.QueryRow(r.Context(), `SELECT `+announcementColumns+` FROM conversation.conversations WHERE id=$1`, r.PathValue("id")))
	if e != nil {
		return e
	}
	if !clear {
		result.State = "active"
	}
	result.CanManage = true
	stored, _ := json.Marshal(result)
	_, e = tx.Exec(r.Context(), `INSERT INTO conversation.announcement_operations(actor_id,request_id,conversation_id,restore_epoch,request_digest,result) VALUES($1,$2,$3,$4::bigint,$5,$6)`, u.ID, q.RequestID, r.PathValue("id"), u.RestoreEpoch, digest, stored)
	if e != nil {
		return fail(409, "IDEMPOTENCY_CONFLICT", "Retry the original announcement operation")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, announcementOperation{OperationID: q.RequestID, Status: "applied", Result: result, Current: true})
	return nil
}

// A frozen local intent must never use the creating send route as a lookup.
func (a *App) messageResult(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q sendPayload
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.ClientMsgID) < 8 || len(q.ClientMsgID) > 128 {
		return fail(400, "INVALID_MESSAGE_ID", "Original clientMsgId required")
	}
	if _, e = a.member(r.Context(), u.ID, r.PathValue("id")); e != nil {
		return e
	}
	var id, conversation string
	var original []byte
	e = a.DB.QueryRow(r.Context(), `SELECT id::text,conversation_id::text,payload_digest FROM message.messages WHERE sender_id=$1 AND client_msg_id=$2`, u.ID, q.ClientMsgID).Scan(&id, &conversation, &original)
	if e == pgx.ErrNoRows {
		return fail(404, "MESSAGE_RESULT_NOT_FOUND", "No committed original message; this lookup never sends it")
	}
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(map[string]any{"conversationId": r.PathValue("id"), "payload": q})
	if conversation != r.PathValue("id") || !bytes.Equal(original, tokenDigest(string(raw))) {
		return fail(409, "IDEMPOTENCY_CONFLICT", "Original message payload differs")
	}
	m, e := a.loadMessage(r.Context(), id, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, m)
	return nil
}
