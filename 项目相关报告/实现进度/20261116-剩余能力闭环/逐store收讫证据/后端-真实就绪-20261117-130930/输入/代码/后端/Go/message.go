package jobsim

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const messageColumns = `id::text,conversation_id::text,sender_id::text,client_msg_id,type,text,COALESCE(reply_to_id::text,''),COALESCE(attachment_id::text,''),created_at,updated_at,version,sequence,deleted,content_unavailable,content_version,export_pending_operation_id IS NOT NULL,COALESCE(sequence_epoch::text,'unknown'),COALESCE(creation_sequence::text,'unknown'),delete_scope,deletion_generation`

// Field order and omitted additions preserve the original committed digest.
type sendPayload struct {
	ClientMsgID          string   `json:"clientMsgId"`
	Type                 string   `json:"type"`
	Text                 string   `json:"text"`
	ReplyToID            string   `json:"replyToId"`
	AttachmentID         string   `json:"attachmentId"`
	ForwardOfID          string   `json:"forwardOfId"`
	RestoreEpoch         string   `json:"restoreEpoch,omitempty"`
	SendAdmission        string   `json:"sendAdmission,omitempty"`
	MentionedUserIDs     []string `json:"mentionedUserIds,omitempty"`
	MentionAll           bool     `json:"mentionAll,omitempty"`
	ForwardSourceVersion string   `json:"forwardSourceVersion,omitempty"`
	ForwardAdmission     string   `json:"forwardAdmission,omitempty"`
}

