package jobsim

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestGroupSafetyFieldsAndOriginalOperationIdentity(t *testing.T) {
	for _, test:=range []struct{ key,value string; valid bool }{
		{"groupMuteAll","false",true},{"groupMuteAll","1",false},
		{"groupSlowSeconds","0",true},{"groupSlowSeconds","3601",false},
		{"groupMaxMembers","10000",true},{"groupMaxMembers","10001",false},
		{"groupRole","owner",false},{"groupRole","admin",true},
		{"groupRoleGeneration","9007199254740993",true},{"groupRoleGeneration","01",false},
		{"groupMutedUntil","",true},{"groupMutedUntil","tomorrow",false},
	} {
		if err:=validateJournalField(test.key,test.value); (err==nil)!=test.valid { t.Fatalf("group safety field %s differs",test.key) }
	}
	q:=groupMutation{RequestID:newID(),Version:"1",Action:"settings",AuthorizationID:newID()}
	original:=groupMutationDigest(q)
	q.AuthorizationID=newID()
	if !bytesEqual(original,groupMutationDigest(q)) { t.Fatal("one-use step-up changed original operation identity") }
	q.Version="2"
	if bytesEqual(original,groupMutationDigest(q)) { t.Fatal("original observed group version was not bound") }
}

func groupDatabaseFixture(t *testing.T) (*App,*pgx.Conn) {
	t.Helper()
	file:=os.Getenv("JOBS_IM_ADMISSION_TEST_ENV")
	if file=="" { t.Skip("explicit isolated private PostgreSQL fixture required") }
	raw,err:=os.ReadFile(file)
	if err!=nil { t.Fatal("private group fixture unavailable") }
	var env struct { BusinessURL string `json:"businessURL"` }
	if json.Unmarshal(raw,&env)!=nil { t.Fatal("invalid private group fixture") }
	ctx:=context.Background()
	admin,err:=pgx.Connect(ctx,env.BusinessURL)
	if err!=nil { t.Fatal("private PostgreSQL operator unavailable") }
	name:="jobsim_group_"+strings.ReplaceAll(newID(),"-","")
	if _,err=admin.Exec(ctx,`CREATE DATABASE `+pgx.Identifier{name}.Sanitize());err!=nil { admin.Close(ctx);t.Fatal("isolated group database unavailable") }
	u,err:=url.Parse(env.BusinessURL)
	if err!=nil { t.Fatal("private URL invalid") }
	u.Path="/"+name
	cfg,err:=PrepareConfig(Config{StateDir:t.TempDir(),DatabaseURL:u.String(),RoleURLs:map[string]string{}})
	if err!=nil { t.Fatal(err) }
	a,err:=NewApp(ctx,cfg,"conversation")
	if err!=nil { t.Fatal(err) }
	m,err:=NewApp(ctx,cfg,"message")
	if err!=nil { a.Close();t.Fatal(err) }
	m.Close()
	conn,err:=pgx.Connect(ctx,u.String())
	if err!=nil { a.Close();t.Fatal("isolated group replay connection unavailable") }
	t.Cleanup(func(){
		conn.Close(ctx)
		a.Close()
		_,e:=admin.Exec(ctx,`DROP DATABASE `+pgx.Identifier{name}.Sanitize())
		admin.Close(ctx)
		if e!=nil { t.Error("isolated group fixture cleanup failed") }
	})
	return a,conn
}

func TestGroupMuteExpiryAndRejoinNeverReviveOldAdmissionPG(t *testing.T) {
	a,_:=groupDatabaseFixture(t)
	ctx:=context.Background()
	id,user:=newID(),newID()
	if _,e:=a.DB.Exec(ctx,`INSERT INTO conversation.conversations(id,kind,title,owner_id) VALUES($1,'group','fixture',$2);`,id,user);e!=nil { t.Fatal(e) }
	grant,allowed,_,e:=a.groupAdmission(ctx,id,user,"member",1)
	if e!=nil || !allowed { t.Fatal("default group admission unavailable") }
	old:=a.sendAdmission(user,id,"group",1,1,grant)
	if _,e=a.DB.Exec(ctx,`INSERT INTO conversation.group_mutes VALUES($1,$2,1,now()+interval '1 minute')`,id,user);e!=nil { t.Fatal(e) }
	_,allowed,_,e=a.groupAdmission(ctx,id,user,"member",1)
	if e!=nil || allowed { t.Fatal("muted original membership can send") }
	if _,e=a.DB.Exec(ctx,`UPDATE conversation.group_mutes SET muted_until=now()-interval '1 second' WHERE conversation_id=$1`,id);e!=nil { t.Fatal(e) }
	grant,allowed,_,e=a.groupAdmission(ctx,id,user,"member",1)
	if e!=nil || !allowed || sameAdmission(old,a.sendAdmission(user,id,"group",1,2,grant)) { t.Fatal("mute expiry revived a pre-mute role admission") }
	if _,e=a.DB.Exec(ctx,`UPDATE conversation.group_mutes SET muted_until=now()+interval '1 minute' WHERE conversation_id=$1`,id);e!=nil { t.Fatal(e) }
	_,allowed,_,e=a.groupAdmission(ctx,id,user,"member",2)
	if e!=nil || !allowed { t.Fatal("old membership mute contaminated explicit rejoin") }
}

