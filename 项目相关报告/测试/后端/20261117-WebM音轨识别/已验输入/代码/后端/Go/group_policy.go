package jobsim

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["conversation"] += `
CREATE TABLE IF NOT EXISTS conversation.group_settings(conversation_id uuid PRIMARY KEY REFERENCES conversation.conversations(id),policy_version bigint NOT NULL DEFAULT 1,mute_all boolean NOT NULL DEFAULT false,slow_seconds integer NOT NULL DEFAULT 0 CHECK(slow_seconds BETWEEN 0 AND 3600),max_members integer NOT NULL DEFAULT 200 CHECK(max_members BETWEEN 2 AND 10000));
CREATE TABLE IF NOT EXISTS conversation.group_mutes(conversation_id uuid NOT NULL,user_id uuid NOT NULL,membership_generation bigint NOT NULL,muted_until timestamptz NOT NULL,PRIMARY KEY(conversation_id,user_id));
CREATE TABLE IF NOT EXISTS conversation.group_operations(conversation_id uuid NOT NULL,actor_id uuid NOT NULL,request_id text NOT NULL,digest bytea NOT NULL,result jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(conversation_id,actor_id,request_id));
CREATE TABLE IF NOT EXISTS conversation.governance_grants(user_id uuid PRIMARY KEY,permission text NOT NULL CHECK(permission='recover_group'),operation_id text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());`
	migrations["message"] += `CREATE TABLE IF NOT EXISTS message.group_send_rates(conversation_id uuid NOT NULL,user_id uuid NOT NULL,accepted_at timestamptz NOT NULL,PRIMARY KEY(conversation_id,user_id));`
}

type groupPolicy struct {
	Version     string `json:"version"`
	MuteAll     bool   `json:"muteAll"`
	SlowSeconds int    `json:"slowSeconds"`
	MaxMembers  int    `json:"maxMembers"`
}

func (a *App) readGroupPolicy(ctx context.Context, id string) (groupPolicy, error) {
	p := groupPolicy{Version: "1", MaxMembers: 200}
	err := a.DB.QueryRow(ctx, `SELECT policy_version::text,mute_all,slow_seconds,max_members FROM conversation.group_settings WHERE conversation_id=$1`, id).Scan(&p.Version, &p.MuteAll, &p.SlowSeconds, &p.MaxMembers)
	if err == pgx.ErrNoRows {
		err = nil
	}
	return p, err
}

