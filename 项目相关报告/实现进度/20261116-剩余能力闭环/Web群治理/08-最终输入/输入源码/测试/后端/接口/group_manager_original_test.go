package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Owned random PG and real handlers; identity/journal transport dependencies
// are explicit fixtures. This does not claim a real browser or HTTPS realm.
type groupOriginalFixture struct {
	base   *retentionPGTransport
	app    *App
	user   User
	issuer groupIssuer
}

func (f *groupOriginalFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	w.Header().Set("X-JobsIM-Admission-Fence", f.base.message.admissionFenceIdentity())
	switch r.URL.Path {
	case "/internal/authorize":
		respond(w, 200, f.user)
	case "/internal/groups/issuer":
		respond(w, 200, f.issuer)
	case "/internal/member":
		f.app.Handler().ServeHTTP(w, r)
	case "/internal/users":
		var q struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			return nil, err
		}
		items := []map[string]any{}
		for _, id := range q.IDs {
			items = append(items, map[string]any{"id": id, "state": "active", "username": "applicant_" + id[:8], "displayName": "Owned applicant " + id[:8], "password": "FORBIDDEN-FIXTURE", "token": "FORBIDDEN-FIXTURE"})
		}
		respond(w, 200, map[string]any{"items": items})
	default:
		return f.base.RoundTrip(r)
	}
	return w.Result(), nil
}

