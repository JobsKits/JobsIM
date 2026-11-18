package jobsim

import (
	"context"
	"net/http"
	"time"
)

type retainedAccessRequest struct {
	Message string `json:"messageId"`
	Object  string `json:"objectId"`
	User    string `json:"userId"`
	Device  string `json:"deviceId"`
	Store   string `json:"storeGeneration"`
	Epoch   string `json:"restoreEpoch"`
	Content string `json:"contentVersion"`
}

func (a *App) authorizedRetainedAttachment(ctx context.Context, q retainedAccessRequest) (Message, error) {
	var m Message
	var epoch string
	var pending bool
	if !syncValidationID(q.Message) || !syncValidationID(q.User) || !syncValidationID(q.Device) || !syncValidationCounter(q.Epoch, false) || q.Store != "" && !syncValidationID(q.Store) {
		return m, fail(400, "INVALID_RETAINED_ACCESS", "Exact current device, physical store and original object required")
	}
	err := a.DB.QueryRow(ctx, `SELECT m.id::text,m.conversation_id::text,COALESCE(m.attachment_id::text,''),m.content_version::text,m.deleted,m.content_unavailable,m.created_at,i.restore_epoch::text,m.export_pending_operation_id IS NOT NULL FROM message.messages m CROSS JOIN message.instance i WHERE m.id=$1 AND i.singleton`, q.Message).Scan(&m.ID, &m.ConversationID, &m.AttachmentID, &m.ContentVersion, &m.Deleted, &m.ContentUnavailable, &m.CreatedAt, &epoch, &pending)
	if err != nil || pending || m.Deleted || !syncValidationID(m.AttachmentID) || q.Object != "" && m.AttachmentID != q.Object || q.Content != "" && m.ContentVersion != q.Content || epoch != q.Epoch {
		return m, fail(403, "ATTACHMENT_UNAVAILABLE", "Original current attachment is unavailable")
	}
	meta, err := a.member(ctx, q.User, m.ConversationID)
	if err != nil {
		return m, err
	}
	if meta["kind"] == "group" {
		joined, e := time.Parse(time.RFC3339Nano, stringValue(meta, "joinedAt"))
		if e != nil || m.CreatedAt.Before(joined) {
			return m, fail(403, "HISTORY_NOT_ALLOWED", "New membership cannot inherit an old attachment")
		}
	}
	gate := Message{ID: m.ID, Deleted: m.Deleted, ContentUnavailable: m.ContentUnavailable}
	if err = a.retentionReadState(ctx, &gate); err != nil {
		return m, err
	}
	if gate.Deleted {
		return m, fail(403, "ATTACHMENT_UNAVAILABLE", "Global erasure rejects all old attachment grants")
	}
	m.ContentUnavailable = gate.ContentUnavailable
	if m.ContentUnavailable {
		var permission struct {
			Allowed bool `json:"allowed"`
		}
		if err = a.rpc(ctx, "sync", "/internal/receipts/retained-access", map[string]string{"messageId": m.ID, "userId": q.User, "deviceId": q.Device, "storeGeneration": q.Store, "restoreEpoch": q.Epoch, "contentVersion": m.ContentVersion, "membershipGeneration": stringValue(meta, "membershipGeneration")}, &permission); err != nil {
			return m, err
		}
		if !permission.Allowed {
			return m, fail(403, "RETAINED_TARGET_REQUIRED", "Only the original qualified physical store with its committed descriptor may resume")
		}
	}
	return m, nil
}
func (a *App) retentionReceiptSource(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "sync" {
		return fail(403, "RETENTION_OWNER_REQUIRED", "Only the qualified receipt owner may read a retained descriptor")
	}
	var q retainedAccessRequest
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationCounter(q.Content, false) {
		return fail(400, "INVALID_RETAINED_ACCESS", "Original descriptor content version required")
	}
	m, err := a.authorizedRetainedAttachment(r.Context(), q)
	if err != nil {
		return err
	}
	respond(w, 200, m)
	return nil
}
