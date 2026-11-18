package jobsim

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

func TestAccountDeletionAdmissionRejectsOrdinaryCredentialLanes(t *testing.T) {
	for _, state := range []string{"requested", "processing", "completed"} {
		t.Run(state, func(t *testing.T) {
			ctx, a, user, q, token, _ := newPasswordChangePGApp(t)
			tx, err := a.DB.Begin(ctx)
			if err != nil {
				t.Fatal("owned issuance transaction unavailable")
			}
			native, err := a.issueSession(ctx, tx, user, "gate-native", 1, 1, 1, false)
			if err != nil {
				tx.Rollback(ctx)
				t.Fatal("valid native family fixture failed")
			}
			web, err := a.issueSession(ctx, tx, user, "gate-browser", 1, 1, 1, true)
			if err != nil || tx.Commit(ctx) != nil {
				t.Fatal("valid browser family fixture failed")
			}
			pcExec(t, ctx, a, `UPDATE auth.sessions SET web_pending=false WHERE id=$1`, web.SessionID)
			pcExec(t, ctx, a, `UPDATE auth.bff_sessions SET state='active',browser_confirmed_at=now() WHERE session_id=$1`, web.SessionID)
			if _, err = a.browserSession(ctx, web.BrowserCookie); err != nil {
				t.Fatal("browser positive control failed before deletion state")
			}
			batch, code := newID(), "non-secret-owned-recovery-code"
			pcExec(t, ctx, a, `UPDATE auth.users SET recovery_generation=1 WHERE id=$1`, user.ID)
			pcExec(t, ctx, a, `INSERT INTO auth.recovery_code_batches(id,user_id,generation,active) VALUES($1,$2,1,true)`, batch, user.ID)
			pcExec(t, ctx, a, `INSERT INTO auth.recovery_codes(id,batch_id,code_digest) VALUES($1,$2,$3)`, newID(), batch, keyedDigest(a.Config.Key, "recovery", []byte(code)))
			grant := "non-secret-owned-password-reset-grant"
			pcExec(t, ctx, a, `INSERT INTO auth.reset_grants(id,user_id,purpose,credential_digest,issued_security_version,restore_epoch,expires_at,state,issue_request_id) VALUES($1,$2,'password_reset',$3,1,1,now()+interval '5 minutes','active',$4)`, newID(), user.ID, keyedDigest(a.Config.Key, "password-reset-grant", []byte(grant)), newID())
			originalHash := pcHash(t, ctx, a, user.ID)
			pcExec(t, ctx, a, `UPDATE auth.users SET deletion_state=$2 WHERE id=$1`, user.ID, state)
			proof, _ := json.Marshal(map[string]string{"token": token, "authorization": "Bearer " + token, "method": "GET", "path": "/api/me"})
			var denied *apiError
			if err = a.authorize(httptest.NewRecorder(), httptest.NewRequest("POST", "/internal/authorize", bytes.NewReader(proof))); !errors.As(err, &denied) || denied.Status != 401 {
				t.Fatal("ordinary access survived deletion state")
			}
			if _, err = a.browserSession(ctx, web.BrowserCookie); err == nil {
				t.Fatal("browser credentials survived deletion state")
			}
			call := func(path string, value any) *httptest.ResponseRecorder {
				raw, _ := json.Marshal(value)
				r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				a.Handler().ServeHTTP(w, r)
				return w
			}
			login := map[string]string{"username": q.Username, "password": q.CurrentPassword, "requestId": newID(), "deviceName": "new-auth-attempt"}
			if w := call("/api/auth/login", login); w.Code != 401 {
				t.Fatal("fresh password login reopened an account in deletion")
			}
			if w := call("/api/auth/refresh", refreshRequest{RefreshToken: native.Result.RefreshToken, RequestID: newID(), FamilyID: native.Result.FamilyID, RefreshGeneration: "1"}); w.Code != 401 || !bytes.Contains(w.Body.Bytes(), []byte("REFRESH_REAUTH_REQUIRED")) {
				t.Fatal("valid refresh family rotated across deletion state")
			}
			for _, recovery := range []struct {
				path  string
				value map[string]string
			}{
				{"/api/auth/recover", map[string]string{"username": q.Username, "password": q.NewPassword, "requestId": newID(), "code": code}},
				{"/api/auth/reset-password", map[string]string{"username": q.Username, "password": q.NewPassword, "requestId": newID(), "grant": grant}},
			} {
				if w := call(recovery.path, recovery.value); w.Code != 403 {
					t.Fatal("valid recovery proof bypassed deletion state")
				}
			}
			if w := pcCall(a, "/api/auth/password-change", token, q); w.Code != 401 {
				t.Fatal("ordinary password change bypassed deletion state")
			}
			raw, _ := json.Marshal(map[string]string{"userId": user.ID, "deviceId": user.DeviceID})
			r := httptest.NewRequest("POST", "/internal/exports/principal", bytes.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-JobsIM-Service-Key", base64.RawURLEncoding.EncodeToString(a.Config.Key))
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatal("formal export granted a deleted/pending-deletion principal")
			}
			if pcHash(t, ctx, a, user.ID) != originalHash || pcCount(t, ctx, a, `SELECT count(*) FROM auth.recovery_codes WHERE consumed_at IS NOT NULL`) != 0 || pcCount(t, ctx, a, `SELECT count(*) FROM auth.reset_grants WHERE state<>'active'`) != 0 || pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`) != 0 {
				t.Fatal("rejected ordinary lane changed credential, consumed recovery proof or added safety facts")
			}
			if pcCount(t, ctx, a, `SELECT count(*) FROM auth.sessions WHERE user_id=$1`, user.ID) != 4 {
				t.Fatal("rejected ordinary lane issued another session family")
			}
		})
	}
}

func TestAccountDeletionAdmissionRetiresPreparedPasswordChange(t *testing.T) {
	ctx, a, user, q, _, _ := newPasswordChangePGApp(t)
	value, err := a.preparePasswordChange(ctx, user, q)
	if err != nil {
		t.Fatal("prepared positive control failed")
	}
	old := pcHash(t, ctx, a, user.ID)
	pcExec(t, ctx, a, `UPDATE auth.users SET deletion_state='requested' WHERE id=$1`, user.ID)
	if err = a.finishPasswordChange(ctx, value); err != nil || pcHash(t, ctx, a, user.ID) != old {
		t.Fatal("prepared credential operation installed a hash after deletion admission closed")
	}
	if pcCount(t, ctx, a, `SELECT count(*) FROM auth.password_changes WHERE state='retired' AND octet_length(replacement_hash_ciphertext)=0`) != 1 {
		t.Fatal("retired password operation retained secret recovery material")
	}
}

func TestAccountDeletionAdmissionHistoricalPasswordResultIsNotCurrent(t *testing.T) {
	ctx, a, user, q, token, _ := newPasswordChangePGApp(t)
	if pcCall(a, "/api/auth/password-change", token, q).Code != 200 {
		t.Fatal("completed positive control failed")
	}
	pcExec(t, ctx, a, `UPDATE auth.users SET deletion_state='requested' WHERE id=$1`, user.ID)
	if w := pcCall(a, "/api/auth/password-change-results", "", q); w.Code != 200 || pcBody(t, w)["current"] != false {
		t.Fatal("historical password result called pending-deletion account current")
	}
}

func TestAccountDeletionAdmissionPreparedAdminResetCannotReopenTarget(t *testing.T) {
	for _, timing := range []string{"before-resume", "after-precheck"} {
		t.Run(timing, func(t *testing.T) {
			ctx, a, actor, _, token, f := newPasswordChangePGApp(t)
			pcExec(t, ctx, a, `UPDATE auth.users SET admin=true WHERE id=$1`, actor.ID)
			target, request, grant := newID(), newID(), newID()
			pcExec(t, ctx, a, `INSERT INTO auth.users(id,username,display_name,password_hash,pending_operation_id) VALUES($1,'reset_target','Non-secret reset target',$2,$3)`, target, pcHash(t, ctx, a, actor.ID), request)
			body := map[string]string{"reason": "non-secret authorized reset reason", "expectedVersion": "1", "requestId": request, "stepUpId": newID()}
			raw, _ := json.Marshal(body)
			canonical, err := canonicalAdminPayload(raw)
			if err != nil {
				t.Fatal("owned canonical reset fixture failed")
			}
			payload, _ := json.Marshal(map[string]string{"userId": target, "generation": "2", "securityVersion": "2"})
			result, _ := json.Marshal(map[string]string{"id": grant, "userId": target, "status": "issued", "version": "2", "requestId": request})
			pcExec(t, ctx, a, `INSERT INTO auth.admin_operations(request_id,actor_id,target_id,action,request_digest,state,result,journal_payload) VALUES($1,$2,$3,'account.reset_grant',$4,'prepared',$5,$6)`, request, actor.ID, target, keyedDigest(a.Config.Key, "reset-grant:"+actor.ID+":"+target, canonical), result, payload)
			pcExec(t, ctx, a, `INSERT INTO auth.reset_grants(id,user_id,purpose,credential_digest,issued_security_version,restore_epoch,expires_at,state,issue_request_id) VALUES($1,$2,'password_reset',$3,2,1,now()+interval '5 minutes','prepared',$4)`, grant, target, tokenDigest("non-secret-prepared-reset"), request)
			deleting := func() { pcExec(t, ctx, a, `UPDATE auth.users SET deletion_state='requested' WHERE id=$1`, target) }
			if timing == "before-resume" {
				deleting()
			} else {
				f.mu.Lock()
				f.beforeAppend = deleting
				f.mu.Unlock()
			}
			r := httptest.NewRequest("POST", "/api/admin/users/"+target+"/reset-grants", bytes.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			if w.Code != 409 {
				t.Fatal("prepared administrator reset crossed deletion admission")
			}
			if pcCount(t, ctx, a, `SELECT count(*) FROM auth.users WHERE id=$1 AND deletion_state='requested' AND credential_generation=1 AND security_version=1 AND recovery_state='normal' AND pending_operation_id=$2`, target, request) != 1 {
				t.Fatal("prepared reset changed or cleared pending deletion/security state")
			}
			if pcCount(t, ctx, a, `SELECT count(*) FROM auth.reset_grants WHERE id=$1 AND state='prepared'`, grant) != 1 || pcCount(t, ctx, a, `SELECT count(*) FROM auth.admin_operations WHERE request_id=$1 AND state='prepared'`, request) != 1 {
				t.Fatal("prepared reset activated credentials or falsely completed its original operation")
			}
			want := 0
			if timing == "after-precheck" {
				want = 1
			}
			if pcCount(t, ctx, a, `SELECT count(*) FROM journal.entries`) != want {
				t.Fatal("reset safety acknowledgements escaped the expected preparation boundary")
			}
		})
	}
}

func TestAccountDeletionAdmissionOriginalAuthenticationDeliveryIsRejected(t *testing.T) {
	for _, purpose := range []string{"login", "registration", "reauthentication"} {
		t.Run(purpose, func(t *testing.T) {
			ctx, a, user, q, _, _ := newPasswordChangePGApp(t)
			tx, err := a.DB.Begin(ctx)
			if err != nil {
				t.Fatal("owned delivery transaction unavailable")
			}
			delivery, err := a.issueSession(ctx, tx, user, "original-delivery", 1, 1, 1, false)
			if err != nil || tx.Commit(ctx) != nil {
				t.Fatal("production initial credential issuance failed")
			}
			request := registerRequest{Username: q.Username, Password: q.CurrentPassword, RequestID: newID(), DeviceName: "original-delivery"}
			path := "/api/auth/login"
			if purpose == "registration" {
				path = "/api/auth/register"
			}
			if purpose == "reauthentication" {
				path = "/api/auth/reauthenticate"
				if a.DB.QueryRow(ctx, `SELECT family_id::text FROM auth.sessions WHERE device_id=$1`, user.DeviceID).Scan(&request.PreviousFamilyID) != nil {
					t.Fatal("owned predecessor family unavailable")
				}
			}
			proof, _ := json.Marshal(struct {
				Request registerRequest `json:"request"`
				Web     bool            `json:"web"`
			}{request, false})
			encoded, _ := json.Marshal(delivery)
			sealed, err := a.seal(string(encoded), a.Config.DeploymentID+":"+purpose+":"+request.RequestID+":"+user.ID+":"+delivery.Result.DeviceID)
			if err != nil {
				t.Fatal("owned initial delivery sealing failed")
			}
			pcExec(t, ctx, a, `INSERT INTO auth.local_auth_requests(request_id,purpose,request_digest,user_id,device_id,delivery_ciphertext,expires_at,delivery_version,session_id,family_id) VALUES($1,$2,$3,$4,$5,$6,now()+interval '5 minutes','refresh-auth-v1',$7,$8)`, request.RequestID, purpose, keyedDigest(a.Config.Key, purpose, proof), user.ID, delivery.Result.DeviceID, sealed, delivery.SessionID, delivery.Result.FamilyID)
			call := func() *httptest.ResponseRecorder {
				raw, _ := json.Marshal(request)
				r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				a.Handler().ServeHTTP(w, r)
				return w
			}
			if w := call(); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(delivery.Result.Token)) {
				t.Fatal("initial original-delivery positive control failed")
			}
			pcExec(t, ctx, a, `UPDATE auth.users SET deletion_state='requested' WHERE id=$1`, user.ID)
			w := call()
			if w.Code != 401 || !bytes.Contains(w.Body.Bytes(), []byte("DELIVERY_INVALIDATED")) || bytes.Contains(w.Body.Bytes(), []byte(delivery.Result.Token)) || bytes.Contains(w.Body.Bytes(), []byte(delivery.Result.RefreshToken)) || len(w.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("original initial authentication redelivered credentials across deletion admission")
			}
		})
	}
}