func TestGroupManagerOriginalPGApplicationProfilesKeepStateAndPagination(t *testing.T) {
	ctx, a, f, id, _ := newGroupOriginalPG(t)
	invite := newID()
	retentionExec(t, ctx, a, `INSERT INTO conversation.group_invites(id,conversation_id,creator_id,request_id,digest,expires_at,max_uses,uses,approval_required) VALUES($1,$2,$3,$4,'list-fixture',now()+interval '1 hour',3,2,true)`, invite, id, f.user.ID, newID())
	for range 2 {
		retentionExec(t, ctx, a, `INSERT INTO conversation.group_join_requests(id,conversation_id,invite_id,user_id,request_id) VALUES($1,$2,$3,$4,$5)`, newID(), id, invite, newID(), newID())
	}
	status, first := callGroupOriginal(a, "GET", "/api/conversations/"+id+"/join-requests?limit=1", nil)
	if status != 200 {
		t.Fatal("public applicant page:", status)
	}
	items, ok := first["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatal("bounded applicant first page")
	}
	applicant := items[0].(map[string]any)
	if applicant["displayName"] == "" || applicant["username"] == "" || applicant["state"] != "pending" || applicant["accountState"] != "active" {
		t.Fatal("applicant profile overwrote its application state")
	}
	raw, _ := json.Marshal(first)
	if bytes.Contains(raw, []byte("FORBIDDEN-FIXTURE")) || bytes.Contains(raw, []byte("password")) || bytes.Contains(raw, []byte("token")) {
		t.Fatal("private upstream profile fields reached group approval page")
	}
	cursor, ok := first["nextCursor"].(string)
	if !ok || !validObjectID(cursor) {
		t.Fatal("next application cursor unavailable")
	}
	status, next := callGroupOriginal(a, "GET", "/api/conversations/"+id+"/join-requests?limit=1&after="+cursor, nil)
	if status != 200 || len(next["items"].([]any)) != 1 || next["items"].([]any)[0].(map[string]any)["id"] == applicant["id"] || next["nextCursor"] != "" {
		t.Fatal("applicant pagination lost or duplicated a row")
	}
}
func newGroupOriginalPG(t *testing.T) (context.Context, *App, *groupOriginalFixture, string, string) {
	t.Helper()
	ctx, m, _, base := newRetentionPG(t)
	a := &App{Config: m.Config, DB: m.DB, Role: "conversation"}
	if err := a.migrate(ctx); err != nil {
		t.Fatal("production group migration:", err)
	}
	f := &groupOriginalFixture{base: base, app: a, user: User{ID: newID(), DeviceID: newID(), RestoreEpoch: "1"}, issuer: groupIssuer{Active: true, Security: "1", Epoch: "1"}}
	a.Client = &http.Client{Transport: f}
	id, target := newID(), newID()
	retentionExec(t, ctx, a, `INSERT INTO conversation.conversations(id,kind,title,owner_id) VALUES($1,'group','owned manager original',$2)`, id, f.user.ID)
	retentionExec(t, ctx, a, `INSERT INTO conversation.members(conversation_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')`, id, f.user.ID, target)
	return ctx, a, f, id, target
}
func callGroupOriginal(a *App, method, path string, body any) (int, map[string]any) {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	var result map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w.Code, result
}
func managerBody(version, role string) map[string]any {
	return map[string]any{"requestId": newID(), "version": version, "membershipGeneration": "1", "roleGeneration": role}
}
func TestGroupManagerOriginalPGInviteCreationRejectsRoleABA(t *testing.T) {
	ctx, a, f, id, _ := newGroupOriginalPG(t)
	path := "/api/conversations/" + id + "/invites"
	body := managerBody("1", "1")
	body["expiresAt"] = time.Now().Add(time.Hour).UTC()
	body["maxUses"] = 2
	body["approvalRequired"] = true
	status, first := callGroupOriginal(a, "POST", path, body)
	if status != 200 {
		t.Fatal("exact original invite:", status)
	}
	retentionExec(t, ctx, a, `UPDATE conversation.members SET role_generation=3 WHERE conversation_id=$1 AND user_id=$2`, id, f.user.ID)
	status, replayed := callGroupOriginal(a, "POST", path, body)
	if status != 200 || replayed["id"] != first["id"] {
		t.Fatal("later role changed own committed invite result")
	}
	body["requestId"] = newID()
	body["version"] = "2"
	status, _ = callGroupOriginal(a, "POST", path, body)
	if status != 409 {
		t.Fatal("old actor role generation authorized new invite after ABA")
	}
	var count int
	if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM conversation.group_invites WHERE conversation_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("rejected invite created another resource")
	}
}
func TestGroupManagerOriginalPGRevokeReadbackIsActorAndNonceBound(t *testing.T) {
	ctx, a, f, id, target := newGroupOriginalPG(t)
	create := managerBody("1", "1")
	create["expiresAt"] = time.Now().Add(time.Hour).UTC()
	create["maxUses"] = 2
	create["approvalRequired"] = true
	_, invite := callGroupOriginal(a, "POST", "/api/conversations/"+id+"/invites", create)
	inviteID, _ := invite["id"].(string)
	if !validObjectID(inviteID) {
		t.Fatal("owned invite unavailable")
	}
	body := managerBody("2", "1")
	path := "/api/conversations/" + id + "/invites/" + inviteID
	status, result := callGroupOriginal(a, "DELETE", path, body)
	if status != 200 || result["action"] != "invite_revoke" || result["targetId"] != inviteID || result["state"] != "revoked" {
		t.Fatal("typed original revoke result:", status)
	}
	retentionExec(t, ctx, a, `UPDATE conversation.members SET role_generation=3 WHERE conversation_id=$1 AND user_id=$2`, id, f.user.ID)
	status, _ = callGroupOriginal(a, "DELETE", path, body)
	if status != 200 {
		t.Fatal("committed revoke reran old role authorization")
	}
	lookup := "/api/conversations/" + id + "/governance/operations/" + body["requestId"].(string)
	status, found := callGroupOriginal(a, "GET", lookup, nil)
	if status != 200 || found["requestId"] != body["requestId"] {
		t.Fatal("own revoke result not available by original nonce")
	}
	f.user.ID = target
	status, _ = callGroupOriginal(a, "GET", lookup, nil)
	if status != 404 {
		t.Fatal("another actor read manager original result")
	}
}
func TestGroupManagerOriginalPGApprovalPreservesDecisionAndLargeGeneration(t *testing.T) {
	ctx, a, f, id, target := newGroupOriginalPG(t)
	invite, application := newID(), newID()
	retentionExec(t, ctx, a, `INSERT INTO conversation.group_invites(id,conversation_id,creator_id,request_id,digest,expires_at,max_uses,uses,approval_required,creator_membership_generation,creator_role_generation,creator_security_version,creator_restore_epoch) VALUES($1,$2,$3,$4,'owned',now()+interval '1 hour',3,1,true,1,1,1,1)`, invite, id, f.user.ID, newID())
	retentionExec(t, ctx, a, `INSERT INTO conversation.group_join_requests(id,conversation_id,invite_id,user_id,request_id) VALUES($1,$2,$3,$4,$5)`, application, id, invite, target, newID())
	body := managerBody("1", "1")
	body["approve"] = false
	path := "/api/conversations/" + id + "/join-requests/" + application
	status, result := callGroupOriginal(a, "POST", path, body)
	if status != 200 || result["action"] != "join_decide" || result["state"] != "rejected" {
		t.Fatal("typed manager original decision:", status)
	}
	retentionExec(t, ctx, a, `UPDATE conversation.members SET role_generation=3 WHERE conversation_id=$1 AND user_id=$2`, id, f.user.ID)
	status, _ = callGroupOriginal(a, "POST", path, body)
	if status != 200 {
		t.Fatal("own committed decision reran later role")
	}
	body["approve"] = true
	status, _ = callGroupOriginal(a, "POST", path, body)
	if status != 409 {
		t.Fatal("same manager nonce changed its original decision")
	}
	application = newID()
	invite = newID()
	retentionExec(t, ctx, a, `INSERT INTO conversation.group_invites(id,conversation_id,creator_id,request_id,digest,expires_at,max_uses,uses,approval_required,creator_membership_generation,creator_role_generation,creator_security_version,creator_restore_epoch) VALUES($1,$2,$3,$4,'owned-new',now()+interval '1 hour',3,1,true,1,3,1,1)`, invite, id, f.user.ID, newID())
	retentionExec(t, ctx, a, `INSERT INTO conversation.group_join_requests(id,conversation_id,invite_id,user_id,request_id) VALUES($1,$2,$3,$4,$5)`, application, id, invite, target, newID())
	retentionExec(t, ctx, a, `UPDATE conversation.members SET state='left',membership_generation=9007199254740992 WHERE conversation_id=$1 AND user_id=$2`, id, target)
	body = managerBody("2", "3")
	body["approve"] = true
	status, result = callGroupOriginal(a, "POST", "/api/conversations/"+id+"/join-requests/"+application, body)
	if status != 200 || result["membershipGeneration"] != "9007199254740993" {
		t.Fatal("approved membership generation lost exact string identity:", status)
	}
}

