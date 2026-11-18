package jobsim

import (
	"context"
	"net/http"
)

func (a *App) ownerLifecycleRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/owner-unavailable", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID      string `json:"userId"`
			OperationID string `json:"operationId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if !validObjectID(q.UserID) || len(q.OperationID) < 8 || len(q.OperationID) > 128 {
			return fail(400, "INVALID_OWNER_OPERATION", "Current owner and original operation required")
		}
		rows, e := a.DB.Query(r.Context(), `SELECT id::text FROM conversation.conversations WHERE kind='group' AND owner_id=$1 AND state='active' ORDER BY id`, q.UserID)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, id := range ids {
			payload := map[string]any{"conversationId": id, "userId": q.UserID}
			operation := "owner-unavailable:" + q.OperationID + ":" + id
			if e = a.protect(r.Context(), operation, "intent", "conversation.owner_unavailable", payload); e != nil {
				return e
			}
			if _, e = a.DB.Exec(r.Context(), `UPDATE conversation.conversations SET state='owner_unavailable',version=version+1,updated_at=now() WHERE id=$1 AND owner_id=$2 AND kind='group' AND state='active'`, id, q.UserID); e != nil {
				return e
			}
			if e = a.protect(r.Context(), operation, "result", "conversation.owner_unavailable", payload); e != nil {
				return e
			}
		}
		respond(w, 200, map[string]any{"status": "restricted", "groups": len(ids)})
		return nil
	}))
}

func (a *App) requireConversationWrite(ctx context.Context, user, conversation string) error {
	return a.rpc(ctx, "conversation", "/internal/member", map[string]string{"userId": user, "conversationId": conversation, "action": "write"}, nil)
}
