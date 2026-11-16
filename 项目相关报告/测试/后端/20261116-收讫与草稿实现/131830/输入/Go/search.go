package jobsim

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func init() {
	migrations["message"] += `CREATE INDEX IF NOT EXISTS favorite_owner ON message.favorites(user_id,message_id);`
}

type readableScope struct {
	ID    string    `json:"id"`
	Floor time.Time `json:"floor"`
}
type messageListCursor struct {
	Owner   string    `json:"owner"`
	Epoch   string    `json:"epoch"`
	Filter  string    `json:"filter"`
	Created time.Time `json:"created"`
	ID      string    `json:"id"`
}

func (a *App) searchRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/favorites", func(w http.ResponseWriter, r *http.Request) error { return a.listReadableMessages(w, r, true) })
	h("GET /api/messages/{id}", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !validObjectID(r.PathValue("id")) {
			return fail(400, "INVALID_MESSAGE_ID", "Stable message ID required")
		}
		message, e := a.loadMessage(r.Context(), r.PathValue("id"), u.ID)
		if e != nil {
			return e
		}
		respond(w, 200, message)
		return nil
	})
}
func (a *App) searchScopeRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/conversations/{id}", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !validObjectID(r.PathValue("id")) {
			return fail(400, "INVALID_CONVERSATION_ID", "Stable conversation ID required")
		}
		if _, e = a.member(r.Context(), u.ID, r.PathValue("id")); e != nil {
			return e
		}
		conversation, e := a.conversationByID(r, r.PathValue("id"), u.ID)
		if e != nil {
			return e
		}
		respond(w, 200, conversation)
		return nil
	})
	h("POST /internal/read-scopes", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if !validObjectID(q.UserID) {
			return fail(400, "INVALID_PRINCIPAL", "Current source principal required")
		}
		rows, e := a.DB.Query(r.Context(), `SELECT c.id::text,c.kind,m.joined_at FROM conversation.members m JOIN conversation.conversations c ON c.id=m.conversation_id WHERE m.user_id=$1 AND m.state='active' AND m.export_pending_operation_id IS NULL AND c.state IN('active','owner_unavailable') ORDER BY c.id`, q.UserID)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []readableScope{}
		for rows.Next() {
			var scope readableScope
			var kind string
			if e = rows.Scan(&scope.ID, &kind, &scope.Floor); e != nil {
				return e
			}
			if kind != "group" {
				scope.Floor = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			items = append(items, scope)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		respond(w, 200, map[string]any{"items": items})
		return nil
	}))
}
func (a *App) encodeMessageCursor(v messageListCursor) string {
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "message-list-cursor-v1", raw))
}
func (a *App) decodeMessageCursor(raw string, u User, filter string) (messageListCursor, error) {
	var v messageListCursor
	if raw == "" || raw == "0" {
		return v, nil
	}
	if len(raw) > 2048 {
		return v, fail(400, "INVALID_CURSOR", "Invalid message list cursor")
	}
	encoded, signature, ok := strings.Cut(raw, ".")
	if !ok {
		return v, fail(400, "INVALID_CURSOR", "Invalid message list cursor")
	}
	body, e := base64.RawURLEncoding.DecodeString(encoded)
	if e != nil {
		return v, fail(400, "INVALID_CURSOR", "Invalid message list cursor")
	}
	signed, e := base64.RawURLEncoding.DecodeString(signature)
	if e != nil || !hmac.Equal(signed, keyedDigest(a.Config.Key, "message-list-cursor-v1", body)) || json.Unmarshal(body, &v) != nil || v.Owner != u.ID || v.Filter != filter || !validObjectID(v.ID) || v.Created.IsZero() {
		return v, fail(400, "INVALID_CURSOR", "Cursor belongs to different owner or query")
	}
	if v.Epoch != u.RestoreEpoch {
		return v, fail(409, "RESTORE_EPOCH_CHANGED", "Message list cursor belongs to an older restore epoch")
	}
	return v, nil
}
func (a *App) search(w http.ResponseWriter, r *http.Request) error {
	return a.listReadableMessages(w, r, false)
}
func (a *App) listReadableMessages(w http.ResponseWriter, r *http.Request, favorites bool) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	feature := "search"
	if favorites {
		feature = "favorites"
	}
	if e = a.requireFeature(r.Context(), feature); e != nil {
		return e
	}
	q := r.URL.Query().Get("q")
	if !favorites && (len(q) < 1 || len(q) > 256 || !utf8.ValidString(q)) {
		return fail(400, "INVALID_QUERY", "Search query 1–256 bytes")
	}
	if favorites {
		q = ""
	}
	conversation := r.URL.Query().Get("conversationId")
	if conversation != "" && !validObjectID(conversation) {
		return fail(400, "INVALID_CONVERSATION_ID", "Stable conversation ID required")
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, e = strconv.Atoi(value)
		if e != nil || limit < 1 || limit > 100 {
			return fail(400, "INVALID_LIMIT", "Limit must be 1–100")
		}
	}
	filter := base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "message-list-filter-v1", []byte(strconv.FormatBool(favorites)+"\x00"+q+"\x00"+conversation)))
	position, e := a.decodeMessageCursor(r.URL.Query().Get("cursor"), u, filter)
	if e != nil {
		return e
	}
	var scopes struct {
		Items []readableScope `json:"items"`
	}
	if e = a.rpc(r.Context(), "conversation", "/internal/read-scopes", map[string]string{"userId": u.ID}, &scopes); e != nil {
		return e
	}
	selected := []readableScope{}
	for _, scope := range scopes.Items {
		if conversation == "" || scope.ID == conversation {
			selected = append(selected, scope)
		}
	}
	scopeJSON, _ := json.Marshal(selected)
	items := []Message{}
	if len(selected) > 0 {
		for len(items) <= limit {
			var before any
			var beforeID any
			if position.ID != "" {
				before = position.Created
				beforeID = position.ID
			}
			rows, e := a.DB.Query(r.Context(), `WITH readable AS(SELECT * FROM jsonb_to_recordset($1::jsonb) AS s(id uuid,floor timestamptz)) SELECT m.id::text,m.created_at FROM message.messages m JOIN readable s ON s.id=m.conversation_id WHERE NOT m.deleted AND m.created_at>=s.floor AND($2='' OR m.type='forward' OR strpos(lower(m.text),lower($2))>0) AND(NOT $3 OR EXISTS(SELECT 1 FROM message.favorites f WHERE f.user_id=$4 AND f.message_id=m.id)) AND($5::timestamptz IS NULL OR(m.created_at,m.id)<($5::timestamptz,$6::uuid)) ORDER BY m.created_at DESC,m.id DESC LIMIT $7`, scopeJSON, q, favorites, u.ID, before, beforeID, limit+1)
			if e != nil {
				return e
			}
			candidates := []messageListCursor{}
			for rows.Next() {
				v := messageListCursor{Owner: u.ID, Epoch: u.RestoreEpoch, Filter: filter}
				if e = rows.Scan(&v.ID, &v.Created); e != nil {
					rows.Close()
					return e
				}
				candidates = append(candidates, v)
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
			if len(candidates) == 0 {
				break
			}
			for _, candidate := range candidates {
				position = candidate
				message, e := a.loadMessage(r.Context(), candidate.ID, u.ID)
				if e != nil {
					var denied *apiError
					if errors.As(e, &denied) && (denied.Status == 403 || denied.Status == 404) {
						continue
					}
					return e
				}
				if message.Forwarded && message.forwardUncertain {
					return fail(503, "FORWARD_QUALIFICATION_UNAVAILABLE", "Search source qualification cannot be proved")
				}
				if message.Deleted || message.ContentUnavailable || message.ForwardUnavailable || favorites && !message.Favorite || (!favorites && !strings.Contains(strings.ToLower(message.Text), strings.ToLower(q))) {
					continue
				}
				items = append(items, message)
				if len(items) > limit {
					break
				}
			}
			if len(candidates) < limit+1 {
				break
			}
		}
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	next := "0"
	if len(items) > 0 {
		last := items[len(items)-1]
		next = a.encodeMessageCursor(messageListCursor{Owner: u.ID, Epoch: u.RestoreEpoch, Filter: filter, Created: last.CreatedAt, ID: last.ID})
	} else if r.URL.Query().Get("cursor") != "" {
		next = r.URL.Query().Get("cursor")
	}
	respond(w, 200, map[string]any{"items": items, "cursor": next, "hasMore": hasMore})
	return nil
}