func scanMessage(row pgx.Row) (Message, error) {
	var m Message
	var version, sequence, contentVersion, deletion int64
	e := row.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.ClientMsgID, &m.Type, &m.Text, &m.ReplyToID, &m.AttachmentID, &m.CreatedAt, &m.UpdatedAt, &version, &sequence, &m.Deleted, &m.ContentUnavailable, &contentVersion, &m.sourcePending, &m.CreationEpoch, &m.CreationSequence, &m.DeleteScope, &deletion)
	if deletion > 0 {
		m.DeletionGeneration = str(deletion)
	}
	m.Version = str(version)
	m.ContentVersion = str(contentVersion)
	m.Sequence = str(sequence)
	m.Reactions = []Reaction{}
	if m.Deleted || m.ContentUnavailable {
		m.Text = ""
		m.AttachmentID = ""
	}
	return m, e
}
func (a *App) enrich(ctx context.Context, m *Message, user string) error {
	rows, e := a.DB.Query(ctx, `SELECT user_id::text,emoji FROM message.reactions WHERE message_id=$1 ORDER BY emoji,user_id`, m.ID)
	if e != nil {
		return e
	}
	for rows.Next() {
		var r Reaction
		if e = rows.Scan(&r.UserID, &r.Emoji); e != nil {
			rows.Close()
			return e
		}
		m.Reactions = append(m.Reactions, r)
	}
	rows.Close()
	if e = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message.pins WHERE message_id=$1),EXISTS(SELECT 1 FROM message.favorites WHERE message_id=$1 AND user_id=$2)`, m.ID, user).Scan(&m.Pinned, &m.Favorite); e != nil {
		return e
	}
	return a.enrichMentions(ctx, m, user)
}
func (a *App) loadMessage(ctx context.Context, id, user string) (Message, error) {
	m, e := scanMessage(a.DB.QueryRow(ctx, `SELECT `+messageColumns+` FROM message.messages WHERE id=$1`, id))
	if e != nil {
		return m, fail(404, "MESSAGE_NOT_FOUND", "Message unavailable")
	}
	meta, e := a.member(ctx, user, m.ConversationID)
	if e != nil {
		return m, e
	}
	if meta["kind"] == "group" {
		if raw, ok := meta["joinedAt"].(string); ok {
			joined, _ := time.Parse(time.RFC3339Nano, raw)
			if m.CreatedAt.Before(joined) {
				return m, fail(403, "HISTORY_NOT_ALLOWED", "Message predates current membership")
			}
		}
	}
	if e = a.retentionReadState(ctx, &m); e != nil {
		return m, e
	}
	e = a.enrich(ctx, &m, user)
	if e == nil {
		a.projectMessage(ctx, &m, user, meta)
	}
	return m, e
}
func (a *App) messageRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.admissionRoutes(h, internal)
	a.reportMessageRoutes(h, internal)
	a.exportMessageRoutes(h, internal)
	a.announcementMessageRoutes(h, internal)
	a.forwardRevocationRoutes(h, internal)
	a.forwardAttachmentRoutes(h, internal)
	h("GET /api/conversations/{id}/messages", a.history)
	h("POST /api/conversations/{id}/messages", a.sendMessage)
	h("POST /api/conversations/{id}/message-results", a.messageResult)
	h("PATCH /api/messages/{id}", a.editMessage)
	h("DELETE /api/messages/{id}", a.deleteMessage)
	h("GET /api/search", a.search)
	h("POST /api/messages/{id}/reactions", func(w http.ResponseWriter, r *http.Request) error { return a.react(w, r, true) })
	h("DELETE /api/messages/{id}/reactions/{emoji}", func(w http.ResponseWriter, r *http.Request) error { return a.react(w, r, false) })
	for _, kind := range []string{"pin", "favorite"} {
		k := kind
		h("PUT /api/messages/{id}/"+k, func(w http.ResponseWriter, r *http.Request) error { return a.markMessage(w, r, k, true) })
		h("DELETE /api/messages/{id}/"+k, func(w http.ResponseWriter, r *http.Request) error { return a.markMessage(w, r, k, false) })
	}
	h("POST /internal/attachment-confirm", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			MessageID string `json:"messageId"`
			ObjectID  string `json:"objectId"`
			UserID    string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		m, e := a.loadMessage(r.Context(), q.MessageID, q.UserID)
		if e != nil {
			return e
		}
		if m.Deleted || m.AttachmentID != q.ObjectID {
			return fail(403, "ATTACHMENT_UNAVAILABLE", "Attachment unavailable")
		}
		respond(w, 200, map[string]bool{"committed": true})
		return nil
	}))
}
func (a *App) history(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	meta, e := a.member(r.Context(), u.ID, r.PathValue("id"))
	if e != nil {
		return e
	}
	after := int64(0)
	if v := r.URL.Query().Get("after"); v != "" {
		parts := strings.Split(v, ":")
		if len(parts) == 2 {
			if parts[0] != u.RestoreEpoch {
				return fail(409, "RESTORE_EPOCH_CHANGED", "Reload the current history baseline")
			}
			v = parts[1]
		} else if v != "0" {
			return fail(409, "RESTORE_EPOCH_CHANGED", "Cursor must include the current restore epoch")
		}
		after, e = strconv.ParseInt(v, 10, 64)
		if e != nil || after < 0 {
			return fail(400, "INVALID_CURSOR", "Invalid cursor")
		}
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
		if limit < 1 || limit > 200 {
			return fail(400, "INVALID_LIMIT", "Limit 1–200")
		}
	}
	rows, e := a.DB.Query(r.Context(), `SELECT sequence,message_id::text FROM message.events WHERE conversation_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`, r.PathValue("id"), after, limit)
	if e != nil {
		return e
	}
	type ev struct {
		Seq int64
		ID  string
	}
	events := []ev{}
	for rows.Next() {
		var v ev
		if e = rows.Scan(&v.Seq, &v.ID); e != nil {
			rows.Close()
			return e
		}
		events = append(events, v)
	}
	rows.Close()
	items := []Message{}
	seen := map[string]bool{}
	cursor := after
	for _, v := range events {
		cursor = v.Seq
		if seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		m, e := scanMessage(a.DB.QueryRow(r.Context(), `SELECT `+messageColumns+` FROM message.messages WHERE id=$1`, v.ID))
		if e != nil {
			return e
		}
		if meta["kind"] == "group" {
			raw, _ := meta["joinedAt"].(string)
			joined, _ := time.Parse(time.RFC3339Nano, raw)
			if m.CreatedAt.Before(joined) {
				continue
			}
		}
		if e = a.retentionReadState(r.Context(), &m); e != nil {
			return e
		}
		if e = a.enrich(r.Context(), &m, u.ID); e != nil {
			return e
		}
		a.projectMessage(r.Context(), &m, u.ID, meta)
		items = append(items, m)
	}
	respond(w, 200, map[string]any{"items": items, "cursor": u.RestoreEpoch + ":" + str(cursor)})
	return nil
}
func nextSequence(ctx context.Context, tx pgx.Tx, conversation string) (int64, error) {
	var seq int64
	e := tx.QueryRow(ctx, `INSERT INTO message.heads(conversation_id,sequence) VALUES($1,1) ON CONFLICT(conversation_id) DO UPDATE SET sequence=message.heads.sequence+1 RETURNING sequence`, conversation).Scan(&seq)
	return seq, e
}
func (a *App) sendMessage(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q sendPayload
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.ClientMsgID) < 8 || len(q.ClientMsgID) > 128 {
		return fail(400, "INVALID_MESSAGE_ID", "A stable clientMsgId is required")
	}
	if !utf8.ValidString(q.Text) || len(q.Text) > 16384 {
		return fail(400, "TEXT_LIMIT", "Text limit is 16 KiB")
	}
	switch q.Type {
	case "text":
		if strings.TrimSpace(q.Text) == "" && q.ForwardOfID == "" {
			return fail(400, "EMPTY_MESSAGE", "Message is empty")
		}
	case "forward":
		if q.ForwardOfID == "" || q.ForwardSourceVersion == "" || q.Text != "" || q.AttachmentID != "" || q.ReplyToID != "" || q.MentionAll || len(q.MentionedUserIDs) > 0 {
			return fail(400, "INVALID_FORWARD", "A forward requires only original source reference and admissions")
		}
	case "image", "audio", "video", "file":
		if q.AttachmentID == "" {
			return fail(400, "ATTACHMENT_REQUIRED", "Attachment required")
		}
	default:
		return fail(400, "UNSUPPORTED_TYPE", "Unsupported message type")
	}
	var meta map[string]any
	if e = a.rpc(r.Context(), "conversation", "/internal/member", map[string]string{"userId": u.ID, "conversationId": r.PathValue("id")}, &meta); e != nil {
		return e
	}
	b, _ := json.Marshal(map[string]any{"conversationId": r.PathValue("id"), "payload": q})
	digest := tokenDigest(string(b))
	var priorDigest []byte
	var priorID string
	e = a.DB.QueryRow(r.Context(), `SELECT id::text,payload_digest FROM message.messages WHERE sender_id=$1 AND client_msg_id=$2`, u.ID, q.ClientMsgID).Scan(&priorID, &priorDigest)
	if e == nil {
		if !bytes.Equal(digest, priorDigest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Same clientMsgId has different content")
		}
		m, e := a.loadMessage(r.Context(), priorID, u.ID)
		if e != nil {
			return e
		}
		if e = a.protectRetentionDeadlines(r.Context(), priorID); e != nil {
			return e
		}
		respond(w, 200, m)
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	currentAdmission, _ := meta["sendAdmission"].(string)
	if !sameAdmission(q.SendAdmission, currentAdmission) {
		return fail(409, "SEND_ADMISSION_CHANGED", "Original send admission is absent or no longer current; keep this Outbox intent frozen")
	}
	if e = a.rpc(r.Context(), "conversation", "/internal/member", map[string]string{"userId": u.ID, "conversationId": r.PathValue("id"), "action": "send"}, nil); e != nil {
		return e
	}
	if e = a.requireFeature(r.Context(), "messages"); e != nil {
		return e
	}
	if q.Type != "text" && q.Type != "forward" {
		if e = a.requireFeature(r.Context(), "media"); e != nil {
			return e
		}
	}
	if q.RestoreEpoch != u.RestoreEpoch && !(u.RestoreEpoch == "1" && q.RestoreEpoch == "") {
		return fail(409, "RESTORE_EPOCH_CHANGED", "Original send epoch is not current; keep the old Outbox frozen")
	}
	var forward *forwardReference
	if q.Type == "forward" {
		if e = a.requireFeature(r.Context(), "forwarding"); e != nil {
			return e
		}
		var source Message
		forward, source, e = a.prepareForward(r.Context(), u.ID, q.ForwardOfID, q.ForwardSourceVersion, q.ForwardAdmission)
		if e != nil {
			return e
		}
		if source.Type != "text" {
			if e = a.requireFeature(r.Context(), "media"); e != nil {
				return e
			}
		}
	} else if q.ForwardOfID != "" || q.ForwardSourceVersion != "" || q.ForwardAdmission != "" {
		return fail(400, "FORWARD_PROTOCOL_REQUIRED", "Use the reference-only forward protocol")
	}
	if q.ReplyToID != "" {
		source, e := a.loadMessage(r.Context(), q.ReplyToID, u.ID)
		if e != nil {
			return e
		}
		if source.ConversationID != r.PathValue("id") || source.Deleted || source.ContentUnavailable || source.ForwardUnavailable {
			return fail(400, "INVALID_REPLY", "Reply target unavailable")
		}
	}
	mentions, e := a.prepareMentions(r.Context(), u.ID, r.PathValue("id"), q.MentionedUserIDs, q.MentionAll)
	if e != nil {
		return e
	}
	id := stableID(a.Config.Key, "message:"+u.ID+":"+q.ClientMsgID)
	retention, e := a.prepareMessageRetention(r.Context(), u, id, r.PathValue("id"))
	if e != nil {
		return e
	}
	if q.Type == "forward" && retention.Policy.Mode == "after_receipt" {
		return fail(409, "RETENTION_CONTENT_UNSUPPORTED", "Reference-only forwarding needs its independent source-holder receipt qualification")
	}
	if q.AttachmentID != "" {
		if e = a.rpc(r.Context(), "media", "/internal/bind", map[string]string{"objectId": q.AttachmentID, "userId": u.ID, "messageId": id, "conversationId": r.PathValue("id"), "type": q.Type}, nil); e != nil {
			return e
		}
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = a.enforceGroupRate(r.Context(), tx, meta, u.ID, r.PathValue("id")); e != nil {
		return e
	}
	seq, e := nextSequence(r.Context(), tx, r.PathValue("id"))
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,reply_to_id,attachment_id,sequence) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid,$10)`, id, r.PathValue("id"), u.ID, q.ClientMsgID, digest, q.Type, q.Text, q.ReplyToID, q.AttachmentID, seq)
	if e != nil {
		return fail(409, "SEND_RACE", "Retry with the original clientMsgId")
	}
	if forward != nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO message.forward_refs(message_id,source_message_id,source_conversation_id,source_content_version,forwarder_id,source_membership_generation,security_version,restore_epoch,depth) VALUES($1,$2,$3,$4::bigint,$5,$6::bigint,$7::bigint,$8::bigint,$9)`, id, forward.SourceMessageID, forward.SourceConversationID, forward.SourceContentVersion, forward.ForwarderID, forward.MembershipGeneration, forward.SecurityVersion, forward.RestoreEpoch, forward.Depth)
		if e != nil {
			return e
		}
	}
	if e = a.persistMentions(r.Context(), tx, id, u.ID, r.PathValue("id"), mentions, q.MentionAll); e != nil {
		return e
	}
	if e = a.persistMessageRetention(r.Context(), tx, id, retention); e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO message.events(conversation_id,sequence,message_id) VALUES($1,$2,$3)`, r.PathValue("id"), seq, id)
	if e != nil {
		return e
	}
	marker, e := a.markAdmissionCommit(r.Context(), tx, admissionMarker{SenderID: u.ID, ClientMsgID: q.ClientMsgID, ConversationID: r.PathValue("id"), Digest: base64.RawURLEncoding.EncodeToString(digest)})
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if e = clearAdmissionMarker(marker); e != nil {
		return fail(503, "ADMISSION_RECOVERY_REQUIRED", "Original send committed; reconcile the original ID after admission recovery")
	}
	if e = a.protectRetentionDeadlines(r.Context(), id); e != nil {
		return e
	}
	m, e := a.loadMessage(r.Context(), id, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, m)
	return nil
}
func (a *App) mutateMessage(r *http.Request, u User, m Message, operation string, update func(pgx.Tx, int64) error) error {
	version, e := strconv.ParseInt(m.Version, 10, 64)
	if e != nil {
		return e
	}
	if e = a.freezeMessageExportsAt(r.Context(), m.ID, operation, version); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	seq, e := nextSequence(r.Context(), tx, m.ConversationID)
	if e != nil {
		return e
	}
	if e = update(tx, seq); e != nil {
		return e
	}
	tag, e := tx.Exec(r.Context(), `UPDATE message.messages SET version=version+1,sequence=$2,updated_at=now(),export_pending_operation_id=NULL WHERE id=$1 AND version=$3 AND export_pending_operation_id=$4`, m.ID, seq, version, operation)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "VERSION_CONFLICT", "The original source operation no longer owns this message version")
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO message.events(conversation_id,sequence,message_id) VALUES($1,$2,$3)`, m.ConversationID, seq, m.ID)
	if e != nil {
		return e
	}
	return tx.Commit(r.Context())
}
func (a *App) editMessage(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	m, e := a.loadMessage(r.Context(), r.PathValue("id"), u.ID)
	if e != nil {
		return e
	}
	if m.SenderID != u.ID || m.Deleted || m.Type != "text" || m.Forwarded {
		return fail(403, "EDIT_FORBIDDEN", "Only your active text messages can be edited")
	}
	if e = a.requireConversationWrite(r.Context(), u.ID, m.ConversationID); e != nil {
		return e
	}
	var q struct {
		Text            string `json:"text"`
		ExpectedVersion string `json:"expectedVersion"`
		RequestID       string `json:"requestId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if strings.TrimSpace(q.Text) == "" || len(q.Text) > 16384 {
		return fail(400, "TEXT_LIMIT", "Invalid text length")
	}
	if q.ExpectedVersion != m.Version {
		return fail(409, "VERSION_CONFLICT", "Reload the current message")
	}
	retentionEdit, e := a.prepareRetentionEdit(r.Context(), m)
	if e != nil {
		return e
	}
	operation := a.messageMutationOperation(u.ID, m.ID, "edit", q)
	version, e := strconv.ParseInt(m.Version, 10, 64)
	if e != nil {
		return e
	}
	if e = a.freezeMessageContentExportsAt(r.Context(), m.ID, operation, version); e != nil {
		return e
	}
	payload := map[string]any{"messageId": m.ID, "version": str(version + 1)}
	if e = a.protect(r.Context(), operation, "intent", "message.content_floor", payload); e != nil {
		return e
	}
	e = a.mutateMessage(r, u, m, operation, func(tx pgx.Tx, seq int64) error {
		tag, e := tx.Exec(r.Context(), `UPDATE message.messages SET text=$2,content_unavailable=false,content_version=content_version+1 WHERE id=$1 AND version=$3 AND NOT deleted AND export_pending_operation_id=$4`, m.ID, q.Text, q.ExpectedVersion, operation)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fail(409, "VERSION_CONFLICT", "Concurrent modification")
		}
		return a.persistRetentionEdit(r.Context(), tx, m.ID, retentionEdit)
	})
	if e != nil {
		return e
	}
	if e = a.protect(r.Context(), operation, "result", "message.content_floor", payload); e != nil {
		return e
	}
	m, e = a.loadMessage(r.Context(), m.ID, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, m)
	return nil
}
func (a *App) deleteMessage(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	m, e := a.loadMessage(r.Context(), r.PathValue("id"), u.ID)
	if e != nil {
		return e
	}
	meta, e := a.member(r.Context(), u.ID, m.ConversationID)
	if e != nil {
		return e
	}
	if m.SenderID != u.ID && meta["ownerId"] != u.ID {
		return fail(403, "DELETE_FORBIDDEN", "Only author or group owner may retract")
	}
	if m.SenderID != u.ID && meta["readOnly"] == true {
		return fail(409, "GROUP_OWNER_UNAVAILABLE", "Only the author retains erasure rights while the group is restricted")
	}
	payload := map[string]any{"messageId": m.ID}
	operation := "retract:" + m.ID
	if !m.Deleted {
		version, e := strconv.ParseInt(m.Version, 10, 64)
		if e != nil {
			return e
		}
		if e = a.freezeMessageContentExportsAt(r.Context(), m.ID, operation, version); e != nil {
			return e
		}
	}
	if e = a.protect(r.Context(), operation, "intent", "message.retract", payload); e != nil {
		return e
	}
	if !m.Deleted {
		e = a.mutateMessage(r, u, m, operation, func(tx pgx.Tx, seq int64) error {
			_, e := tx.Exec(r.Context(), `UPDATE message.messages SET deleted=true,text='',content_unavailable=false,content_version=content_version+1,delete_scope='global',deletion_generation=deletion_generation+CASE WHEN delete_scope='global' THEN 0 ELSE 1 END WHERE id=$1 AND export_pending_operation_id=$2`, m.ID, operation)
			return e
		})
		if e != nil {
			return e
		}
	}
	if e = a.protect(r.Context(), operation, "result", "message.retract", payload); e != nil {
		return e
	}
	m, e = a.loadMessage(r.Context(), m.ID, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, m)
	return nil
}
func (a *App) react(w http.ResponseWriter, r *http.Request, add bool) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	m, e := a.loadMessage(r.Context(), r.PathValue("id"), u.ID)
	if e != nil {
		return e
	}
	if add && (m.ContentUnavailable || m.ForwardUnavailable) {
		return fail(409, "FORWARD_SOURCE_UNAVAILABLE", "Message source unavailable")
	}
	if m.Deleted {
		return fail(409, "MESSAGE_DELETED", "Message is deleted")
	}
	if e = a.requireConversationWrite(r.Context(), u.ID, m.ConversationID); e != nil {
		return e
	}
	emoji := r.PathValue("emoji")
	if add {
		if e = a.requireFeature(r.Context(), "reactions"); e != nil {
			return e
		}
		var q struct {
			Emoji string `json:"emoji"`
		}
		if e = decode(r, &q); e != nil {
			return e
		}
		emoji = q.Emoji
	}
	if emoji == "" || len(emoji) > 64 {
		return fail(400, "INVALID_REACTION", "Invalid reaction")
	}
	operation := a.messageMutationOperation(u.ID, m.ID, "reaction", map[string]any{"emoji": emoji, "add": add, "version": m.Version})
	e = a.mutateMessage(r, u, m, operation, func(tx pgx.Tx, seq int64) error {
		var e error
		if add {
			_, e = tx.Exec(r.Context(), `INSERT INTO message.reactions(message_id,user_id,emoji) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, m.ID, u.ID, emoji)
		} else {
			_, e = tx.Exec(r.Context(), `DELETE FROM message.reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3`, m.ID, u.ID, emoji)
		}
		return e
	})
	if e != nil {
		return e
	}
	m, e = a.loadMessage(r.Context(), m.ID, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, m)
	return nil
}
func (a *App) markMessage(w http.ResponseWriter, r *http.Request, kind string, add bool) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	m, e := a.loadMessage(r.Context(), r.PathValue("id"), u.ID)
	if e != nil {
		return e
	}
	if add && (m.ContentUnavailable || m.ForwardUnavailable) {
		return fail(409, "FORWARD_SOURCE_UNAVAILABLE", "Message source unavailable")
	}
	if m.Deleted {
		return fail(409, "MESSAGE_DELETED", "Message deleted")
	}
	if add {
		feature := "favorites"
		if kind == "pin" {
			feature = "pins"
		}
		if e = a.requireFeature(r.Context(), feature); e != nil {
			return e
		}
	}
	if kind == "pin" {
		if e = a.requireConversationWrite(r.Context(), u.ID, m.ConversationID); e != nil {
			return e
		}
		meta, e := a.member(r.Context(), u.ID, m.ConversationID)
		if e != nil {
			return e
		}
		if meta["kind"] == "group" && meta["ownerId"] != u.ID {
			return fail(403, "OWNER_REQUIRED", "Group owner required for shared pin")
		}
	}
	operation := a.messageMutationOperation(u.ID, m.ID, "mark", map[string]any{"kind": kind, "add": add, "version": m.Version})
	e = a.mutateMessage(r, u, m, operation, func(tx pgx.Tx, seq int64) error {
		var e error
		if kind == "pin" {
			if add {
				_, e = tx.Exec(r.Context(), `INSERT INTO message.pins VALUES($1,$2) ON CONFLICT DO NOTHING`, m.ID, u.ID)
			} else {
				_, e = tx.Exec(r.Context(), `DELETE FROM message.pins WHERE message_id=$1`, m.ID)
			}
		} else {
			if add {
				_, e = tx.Exec(r.Context(), `INSERT INTO message.favorites VALUES($1,$2) ON CONFLICT DO NOTHING`, m.ID, u.ID)
			} else {
				_, e = tx.Exec(r.Context(), `DELETE FROM message.favorites WHERE message_id=$1 AND user_id=$2`, m.ID, u.ID)
			}
		}
		return e
	})
	if e != nil {
		return e
	}
	m, e = a.loadMessage(r.Context(), m.ID, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, m)
	return nil
}