func TestGroupSlowRateSameTransactionRollbackAndParallelUserPG(t *testing.T) {
	a,_:=groupDatabaseFixture(t)
	ctx:=context.Background()
	meta:=map[string]any{"kind":"group","role":"member","slowSeconds":"60"}
	id,user:=newID(),newID()
	tx,e:=a.DB.Begin(ctx)
	if e!=nil { t.Fatal(e) }
	if e=a.enforceGroupRate(ctx,tx,meta,user,id);e!=nil { t.Fatal(e) }
	if e=tx.Rollback(ctx);e!=nil { t.Fatal(e) }
	tx,e=a.DB.Begin(ctx)
	if e!=nil { t.Fatal(e) }
	if e=a.enforceGroupRate(ctx,tx,meta,user,id);e!=nil { t.Fatal("rollback charged a send",e) }
	blocked:=make(chan error,1)
	go func(){
		other,err:=a.DB.Begin(ctx)
		if err!=nil { blocked<-err;return }
		defer other.Rollback(ctx)
		blocked<-a.enforceGroupRate(ctx,other,meta,user,id)
	}()
	select { case <-blocked: t.Fatal("parallel physical sender bypassed uncommitted same-user rate row");case <-time.After(30*time.Millisecond): }
	if e=tx.Commit(ctx);e!=nil { t.Fatal(e) }
	if e=<-blocked;e==nil { t.Fatal("second device/user send escaped shared interval") }
	tx,e=a.DB.Begin(ctx)
	if e!=nil { t.Fatal(e) }
	defer tx.Rollback(ctx)
	if e=a.enforceGroupRate(ctx,tx,meta,newID(),id);e!=nil { t.Fatal("one user rate blocked another user") }
}

func TestGroupSafetyReplayRestrictsWithoutGrantOrMembershipABAPG(t *testing.T) {
	a,conn:=groupDatabaseFixture(t)
	ctx:=context.Background()
	id,owner,target,invite:=newID(),newID(),newID(),newID()
	if _,e:=a.DB.Exec(ctx,`INSERT INTO conversation.conversations(id,kind,title,owner_id) VALUES($1,'group','fixture',$2)`,id,owner);e!=nil { t.Fatal(e) }
	if _,e:=a.DB.Exec(ctx,`INSERT INTO conversation.members(conversation_id,user_id,role,membership_generation,role_generation) VALUES($1,$2,'member',2,8)`,id,target);e!=nil { t.Fatal(e) }
	replay:=func(kind string,payload map[string]any){
		t.Helper()
		handled,e:=ReplayGroupSafety(ctx,conn,journalRecord{Owner:"conversation",Kind:kind,Phase:"intent",Payload:payload})
		if e!=nil || !handled { t.Fatal("group safety replay failed",e) }
	}
	base:=func() map[string]any { return map[string]any{"conversationId":id,"userId":owner,"version":"9"} }
	p:=base();p["targetId"]=target;p["membershipGeneration"]="1";p["groupRoleGeneration"]="99";p["groupRole"]="member";p["groupMutedUntil"]=time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	replay("conversation.group_member",p)
	var generation int64
	var muted bool
	if e:=a.DB.QueryRow(ctx,`SELECT role_generation,EXISTS(SELECT 1 FROM conversation.group_mutes WHERE conversation_id=$1) FROM conversation.members WHERE conversation_id=$1 AND user_id=$2`,id,target).Scan(&generation,&muted);e!=nil || generation!=8 || muted { t.Fatal("old membership restriction crossed a rejoin") }
	p["membershipGeneration"]="2";p["groupRole"]="admin"
	replay("conversation.group_member",p)
	var role string
	if e:=a.DB.QueryRow(ctx,`SELECT role FROM conversation.members WHERE conversation_id=$1 AND user_id=$2`,id,target).Scan(&role);e!=nil || role!="member" { t.Fatal("uncertain historical intent granted administrator") }
	p=base();p["groupMuteAll"]="true";p["groupSlowSeconds"]="60";p["groupMaxMembers"]="500"
	replay("conversation.group_policy",p)
	p["version"]="10";p["groupMuteAll"]="false";p["groupSlowSeconds"]="0";p["groupMaxMembers"]="10000"
	replay("conversation.group_policy",p)
	policy,e:=a.readGroupPolicy(ctx,id)
	if e!=nil || !policy.MuteAll || policy.SlowSeconds!=60 || policy.MaxMembers!=500 { t.Fatal("historical replay relaxed original denial policy") }
	if _,e=a.DB.Exec(ctx,`INSERT INTO conversation.group_invites(id,conversation_id,creator_id,request_id,digest,expires_at,max_uses,uses,approval_required) VALUES($1,$2,$3,'original-invite','x',now()+interval '1 hour',2,0,true)`,invite,id,owner);e!=nil { t.Fatal(e) }
	replay("conversation.group_invite_use",map[string]any{"conversationId":id,"userId":target,"inviteId":invite,"groupInviteUses":"2"})
	replay("conversation.group_invite_use",map[string]any{"conversationId":id,"userId":target,"inviteId":invite,"groupInviteUses":"1"})
	replay("conversation.group_invite_revoke",map[string]any{"conversationId":id,"userId":owner,"inviteId":invite})
	var uses int
	var revoked bool
	if e=a.DB.QueryRow(ctx,`SELECT uses,revoked_at IS NOT NULL FROM conversation.group_invites WHERE id=$1`,invite).Scan(&uses,&revoked);e!=nil || uses!=2 || !revoked { t.Fatal("replay revived a consumed or revoked invitation") }
	replay("conversation.group_dissolve",base())
	var state string
	if e=a.DB.QueryRow(ctx,`SELECT state FROM conversation.conversations WHERE id=$1`,id).Scan(&state);e!=nil || state!="dissolved" { t.Fatal("dissolve safety floor missing") }
}
