package jobsim

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"strings"
)

func pairKey(a, b string) string { v := []string{a, b}; sort.Strings(v); return strings.Join(v, ":") }
func relationLock(ctx context.Context, tx pgx.Tx, a, b string) error {
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, pairKey(a, b))
	return e
}
func (a *App) relationship(ctx context.Context, first, second string) (bool, bool, error) {
	var v struct {
		Blocked bool `json:"blocked"`
		Friends bool `json:"friends"`
	}
	e := a.rpc(ctx, "directory", "/internal/relationship", map[string]string{"first": first, "second": second}, &v)
	return v.Blocked, v.Friends, e
}
func (a *App) directoryRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.avatarDirectoryRoutes(h, internal)
	a.profileDirectoryRoutes(h, internal)
	h("POST /internal/profiles", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			IDs []string `json:"ids"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if len(q.IDs) > 200 {
			return fail(400, "TOO_MANY_USERS", "Limit 200 profiles")
		}
		rows, e := a.DB.Query(r.Context(), `SELECT user_id::text,display_name,row_version,avatar_object_id::text,avatar_thumbnail_id::text FROM directory.profiles WHERE user_id=ANY($1::uuid[])`, q.IDs)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, name string
			var version int64
			var avatar, thumbnail *string
			if e = rows.Scan(&id, &name, &version, &avatar, &thumbnail); e != nil {
				return e
			}
			fields := map[string]string{"id": id, "displayName": name, "profileVersion": str(version)}
			if avatar != nil {
				fields["avatarURL"] = "/api/avatars/" + *avatar
			}
			if thumbnail != nil {
				fields["avatarThumbnailURL"] = "/api/avatars/" + *thumbnail
			}
			items = append(items, fields)
		}
		respond(w, 200, map[string]any{"items": items})
		return rows.Err()
	}))
	h("POST /internal/relationship", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			First  string `json:"first"`
			Second string `json:"second"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		var blocked, friends bool
		var permission int64
		e := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.blocks WHERE (owner_id=$1 AND peer_id=$2) OR(owner_id=$2 AND peer_id=$1)),EXISTS(SELECT 1 FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2 AND state='friend'),COALESCE((SELECT permission_generation FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2),0)`, q.First, q.Second).Scan(&blocked, &friends, &permission)
		if e != nil {
			return e
		}
		respond(w, 200, map[string]any{"blocked": blocked, "friends": friends, "permissionGeneration": str(permission)})
		return nil
	}))
	h("POST /internal/profile", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		profile, e := a.avatarProfileFields(r.Context(), q.UserID)
		if e != nil {
			return e
		}
		respond(w, 200, profile)
		return nil
	}))
	h("PATCH /api/me", a.patchOwnProfile)
	h("POST /internal/export", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		rows, e := a.DB.Query(r.Context(), `SELECT peer_id::text,state FROM directory.contacts WHERE owner_id=$1 ORDER BY peer_id`, q.UserID)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, state string
			if e = rows.Scan(&id, &state); e != nil {
				return e
			}
			items = append(items, map[string]string{"peerId": id, "state": state})
		}
		respond(w, 200, map[string]any{"items": items})
		return rows.Err()
	}))
	h("GET /api/contacts", a.contacts)
	h("GET /api/contacts/{userId}/relationship", a.contactRelationship)
	h("POST /api/contacts/requests", a.contactRequest)
	h("POST /api/contacts/requests/{id}/accept", func(w http.ResponseWriter, r *http.Request) error { return a.resolveContact(w, r, true) })
	h("POST /api/contacts/requests/{id}/reject", func(w http.ResponseWriter, r *http.Request) error { return a.resolveContact(w, r, false) })
	h("DELETE /api/contacts/{userId}", a.removeContact)
	h("PUT /api/blocks/{userId}", func(w http.ResponseWriter, r *http.Request) error { return a.block(w, r, true) })
	h("DELETE /api/blocks/{userId}", func(w http.ResponseWriter, r *http.Request) error { return a.block(w, r, false) })
	h("POST /api/contacts/{userId}/reopen", a.reopenRequestGate)
}
func (a *App) contacts(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	rows, e := a.DB.Query(r.Context(), `SELECT peer_id::text FROM directory.contacts WHERE owner_id=$1 AND state='friend' ORDER BY updated_at`, u.ID)
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
	var users struct {
		Items []User `json:"items"`
	}
	if len(ids) > 0 {
		if e = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": ids, "includeUnavailable": true, "skipUnavailable": true}, &users); e != nil {
			return e
		}
	}
	if users.Items == nil {
		users.Items = []User{}
	}
	rows, e = a.DB.Query(r.Context(), `SELECT r.id::text,r.requester_id::text,r.recipient_id::text,r.state,
 (r.requester_id=$1 OR (NOT EXISTS(SELECT 1 FROM directory.blocks b WHERE(b.owner_id=$1 AND b.peer_id=r.requester_id)OR(b.owner_id=r.requester_id AND b.peer_id=$1)) AND (EXISTS(SELECT 1 FROM directory.contacts c WHERE c.owner_id=$1 AND c.peer_id=r.requester_id AND c.state='friend') OR(r.recipient_id=$1 AND r.state='pending' AND EXISTS(SELECT 1 FROM directory.contacts c WHERE c.owner_id=r.recipient_id AND c.peer_id=r.requester_id AND c.request_gate='open' AND c.request_generation=r.generation))))),
 (r.requester_id=$1 OR EXISTS(SELECT 1 FROM directory.contacts c WHERE c.owner_id=$1 AND c.peer_id=r.requester_id AND c.state='friend'))
 FROM directory.contact_requests r WHERE r.recipient_id=$1 OR r.requester_id=$1 ORDER BY r.created_at DESC LIMIT 100`, u.ID)
	if e != nil {
		return e
	}
	requests := []map[string]any{}
	eligible := map[string]bool{}
	avatarAllowed := map[string]bool{}
	for rows.Next() {
		var id, from, to, state string
		var visible, avatar bool
		if e = rows.Scan(&id, &from, &to, &state, &visible, &avatar); e != nil {
			rows.Close()
			return e
		}
		requests = append(requests, map[string]any{"id": id, "requesterId": from, "recipientId": to, "state": state})
		if visible {
			eligible[from] = true
			avatarAllowed[from] = avatarAllowed[from] || avatar
		}
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	requesterIDs := []string{}
	for id := range eligible {
		requesterIDs = append(requesterIDs, id)
	}
	var profiles struct {
		Items []User `json:"items"`
	}
	if len(requesterIDs) > 0 {
		if e = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": requesterIDs, "skipUnavailable": true}, &profiles); e != nil {
			return e
		}
	}
	byID := map[string]User{}
	for _, profile := range profiles.Items {
		profile.Admin = false
		profile.DeviceID = ""
		profile.RestoreEpoch = ""
		if !avatarAllowed[profile.ID] && profile.AvatarURL != "" {
			var common struct {
				Visible bool `json:"visible"`
			}
			if err := a.rpc(r.Context(), "conversation", "/internal/profile-visible", map[string]string{"first": u.ID, "second": profile.ID}, &common); err == nil && common.Visible {
				avatarAllowed[profile.ID] = true
			}
		}
		if !avatarAllowed[profile.ID] {
			profile.AvatarURL = ""
			profile.AvatarThumbnailURL = ""
		}
		byID[profile.ID] = profile
	}
	for _, request := range requests {
		id, _ := request["requesterId"].(string)
		if profile, ok := byID[id]; ok {
			request["requester"] = profile
		}
	}
	for i := range users.Items {
		if users.Items[i].State != "" && users.Items[i].State != "active" {
			users.Items[i].AvatarURL = ""
			users.Items[i].AvatarThumbnailURL = ""
		}
	}
	respond(w, 200, map[string]any{"items": users.Items, "requests": requests})
	return nil
}
func (a *App) contactRequest(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q struct {
		UserID    string `json:"userId"`
		RequestID string `json:"requestId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if u.ID == q.UserID {
		return fail(400, "SELF_CONTACT", "Cannot add yourself")
	}
	if q.RequestID == "" {
		q.RequestID = newID()
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 {
		return fail(400, "REQUEST_ID_REQUIRED", "Stable contact requestId must be 8–128 bytes")
	}
	var targets any
	if e = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": []string{q.UserID}}, &targets); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = relationLock(r.Context(), tx, u.ID, q.UserID); e != nil {
		return e
	}
	for _, p := range [][2]string{{u.ID, q.UserID}, {q.UserID, u.ID}} {
		_, e = tx.Exec(r.Context(), `INSERT INTO directory.contacts(owner_id,peer_id,state) VALUES($1,$2,'removed') ON CONFLICT DO NOTHING`, p[0], p[1])
		if e != nil {
			return e
		}
	}
	var blocked bool
	var gate, state string
	var gen int64
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.blocks WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1))`, u.ID, q.UserID).Scan(&blocked); e != nil {
		return e
	}
	if e = tx.QueryRow(r.Context(), `SELECT request_gate,state,request_generation FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2 FOR UPDATE`, q.UserID, u.ID).Scan(&gate, &state, &gen); e != nil {
		return e
	}
	var priorID, priorPeer, priorState string
	var priorGeneration int64
	lookup := tx.QueryRow(r.Context(), `SELECT id::text,recipient_id::text,state,generation FROM directory.contact_requests WHERE requester_id=$1 AND request_id=$2`, u.ID, q.RequestID).Scan(&priorID, &priorPeer, &priorState, &priorGeneration)
	if lookup == nil {
		if priorPeer != q.UserID {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original contact request targeted another peer")
		}
		if blocked || priorGeneration != gen || priorState != "pending" && !(priorState == "accepted" && state == "friend") {
			return fail(409, "REQUEST_STALE", "The old request is settled or revoked; a new explicit requestId is required")
		}
		respond(w, 200, map[string]string{"id": priorID, "state": priorState})
		return nil
	}
	if lookup != pgx.ErrNoRows {
		return lookup
	}
	if blocked {
		return fail(403, "REQUEST_NOT_ALLOWED", "Contact request is not allowed")
	}
	if state == "friend" {
		respond(w, 200, map[string]string{"state": "accepted"})
		return nil
	}
	var id string
	e = tx.QueryRow(r.Context(), `SELECT id::text FROM directory.contact_requests WHERE requester_id=$1 AND recipient_id=$2 AND state='pending' AND generation=$3`, u.ID, q.UserID, gen).Scan(&id)
	if e == nil {
		return fail(409, "CONTACT_REQUEST_PENDING", "An application is already pending; reload its original request")
	}
	if e == pgx.ErrNoRows {
		if e = contactRequestRate(r.Context(), tx, u.ID, q.UserID); e != nil {
			return e
		}
		if gate != "open" {
			if _, e = tx.Exec(r.Context(), `UPDATE directory.contacts SET request_gate='open' WHERE owner_id=$1 AND peer_id=$2`, q.UserID, u.ID); e != nil {
				return e
			}
		}
		id = newID()
		_, e = tx.Exec(r.Context(), `INSERT INTO directory.contact_requests(id,request_id,requester_id,recipient_id,state,generation) VALUES($1,$2,$3,$4,'pending',$5)`, id, q.RequestID, u.ID, q.UserID, gen)
	}
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"id": id, "state": "pending"})
	return nil
}
func (a *App) resolveContact(w http.ResponseWriter, r *http.Request, accept bool) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var requester, recipient string
	e = a.DB.QueryRow(r.Context(), `SELECT requester_id::text,recipient_id::text FROM directory.contact_requests WHERE id=$1`, r.PathValue("id")).Scan(&requester, &recipient)
	if e != nil || recipient != u.ID {
		return fail(404, "REQUEST_NOT_FOUND", "Request unavailable")
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = relationLock(r.Context(), tx, requester, recipient); e != nil {
		return e
	}
	var state, gate string
	var requestGen, currentGen int64
	if e = tx.QueryRow(r.Context(), `SELECT state,generation FROM directory.contact_requests WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&state, &requestGen); e != nil {
		return e
	}
	var blocked bool
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.blocks WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1))`, requester, recipient).Scan(&blocked); e != nil {
		return e
	}
	if e = tx.QueryRow(r.Context(), `SELECT request_gate,request_generation FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2 FOR UPDATE`, recipient, requester).Scan(&gate, &currentGen); e != nil {
		return e
	}
	desired := "rejected"
	if accept {
		desired = "accepted"
	}
	if state == desired {
		if accept && (blocked || requestGen != currentGen) {
			return fail(409, "REQUEST_STALE", "Original acceptance has been revoked")
		}
		respond(w, 200, map[string]string{"state": state})
		return nil
	}
	if state != "pending" || blocked || requestGen != currentGen {
		return fail(409, "REQUEST_STALE", "Request is no longer actionable")
	}
	_, e = tx.Exec(r.Context(), `UPDATE directory.contact_requests SET state=$2 WHERE id=$1`, r.PathValue("id"), desired)
	if e != nil {
		return e
	}
	if accept {
		_, e = tx.Exec(r.Context(), `UPDATE directory.contacts SET state='friend',permission_generation=permission_generation+1,updated_at=now() WHERE ((owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1)) AND state<>'friend'`, requester, recipient)
	} else {
		_, e = tx.Exec(r.Context(), `UPDATE directory.contacts SET request_gate='open',request_generation=request_generation+1 WHERE owner_id=$1 AND peer_id=$2`, recipient, requester)
	}
	if e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `UPDATE directory.contact_requests SET state='rejected' WHERE ((requester_id=$1 AND recipient_id=$2)OR(requester_id=$2 AND recipient_id=$1)) AND state='pending'`, requester, recipient); e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"state": desired})
	return nil
}
func (a *App) removeContact(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = relationLock(r.Context(), tx, u.ID, r.PathValue("userId")); e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE directory.contacts SET state='removed',request_gate='open',request_generation=request_generation+1,permission_generation=permission_generation+1,updated_at=now() WHERE ((owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1)) AND (state<>'removed' OR EXISTS(SELECT 1 FROM directory.contact_requests WHERE ((requester_id=$1 AND recipient_id=$2)OR(requester_id=$2 AND recipient_id=$1)) AND state='pending'))`, u.ID, r.PathValue("userId"))
	if e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `UPDATE directory.contact_requests SET state='rejected' WHERE ((requester_id=$1 AND recipient_id=$2)OR(requester_id=$2 AND recipient_id=$1)) AND state='pending'`, u.ID, r.PathValue("userId")); e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"status": "removed"})
	return nil
}
func (a *App) block(w http.ResponseWriter, r *http.Request, block bool) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	peer := r.PathValue("userId")
	if peer == u.ID {
		return fail(400, "SELF_BLOCK", "Cannot block yourself")
	}
	kind := "directory.unblock"
	if block {
		kind = "directory.block"
	}
	operation := kind + ":" + u.ID + ":" + peer + ":" + newID()
	payload := map[string]any{"userId": u.ID, "peerId": peer}
	if e = a.protect(r.Context(), operation, "intent", kind, payload); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = relationLock(r.Context(), tx, u.ID, peer); e != nil {
		return e
	}
	if block {
		inserted, insertError := tx.Exec(r.Context(), `INSERT INTO directory.blocks(owner_id,peer_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, u.ID, peer)
		e = insertError
		if e != nil {
			return e
		}
		for _, p := range [][2]string{{u.ID, peer}, {peer, u.ID}} {
			advance := int64(0)
			if inserted.RowsAffected() > 0 {
				advance = 1
			}
			_, e = tx.Exec(r.Context(), `INSERT INTO directory.contacts(owner_id,peer_id,state,request_gate) VALUES($1,$2,'removed','open') ON CONFLICT(owner_id,peer_id) DO UPDATE SET request_gate='open',request_generation=directory.contacts.request_generation+$3,permission_generation=directory.contacts.permission_generation+$3`, p[0], p[1], advance)
			if e != nil {
				return e
			}
		}
		_, e = tx.Exec(r.Context(), `UPDATE directory.contact_requests SET state='rejected' WHERE ((requester_id=$1 AND recipient_id=$2) OR(requester_id=$2 AND recipient_id=$1)) AND state='pending'`, u.ID, peer)
	} else {
		removed, err := tx.Exec(r.Context(), `DELETE FROM directory.blocks WHERE owner_id=$1 AND peer_id=$2`, u.ID, peer)
		e = err
		if e == nil && removed.RowsAffected() > 0 {
			_, e = tx.Exec(r.Context(), `UPDATE directory.contacts SET permission_generation=permission_generation+1,request_gate='open',updated_at=now() WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1)`, u.ID, peer)
		}
	}
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if e = a.protect(r.Context(), operation, "result", kind, payload); e != nil {
		return e
	}
	respond(w, 200, map[string]bool{"blocked": block})
	return nil
}
