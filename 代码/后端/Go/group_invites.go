package jobsim

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["conversation"] += `
CREATE TABLE IF NOT EXISTS conversation.group_invites(id uuid PRIMARY KEY,conversation_id uuid NOT NULL REFERENCES conversation.conversations(id),creator_id uuid NOT NULL,request_id text NOT NULL,digest bytea NOT NULL,expires_at timestamptz NOT NULL,max_uses integer NOT NULL CHECK(max_uses BETWEEN 1 AND 10000),uses integer NOT NULL DEFAULT 0,approval_required boolean NOT NULL,revoked_at timestamptz,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(conversation_id,creator_id,request_id));
CREATE TABLE IF NOT EXISTS conversation.group_join_requests(id uuid PRIMARY KEY,conversation_id uuid NOT NULL,invite_id uuid NOT NULL REFERENCES conversation.group_invites(id),user_id uuid NOT NULL,request_id text NOT NULL,state text NOT NULL DEFAULT 'pending',membership_generation bigint,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(user_id,request_id));
CREATE INDEX IF NOT EXISTS group_join_request_page ON conversation.group_join_requests(conversation_id,id);`
	migrations["conversation"] += `ALTER TABLE conversation.group_invites ADD COLUMN IF NOT EXISTS creator_membership_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE conversation.group_invites ADD COLUMN IF NOT EXISTS creator_role_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE conversation.group_invites ADD COLUMN IF NOT EXISTS creator_security_version bigint NOT NULL DEFAULT 0;
ALTER TABLE conversation.group_invites ADD COLUMN IF NOT EXISTS creator_restore_epoch bigint NOT NULL DEFAULT 0;`
}

func (a *App) groupInviteToken(id string) string {
	return id + "." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "group-invite-v1", []byte(a.Config.DeploymentID+":"+id)))
}

func (a *App) groupInviteRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/conversations/{id}/invites", a.groupInviteList)
	h("POST /api/conversations/{id}/invites", a.groupInviteCreate)
	h("GET /api/conversations/{id}/invites/operations/{requestId}", a.groupInviteResult)
	h("DELETE /api/conversations/{id}/invites/{inviteId}", a.groupInviteRevoke)
	h("POST /api/conversations/{id}/join-requests", a.groupJoinRequest)
	h("GET /api/conversations/{id}/join-requests/operations/{requestId}", a.groupJoinResult)
	h("GET /api/conversations/{id}/join-requests", a.groupJoinList)
	h("POST /api/conversations/{id}/join-requests/{requestId}", a.groupJoinDecide)
}

func (a *App) requireGroupManager(r *http.Request) (User, error) {
	u, err := a.current(r)
	if err != nil {
		return u, err
	}
	meta, err := a.member(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		return u, err
	}
	if meta["kind"] != "group" || meta["state"] != "active" || meta["role"] != "owner" && meta["role"] != "admin" {
		return u, fail(403, "GROUP_ADMIN_REQUIRED", "Current active group administrator required")
	}
	return u, nil
}

func groupPage(r *http.Request) (string, int, error) {
	after := r.URL.Query().Get("after")
	if after != "" && !validObjectID(after) {
		return "", 0, fail(400, "INVALID_CURSOR", "Original group page cursor required")
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return "", 0, fail(400, "INVALID_PAGE_LIMIT", "Page size must be between 1 and 100")
		}
		limit = n
	}
	return after, limit, nil
}

type groupManagerSnapshot struct {
	RequestID            string `json:"requestId"`
	Version              string `json:"version"`
	MembershipGeneration string `json:"membershipGeneration"`
	RoleGeneration       string `json:"roleGeneration"`
}

func lockGroupManagerVersion(r *http.Request, tx pgx.Tx) (string, string, error) {
	var version, state string
	err := tx.QueryRow(r.Context(), `SELECT version::text,state FROM conversation.conversations WHERE id=$1 AND kind='group' FOR UPDATE`, r.PathValue("id")).Scan(&version, &state)
	if err != nil {
		return "", "", fail(403, "NOT_A_MEMBER", "Original group is unavailable")
	}
	return version, state, nil
}

