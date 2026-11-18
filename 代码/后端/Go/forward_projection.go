package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["message"] += `
CREATE TABLE IF NOT EXISTS message.forward_refs(message_id uuid PRIMARY KEY REFERENCES message.messages(id),source_message_id uuid NOT NULL,source_conversation_id uuid NOT NULL,source_content_version bigint NOT NULL,forwarder_id uuid NOT NULL,source_membership_generation bigint NOT NULL,security_version bigint NOT NULL,restore_epoch bigint NOT NULL,depth integer NOT NULL CHECK(depth BETWEEN 1 AND 8),state text NOT NULL DEFAULT 'active' CHECK(state IN('active','invalidating','unavailable')),created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS forward_source ON message.forward_refs(source_message_id);
CREATE INDEX IF NOT EXISTS forward_principal ON message.forward_refs(forwarder_id,source_conversation_id,source_membership_generation);`
}

type forwardPrincipal struct {
	SecurityVersion string `json:"securityVersion"`
	RestoreEpoch    string `json:"restoreEpoch"`
}

type forwardReference struct {
	MessageID            string
	SourceMessageID      string
	SourceConversationID string
	SourceContentVersion string
	ForwarderID          string
	MembershipGeneration string
	SecurityVersion      string
	RestoreEpoch         string
	Depth                int
	State                string
}

func (a *App) forwardIdentityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/forwards/principal", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		epoch, e := a.currentIdentityEpoch(r.Context())
		if e != nil {
			return e
		}
		var security int64
		if e = a.DB.QueryRow(r.Context(), `SELECT security_version FROM auth.users WHERE id=$1 AND state='active' AND recovery_state='normal' AND pending_operation_id IS NULL`, q.UserID).Scan(&security); e != nil {
			if e == pgx.ErrNoRows {
				return fail(403, "FORWARD_PRINCIPAL_INVALID", "Original source principal is not currently eligible")
			}
			return e
		}
		// A sent reference survives ordinary logout or revocation of one device.
		respond(w, 200, forwardPrincipal{str(security), str(epoch)})
		return nil
	}))
}

func (a *App) readForwardPrincipal(ctx context.Context, user string) (forwardPrincipal, error) {
	var p forwardPrincipal
	e := a.rpc(ctx, "identity", "/internal/forwards/principal", map[string]string{"userId": user}, &p)
	return p, e
}

func (a *App) forwardAdmission(user string, m Message, membership string, p forwardPrincipal) string {
	material := strings.Join([]string{user, m.ID, m.ContentVersion, m.ConversationID, membership, p.SecurityVersion, p.RestoreEpoch}, "\x00")
	return "fa1." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "forward-admission-v1", []byte(material)))
}

func (a *App) replyAdmission(user string, m Message, membership string, p forwardPrincipal) string {
	material := strings.Join([]string{user, m.ID, m.ContentVersion, m.ConversationID, membership, p.SecurityVersion, p.RestoreEpoch}, "\x00")
	return "dr1." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "draft-reply-admission-v1", []byte(material)))
}

func forwardUncertain(e error) bool {
	var api *apiError
	return !errors.As(e, &api) || api.Status >= 500
}

func unavailableForward(m *Message, uncertain bool) {
	m.Forwarded, m.ForwardUnavailable = true, true
	m.Text, m.AttachmentID, m.AttachmentURL, m.ForwardAdmission = "", "", "", ""
	m.ReplyAdmission = ""
	m.forwardUncertain = uncertain
	m.forwardLeafMessage, m.forwardLeafAttachment = "", ""
}