// Every policy and individual restriction has a monotone admission version.
// Expiry allows a fresh send, but never makes a previously rejected token valid.
func (a *App) groupAdmission(ctx context.Context, id, user, role string, membership int64) (string, bool, groupPolicy, error) {
	p, err := a.readGroupPolicy(ctx, id)
	if err != nil {
		return "", false, p, err
	}
	var muted bool
	err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversation.group_mutes WHERE conversation_id=$1 AND user_id=$2 AND membership_generation=$3 AND muted_until>clock_timestamp())`, id, user, membership).Scan(&muted)
	allowed := !muted && (!p.MuteAll || role == "owner" || role == "admin")
	return "group-policy:" + p.Version, allowed, p, err
}

// The rate is charged in the same transaction as a newly committed message.
// Independent devices share this row; rollback and nonce reconciliation cost nothing.
func (a *App) enforceGroupRate(ctx context.Context, tx pgx.Tx, meta map[string]any, user, conversation string) error {
	if meta["kind"] != "group" || meta["role"] == "owner" || meta["role"] == "admin" {
		return nil
	}
	seconds, err := strconv.Atoi(stringValue(meta, "slowSeconds"))
	if err != nil || seconds < 0 || seconds > 3600 {
		return fail(503, "GROUP_POLICY_UNAVAILABLE", "Current group send policy unavailable")
	}
	if seconds == 0 {
		return nil
	}
	var accepted bool
	err = tx.QueryRow(ctx, `INSERT INTO message.group_send_rates(conversation_id,user_id,accepted_at) VALUES($1,$2,clock_timestamp()) ON CONFLICT(conversation_id,user_id) DO UPDATE SET accepted_at=clock_timestamp() WHERE message.group_send_rates.accepted_at+make_interval(secs=>$3)<=clock_timestamp() RETURNING true`, conversation, user, seconds).Scan(&accepted)
	if err == pgx.ErrNoRows {
		return fail(429, "GROUP_SLOW_MODE", "Wait for the group send interval before creating another message")
	}
	return err
}

type groupMutation struct {
	RequestID                  string `json:"requestId"`
	Version                    string `json:"version"`
	MembershipGeneration       string `json:"membershipGeneration"`
	RoleGeneration             string `json:"roleGeneration"`
	Action                     string `json:"action"`
	TargetID                   string `json:"targetId,omitempty"`
	TargetMembershipGeneration string `json:"targetMembershipGeneration,omitempty"`
	TargetRoleGeneration       string `json:"targetRoleGeneration,omitempty"`
	Role                       string `json:"role,omitempty"`
	MutedUntil                 string `json:"mutedUntil,omitempty"`
	MuteAll                    *bool  `json:"muteAll,omitempty"`
	SlowSeconds                *int   `json:"slowSeconds,omitempty"`
	MaxMembers                 *int   `json:"maxMembers,omitempty"`
	AuthorizationID            string `json:"stepUpId,omitempty"`
}

func groupMutationDigest(q groupMutation) []byte {
	q.AuthorizationID = ""
	raw, _ := json.Marshal(q)
	return tokenDigest(string(raw))
}

func (a *App) groupRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/conversations/{id}/governance", a.groupGovernanceRead)
	h("POST /api/conversations/{id}/governance", a.groupGovernanceWrite)
	h("GET /api/conversations/{id}/governance/operations/{requestId}", a.groupGovernanceResult)
	a.groupInviteRoutes(h)
}

func (a *App) groupGovernanceRead(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	c, err := a.conversationByID(r, r.PathValue("id"), u.ID)
	if err != nil {
		return err
	}
	if c.Kind != "group" {
		return fail(400, "GROUP_REQUIRED", "Group conversation required")
	}
	p, err := a.readGroupPolicy(r.Context(), c.ID)
	if err != nil {
		return err
	}
	var version, role string
	var muted *time.Time
	err = a.DB.QueryRow(r.Context(), `SELECT c.version::text,m.role,(SELECT muted_until FROM conversation.group_mutes x WHERE x.conversation_id=c.id AND x.user_id=m.user_id AND x.membership_generation=m.membership_generation) FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id WHERE c.id=$1 AND m.user_id=$2 AND m.state='active'`, c.ID, u.ID).Scan(&version, &role, &muted)
	if err != nil {
		return err
	}
	var canRecover bool
	if u.Admin && c.State == "owner_unavailable" {
		if err = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.governance_grants WHERE user_id=$1 AND permission='recover_group')`, u.ID).Scan(&canRecover); err != nil {
			return err
		}
	}
	respond(w, 200, map[string]any{"canRecover": canRecover, "policy": p, "version": version, "role": role, "state": c.State, "readOnly": c.ReadOnly, "membershipGeneration": c.MembershipGeneration, "roleGeneration": c.RoleGeneration, "mutedUntil": muted, "privilegedRolesExempt": true})
	return nil
}