func checkGroupManagerSnapshot(r *http.Request, tx pgx.Tx, user string, q groupManagerSnapshot, version, state string) error {
	if !validReceiptNonce(q.RequestID) || !syncValidationCounter(q.Version, false) || !syncValidationCounter(q.MembershipGeneration, false) || !syncValidationCounter(q.RoleGeneration, false) {
		return fail(400, "INVALID_GROUP_OPERATION", "Original nonce and observed actor versions required")
	}
	var role, membership, generation string
	err := tx.QueryRow(r.Context(), `SELECT role,membership_generation::text,role_generation::text FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active' AND export_pending_operation_id IS NULL FOR UPDATE`, r.PathValue("id"), user).Scan(&role, &membership, &generation)
	if err != nil || role != "owner" && role != "admin" {
		return fail(403, "GROUP_ADMIN_REQUIRED", "Current original manager required")
	}
	if version != q.Version || membership != q.MembershipGeneration || generation != q.RoleGeneration {
		return fail(409, "GROUP_PERMISSION_CHANGED", "Original observed group or actor permission changed")
	}
	if state != "active" {
		return fail(409, "GROUP_READ_ONLY", "Original group is read-only")
	}
	return nil
}

func readManagerGroupOperation(r *http.Request, tx pgx.Tx, user, nonce string, digest []byte) ([]byte, error) {
	var previous, old []byte
	err := tx.QueryRow(r.Context(), `SELECT digest,result FROM conversation.group_operations WHERE conversation_id=$1 AND actor_id=$2 AND request_id=$3`, r.PathValue("id"), user, nonce).Scan(&old, &previous)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !bytesEqual(old, digest) {
		return nil, fail(409, "GROUP_OPERATION_CONFLICT", "Original manager operation differs")
	}
	return previous, nil
}

func writeManagerGroupResult(w http.ResponseWriter, result []byte) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_, err := w.Write(result)
	return err
}

func saveManagerGroupOperation(r *http.Request, tx pgx.Tx, user, nonce string, digest []byte, result map[string]any) ([]byte, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO conversation.group_operations(conversation_id,actor_id,request_id,digest,result) VALUES($1,$2,$3,$4,$5)`, r.PathValue("id"), user, nonce, digest, encoded)
	return encoded, err
}

func (a *App) verifyGroupInviteIssuer(r *http.Request, tx pgx.Tx, invite string) error {
	var creator, membership, role, security, epoch string
	err := tx.QueryRow(r.Context(), `SELECT creator_id::text,creator_membership_generation::text,creator_role_generation::text,creator_security_version::text,creator_restore_epoch::text FROM conversation.group_invites WHERE conversation_id=$1 AND id=$2`, r.PathValue("id"), invite).Scan(&creator, &membership, &role, &security, &epoch)
	if err != nil {
		return err
	}
	if !syncValidationCounter(membership, false) || !syncValidationCounter(role, false) || !syncValidationCounter(security, false) || !syncValidationCounter(epoch, false) {
		return fail(403, "GROUP_INVITE_ISSUER_CHANGED", "Legacy or unverified delegation cannot grant membership")
	}
	var current bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active' AND role IN('owner','admin') AND membership_generation=$3 AND role_generation=$4 AND export_pending_operation_id IS NULL)`, r.PathValue("id"), creator, membership, role).Scan(&current); err != nil {
		return err
	}
	if !current {
		return fail(403, "GROUP_INVITE_ISSUER_CHANGED", "Original invitation creator membership or role changed")
	}
	issuer, err := a.groupInvitationIssuer(r.Context(), creator)
	if err != nil {
		return err
	}
	if !issuer.Active || issuer.Security != security || issuer.Epoch != epoch {
		return fail(403, "GROUP_INVITE_ISSUER_CHANGED", "Original invitation creator security or restore epoch changed")
	}
	return nil
}