func (a *App) rawForwardSource(ctx context.Context, id, user string) (Message, map[string]any, error) {
	m, e := scanMessage(a.DB.QueryRow(ctx, `SELECT `+messageColumns+` FROM message.messages WHERE id=$1`, id))
	if e == pgx.ErrNoRows {
		return m, nil, fail(404, "MESSAGE_NOT_FOUND", "Forward source unavailable")
	}
	if e != nil {
		return m, nil, e
	}
	if e = a.retentionReadState(ctx, &m); e != nil {
		return m, nil, e
	}
	meta, e := a.member(ctx, user, m.ConversationID)
	if e != nil {
		return m, nil, e
	}
	if meta["kind"] == "group" {
		joined, parseError := time.Parse(time.RFC3339Nano, stringValue(meta, "joinedAt"))
		if parseError != nil || m.CreatedAt.Before(joined) {
			return m, nil, fail(403, "HISTORY_NOT_ALLOWED", "Source predates current membership")
		}
	}
	return m, meta, nil
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

// Caller holds the shared-host read/send fence (or the content writer EX fence).
// Inner source reads do not reacquire it and never grant the target source ACL.
func (a *App) projectForward(ctx context.Context, m *Message, depth int, visited map[string]bool) {
	if m.Type != "forward" {
		return
	}
	m.Forwarded = true
	if m.Deleted || m.ContentUnavailable || m.sourcePending || depth >= 8 {
		unavailableForward(m, m.sourcePending)
		return
	}
	var ref forwardReference
	var content, membership, security, epoch int64
	e := a.DB.QueryRow(ctx, `SELECT source_message_id::text,source_conversation_id::text,source_content_version,forwarder_id::text,source_membership_generation,security_version,restore_epoch,depth,state FROM message.forward_refs WHERE message_id=$1`, m.ID).Scan(&ref.SourceMessageID, &ref.SourceConversationID, &content, &ref.ForwarderID, &membership, &security, &epoch, &ref.Depth, &ref.State)
	if e != nil {
		unavailableForward(m, e != pgx.ErrNoRows)
		return
	}
	if ref.State != "active" || visited[ref.SourceMessageID] {
		unavailableForward(m, false)
		return
	}
	principal, e := a.readForwardPrincipal(ctx, ref.ForwarderID)
	if e != nil {
		unavailableForward(m, forwardUncertain(e))
		return
	}
	if principal.SecurityVersion != str(security) || principal.RestoreEpoch != str(epoch) {
		unavailableForward(m, false)
		return
	}
	source, meta, e := a.rawForwardSource(ctx, ref.SourceMessageID, ref.ForwarderID)
	if e != nil {
		unavailableForward(m, forwardUncertain(e))
		return
	}
	if source.ConversationID != ref.SourceConversationID || source.ContentVersion != str(content) || stringValue(meta, "membershipGeneration") != str(membership) || source.Deleted || source.ContentUnavailable || source.sourcePending {
		unavailableForward(m, false)
		return
	}
	visited[source.ID] = true
	a.projectForward(ctx, &source, depth+1, visited)
	delete(visited, source.ID)
	if source.ForwardUnavailable || source.Type == "forward" || ref.Depth != source.forwardDepth+1 {
		unavailableForward(m, source.forwardUncertain)
		return
	}
	switch source.Type {
	case "text", "image", "audio", "video", "file":
	default:
		unavailableForward(m, false)
		return
	}
	m.Type, m.Text = source.Type, source.Text
	m.forwardDepth = ref.Depth
	m.AttachmentID = ""
	if source.Type != "text" {
		leafMessage, leafAttachment := source.ID, source.AttachmentID
		if source.Forwarded {
			leafMessage, leafAttachment = source.forwardLeafMessage, source.forwardLeafAttachment
		}
		if leafAttachment == "" {
			unavailableForward(m, false)
			return
		}
		m.forwardLeafMessage, m.forwardLeafAttachment = leafMessage, leafAttachment
		m.AttachmentURL = "/api/messages/" + m.ID + "/attachment"
	}
}

func (a *App) projectMessage(ctx context.Context, m *Message, user string, meta map[string]any) {
	if err := a.retentionReadState(ctx, m); err != nil {
		m.ContentUnavailable = true
		m.Text = ""
		m.AttachmentID = ""
	}
	visited := map[string]bool{m.ID: true}
	a.projectForward(ctx, m, 0, visited)
	if m.Deleted || m.ContentUnavailable || m.ForwardUnavailable || m.sourcePending {
		m.forwardUncertain = m.forwardUncertain || m.sourcePending
		return
	}
	if meta == nil {
		var e error
		meta, e = a.member(ctx, user, m.ConversationID)
		if e != nil {
			m.forwardUncertain = forwardUncertain(e)
			return
		}
	}
	membership := stringValue(meta, "membershipGeneration")
	if _, ok := canonicalCounter(membership, false); !ok {
		return
	}
	principal, e := a.readForwardPrincipal(ctx, user)
	if e != nil {
		m.forwardUncertain = forwardUncertain(e)
		return
	}
	m.ForwardAdmission = a.forwardAdmission(user, *m, membership, principal)
	m.ReplyAdmission = a.replyAdmission(user, *m, membership, principal)
	m.forwardMembership, m.forwardSecurity, m.forwardEpoch = membership, principal.SecurityVersion, principal.RestoreEpoch
}

func (a *App) prepareForward(ctx context.Context, user, sourceID, sourceVersion, proof string) (*forwardReference, Message, error) {
	var zero Message
	if !validObjectID(sourceID) {
		return nil, zero, fail(400, "INVALID_FORWARD", "Original source identity required")
	}
	if _, ok := canonicalCounter(sourceVersion, false); !ok {
		return nil, zero, fail(400, "INVALID_FORWARD", "Original source content version required")
	}
	source, e := a.loadMessage(ctx, sourceID, user)
	if e != nil {
		return nil, zero, e
	}
	if source.forwardUncertain {
		return nil, zero, fail(503, "FORWARD_QUALIFICATION_UNAVAILABLE", "Current source qualification cannot be proved; keep the original intent")
	}
	if source.Deleted || source.ContentUnavailable || source.ForwardUnavailable || source.sourcePending {
		return nil, zero, fail(409, "FORWARD_SOURCE_UNAVAILABLE", "Original source no longer available")
	}
	if source.ContentVersion != sourceVersion {
		return nil, zero, fail(409, "FORWARD_SOURCE_CHANGED", "Original source content version changed")
	}
	if proof == "" || source.ForwardAdmission == "" || !hmac.Equal([]byte(proof), []byte(source.ForwardAdmission)) {
		return nil, zero, fail(409, "FORWARD_ADMISSION_CHANGED", "Original source admission missing or no longer current; never patch the original intent")
	}
	if source.forwardDepth >= 8 {
		return nil, zero, fail(400, "FORWARD_DEPTH_LIMIT", "Forward chain limit is eight")
	}
	ref := &forwardReference{SourceMessageID: source.ID, SourceConversationID: source.ConversationID, SourceContentVersion: sourceVersion, ForwarderID: user, MembershipGeneration: source.forwardMembership, SecurityVersion: source.forwardSecurity, RestoreEpoch: source.forwardEpoch, Depth: source.forwardDepth + 1, State: "active"}
	return ref, source, nil
}
