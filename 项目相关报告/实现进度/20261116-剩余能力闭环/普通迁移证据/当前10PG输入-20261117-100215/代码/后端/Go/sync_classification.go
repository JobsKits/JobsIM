package jobsim

import (
	"context"
	"net/http"
	"time"
)

// Historical membership permits only an already-known global tombstone. It
// never grants current source bodies, media, or a new conversation admission.
func (a *App) syncHistoricalAccessRoute(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	if a.Role != "conversation" {
		return
	}
	h("POST /internal/sync/historical-scopes", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "message" {
			return fail(403, "SYNC_SERVICE_REQUIRED", "Private classification owner protocol required")
		}
		var q struct {
			UserID string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if !validObjectID(q.UserID) {
			return fail(400, "INVALID_PRINCIPAL", "Current principal required")
		}
		rows, e := a.DB.Query(r.Context(), `SELECT conversation_id::text,joined_at,left_at FROM conversation.membership_periods WHERE user_id=$1 ORDER BY conversation_id,generation`, q.UserID)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []syncMembershipPeriod{}
		for rows.Next() {
			var period syncMembershipPeriod
			if e = rows.Scan(&period.ID, &period.JoinedAt, &period.LeftAt); e != nil {
				return e
			}
			items = append(items, period)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		respond(w, 200, map[string]any{"items": items})
		return nil
	}))
	h("POST /internal/sync/historical-access", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "message" {
			return fail(403, "SYNC_SERVICE_REQUIRED", "Private classification owner protocol required")
		}
		var q struct {
			UserID         string    `json:"userId"`
			ConversationID string    `json:"conversationId"`
			CreatedAt      time.Time `json:"createdAt"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if !validObjectID(q.UserID) || !validObjectID(q.ConversationID) || q.CreatedAt.IsZero() {
			return fail(400, "INVALID_CLASSIFICATION_REQUEST", "Original message creation facts required")
		}
		var allowed bool
		e := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.membership_periods WHERE user_id=$1 AND conversation_id=$2 AND joined_at<=$3 AND (left_at IS NULL OR left_at>$3))`, q.UserID, q.ConversationID, q.CreatedAt).Scan(&allowed)
		if e != nil {
			return e
		}
		respond(w, 200, map[string]bool{"historicallyReadable": allowed})
		return nil
	}))
}

type syncMembershipPeriod struct {
	ID       string     `json:"id"`
	JoinedAt time.Time  `json:"joinedAt"`
	LeftAt   *time.Time `json:"leftAt"`
}

func (a *App) syncHistoricalGlobal(ctx context.Context, user, id string) (any, bool, error) {
	var conv, version, contentVersion, creationEpoch, creationSequence string
	var created time.Time
	var deleted bool
	e := a.DB.QueryRow(ctx, `SELECT conversation_id::text,version::text,content_version::text,created_at,deleted,COALESCE(sequence_epoch::text,'unknown'),COALESCE(creation_sequence::text,'unknown') FROM message.messages WHERE id=$1`, id).Scan(&conv, &version, &contentVersion, &created, &deleted, &creationEpoch, &creationSequence)
	if e != nil || !deleted {
		return nil, false, nil
	}
	var result struct {
		Allowed bool `json:"historicallyReadable"`
	}
	if e = a.rpc(ctx, "conversation", "/internal/sync/historical-access", map[string]any{"userId": user, "conversationId": conv, "createdAt": created}, &result); e != nil {
		return nil, false, e
	}
	if !result.Allowed {
		return nil, false, nil
	}
	return map[string]any{"id": id, "deleted": true, "text": "", "version": version, "contentVersion": contentVersion, "creationEpoch": creationEpoch, "creationSequence": creationSequence}, true, nil
}
