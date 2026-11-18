package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type browserSession struct {
	ID, SessionID, State string
	Credentials          authResult
}

func (a *App) browserFamilyContext(method, path, family, learn string, b browserSession) error {
	if family == b.Credentials.FamilyID && family != "" {
		return nil
	}
	if family == "" && learn == "1" && method == "GET" && (path == "/api/auth/context" || path == "/api/auth/refresh-context") {
		return nil
	}
	if family == "" && b.State == "awaiting_browser_confirmation" && (path == "/api/auth/csrf" || path == "/api/auth/confirm") {
		return nil
	}
	return fail(409, "AUTH_CONTEXT_CHANGED", "Browser account context changed; discard this tab context without revoking the current cookie family")
}

func (a *App) browserSession(ctx context.Context, cookie string) (browserSession, error) {
	var b browserSession
	if cookie == "" {
		return b, fail(401, "UNAUTHORIZED", "Sign in again")
	}
	epoch, e := a.currentIdentityEpoch(ctx)
	if e != nil {
		return b, e
	}
	var family, generation string
	var sealed []byte
	e = a.DB.QueryRow(ctx, `SELECT b.id::text,b.session_id::text,b.state,b.family_id::text,b.refresh_generation::text,b.upstream_credentials_ciphertext FROM auth.bff_sessions b JOIN auth.sessions s ON s.id=b.session_id JOIN auth.users u ON u.id=s.user_id JOIN auth.instance i ON i.singleton WHERE b.cookie_digest=$1 AND b.state IN('awaiting_browser_confirmation','active') AND b.expires_at>now() AND b.restore_epoch=$2 AND b.refresh_generation=s.refresh_generation AND b.family_id=s.family_id AND b.security_version=u.security_version AND b.source_generation=i.personal_source_generation AND b.source_context_id='personal' AND s.revoked_at IS NULL AND s.expires_at>now() AND s.generation=u.credential_generation AND s.security_version=u.security_version AND s.restore_epoch=$2 AND s.source_generation=i.personal_source_generation AND s.source_context_id='personal' AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL`, tokenDigest(cookie), epoch).Scan(&b.ID, &b.SessionID, &b.State, &family, &generation, &sealed)
	if e != nil {
		if e == pgx.ErrNoRows {
			return b, fail(401, "REFRESH_REAUTH_REQUIRED", "Browser session no longer valid")
		}
		return b, e
	}
	plain, e := a.unseal(sealed, a.Config.DeploymentID+":bff:"+b.ID+":"+family+":"+generation+":"+str(epoch))
	if e != nil || json.Unmarshal([]byte(plain), &b.Credentials) != nil || b.Credentials.FamilyID != family || b.Credentials.RefreshGeneration != generation || b.Credentials.RestoreEpoch != str(epoch) || b.Credentials.Token == "" || b.Credentials.RefreshToken == "" {
		return b, fail(401, "REFRESH_REAUTH_REQUIRED", "Browser credential storage is unavailable; reauthenticate")
	}
	return b, nil
}

func (a *App) browserRefreshContext(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Client") != "web" || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return fail(400, "BROWSER_CONTEXT_REQUIRED", "Same-origin browser session required")
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != a.Config.PublicURL {
		return fail(403, "ORIGIN_REJECTED", "Cross-origin request rejected")
	}
	b, e := a.browserSession(r.Context(), bearer(r))
	if e != nil {
		return e
	}
	if b.State != "active" {
		return fail(401, "BROWSER_CONFIRMATION_REQUIRED", "Confirm browser login before refresh")
	}
	if e = a.browserFamilyContext(r.Method, r.URL.Path, r.Header.Get("X-JobsIM-Auth-Family"), r.Header.Get("X-JobsIM-Context-Learn"), b); e != nil {
		return e
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]any{"familyId": b.Credentials.FamilyID, "refreshGeneration": b.Credentials.RefreshGeneration, "accessExpiresAt": b.Credentials.AccessExpiresAt, "idleExpiresAt": b.Credentials.IdleExpiresAt, "absoluteExpiresAt": b.Credentials.AbsoluteExpiresAt, "sourceContextId": b.Credentials.SourceContextID, "restoreEpoch": b.Credentials.RestoreEpoch, "csrfToken": a.csrf(bearer(r))})
	return nil
}

func (a *App) validateBrowserRefresh(r *http.Request) (browserSession, error) {
	var empty browserSession
	if r.Header.Get("X-JobsIM-Client") != "web" || r.Header.Get("Origin") != a.Config.PublicURL || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return empty, fail(403, "ORIGIN_REJECTED", "Same-origin browser refresh required")
	}
	if !hmac.Equal([]byte(r.Header.Get("X-CSRF-Token")), []byte(a.csrf(bearer(r)))) {
		return empty, fail(403, "CSRF_REQUIRED", "Browser session CSRF required")
	}
	b, e := a.browserSession(r.Context(), bearer(r))
	if e != nil {
		return b, e
	}
	if b.State != "active" {
		return b, fail(401, "BROWSER_CONFIRMATION_REQUIRED", "Confirm browser login before refresh")
	}
	if e = a.browserFamilyContext(r.Method, r.URL.Path, r.Header.Get("X-JobsIM-Auth-Family"), "", b); e != nil {
		return b, e
	}
	return b, nil
}

func (a *App) confirmBrowser(ctx context.Context, cookie string) error {
	b, e := a.browserSession(ctx, cookie)
	if e != nil {
		return e
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var pending bool
	var created time.Time
	if e = tx.QueryRow(ctx, `SELECT web_pending,created_at FROM auth.sessions WHERE id=$1 AND revoked_at IS NULL FOR UPDATE`, b.SessionID).Scan(&pending, &created); e != nil {
		return e
	}
	if pending && time.Since(created) > 5*time.Minute {
		return fail(401, "BROWSER_CONFIRMATION_REQUIRED", "Browser confirmation window expired")
	}
	if _, e = tx.Exec(ctx, `UPDATE auth.sessions SET web_pending=false WHERE id=$1`, b.SessionID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE auth.bff_sessions SET state='active',browser_confirmed_at=COALESCE(browser_confirmed_at,now()) WHERE session_id=$1 AND state IN('active','awaiting_browser_confirmation')`, b.SessionID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now()) WHERE session_id=$1`, b.SessionID); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
