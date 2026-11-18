package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func newCLIGroupFixture(t *testing.T) *cliSyncFixture {
	f := newCLISyncFixture(t)
	f.client.auth = &authManager{c: f.client, state: authState{Status: "active"}}
	f.client.auth.pair.Token = f.client.token
	f.client.auth.pair.FamilyID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	f.client.auth.pair.DeviceID = f.identity.DeviceID
	f.client.auth.pair.User.ID = f.identity.UserID
	return f
}

func TestGroupRecoveryFreshMFAStaysInMemoryAndRetryUsesOriginalPayload(t *testing.T) {
	f := newCLIGroupFixture(t)
	f.client.reader = bufio.NewReader(strings.NewReader("123456\n654321\n"))
	group, target := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	stepUp := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	var writes []map[string]any
	var proofs []map[string]any
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/governance/operations/") {
			cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "GROUP_OPERATION_NOT_FOUND"}})
			return true
		}
		if r.URL.Path == "/api/auth/step-up" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			proofs = append(proofs, body)
			cliSyncJSON(w, 200, map[string]any{"authorizationId": stepUp})
			return true
		}
		if r.URL.Path == "/api/conversations/"+group+"/governance" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			writes = append(writes, body)
			if len(writes) == 1 {
				cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
			} else {
				cliSyncJSON(w, 200, map[string]any{"requestId": body["requestId"], "status": "applied", "action": "recover", "version": "10"})
			}
			return true
		}
		return false
	})
	if _, err := f.client.groupCommand("group-recover", group+" "+target+" 9 7"); err == nil {
		t.Fatal("unknown recovery accepted")
	}
	var id string
	if err := f.store.db.QueryRow(`SELECT id FROM group_intents WHERE state='pending'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	row, err := f.client.groupOriginal(id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(row.Body), "123456") || strings.Contains(string(row.Body), "stepUpId") || strings.Contains(string(row.Body), stepUp) {
		t.Fatal("RAM recovery proof persisted")
	}
	if err = f.client.groupTransmit(row, false, nil); err == nil || len(proofs) != 1 || len(writes) != 1 {
		t.Fatal("pure lookup obtained or used a new proof")
	}
	if err = f.client.groupTransmit(row, true, nil); err != nil {
		t.Fatal(err)
	}
	if len(proofs) != 2 || proofs[0]["otp"] != "123456" || proofs[1]["otp"] != "654321" || len(writes) != 2 || writes[0]["requestId"] != writes[1]["requestId"] || writes[0]["targetMembershipGeneration"] != "7" {
		t.Fatal("original recovery payload changed or stale MFA reused")
	}
}

func TestGroupOriginalUnknownPersistsExactPermissionAndReadOnlyLookup(t *testing.T) {
	f := newCLIGroupFixture(t)
	group := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	path := "/api/conversations/" + group + "/governance"
	finish := false
	bodies := []string{}
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, path+"/operations/") {
			cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "GROUP_OPERATION_NOT_FOUND"}})
			return true
		}
		if r.URL.Path != path {
			return false
		}
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		if !finish {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
			return true
		}
		var body map[string]any
		json.Unmarshal(raw, &body)
		cliSyncJSON(w, 200, map[string]any{"requestId": body["requestId"], "status": "applied", "version": "9007199254740994"})
		return true
	})
	if err := f.client.groupCreate(group, "governance", map[string]any{"action": "dissolve", "version": "9007199254740993", "membershipGeneration": "2", "roleGeneration": "8"}, nil); err == nil {
		t.Fatal("unknown result accepted")
	}
	var id string
	if err := f.store.db.QueryRow(`SELECT id FROM group_intents WHERE state='pending'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	row, err := f.client.groupOriginal(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(row.Body), "9007199254740993") {
		t.Fatal("observed exact version changed")
	}
	if err = f.client.groupTransmit(row, false, nil); err == nil || len(bodies) != 1 {
		t.Fatal("pure original lookup created another write")
	}
	finish = true
	if err = f.client.groupTransmit(row, true, nil); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatal("retry rebased nonce or original permission")
	}
	row, err = f.client.groupOriginal(id)
	if err != nil || row.State != "complete" {
		t.Fatal("original completion not durable")
	}
}