func (a *App) groupGovernanceWrite(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	var q groupMutation
	raw, err := readAdminBody(r, &q)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if !validObjectID(id) || len(q.RequestID) < 8 || len(q.RequestID) > 128 || !syncValidationCounter(q.Version, false) || q.Action != "recover" && (!syncValidationCounter(q.MembershipGeneration, false) || !syncValidationCounter(q.RoleGeneration, false)) {
		return fail(400, "INVALID_GROUP_OPERATION", "Stable original nonce and permission versions required")
	}
	digest := groupMutationDigest(q)
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var owner, state, actorRole string
	var version, membership, roleGeneration int64
	err = tx.QueryRow(r.Context(), `SELECT owner_id::text,state,version FROM conversation.conversations WHERE id=$1 AND kind='group' FOR UPDATE`, id).Scan(&owner, &state, &version)
	if err != nil {
		return fail(403, "NOT_A_MEMBER", "Current group membership required")
	}
	var previous, oldDigest []byte
	err = tx.QueryRow(r.Context(), `SELECT digest,result FROM conversation.group_operations WHERE conversation_id=$1 AND actor_id=$2 AND request_id=$3`, id, u.ID, q.RequestID).Scan(&oldDigest, &previous)
	if err == nil {
		if !bytesEqual(oldDigest, digest) {
			return fail(409, "GROUP_OPERATION_CONFLICT", "Original group request differs")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(200)
		_, err = w.Write(previous)
		return err
	}
	if err != pgx.ErrNoRows {
		return err
	}
	if q.Action != "recover" {
		err = tx.QueryRow(r.Context(), `SELECT role,membership_generation,role_generation FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active' AND export_pending_operation_id IS NULL FOR UPDATE`, id, u.ID).Scan(&actorRole, &membership, &roleGeneration)
		if err != nil {
			return fail(403, "NOT_A_MEMBER", "Current group membership required")
		}
	}
	if str(version) != q.Version || q.Action != "recover" && (str(membership) != q.MembershipGeneration || str(roleGeneration) != q.RoleGeneration) {
		return fail(409, "GROUP_PERMISSION_CHANGED", "Reload current group permissions before creating a new operation")
	}
	if state != "active" && !(q.Action == "recover" && state == "owner_unavailable") {
		return fail(409, "GROUP_READ_ONLY", "Group is read-only")
	}
	if q.Action != "recover" && actorRole != "owner" && actorRole != "admin" {
		return fail(403, "GROUP_ADMIN_REQUIRED", "Current group administrator required")
	}
	if (q.Action == "settings" || q.Action == "role" || q.Action == "transfer" || q.Action == "dissolve") && actorRole != "owner" {
		return fail(403, "OWNER_REQUIRED", "Current group owner required")
	}
	newVersion := version + 1
	payload := map[string]any{"conversationId": id, "userId": u.ID, "version": str(newVersion)}
	kind := ""
	switch q.Action {
	case "settings":
		if q.MuteAll == nil || q.SlowSeconds == nil || q.MaxMembers == nil || *q.SlowSeconds < 0 || *q.SlowSeconds > 3600 || *q.MaxMembers < 2 || *q.MaxMembers > 10000 {
			return fail(400, "INVALID_GROUP_POLICY", "Explicit mute, interval and bounded capacity required")
		}
		var count int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM conversation.members WHERE conversation_id=$1 AND state='active'`, id).Scan(&count); err != nil {
			return err
		}
		if *q.MaxMembers < count {
			return fail(409, "GROUP_CAPACITY_BELOW_MEMBERS", "Capacity cannot discard current members")
		}
		kind = "conversation.group_policy"
		payload["groupMuteAll"] = strconv.FormatBool(*q.MuteAll)
		payload["groupSlowSeconds"] = strconv.Itoa(*q.SlowSeconds)
		payload["groupMaxMembers"] = strconv.Itoa(*q.MaxMembers)
	case "mute", "role":
		if !validObjectID(q.TargetID) || !syncValidationCounter(q.TargetMembershipGeneration, false) || !syncValidationCounter(q.TargetRoleGeneration, false) {
			return fail(400, "INVALID_GROUP_TARGET", "Original target membership and role versions required")
		}
		var targetRole string
		var targetMembership, targetRoleGeneration int64
		err = tx.QueryRow(r.Context(), `SELECT role,membership_generation,role_generation FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active' AND export_pending_operation_id IS NULL FOR UPDATE`, id, q.TargetID).Scan(&targetRole, &targetMembership, &targetRoleGeneration)
		if err != nil || str(targetMembership) != q.TargetMembershipGeneration || str(targetRoleGeneration) != q.TargetRoleGeneration {
			return fail(409, "GROUP_TARGET_CHANGED", "Target membership or role changed")
		}
		if targetRole == "owner" || q.Action == "mute" && targetRole == "admin" && actorRole != "owner" {
			return fail(403, "GROUP_ROLE_BOUNDARY", "Owner and administrator role boundary required")
		}
		kind = "conversation.group_member"
		payload["targetId"] = q.TargetID
		payload["membershipGeneration"] = str(targetMembership)
		payload["groupRoleGeneration"] = str(targetRoleGeneration + 1)
		payload["groupRole"] = targetRole
		payload["groupMutedUntil"] = ""
		if q.Action == "role" {
			if q.Role != "admin" && q.Role != "member" {
				return fail(400, "INVALID_GROUP_ROLE", "Explicit administrator or member role required")
			}
			payload["groupRole"] = q.Role
		} else if q.MutedUntil != "" {
			until, e := time.Parse(time.RFC3339Nano, q.MutedUntil)
			if e != nil || !until.After(time.Now()) || until.After(time.Now().Add(365*24*time.Hour)) {
				return fail(400, "INVALID_GROUP_MUTE", "Future absolute mute deadline within one year required")
			}
			payload["groupMutedUntil"] = until.UTC().Format(time.RFC3339Nano)
		}
	case "transfer":
		if !validObjectID(q.TargetID) || q.TargetID == owner || !syncValidationCounter(q.TargetMembershipGeneration, false) || !syncValidationCounter(q.TargetRoleGeneration, false) {
			return fail(400, "INVALID_GROUP_TARGET", "Original active successor membership and role versions required")
		}
		var targetMembership, targetRole string
		err = tx.QueryRow(r.Context(), `SELECT membership_generation::text,role_generation::text FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active' AND export_pending_operation_id IS NULL FOR UPDATE`, id, q.TargetID).Scan(&targetMembership, &targetRole)
		if err != nil || targetMembership != q.TargetMembershipGeneration || targetRole != q.TargetRoleGeneration {
			return fail(409, "GROUP_TARGET_CHANGED", "Original successor membership or role changed")
		}
		var users struct {
			Items []User `json:"items"`
		}
		if err = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": []string{q.TargetID}}, &users); err != nil {
			return err
		}
		if len(users.Items) != 1 || users.Items[0].ID != q.TargetID || users.Items[0].State != "" && users.Items[0].State != "active" {
			return fail(409, "GROUP_TARGET_UNAVAILABLE", "Current active successor required")
		}
		kind = "conversation.group_transfer"
		payload["ownerId"] = owner
		payload["targetId"] = q.TargetID
		payload["membershipGeneration"] = q.TargetMembershipGeneration
	case "dissolve":
		kind = "conversation.group_dissolve"
	case "recover":
		var granted bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.governance_grants WHERE user_id=$1 AND permission='recover_group')`, u.ID).Scan(&granted); err != nil {
			return err
		}
		if !granted || !u.Admin {
			return fail(403, "INDEPENDENT_GOVERNANCE_REQUIRED", "Independent operator grant and current administrator authentication required")
		}
		var targetGeneration string
		err = tx.QueryRow(r.Context(), `SELECT membership_generation::text FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND state='active' AND export_pending_operation_id IS NULL`, id, q.TargetID).Scan(&targetGeneration)
		if err != nil || targetGeneration != q.TargetMembershipGeneration {
			return fail(409, "GROUP_TARGET_CHANGED", "Existing active successor membership required")
		}
		var account struct {
			Items []User `json:"items"`
		}
		if err = a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": []string{q.TargetID}}, &account); err != nil {
			return err
		}
		if len(account.Items) != 1 || account.Items[0].ID != q.TargetID || (account.Items[0].State != "" && account.Items[0].State != "active") {
			return fail(409, "GROUP_TARGET_UNAVAILABLE", "Current active successor account required")
		}
		if err = a.adminStepUp(r, u, q.AuthorizationID, "group.recover", id, q.RequestID, raw); err != nil {
			return err
		}
		kind = "conversation.group_recover"
		payload["ownerId"] = owner
		payload["targetId"] = q.TargetID
	default:
		return fail(400, "INVALID_GROUP_ACTION", "Known group governance action required")
	}
	operation := "group:" + stableID(a.Config.Key, "group-operation:"+id+":"+u.ID+":"+q.RequestID)
	if err = a.protect(r.Context(), operation, "intent", kind, payload); err != nil {
		return err
	}
	switch q.Action {
	case "settings":
		_, err = tx.Exec(r.Context(), `INSERT INTO conversation.group_settings(conversation_id,policy_version,mute_all,slow_seconds,max_members) VALUES($1,$2,$3,$4,$5) ON CONFLICT(conversation_id) DO UPDATE SET policy_version=$2,mute_all=$3,slow_seconds=$4,max_members=$5`, id, newVersion, *q.MuteAll, *q.SlowSeconds, *q.MaxMembers)
	case "mute", "role":
		_, err = tx.Exec(r.Context(), `UPDATE conversation.members SET role=$3,role_generation=$4 WHERE conversation_id=$1 AND user_id=$2`, id, q.TargetID, payload["groupRole"], payload["groupRoleGeneration"])
		if err == nil && q.Action == "mute" {
			if q.MutedUntil == "" {
				_, err = tx.Exec(r.Context(), `DELETE FROM conversation.group_mutes WHERE conversation_id=$1 AND user_id=$2`, id, q.TargetID)
			} else {
				_, err = tx.Exec(r.Context(), `INSERT INTO conversation.group_mutes(conversation_id,user_id,membership_generation,muted_until) VALUES($1,$2,$3,$4) ON CONFLICT(conversation_id,user_id) DO UPDATE SET membership_generation=$3,muted_until=$4`, id, q.TargetID, q.TargetMembershipGeneration, payload["groupMutedUntil"])
			}
		}
	case "dissolve":
		_, err = tx.Exec(r.Context(), `UPDATE conversation.conversations SET state='dissolved' WHERE id=$1`, id)
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE conversation.group_invites SET revoked_at=COALESCE(revoked_at,now()) WHERE conversation_id=$1`, id)
		}
	case "recover", "transfer":
		_, err = tx.Exec(r.Context(), `UPDATE conversation.members SET role=CASE WHEN user_id=$2 THEN 'owner' ELSE 'member' END,role_generation=role_generation+1 WHERE conversation_id=$1 AND (role='owner' OR user_id=$2)`, id, q.TargetID)
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE conversation.conversations SET owner_id=$2,state='active' WHERE id=$1`, id, q.TargetID)
		}
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `UPDATE conversation.conversations SET version=$2,updated_at=now() WHERE id=$1`, id, newVersion)
	if err != nil {
		return err
	}
	result := map[string]any{"status": "applied", "requestId": q.RequestID, "action": q.Action, "version": str(newVersion)}
	encoded, _ := json.Marshal(result)
	_, err = tx.Exec(r.Context(), `INSERT INTO conversation.group_operations(conversation_id,actor_id,request_id,digest,result) VALUES($1,$2,$3,$4,$5)`, id, u.ID, q.RequestID, digest, encoded)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	if err = a.protect(r.Context(), operation, "result", kind, payload); err != nil {
		return err
	}
	respond(w, 200, result)
	return nil
}

func (a *App) groupGovernanceResult(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	if !validObjectID(r.PathValue("id")) || !validReceiptNonce(r.PathValue("requestId")) {
		return fail(400, "INVALID_GROUP_OPERATION", "Original group and stable request identity required")
	}
	var result []byte
	err = a.DB.QueryRow(r.Context(), `SELECT result FROM conversation.group_operations WHERE conversation_id=$1 AND actor_id=$2 AND request_id=$3`, r.PathValue("id"), u.ID, r.PathValue("requestId")).Scan(&result)
	if err == pgx.ErrNoRows {
		return fail(404, "GROUP_OPERATION_NOT_FOUND", "Original group operation has no committed result")
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_, err = w.Write(result)
	return err
}