func TestGroupManagerOriginalPGIssuerChangesInvalidateUnconsumedInvites(t *testing.T) {
	for _, change := range []string{"membership", "role", "security", "epoch", "inactive"} {
		t.Run(change, func(t *testing.T) {
			ctx, a, f, id, _ := newGroupOriginalPG(t)
			creator := f.user.ID
			body := managerBody("1", "1")
			body["expiresAt"] = time.Now().Add(time.Hour).UTC()
			body["maxUses"] = 2
			body["approvalRequired"] = true
			status, invite := callGroupOriginal(a, "POST", "/api/conversations/"+id+"/invites", body)
			if status != 200 {
				t.Fatal("original issuer fixture:", status)
			}
			switch change {
			case "membership":
				retentionExec(t, ctx, a, `UPDATE conversation.members SET membership_generation=2 WHERE conversation_id=$1 AND user_id=$2`, id, creator)
			case "role":
				retentionExec(t, ctx, a, `UPDATE conversation.members SET role_generation=3 WHERE conversation_id=$1 AND user_id=$2`, id, creator)
			case "security":
				f.issuer.Security = "2"
			case "epoch":
				f.issuer.Epoch = "2"
			case "inactive":
				f.issuer.Active = false
			}
			f.user.ID = newID()
			status, _ = callGroupOriginal(a, "POST", "/api/conversations/"+id+"/join-requests", map[string]any{"requestId": newID(), "inviteId": invite["id"], "token": invite["token"]})
			if status != 403 {
				t.Fatal("changed issuer delegation admitted new membership:", change, status)
			}
			var count int
			if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM conversation.group_join_requests WHERE conversation_id=$1`, id).Scan(&count); err != nil || count != 0 {
				t.Fatal("invalidated creator produced a pending application")
			}
		})
	}
}

func TestGroupManagerOriginalPGApprovalRechecksOriginalIssuer(t *testing.T) {
	ctx, a, f, id, admin := newGroupOriginalPG(t)
	body := managerBody("1", "1")
	body["expiresAt"] = time.Now().Add(time.Hour).UTC()
	body["maxUses"] = 2
	body["approvalRequired"] = true
	_, invite := callGroupOriginal(a, "POST", "/api/conversations/"+id+"/invites", body)
	f.user.ID = newID()
	status, joined := callGroupOriginal(a, "POST", "/api/conversations/"+id+"/join-requests", map[string]any{"requestId": newID(), "inviteId": invite["id"], "token": invite["token"]})
	if status != 200 || joined["state"] != "pending" {
		t.Fatal("original pending application:", status)
	}
	retentionExec(t, ctx, a, `UPDATE conversation.members SET role='admin',role_generation=2 WHERE conversation_id=$1 AND user_id=$2`, id, admin)
	f.issuer.Active = false
	f.user.ID = admin
	decision := managerBody("2", "2")
	decision["approve"] = true
	status, _ = callGroupOriginal(a, "POST", "/api/conversations/"+id+"/join-requests/"+joined["id"].(string), decision)
	if status != 403 {
		t.Fatal("another current administrator approved a revoked creator delegation:", status)
	}
	var state string
	if err := a.DB.QueryRow(ctx, `SELECT state FROM conversation.group_join_requests WHERE id=$1`, joined["id"]).Scan(&state); err != nil || state != "pending" {
		t.Fatal("issuer rejection changed original pending result")
	}
}