func (a *App) groupInviteList(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.requireGroupManager(r); err != nil {
		return err
	}
	after, limit, err := groupPage(r)
	if err != nil {
		return err
	}
	rows, err := a.DB.Query(r.Context(), `SELECT id::text,expires_at,max_uses,uses,approval_required,revoked_at FROM conversation.group_invites WHERE conversation_id=$1 AND ($2='' OR id>NULLIF($2,'')::uuid) ORDER BY id LIMIT $3`, r.PathValue("id"), after, limit+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id string
		var expires time.Time
		var max, uses int
		var approval bool
		var revoked *time.Time
		if err = rows.Scan(&id, &expires, &max, &uses, &approval, &revoked); err != nil {
			return err
		}
		items = append(items, map[string]any{"id": id, "expiresAt": expires, "maxUses": max, "uses": uses, "approvalRequired": approval, "revokedAt": revoked, "token": a.groupInviteToken(id)})
	}
	if err = rows.Err(); err != nil {
		return err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = stringValue(items[limit-1], "id")
	}
	respond(w, 200, map[string]any{"items": items, "nextCursor": next})
	return nil
}

func (a *App) groupInviteCreate(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	var q struct {
		groupManagerSnapshot
		ExpiresAt        time.Time `json:"expiresAt"`
		MaxUses          int       `json:"maxUses"`
		ApprovalRequired *bool     `json:"approvalRequired"`
	}
	if err = decode(r, &q); err != nil {
		return err
	}
	if !validReceiptNonce(q.RequestID) || !validObjectID(r.PathValue("id")) || q.MaxUses < 1 || q.MaxUses > 10000 || q.ApprovalRequired == nil {
		return fail(400, "INVALID_GROUP_INVITE", "Stable nonce, explicit approval policy, bounded expiry and use count required")
	}
	raw, _ := json.Marshal(q)
	digest := tokenDigest(string(raw))
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	version, state, err := lockGroupManagerVersion(r, tx)
	if err != nil {
		return err
	}
	var oldID string
	var old []byte
	err = tx.QueryRow(r.Context(), `SELECT id::text,digest FROM conversation.group_invites WHERE conversation_id=$1 AND creator_id=$2 AND request_id=$3`, r.PathValue("id"), u.ID, q.RequestID).Scan(&oldID, &old)
	if err == nil {
		if !bytesEqual(old, digest) {
			return fail(409, "GROUP_INVITE_CONFLICT", "Original invitation differs")
		}
		respond(w, 200, map[string]any{"requestId": q.RequestID, "id": oldID, "token": a.groupInviteToken(oldID), "expiresAt": q.ExpiresAt, "maxUses": q.MaxUses, "approvalRequired": *q.ApprovalRequired})
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	if err = checkGroupManagerSnapshot(r, tx, u.ID, q.groupManagerSnapshot, version, state); err != nil {
		return err
	}
	if !q.ExpiresAt.After(time.Now()) || q.ExpiresAt.After(time.Now().Add(30*24*time.Hour)) {
		return fail(400, "INVALID_GROUP_INVITE", "New invitation expiry must be within thirty days")
	}
	issuer, err := a.groupInvitationIssuer(r.Context(), u.ID)
	if err != nil {
		return err
	}
	if !issuer.Active || issuer.Epoch != u.RestoreEpoch || !syncValidationCounter(issuer.Security, false) || !syncValidationCounter(issuer.Epoch, false) {
		return fail(403, "GROUP_INVITE_ISSUER_CHANGED", "Current trusted issuer identity required")
	}
	id := newID()
	_, err = tx.Exec(r.Context(), `INSERT INTO conversation.group_invites(id,conversation_id,creator_id,request_id,digest,expires_at,max_uses,approval_required,creator_membership_generation,creator_role_generation,creator_security_version,creator_restore_epoch) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, r.PathValue("id"), u.ID, q.RequestID, digest, q.ExpiresAt, q.MaxUses, *q.ApprovalRequired, q.MembershipGeneration, q.RoleGeneration, issuer.Security, issuer.Epoch)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE conversation.conversations SET version=version+1,updated_at=now() WHERE id=$1`, r.PathValue("id")); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"requestId": q.RequestID, "id": id, "token": a.groupInviteToken(id), "expiresAt": q.ExpiresAt, "maxUses": q.MaxUses, "approvalRequired": *q.ApprovalRequired})
	return nil
}

