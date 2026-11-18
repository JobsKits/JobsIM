package jobsim

import (
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
)

func (a *App) conversationRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.avatarConversationRoutes(h, internal)
	a.ownerLifecycleRoutes(h, internal)
	a.mentionAnnouncementConversationRoutes(h, internal)
	a.forwardPendingConversationRoutes(h, internal)
	a.groupRoutes(h)
	h("POST /internal/export", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		rows, e := a.DB.Query(r.Context(), `SELECT c.id::text,c.kind,c.title FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id WHERE m.user_id=$1 AND m.state='active' ORDER BY c.id`, q.UserID)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, kind, title string
			if e = rows.Scan(&id, &kind, &title); e != nil {
				return e
			}
			items = append(items, map[string]string{"id": id, "kind": kind, "title": title})
		}
		respond(w, 200, map[string]any{"items": items})
		return rows.Err()
	}))
	h("GET /api/conversations", a.conversations)
	h("POST /api/conversations", a.createConversation)
	h("GET /api/conversations/{id}/members", a.members)
	h("POST /api/conversations/{id}/members", a.addMember)
	h("DELETE /api/conversations/{id}/members/{userId}", a.removeMember)
	h("PATCH /api/conversations/{id}", a.updateConversation)
	h("POST /internal/member", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID         string `json:"userId"`
			ConversationID string `json:"conversationId"`
			Action         string `json:"action"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		var kind, owner, role, state string
		var generation, roleGeneration int64
		var joined time.Time
		e := a.DB.QueryRow(r.Context(), `SELECT c.kind,c.owner_id::text,m.role,c.state,m.membership_generation,m.role_generation,m.joined_at FROM conversation.members m JOIN conversation.conversations c ON c.id=m.conversation_id WHERE m.conversation_id=$1 AND m.user_id=$2 AND m.state='active' AND m.export_pending_operation_id IS NULL`, q.ConversationID, q.UserID).Scan(&kind, &owner, &role, &state, &generation, &roleGeneration, &joined)
		if e != nil || state != "active" && state != "owner_unavailable" {
			return fail(403, "NOT_A_MEMBER", "Current active membership required")
		}
		if state == "owner_unavailable" && (q.Action == "send" || q.Action == "write") {
			return fail(409, "GROUP_OWNER_UNAVAILABLE", "Group is read-only pending independent governance recovery")
		}
		var peer string
		permission := ""
		allowed := state == "active"
		if kind == "direct" {
			e = a.DB.QueryRow(r.Context(), `SELECT user_id::text FROM conversation.members WHERE conversation_id=$1 AND user_id<>$2 AND state='active'`, q.ConversationID, q.UserID).Scan(&peer)
			if e != nil {
				return e
			}
			blocked, friends, grant, e := a.relationshipAdmission(r.Context(), q.UserID, peer)
			if e != nil {
				return e
			}
			if q.Action == "send" && (blocked || !friends) {
				return fail(403, "DIRECT_PERMISSION_REQUIRED", "Current friendship and privacy permission required")
			}
			permission = grant
			allowed = allowed && !blocked && friends
		}
		slowSeconds := 0
		if kind == "group" {
			grant, groupAllowed, policy, err := a.groupAdmission(r.Context(), q.ConversationID, q.UserID, role, generation)
			if err != nil {
				return err
			}
			permission = grant
			allowed = allowed && groupAllowed
			slowSeconds = policy.SlowSeconds
			if q.Action == "send" && !groupAllowed {
				return fail(403, "GROUP_MUTED", "Current group mute policy forbids sending")
			}
		}
		admission := ""
		if allowed {
			admission = a.sendAdmission(q.UserID, q.ConversationID, kind, generation, roleGeneration, permission)
		}
		respond(w, 200, map[string]any{"kind": kind, "ownerId": owner, "role": role, "state": state, "readOnly": state == "owner_unavailable", "membershipGeneration": str(generation), "roleGeneration": str(roleGeneration), "sendAdmission": admission, "joinedAt": joined, "peerId": peer, "slowSeconds": strconv.Itoa(slowSeconds)})
		return nil
	}))
}
func (a *App) conversationByID(r *http.Request, id, user string) (Conversation, error) {
	var c Conversation
	var version int64
	e := a.DB.QueryRow(r.Context(), `SELECT id::text,kind,title,owner_id::text,updated_at,version,state FROM conversation.conversations WHERE id=$1`, id).Scan(&c.ID, &c.Kind, &c.Title, &c.OwnerID, &c.UpdatedAt, &version, &c.State)
	if e != nil {
		return c, e
	}
	rows, e := a.DB.Query(r.Context(), `SELECT user_id::text FROM conversation.members WHERE conversation_id=$1 AND state='active' ORDER BY user_id LIMIT 200`, id)
	if e != nil {
		return c, e
	}
	c.MemberIDs = []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return c, e
		}
		c.MemberIDs = append(c.MemberIDs, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return c, e
	}
	c.Cursor = "0"
	c.ReadOnly = c.State != "active"
	var generation, roleGeneration int64
	var memberRole string
	if e = a.DB.QueryRow(r.Context(), `SELECT membership_generation,role_generation,role FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active' AND export_pending_operation_id IS NULL`, id, user).Scan(&generation, &roleGeneration, &memberRole); e != nil {
		return c, fail(403, "NOT_A_MEMBER", "Current active membership required")
	}
	c.MembershipGeneration = str(generation)
	c.RoleGeneration = str(roleGeneration)
	permission := ""
	allowed := c.State == "active"
	if c.Kind == "direct" {
		for _, peer := range c.MemberIDs {
			if peer != user {
				blocked, friends, grant, lookupError := a.relationshipAdmission(r.Context(), user, peer)
				if lookupError != nil {
					return c, lookupError
				}
				allowed = allowed && !blocked && friends
				permission = grant
			}
		}
	}
	if c.Kind == "group" {
		grant, groupAllowed, _, err := a.groupAdmission(r.Context(), id, user, memberRole, generation)
		if err != nil {
			return c, err
		}
		permission = grant
		allowed = allowed && groupAllowed
	}
	if allowed {
		c.SendAdmission = a.sendAdmission(user, id, c.Kind, generation, roleGeneration, permission)
	}
	return c, nil
}
func (a *App) conversations(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	rows, e := a.DB.Query(r.Context(), `SELECT c.id::text FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id WHERE m.user_id=$1 AND m.state='active' ORDER BY c.updated_at DESC LIMIT 100`, u.ID)
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
	items := []Conversation{}
	for _, id := range ids {
		c, e := a.conversationByID(r, id, u.ID)
		if e != nil {
			return e
		}
		items = append(items, c)
	}
	respond(w, 200, map[string]any{"items": items})
	return nil
}
func (a *App) createConversation(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q struct {
		Kind      string   `json:"kind"`
		Title     string   `json:"title"`
		MemberIDs []string `json:"memberIds"`
		RequestID string   `json:"requestId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if q.Kind == "group" {
		if e = a.requireFeature(r.Context(), "groups"); e != nil {
			return e
		}
	}
	if q.Kind != "direct" && q.Kind != "group" && q.Kind != "self" {
		return fail(400, "INVALID_KIND", "Unsupported conversation kind")
	}
	if len(q.Title) > 200 || len(q.MemberIDs) > 199 {
		return fail(400, "GROUP_LIMIT", "Title or member limit exceeded")
	}
	ids := []string{u.ID}
	seen := map[string]bool{u.ID: true}
	for _, id := range q.MemberIDs {
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if (q.Kind == "direct" && len(ids) != 2) || (q.Kind == "self" && len(ids) != 1) {
		return fail(400, "INVALID_PARTICIPANTS", "Invalid participants")
	}
	for _, id := range ids {
		if id != u.ID {
			blocked, friends, e := a.relationship(r.Context(), u.ID, id)
			if e != nil {
				return e
			}
			if !friends || blocked && q.Kind != "direct" {
				return fail(403, "FRIENDSHIP_REQUIRED", "Add and accept this contact first")
			}
		}
	}
	var users any
	if e = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": ids}, &users); e != nil {
		return e
	}
	var pair any
	if q.Kind == "direct" {
		pair = pairKey(ids[0], ids[1])
	} else if q.Kind == "self" {
		pair = "self:" + u.ID
	}
	id := newID()
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if pair != nil {
		e = tx.QueryRow(r.Context(), `INSERT INTO conversation.conversations(id,kind,pair_key,title,owner_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(pair_key) DO UPDATE SET pair_key=EXCLUDED.pair_key RETURNING id::text`, id, q.Kind, pair, q.Title, u.ID).Scan(&id)
	} else {
		_, e = tx.Exec(r.Context(), `INSERT INTO conversation.conversations(id,kind,title,owner_id) VALUES($1,$2,$3,$4)`, id, q.Kind, q.Title, u.ID)
	}
	if e != nil {
		return e
	}
	for _, uid := range ids {
		role := "member"
		if uid == u.ID && q.Kind != "direct" {
			role = "owner"
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO conversation.members(conversation_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, uid, role)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO conversation.membership_periods(conversation_id,user_id,generation) VALUES($1,$2,1) ON CONFLICT DO NOTHING`, id, uid)
		if e != nil {
			return e
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	c, e := a.conversationByID(r, id, u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, c)
	return nil
}
func (a *App) members(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	if _, e = a.member(r.Context(), u.ID, r.PathValue("id")); e != nil {
		return e
	}
	after, limit, e := groupPage(r)
	if e != nil {
		return e
	}
	rows, e := a.DB.Query(r.Context(), `SELECT user_id::text,role,membership_generation::text,role_generation::text FROM conversation.members WHERE conversation_id=$1 AND state='active' AND ($2='' OR user_id>NULLIF($2,'')::uuid) ORDER BY user_id LIMIT $3`, r.PathValue("id"), after, limit+1)
	if e != nil {
		return e
	}
	ids := []string{}
	roles := map[string][3]string{}
	for rows.Next() {
		var id, role, membership, generation string
		if e = rows.Scan(&id, &role, &membership, &generation); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
		roles[id] = [3]string{role, membership, generation}
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	next := ""
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[limit-1]
	}
	var users struct {
		Items []User `json:"items"`
	}
	if e = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": ids, "includeUnavailable": true}, &users); e != nil {
		return e
	}
	for i := range users.Items {
		role := roles[users.Items[i].ID]
		users.Items[i].Role = role[0]
		users.Items[i].MembershipGeneration = role[1]
		users.Items[i].RoleGeneration = role[2]
	}
	respond(w, 200, map[string]any{"items": users.Items, "nextCursor": next})
	return nil
}
func (a *App) addMember(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	meta, e := a.member(r.Context(), u.ID, r.PathValue("id"))
	if e != nil {
		return e
	}
	if meta["ownerId"] != u.ID || meta["kind"] != "group" {
		return fail(403, "OWNER_REQUIRED", "Group owner required")
	}
	var q struct {
		UserID string `json:"userId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	blocked, friends, e := a.relationship(r.Context(), u.ID, q.UserID)
	if e != nil {
		return e
	}
	if blocked || !friends {
		return fail(403, "FRIENDSHIP_REQUIRED", "Accepted contact required")
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var owner string
	if e = tx.QueryRow(r.Context(), `SELECT owner_id::text FROM conversation.conversations WHERE id=$1 AND kind='group' AND state='active' FOR UPDATE`, r.PathValue("id")).Scan(&owner); e != nil || owner != u.ID {
		return fail(403, "OWNER_REQUIRED", "Current group owner required")
	}
	var count int
	if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM conversation.members WHERE conversation_id=$1 AND state='active'`, r.PathValue("id")).Scan(&count); e != nil {
		return e
	}
	var maxMembers int
	if e = tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT max_members FROM conversation.group_settings WHERE conversation_id=$1),200)`, r.PathValue("id")).Scan(&maxMembers); e != nil {
		return e
	}
	if count >= maxMembers {
		return fail(409, "GROUP_FULL", "Group limit reached")
	}
	var gen int64
	e = tx.QueryRow(r.Context(), `INSERT INTO conversation.members(conversation_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT(conversation_id,user_id) DO UPDATE SET role='member',state='active',membership_generation=conversation.members.membership_generation+1,role_generation=conversation.members.role_generation+1,joined_at=now() WHERE conversation.members.state<>'active' RETURNING membership_generation`, r.PathValue("id"), q.UserID).Scan(&gen)
	if e == pgx.ErrNoRows {
		respond(w, 200, map[string]string{"status": "already_member"})
		return nil
	}
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO conversation.membership_periods(conversation_id,user_id,generation) VALUES($1,$2,$3)`, r.PathValue("id"), q.UserID, gen)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"status": "joined"})
	return nil
}
func (a *App) removeMember(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	target := r.PathValue("userId")
	if !validObjectID(target) || !validObjectID(r.PathValue("id")) {
		return fail(400, "INVALID_MEMBERSHIP_ID", "Stable conversation and member IDs required")
	}
	expected, e := strconv.ParseInt(r.URL.Query().Get("membershipGeneration"), 10, 64)
	if e != nil || expected <= 0 {
		return fail(400, "MEMBERSHIP_GENERATION_REQUIRED", "Use the original membershipGeneration shown by the member list")
	}
	resumeSelf := false
	if target == u.ID {
		var state string
		var generation int64
		var pending *string
		e = a.DB.QueryRow(r.Context(), `SELECT state,membership_generation,export_pending_operation_id FROM conversation.members WHERE conversation_id=$1 AND user_id=$2`, r.PathValue("id"), target).Scan(&state, &generation, &pending)
		if e != nil {
			return fail(403, "NOT_A_MEMBER", "Original membership unavailable")
		}
		if generation != expected {
			return fail(409, "MEMBERSHIP_CHANGED", "Original membership generation is no longer current")
		}
		if state == "left" {
			respond(w, 200, map[string]string{"status": "left", "membershipGeneration": str(generation)})
			return nil
		}
		resumeSelf = pending != nil && *pending == "leave:"+r.PathValue("id")+":"+target+":"+str(expected)
	}
	if !resumeSelf {
		meta, e := a.member(r.Context(), u.ID, r.PathValue("id"))
		if e != nil {
			return e
		}
		if meta["kind"] != "group" || (meta["ownerId"] != u.ID && target != u.ID) || meta["ownerId"] == target {
			return fail(403, "MEMBER_CHANGE_FORBIDDEN", "Current member removal permission required")
		}
	}
	operation, e := a.freezeMembershipExportsAt(r.Context(), r.PathValue("id"), target, u.ID, expected)
	if e != nil {
		return e
	}
	payload := map[string]any{"conversationId": r.PathValue("id"), "userId": target, "membershipGeneration": str(expected)}
	if e = a.protect(r.Context(), operation, "intent", "conversation.leave", payload); e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var owner, kind string
	if e = tx.QueryRow(r.Context(), `SELECT owner_id::text,kind FROM conversation.conversations WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&owner, &kind); e != nil {
		return fail(404, "NOT_FOUND", "Group unavailable")
	}
	if kind != "group" || (u.ID != owner && u.ID != target) {
		return fail(403, "FORBIDDEN", "Cannot remove this member")
	}
	if target == owner {
		return fail(409, "TRANSFER_REQUIRED", "Transfer ownership before leaving")
	}
	tag, e := tx.Exec(r.Context(), `UPDATE conversation.members SET state='left',export_pending_operation_id=NULL WHERE conversation_id=$1 AND user_id=$2 AND membership_generation=$3 AND export_pending_operation_id=$4`, r.PathValue("id"), target, expected, operation)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "MEMBERSHIP_CHANGED", "Original leave operation no longer owns this membership")
	}
	_, e = tx.Exec(r.Context(), `UPDATE conversation.membership_periods SET left_at=now() WHERE conversation_id=$1 AND user_id=$2 AND left_at IS NULL`, r.PathValue("id"), target)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if e = a.protect(r.Context(), operation, "result", "conversation.leave", payload); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"status": "left", "membershipGeneration": str(expected)})
	return nil
}
func (a *App) updateConversation(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q struct {
		Title   *string `json:"title"`
		OwnerID *string `json:"ownerId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if q.OwnerID != nil {
		return fail(400, "GROUP_ORIGINAL_PERMISSION_REQUIRED", "Transfer requires the original governance nonce, group version, actor and successor permission versions")
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var owner, kind string
	if e = tx.QueryRow(r.Context(), `SELECT owner_id::text,kind FROM conversation.conversations WHERE id=$1 AND state='active' FOR UPDATE`, r.PathValue("id")).Scan(&owner, &kind); e != nil || owner != u.ID || kind != "group" {
		return fail(403, "OWNER_REQUIRED", "Group owner required")
	}
	if q.Title != nil {
		if len(*q.Title) > 200 {
			return fail(400, "TITLE_TOO_LONG", "Title too long")
		}
		_, e = tx.Exec(r.Context(), `UPDATE conversation.conversations SET title=$2,version=version+1,updated_at=now() WHERE id=$1`, r.PathValue("id"), *q.Title)
		if e != nil {
			return e
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	c, e := a.conversationByID(r, r.PathValue("id"), u.ID)
	if e != nil {
		return e
	}
	respond(w, 200, c)
	return nil
}
