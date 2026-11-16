package jobsim

import (
	"context"
	"crypto/hmac"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type refreshRequest struct {
	RefreshToken      string `json:"refreshToken,omitempty"`
	RequestID         string `json:"requestId"`
	FamilyID          string `json:"familyId"`
	RefreshGeneration string `json:"refreshGeneration"`
}

func (a *App) sessionRefreshRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/auth/refresh-context", a.browserRefreshContext)
	h("POST /api/auth/refresh", a.refreshSession)
	h("POST /api/auth/reauthenticate", a.login)
}

func (a *App) refreshSession(w http.ResponseWriter, r *http.Request) error {
	var q refreshRequest
	if e := decode(r, &q); e != nil {
		return e
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 || !validObjectID(q.FamilyID) {
		return fail(400, "INVALID_REFRESH_REQUEST", "Stable request and original family required")
	}
	if _, ok := canonicalCounter(q.RefreshGeneration, false); !ok {
		return fail(400, "INVALID_REFRESH_REQUEST", "Original decimal generation required")
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if e := a.rate(r.Context(), "refresh-ip:"+host, 120); e != nil {
		return e
	}
	web := r.Header.Get("X-JobsIM-Client") == "web"
	var browser browserSession
	var sessionID, userID string
	var e error
	if web {
		if q.RefreshToken != "" {
			return fail(400, "BROWSER_REFRESH_SECRET_REJECTED", "Browser must not handle upstream refresh credentials")
		}
		browser, e = a.validateBrowserRefresh(r)
		if e != nil {
			return e
		}
		sessionID, userID = browser.SessionID, browser.Credentials.User.ID
		if q.FamilyID != browser.Credentials.FamilyID {
			return fail(409, "AUTH_CONTEXT_CHANGED", "Original browser family differs from current cookie; do not revoke another login")
		}
	} else {
		if len(q.RefreshToken) < 32 || len(q.RefreshToken) > 256 {
			return fail(401, "REFRESH_INVALID", "Refresh credential invalid")
		}
		e = a.DB.QueryRow(r.Context(), `SELECT h.session_id::text,s.user_id::text FROM auth.refresh_token_history h JOIN auth.sessions s ON s.id=h.session_id WHERE h.token_digest=$1 AND h.scope_id=$2`, tokenDigest(q.RefreshToken), a.Config.DeploymentID).Scan(&sessionID, &userID)
		if e != nil {
			if e == pgx.ErrNoRows {
				return fail(401, "REFRESH_INVALID", "Refresh credential invalid")
			}
			return e
		}
	}
	epoch, e := a.currentIdentityEpoch(r.Context())
	if e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	// All issuance/rotation paths lock the principal before the stable session.
	var security, credentialGeneration, currentSource int64
	var username, displayName, state, recovery string
	var admin bool
	var userPending *string
	if e = tx.QueryRow(r.Context(), `SELECT security_version,credential_generation,username,display_name,admin,state,recovery_state,pending_operation_id FROM auth.users WHERE id=$1 FOR UPDATE`, userID).Scan(&security, &credentialGeneration, &username, &displayName, &admin, &state, &recovery, &userPending); e != nil {
		return e
	}
	if e = tx.QueryRow(r.Context(), `SELECT personal_source_generation FROM auth.instance WHERE singleton`).Scan(&currentSource); e != nil {
		return e
	}
	var family, device, kind, sourceContext string
	var generation, sessionSecurity, sessionCredential, sessionEpoch, sessionSource int64
	var currentDigest []byte
	var revoked *time.Time
	var exportPending *string
	var idle, absolute time.Time
	var webPending bool
	if e = tx.QueryRow(r.Context(), `SELECT family_id::text,device_id::text,auth_kind,source_context_id,refresh_generation,refresh_digest,security_version,generation,restore_epoch,source_generation,revoked_at,export_pending_operation_id,idle_expires_at,absolute_expires_at,web_pending FROM auth.sessions WHERE id=$1 FOR UPDATE`, sessionID).Scan(&family, &device, &kind, &sourceContext, &generation, &currentDigest, &sessionSecurity, &sessionCredential, &sessionEpoch, &sessionSource, &revoked, &exportPending, &idle, &absolute, &webPending); e != nil {
		return e
	}
	revoke := func(code string, status int) error {
		if _, e = tx.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, sessionID); e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return fail(503, "REFRESH_REAUTH_REQUIRED", "Refresh outcome unknown; do not reuse old credentials")
		}
		return fail(status, code, "This session family requires verified reauthentication")
	}
	var historyState string
	var historyGeneration int64
	if web {
		// A stale browser command never borrows the BFF's newer sealed token.
		var duplicate bool
		if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.refresh_operations WHERE session_id=$1 AND request_id=$2)`, sessionID, q.RequestID).Scan(&duplicate); e != nil {
			return e
		}
		if duplicate || family != q.FamilyID || str(generation) != q.RefreshGeneration || browser.Credentials.RefreshGeneration != str(generation) || kind != "web" {
			return revoke("REFRESH_REAUTH_REQUIRED", 409)
		}
		q.RefreshToken = browser.Credentials.RefreshToken
	}
	if e = tx.QueryRow(r.Context(), `SELECT state,refresh_generation FROM auth.refresh_token_history WHERE token_digest=$1 AND session_id=$2 FOR UPDATE`, tokenDigest(q.RefreshToken), sessionID).Scan(&historyState, &historyGeneration); e != nil {
		if e == pgx.ErrNoRows {
			return fail(401, "REFRESH_INVALID", "Refresh credential invalid")
		}
		return e
	}
	if historyState == "used" {
		return revoke("REFRESH_REUSE_DETECTED", 401)
	}
	if !time.Now().Before(idle) || !time.Now().Before(absolute) {
		return revoke("REFRESH_EXPIRED", 401)
	}
	if revoked != nil || exportPending != nil || state != "active" || recovery != "normal" || userPending != nil || sessionSecurity != security || sessionCredential != credentialGeneration || sessionEpoch != epoch || sessionSource != currentSource || sourceContext != "personal" || webPending {
		return revoke("REFRESH_REAUTH_REQUIRED", 401)
	}
	if family != q.FamilyID || q.RefreshGeneration != str(generation) || historyGeneration != generation || historyState != "active" || !hmac.Equal(currentDigest, tokenDigest(q.RefreshToken)) || (!web && kind != "native") {
		return fail(401, "REFRESH_INVALID", "Original family and generation do not match credential")
	}
	var duplicate bool
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.refresh_operations WHERE session_id=$1 AND request_id=$2)`, sessionID, q.RequestID).Scan(&duplicate); e != nil {
		return e
	}
	if duplicate {
		return revoke("REFRESH_REAUTH_REQUIRED", 409)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	absolute = absolute.UTC()
	newIdle := now.Add(refreshIdleLifetime)
	if absolute.Before(newIdle) {
		newIdle = absolute
	}
	accessExpiry := now.Add(accessLifetime)
	if newIdle.Before(accessExpiry) {
		accessExpiry = newIdle
	}
	access, refresh := randomToken(32), randomToken(48)
	next := generation + 1
	if next <= generation {
		return revoke("REFRESH_REAUTH_REQUIRED", 401)
	}
	if _, e = tx.Exec(r.Context(), `UPDATE auth.refresh_token_history SET state='used',consumed_at=now() WHERE session_id=$1 AND refresh_generation=$2 AND state='active'`, sessionID, generation); e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO auth.refresh_token_history(scope_id,session_id,family_id,refresh_generation,token_digest,issued_at,expires_at,state) VALUES($1,$2,$3,$4,$5,$6,$7,'active')`, a.Config.DeploymentID, sessionID, family, next, tokenDigest(refresh), now, newIdle); e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `UPDATE auth.sessions SET token_digest=$2,refresh_digest=$3,refresh_generation=$4,access_expires_at=$5,idle_expires_at=$6,expires_at=LEAST($6,absolute_expires_at) WHERE id=$1`, sessionID, tokenDigest(access), tokenDigest(refresh), next, accessExpiry, newIdle); e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO auth.refresh_operations(session_id,request_id,refresh_generation,state) VALUES($1,$2,$3,'consumed')`, sessionID, q.RequestID, generation); e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now()) WHERE session_id=$1`, sessionID); e != nil {
		return e
	}
	result := authResult{Token: access, RefreshToken: refresh, User: User{ID: userID, Username: username, DisplayName: displayName, Admin: admin}, DeviceID: device, RestoreEpoch: str(epoch), sessionMetadata: sessionMetadata{family, str(next), accessExpiry, newIdle, absolute, sourceContext}}
	if web {
		sealed, e := a.sealSessionCredentials(result, browser.ID)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `UPDATE auth.bff_sessions SET refresh_generation=$2,upstream_credentials_ciphertext=$3,state='active',expires_at=$4 WHERE id=$1 AND state='active'`, browser.ID, next, sealed, newIdle); e != nil {
			return e
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return fail(503, "REFRESH_REAUTH_REQUIRED", "Refresh outcome unknown; do not reuse old credentials")
	}
	if web {
		return a.deliverSession(w, r, initialAuthDelivery{Result: result, BrowserCookie: bearer(r)})
	}
	return a.deliverSession(w, r, initialAuthDelivery{Result: result})
}

// Revocation facts survive HTTP response loss. Only family/device artifacts are
// drained here; a routine session rotation never revokes sent forward refs.
func (a *App) StartSessionWorkers(ctx context.Context) {
	go func() {
		timer := time.NewTicker(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
			fence, fenceError := a.admissionFence(attempt, true)
			if fenceError != nil {
				cancel()
				continue
			}
			attempt, expireFence := a.heldAdmissionScope(attempt, "exclusive")
			markers, markerError := a.admissionMarkers()
			if markerError != nil {
				expireFence()
				closeExportFence(fence)
				cancel()
				continue
			}
			if len(markers) > 0 {
				if recoveryError := a.rpc(attempt, "message", "/internal/admission/recover", map[string]string{}, nil); recoveryError != nil {
					expireFence()
					closeExportFence(fence)
					cancel()
					continue
				}
			}
			_, _ = a.DB.Exec(attempt, `UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex') WHERE expires_at<=now() AND octet_length(delivery_ciphertext)>0`)
			_, _ = a.DB.Exec(attempt, `UPDATE auth.sessions s SET revoked_at=now() FROM auth.users u,auth.instance i WHERE i.singleton AND s.user_id=u.id AND s.revoked_at IS NULL AND s.family_id IS NOT NULL AND (s.expires_at<=now() OR s.generation<>u.credential_generation OR s.security_version<>u.security_version OR s.restore_epoch<>i.restore_epoch OR s.source_generation<>i.personal_source_generation OR u.state IN('banned','tombstoned') OR u.recovery_state<>'normal')`)
			rows, e := a.DB.Query(attempt, `SELECT session_id::text,user_id::text,device_id::text,operation_id FROM auth.session_revocations WHERE state='pending' ORDER BY last_attempt_at,created_at LIMIT 10`)
			if e != nil {
				expireFence()
				closeExportFence(fence)
				cancel()
				continue
			}
			type job struct{ id, user, device, op string }
			jobs := []job{}
			for rows.Next() {
				var j job
				if rows.Scan(&j.id, &j.user, &j.device, &j.op) == nil {
					jobs = append(jobs, j)
				}
			}
			rows.Close()
			for _, j := range jobs {
				_, _ = a.DB.Exec(attempt, `UPDATE auth.session_revocations SET last_attempt_at=now() WHERE session_id=$1`, j.id)
				payload := map[string]any{"userId": j.user, "deviceId": j.device}
				if a.protect(attempt, j.op, "intent", "auth.revoke_device", payload) != nil {
					continue
				}
				if a.rpc(attempt, "governance", "/internal/exports/invalidate", map[string]string{"userId": j.user, "deviceId": j.device, "operationId": j.op}, nil) != nil {
					continue
				}
				if a.protect(attempt, j.op, "result", "auth.revoke_device", payload) != nil {
					continue
				}
				_, _ = a.DB.Exec(attempt, `UPDATE auth.session_revocations SET state='cleared' WHERE session_id=$1`, j.id)
			}
			_ = a.drainPersonalExportInvalidations(attempt)
			expireFence()
			closeExportFence(fence)
			cancel()
			timer.Reset(time.Second)
		}
	}()
}