func (a *App) groupInviteRevoke(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	id := r.PathValue("inviteId")
	var q groupManagerSnapshot
	if err = decode(r, &q); err != nil {
		return err
	}
	if !validObjectID(id) || !validObjectID(r.PathValue("id")) || !validReceiptNonce(q.RequestID) {
		return fail(400, "INVALID_GROUP_INVITE", "Original invite required")
	}
	raw, _ := json.Marshal(map[string]any{"action": "invite_revoke", "targetId": id, "body": q})
	digest := tokenDigest(string(raw))
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	version, state, err := lockGroupManagerVersion(r, tx)
	if err != nil {
		return err
	}
	previous, err := readManagerGroupOperation(r, tx, u.ID, q.RequestID, digest)
	if err != nil {
		return err
	}
	if previous != nil {
		return writeManagerGroupResult(w, previous)
	}
	if err = checkGroupManagerSnapshot(r, tx, u.ID, q, version, state); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.group_invites WHERE conversation_id=$1 AND id=$2)`, r.PathValue("id"), id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fail(404, "NOT_FOUND", "Invite unavailable")
	}
	payload := map[string]any{"conversationId": r.PathValue("id"), "userId": u.ID, "inviteId": id}
	operation := "group-invite-revoke:" + stableID(a.Config.Key, "group-invite-revoke:"+r.PathValue("id")+":"+u.ID+":"+q.RequestID)
	if err = a.protect(r.Context(), operation, "intent", "conversation.group_invite_revoke", payload); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE conversation.group_invites SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE conversation.conversations SET version=version+1,updated_at=now() WHERE id=$1`, r.PathValue("id")); err != nil {
		return err
	}
	result := map[string]any{"status": "applied", "requestId": q.RequestID, "action": "invite_revoke", "targetId": id, "state": "revoked"}
	encoded, err := saveManagerGroupOperation(r, tx, u.ID, q.RequestID, digest, result)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	if err = a.protect(r.Context(), operation, "result", "conversation.group_invite_revoke", payload); err != nil {
		return err
	}
	return writeManagerGroupResult(w, encoded)
}