func TestGroupInvitationCapabilityNeverEntersSQLiteAndColdRetryKeepsNonce(t *testing.T) {
	f := newCLIGroupFixture(t)
	group, invite := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	capability := "only-in-memory-invitation-fixture"
	raw, _ := json.Marshal(map[string]string{"deploymentId": f.identity.DeploymentID, "conversationId": group, "inviteId": invite, "token": capability})
	code := "JobsIM-Invite-1:" + base64.StdEncoding.EncodeToString(raw)
	finish := false
	requests := []map[string]string{}
	path := "/api/conversations/" + group + "/join-requests"
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, path+"/operations/") {
			cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "GROUP_OPERATION_NOT_FOUND"}})
			return true
		}
		if r.URL.Path != path {
			return false
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		if body["token"] != capability {
			t.Error("original in-memory invitation not supplied")
		}
		if !finish {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
		} else {
			cliSyncJSON(w, 200, map[string]any{"requestId": body["requestId"], "id": invite, "state": "pending", "membershipGeneration": nil})
		}
		return true
	})
	if _, err := f.client.groupCommand("join-group", code); err == nil {
		t.Fatal("unknown join accepted")
	}
	var body, id string
	if err := f.store.db.QueryRow(`SELECT id,body FROM group_intents`).Scan(&id, &body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, capability) || strings.Contains(body, "token") || strings.Contains(body, code) {
		t.Fatal("invitation proof persisted with ordinary responsibility")
	}
	old := f.store
	if err := old.close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(old.path, f.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.close() })
	f.client.store = reopened
	finish = true
	if _, err = f.client.groupCommand("join-group", code); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0]["requestId"] != requests[1]["requestId"] || requests[1]["requestId"] != id {
		t.Fatal("cold join created a new operation")
	}
}

func TestGroupSQLFailureAndOldUnversionedTransferNeverReachHTTP(t *testing.T) {
	f := newCLIGroupFixture(t)
	if err := f.client.groupTable(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`CREATE TRIGGER fail_group_prepare BEFORE INSERT ON group_intents BEGIN SELECT RAISE(FAIL,'injected persistence failure');END;`); err != nil {
		t.Fatal(err)
	}
	group := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if err := f.client.groupCreate(group, "governance", map[string]any{"action": "dissolve"}, nil); err == nil {
		t.Fatal("failed local prepare accepted")
	}
	if len(f.calls("/api/conversations/"+group+"/governance")) != 0 {
		t.Fatal("write escaped before durable prepare")
	}
	if _, err := f.client.groupCommand("transfer", group+" bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"); err == nil {
		t.Fatal("unobserved owner transfer accepted")
	}
}

func TestGroupSuccessorAndLegacyFamilyNeverCreateOldOperationAndTypedTargetMatches(t *testing.T) {
	f := newCLIGroupFixture(t)
	group, invite := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	var original string
	var authoritative map[string]any
	writes := 0
	f.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/governance/operations/") {
			if authoritative == nil {
				cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "GROUP_OPERATION_NOT_FOUND"}})
			} else {
				cliSyncJSON(w, 200, authoritative)
			}
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/invites/"+invite) {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			original, _ = body["requestId"].(string)
			writes++
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
			return true
		}
		return false
	})
	if err := f.client.groupCreate(group, "invite_revoke:"+invite, map[string]any{"version": "2", "membershipGeneration": "3", "roleGeneration": "4"}, nil); err == nil {
		t.Fatal("expected unknown revoke")
	}
	row, err := f.client.groupOriginal(original)
	if err != nil {
		t.Fatal(err)
	}
	f.client.auth.pair.FamilyID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	if err = f.client.groupTransmit(row, true, nil); err == nil || writes != 1 {
		t.Fatal("successor replayed an unseen old operation")
	}
	row.Family = ""
	if err = f.client.groupTransmit(row, true, nil); err == nil || writes != 1 {
		t.Fatal("legacy record acquired a guessed family")
	}
	authoritative = map[string]any{"requestId": original, "status": "applied", "action": "invite_revoke", "targetId": group, "state": "revoked"}
	if err = f.client.groupTransmit(row, false, nil); err == nil {
		t.Fatal("wrong typed target completed original")
	}
	authoritative["targetId"] = invite
	if err = f.client.groupTransmit(row, false, nil); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatal("pure lookup created a write")
	}
}