func (a *App) groupJoinRequest(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	var q struct {
		RequestID string `json:"requestId"`
		InviteID  string `json:"inviteId"`
		Token     string `json:"token"`
	}
	if err = decode(r, &q); err != nil {
		return err
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 || !validObjectID(q.InviteID) || !sameAdmission(q.Token, a.groupInviteToken(q.InviteID)) {
		return fail(403, "GROUP_INVITE_REQUIRED", "Valid purpose-bound group invitation required")
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var invite, oldID, state string
	var generation *int64
	err = tx.QueryRow(r.Context(), `SELECT id::text,invite_id::text,state,membership_generation FROM conversation.group_join_requests WHERE conversation_id=$1 AND user_id=$2 AND request_id=$3`, r.PathValue("id"), u.ID, q.RequestID).Scan(&oldID, &invite, &state, &generation)
	if err == nil {
		if invite != q.InviteID {
			return fail(409, "GROUP_JOIN_CONFLICT", "Original invitation differs")
		}
		respond(w, 200, map[string]any{"requestId": q.RequestID, "id": oldID, "state": state, "membershipGeneration": nullableGroupGeneration(generation)})
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	var groupState string
	if err = tx.QueryRow(r.Context(), `SELECT state FROM conversation.conversations WHERE id=$1 AND kind='group' FOR UPDATE`, r.PathValue("id")).Scan(&groupState); err != nil || groupState != "active" {
		return fail(409, "GROUP_READ_ONLY", "Active group required")
	}
	var approval bool
	var uses int
	err = tx.QueryRow(r.Context(), `SELECT approval_required,uses FROM conversation.group_invites WHERE id=$1 AND conversation_id=$2 AND revoked_at IS NULL AND expires_at>clock_timestamp() AND uses<max_uses FOR UPDATE`, q.InviteID, r.PathValue("id")).Scan(&approval, &uses)
	if err != nil {
		return fail(403, "GROUP_INVITE_UNAVAILABLE", "Invitation expired, exhausted or revoked")
	}
	if err = a.verifyGroupInviteIssuer(r, tx, q.InviteID); err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active')`, r.PathValue("id"), u.ID).Scan(&active); err != nil {
		return err
	}
	if active {
		return fail(409, "ALREADY_A_MEMBER", "Current membership already exists")
	}
	var pending bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.group_join_requests WHERE conversation_id=$1 AND user_id=$2 AND state='pending')`, r.PathValue("id"), u.ID).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return fail(409, "GROUP_JOIN_PENDING", "Reconcile the original pending application")
	}
	id := stableID(a.Config.Key, "group-join:"+u.ID+":"+q.RequestID)
	payload := map[string]any{"conversationId": r.PathValue("id"), "userId": u.ID, "inviteId": q.InviteID, "groupInviteUses": strconv.Itoa(uses + 1)}
	operation := "group-invite-use:" + id
	if err = a.protect(r.Context(), operation, "intent", "conversation.group_invite_use", payload); err != nil {
		return err
	}
	state = "pending"
	if !approval {
		gen, e := a.joinGroupTx(r, tx, u.ID)
		if e != nil {
			return e
		}
		generation = &gen
		state = "approved"
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO conversation.group_join_requests(id,conversation_id,invite_id,user_id,request_id,state,membership_generation) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, r.PathValue("id"), q.InviteID, u.ID, q.RequestID, state, generation)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE conversation.group_invites SET uses=uses+1 WHERE id=$1`, q.InviteID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	if err = a.protect(r.Context(), operation, "result", "conversation.group_invite_use", payload); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"requestId": q.RequestID, "id": id, "state": state, "membershipGeneration": nullableGroupGeneration(generation)})
	return nil
}

func (a *App) joinGroupTx(r *http.Request, tx pgx.Tx, user string) (int64, error) {
	var count, max int
	err := tx.QueryRow(r.Context(), `SELECT count(*),COALESCE((SELECT max_members FROM conversation.group_settings WHERE conversation_id=$1),200) FROM conversation.members WHERE conversation_id=$1 AND state='active'`, r.PathValue("id")).Scan(&count, &max)
	if err != nil {
		return 0, err
	}
	if count >= max {
		return 0, fail(409, "GROUP_FULL", "Current group capacity reached")
	}
	var generation int64
	err = tx.QueryRow(r.Context(), `INSERT INTO conversation.members(conversation_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT(conversation_id,user_id) DO UPDATE SET role='member',state='active',membership_generation=conversation.members.membership_generation+1,role_generation=conversation.members.role_generation+1,joined_at=clock_timestamp(),export_pending_operation_id=NULL WHERE conversation.members.state<>'active' RETURNING membership_generation`, r.PathValue("id"), user).Scan(&generation)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO conversation.membership_periods(conversation_id,user_id,generation,joined_at) SELECT conversation_id,user_id,membership_generation,joined_at FROM conversation.members WHERE conversation_id=$1 AND user_id=$2`, r.PathValue("id"), user)
	return generation, err
}

func (a *App) groupJoinList(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.requireGroupManager(r); err != nil {
		return err
	}
	after, limit, err := groupPage(r)
	if err != nil {
		return err
	}
	rows, err := a.DB.Query(r.Context(), `SELECT id::text,user_id::text,state,created_at FROM conversation.group_join_requests WHERE conversation_id=$1 AND ($2='' OR id>NULLIF($2,'')::uuid) ORDER BY id LIMIT $3`, r.PathValue("id"), after, limit+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, user, state string
		var at time.Time
		if err = rows.Scan(&id, &user, &state, &at); err != nil {
			return err
		}
		items = append(items, map[string]any{"id": id, "userId": user, "state": state, "createdAt": at})
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = stringValue(items[limit-1], "id")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		id := stringValue(item, "userId")
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	var profiles struct {
		Items []User `json:"items"`
	}
	if len(ids) > 0 {
		if err = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": ids, "includeUnavailable": true, "skipUnavailable": true}, &profiles); err != nil {
			return err
		}
	}
	byID := map[string]User{}
	for _, profile := range profiles.Items {
		byID[profile.ID] = profile
	}
	for _, item := range items {
		profile, found := byID[stringValue(item, "userId")]
		item["username"], item["displayName"], item["accountState"] = profile.Username, profile.DisplayName, profile.State
		if !found {
			item["accountState"] = "unavailable"
		} else if profile.State == "" {
			item["accountState"] = "active"
		}
	}
	respond(w, 200, map[string]any{"items": items, "nextCursor": next})
	return nil
}

func (a *App) groupJoinDecide(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	var q struct {
		groupManagerSnapshot
		Approve *bool `json:"approve"`
	}
	if err := decode(r, &q); err != nil {
		return err
	}
	if q.Approve == nil || !validObjectID(r.PathValue("requestId")) || !validObjectID(r.PathValue("id")) || !validReceiptNonce(q.RequestID) {
		return fail(400, "INVALID_GROUP_DECISION", "Original application and explicit decision required")
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	version, groupState, err := lockGroupManagerVersion(r, tx)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{"action": "join_decide", "targetId": r.PathValue("requestId"), "body": q})
	digest := tokenDigest(string(raw))
	previous, err := readManagerGroupOperation(r, tx, u.ID, q.RequestID, digest)
	if err != nil {
		return err
	}
	if previous != nil {
		return writeManagerGroupResult(w, previous)
	}
	if err = checkGroupManagerSnapshot(r, tx, u.ID, q.groupManagerSnapshot, version, groupState); err != nil {
		return err
	}
	finish := func(state string, generation *int64) error {
		if _, e := tx.Exec(r.Context(), `UPDATE conversation.conversations SET version=version+1,updated_at=now() WHERE id=$1`, r.PathValue("id")); e != nil {
			return e
		}
		result := map[string]any{"status": "applied", "requestId": q.RequestID, "action": "join_decide", "targetId": r.PathValue("requestId"), "state": state, "membershipGeneration": nullableGroupGeneration(generation)}
		encoded, e := saveManagerGroupOperation(r, tx, u.ID, q.RequestID, digest, result)
		if e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
		return writeManagerGroupResult(w, encoded)
	}
	var user, state string
	var generation *int64
	err = tx.QueryRow(r.Context(), `SELECT user_id::text,state,membership_generation FROM conversation.group_join_requests WHERE conversation_id=$1 AND id=$2 FOR UPDATE`, r.PathValue("id"), r.PathValue("requestId")).Scan(&user, &state, &generation)
	if err != nil {
		return fail(404, "NOT_FOUND", "Original join application unavailable")
	}
	desired := "rejected"
	if *q.Approve {
		desired = "approved"
	}
	if state != "pending" {
		if state != desired {
			return fail(409, "GROUP_JOIN_ALREADY_DECIDED", "Original application has another decision")
		}
		return finish(state, generation)
	}
	if *q.Approve {
		var live bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.group_join_requests j JOIN conversation.group_invites i ON i.id=j.invite_id WHERE j.id=$1 AND i.revoked_at IS NULL AND i.expires_at>clock_timestamp() AND i.uses<=i.max_uses)`, r.PathValue("requestId")).Scan(&live); err != nil {
			return err
		}
		if !live {
			return fail(409, "GROUP_INVITE_UNAVAILABLE", "Original invitation is no longer valid for approval")
		}
		var invitation string
		if err = tx.QueryRow(r.Context(), `SELECT invite_id::text FROM conversation.group_join_requests WHERE id=$1 AND conversation_id=$2`, r.PathValue("requestId"), r.PathValue("id")).Scan(&invitation); err != nil {
			return err
		}
		if err = a.verifyGroupInviteIssuer(r, tx, invitation); err != nil {
			return err
		}
		var users struct {
			Items []User `json:"items"`
		}
		if err = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": []string{user}}, &users); err != nil {
			return err
		}
		if len(users.Items) != 1 || users.Items[0].ID != user || (users.Items[0].State != "" && users.Items[0].State != "active") {
			return fail(409, "GROUP_TARGET_UNAVAILABLE", "Current active applicant required")
		}
		gen, e := a.joinGroupTx(r, tx, user)
		if e != nil {
			return e
		}
		generation = &gen
	}
	if _, err = tx.Exec(r.Context(), `UPDATE conversation.group_join_requests SET state=$3,membership_generation=$4 WHERE conversation_id=$1 AND id=$2`, r.PathValue("id"), r.PathValue("requestId"), desired, generation); err != nil {
		return err
	}
	return finish(desired, generation)
}

func (a *App) groupInviteResult(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	if !validObjectID(r.PathValue("id")) || !validReceiptNonce(r.PathValue("requestId")) {
		return fail(400, "INVALID_GROUP_OPERATION", "Original group and invitation request required")
	}
	var id string
	var expires time.Time
	var max, uses int
	var approval bool
	var revoked *time.Time
	err = a.DB.QueryRow(r.Context(), `SELECT id::text,expires_at,max_uses,uses,approval_required,revoked_at FROM conversation.group_invites WHERE conversation_id=$1 AND creator_id=$2 AND request_id=$3`, r.PathValue("id"), u.ID, r.PathValue("requestId")).Scan(&id, &expires, &max, &uses, &approval, &revoked)
	if err == pgx.ErrNoRows {
		return fail(404, "GROUP_OPERATION_NOT_FOUND", "Original invitation has no committed result")
	}
	if err != nil {
		return err
	}
	respond(w, 200, map[string]any{"requestId": r.PathValue("requestId"), "id": id, "token": a.groupInviteToken(id), "expiresAt": expires, "maxUses": max, "uses": uses, "approvalRequired": approval, "revokedAt": revoked})
	return nil
}

func (a *App) groupJoinResult(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	if !validObjectID(r.PathValue("id")) || !validReceiptNonce(r.PathValue("requestId")) {
		return fail(400, "INVALID_GROUP_OPERATION", "Original group and join request required")
	}
	var id, state string
	var generation *int64
	err = a.DB.QueryRow(r.Context(), `SELECT id::text,state,membership_generation FROM conversation.group_join_requests WHERE conversation_id=$1 AND user_id=$2 AND request_id=$3`, r.PathValue("id"), u.ID, r.PathValue("requestId")).Scan(&id, &state, &generation)
	if err == pgx.ErrNoRows {
		return fail(404, "GROUP_OPERATION_NOT_FOUND", "Original join request has no committed result")
	}
	if err != nil {
		return err
	}
	respond(w, 200, map[string]any{"requestId": r.PathValue("requestId"), "id": id, "state": state, "membershipGeneration": nullableGroupGeneration(generation)})
	return nil
}

func nullableGroupGeneration(value *int64) any {
	if value == nil {
		return nil
	}
	return str(*value)
}
