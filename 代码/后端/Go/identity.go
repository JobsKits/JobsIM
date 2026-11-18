package jobsim

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net"
	"net/http"
	"strings"
	"time"
)

type registerRequest struct {
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	RequestID          string   `json:"requestId"`
	DeviceName         string   `json:"deviceName"`
	OTP                string   `json:"otp,omitempty"`
	PreviousFamilyID   string   `json:"previousFamilyId,omitempty"`
	PreviousRequestID  string   `json:"previousRequestId,omitempty"`
	PreviousRequestIDs []string `json:"previousRequestIds,omitempty"`
}

func (a *App) seal(secret, aad string) ([]byte, error) {
	b, e := aes.NewCipher(keyedDigest(a.Config.Key, "credential-delivery-aead", nil))
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return g.Seal(nonce, nonce, []byte(secret), []byte(aad)), nil
}
func (a *App) unseal(sealed []byte, aad string) (string, error) {
	b, _ := aes.NewCipher(keyedDigest(a.Config.Key, "credential-delivery-aead", nil))
	g, _ := cipher.NewGCM(b)
	if len(sealed) < g.NonceSize() {
		return "", fail(401, "DELIVERY_EXPIRED", "Reauthenticate with a new request")
	}
	v, e := g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], []byte(aad))
	return string(v), e
}
func (a *App) passwordSlot(ctx context.Context) error {
	select {
	case a.hashSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (a *App) rate(ctx context.Context, subject string, max int) error {
	var count int
	e := a.DB.QueryRow(ctx, `INSERT INTO auth.rate_windows(subject,window_start,count) VALUES($1,date_trunc('minute',now()),1) ON CONFLICT(subject,window_start) DO UPDATE SET count=auth.rate_windows.count+1 RETURNING count`, subject).Scan(&count)
	if e != nil {
		return e
	}
	if count > max {
		return fail(429, "RATE_LIMITED", "Try again later")
	}
	return nil
}
func (a *App) identityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	a.profileIdentityRoutes(h)
	h("PATCH /api/me/username", a.changeOwnUsername)
	a.groupIssuerRoutes(h, internal)
	a.mfaRoutes(h, internal)
	a.exportIdentityRoutes(h, internal)
	a.adminIdentityRoutes(h)
	a.sessionRefreshRoutes(h)
	a.passwordChangeRoutes(h)
	h("POST /api/auth/register", a.register)
	h("POST /api/auth/login", a.login)
	h("POST /internal/authorize", internal(a.authorize))
	h("POST /internal/users", internal(a.internalUsers))
	h("GET /api/me", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		var profile AvatarProfile
		if e = a.rpc(r.Context(), "directory", "/internal/profile", map[string]string{"userId": u.ID}, &profile); e != nil {
			return e
		}
		applyUserProfile(&u, profile)
		u.ContactBindings, e = a.ownContactBindings(r.Context(), u.ID)
		if e != nil {
			return e
		}
		respond(w, 200, u)
		return nil
	})
	h("GET /api/auth/context", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		var profile AvatarProfile
		if e = a.rpc(r.Context(), "directory", "/internal/profile", map[string]string{"userId": u.ID}, &profile); e != nil {
			return e
		}
		applyUserProfile(&u, profile)
		meta, e := a.sessionMetadataForDevice(r.Context(), u.ID, u.DeviceID)
		if e != nil {
			return e
		}
		respond(w, 200, struct {
			User         User   `json:"user"`
			DeviceID     string `json:"deviceId"`
			RestoreEpoch string `json:"restoreEpoch"`
			sessionMetadata
		}{u, u.DeviceID, u.RestoreEpoch, meta})
		return nil
	})
	h("GET /api/me/export", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		var contacts, groups any
		if e = a.rpc(r.Context(), "directory", "/internal/export", map[string]string{"userId": u.ID}, &contacts); e != nil {
			return e
		}
		if e = a.rpc(r.Context(), "conversation", "/internal/export", map[string]string{"userId": u.ID}, &groups); e != nil {
			return e
		}
		w.Header().Set("Content-Disposition", "attachment; filename=jobsim-account.json")
		respond(w, 200, map[string]any{"formatVersion": 1, "deploymentId": a.Config.DeploymentID, "sourceContextId": "personal", "user": u, "contacts": contacts, "conversations": groups, "includesChatBodies": false, "includesCredentials": false})
		return nil
	})
	h("GET /api/users", a.users)
	h("GET /api/auth/csrf", func(w http.ResponseWriter, r *http.Request) error {
		if _, e := a.current(r); e != nil {
			return e
		}
		respond(w, 200, map[string]string{"csrfToken": a.csrf(bearer(r))})
		return nil
	})
	h("POST /api/auth/confirm", func(w http.ResponseWriter, r *http.Request) error {
		if _, e := a.current(r); e != nil {
			return e
		}
		if e := a.confirmBrowser(r.Context(), bearer(r)); e != nil {
			return e
		}
		respond(w, 200, map[string]string{"status": "confirmed"})
		return nil
	})
	h("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		payload := map[string]any{"userId": u.ID, "deviceId": u.DeviceID}
		op := "logout:" + u.DeviceID
		if e = a.protect(r.Context(), op, "intent", "auth.revoke_device", payload); e != nil {
			return e
		}
		if e = a.freezeDeviceExports(r.Context(), u.ID, u.DeviceID, op); e != nil {
			return e
		}
		_, e = a.DB.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=now() WHERE device_id=$1 AND user_id=$2`, u.DeviceID, u.ID)
		if e != nil {
			return e
		}
		if e = a.protect(r.Context(), op, "result", "auth.revoke_device", payload); e != nil {
			return e
		}
		a.clearCookie(w)
		respond(w, 200, map[string]string{"status": "logged_out"})
		return nil
	})
	h("GET /api/auth/sessions", a.sessions)
	h("DELETE /api/auth/sessions/{deviceId}", a.revokeDevice)
	h("POST /api/auth/recovery-codes", a.recoveryCodes)
	h("POST /api/auth/recover", a.recoverAccount)
	h("POST /api/auth/reset-password", func(w http.ResponseWriter, r *http.Request) error { return a.recoverWithCredential(w, r, true) })
}
func (a *App) csrf(token string) string {
	return stringBase64(keyedDigest(a.Config.Key, "web-csrf", []byte(token)))
}
func stringBase64(v []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(v)*2)
	for i, b := range v {
		out[i*2] = alphabet[b>>4]
		out[i*2+1] = alphabet[b&15]
	}
	return string(out)
}
func (a *App) authorize(w http.ResponseWriter, r *http.Request) error {
	var q struct {
		Token         string `json:"token"`
		Method        string `json:"method"`
		Origin        string `json:"origin"`
		CSRF          string `json:"csrf"`
		Authorization string `json:"authorization"`
		Path          string `json:"path"`
		AuthFamily    string `json:"authFamily"`
		ContextLearn  string `json:"contextLearn"`
	}
	if e := decode(r, &q); e != nil {
		return e
	}
	epoch, e := a.epoch(r.Context())
	if e != nil {
		return e
	}
	var baseline int64
	if e = a.DB.QueryRow(r.Context(), `SELECT restore_epoch FROM auth.instance WHERE singleton`).Scan(&baseline); e != nil {
		return e
	}
	if baseline != epoch {
		return fail(503, "RESTORE_GATE_CLOSED", "Business database has not passed the current restore gate")
	}
	var u User
	var pending bool
	var created, accessExpires, registered time.Time
	sessionToken, kind := q.Token, "native"
	if !strings.HasPrefix(q.Authorization, "Bearer ") {
		browser, e := a.browserSession(r.Context(), q.Token)
		if e != nil {
			return e
		}
		if browser.State == "awaiting_browser_confirmation" && q.Path != "/api/auth/csrf" && q.Path != "/api/auth/confirm" && q.Path != "/api/auth/logout" {
			return fail(401, "BROWSER_CONFIRMATION_REQUIRED", "Confirm browser login")
		}
		if e = a.browserFamilyContext(q.Method, q.Path, q.AuthFamily, q.ContextLearn, browser); e != nil {
			return e
		}
		sessionToken, kind = browser.Credentials.Token, "web"
	}
	e = a.DB.QueryRow(r.Context(), `SELECT u.id::text,u.username,u.display_name,u.admin,u.created_at,s.web_pending,s.created_at,s.device_id::text,s.access_expires_at FROM auth.sessions s JOIN auth.users u ON u.id=s.user_id JOIN auth.instance i ON i.singleton WHERE s.token_digest=$1 AND s.auth_kind=$5 AND s.family_id IS NOT NULL AND s.revoked_at IS NULL AND (s.export_pending_operation_id IS NULL OR ((s.export_pending_operation_id='logout:'||s.device_id::text OR s.export_pending_operation_id='revoke-device:'||s.user_id::text||':'||s.device_id::text) AND (($3='GET' AND $4='/api/auth/csrf') OR ($3='POST' AND $4='/api/auth/logout' AND s.export_pending_operation_id='logout:'||s.device_id::text) OR ($3='DELETE' AND $4='/api/auth/sessions/'||s.device_id::text AND s.export_pending_operation_id='revoke-device:'||s.user_id::text||':'||s.device_id::text)))) AND s.expires_at>now() AND s.generation=u.credential_generation AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL AND s.security_version=u.security_version AND s.restore_epoch=$2 AND s.source_context_id='personal' AND s.source_generation=i.personal_source_generation`, tokenDigest(sessionToken), epoch, q.Method, q.Path, kind).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Admin, &registered, &pending, &created, &u.DeviceID, &accessExpires)
	if e != nil {
		if e == pgx.ErrNoRows {
			return fail(401, "UNAUTHORIZED", "Sign in again")
		}
		return e
	}
	if pending && (time.Since(created) > 5*time.Minute || (q.Path != "/api/auth/csrf" && q.Path != "/api/auth/confirm" && q.Path != "/api/auth/logout")) {
		return fail(401, "BROWSER_CONFIRMATION_REQUIRED", "Confirm browser login")
	}
	if !time.Now().Before(accessExpires) {
		return fail(401, "ACCESS_EXPIRED", "Access credential expired; coordinate one refresh")
	}
	if !strings.HasPrefix(q.Authorization, "Bearer ") && q.Method != "GET" && q.Method != "HEAD" {
		if !bytes.Equal([]byte(q.CSRF), []byte(a.csrf(q.Token))) {
			return fail(403, "CSRF_REQUIRED", "Invalid CSRF proof")
		}
		if q.Origin == "" || q.Origin != a.Config.PublicURL {
			return fail(403, "ORIGIN_REJECTED", "Cross-origin request rejected")
		}
	}
	u.CreatedAt = registered.UTC().Format(time.RFC3339Nano)
	u.RestoreEpoch = str(epoch)
	respond(w, 200, u)
	return nil
}
func (a *App) register(w http.ResponseWriter, r *http.Request) error {
	return a.authenticate(w, r, true)
}
func (a *App) login(w http.ResponseWriter, r *http.Request) error { return a.authenticate(w, r, false) }
func (a *App) authenticate(w http.ResponseWriter, r *http.Request, register bool) error {
	if e := a.anonymousAuthOrigin(r); e != nil {
		return e
	}
	var q registerRequest
	if e := decode(r, &q); e != nil {
		return e
	}
	reauthenticate := r.URL.Path == "/api/auth/reauthenticate"
	predecessors, e := authenticationPredecessors(q, reauthenticate)
	if e != nil {
		return e
	}
	epoch, e := a.epoch(r.Context())
	if e != nil {
		return e
	}
	var baseline int64
	if e = a.DB.QueryRow(r.Context(), `SELECT restore_epoch FROM auth.instance WHERE singleton`).Scan(&baseline); e != nil {
		return e
	}
	if baseline != epoch {
		return fail(503, "RESTORE_GATE_CLOSED", "Activate the reconciled restore before authentication")
	}
	name, e := normalizeUsername(q.Username)
	if e != nil {
		return e
	}
	password, e := normalizePassword(q.Password)
	if e != nil {
		return e
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 || len(q.DeviceName) > 100 {
		return fail(400, "INVALID_REQUEST_ID", "A stable requestId of 8–128 bytes is required")
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if e = a.rate(r.Context(), "ip:"+host, 30); e != nil {
		return e
	}
	if e = a.rate(r.Context(), "user:"+name, 5); e != nil {
		return e
	}
	purpose := "login"
	if reauthenticate {
		purpose = "reauthentication"
	}
	if register {
		if e = a.requireFeature(r.Context(), "registration"); e != nil {
			return e
		}
		reserved := map[string]bool{"admin": true, "administrator": true, "root": true, "system": true, "jobsim_admin": true}
		if reserved[name] {
			return fail(400, "RESERVED_USERNAME", "Username reserved for controlled initialization")
		}
		purpose = "registration"
	}
	q.Password = password
	q.Username = name
	web := r.Header.Get("X-JobsIM-Client") == "web"
	b, _ := json.Marshal(struct {
		Request registerRequest `json:"request"`
		Web     bool            `json:"web"`
	}{q, web})
	digest := keyedDigest(a.Config.Key, purpose, b)
	var priorDigest, delivery []byte
	var priorUser, priorDevice, deliveryVersion string
	var expires time.Time
	e = a.DB.QueryRow(r.Context(), `SELECT request_digest,user_id::text,device_id::text,delivery_ciphertext,expires_at,delivery_version FROM auth.local_auth_requests WHERE request_id=$1 AND purpose=$2`, q.RequestID, purpose).Scan(&priorDigest, &priorUser, &priorDevice, &delivery, &expires, &deliveryVersion)
	if e == nil {
		if deliveryVersion != "refresh-auth-v1" {
			return fail(401, "DELIVERY_EXPIRED", "Legacy authentication delivery was invalidated; authenticate with a new request")
		}
		if !bytes.Equal(priorDigest, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Same requestId with different content")
		}
		if time.Now().After(expires) {
			return fail(401, "DELIVERY_EXPIRED", "Use a new authenticated request")
		}
		encoded, e := a.unseal(delivery, a.Config.DeploymentID+":"+purpose+":"+q.RequestID+":"+priorUser+":"+priorDevice)
		if e != nil {
			return e
		}
		var original initialAuthDelivery
		if json.Unmarshal([]byte(encoded), &original) != nil || original.Result.FamilyID == "" || web != (original.BrowserCookie != "") {
			return fail(401, "DELIVERY_INVALIDATED", "Original authentication delivery is no longer valid")
		}
		if register {
			if e = a.completeRegistration(r.Context(), priorUser, name, q.RequestID); e != nil {
				return e
			}
		}
		var validDelivery bool
		if e = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.sessions s JOIN auth.users u ON u.id=s.user_id JOIN auth.instance i ON i.singleton WHERE s.token_digest=$1 AND s.refresh_digest=$5 AND s.refresh_generation=$6::bigint AND s.family_id=$7 AND s.user_id=$2 AND s.device_id=$3 AND s.restore_epoch=$4 AND s.generation=u.credential_generation AND s.security_version=u.security_version AND s.source_generation=i.personal_source_generation AND s.source_context_id='personal' AND s.revoked_at IS NULL AND s.access_expires_at>now() AND s.expires_at>now() AND s.export_pending_operation_id IS NULL AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL)`, tokenDigest(original.Result.Token), priorUser, priorDevice, epoch, tokenDigest(original.Result.RefreshToken), original.Result.RefreshGeneration, original.Result.FamilyID).Scan(&validDelivery); e != nil {
			return e
		}
		if !validDelivery {
			return fail(401, "DELIVERY_INVALIDATED", "Original session no longer valid")
		}
		u, e := a.getUser(r.Context(), priorUser)
		if e != nil {
			return e
		}
		original.Result.User = u
		return a.deliverSession(w, r, original)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if e = a.passwordSlot(r.Context()); e != nil {
		return e
	}
	defer func() { <-a.hashSlots }()
	var u User
	var hash string
	var generation int64
	if register {
		hash = hashPassword(password)
		u = User{ID: newID(), Username: name, DisplayName: name}
	} else {
		e = a.DB.QueryRow(r.Context(), `SELECT id::text,username,display_name,admin,password_hash,credential_generation FROM auth.users WHERE username=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none' AND pending_operation_id IS NULL`, name).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Admin, &hash, &generation)
		if e != nil {
			_ = verifyPassword(hashPassword("dummy timing password value"), password)
			return fail(401, "INVALID_CREDENTIALS", "Invalid credentials")
		}
		if !verifyPassword(hash, password) {
			return fail(401, "INVALID_CREDENTIALS", "Invalid credentials")
		}
		if u.Admin {
			if e = a.consumeMFA(r.Context(), u.ID, q.OTP); e != nil {
				return e
			}
		}
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if register {
		generation = 1
		var occupied bool
		if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.identity_reservations WHERE username=$1)`, name).Scan(&occupied); e != nil {
			return e
		}
		if occupied {
			return fail(409, "USERNAME_RESERVED", "Username reservation requires controlled recovery")
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO auth.users(id,username,display_name,password_hash,state) VALUES($1,$2,$3,$4,'pending')`, u.ID, name, name, hash)
		if e != nil {
			return fail(409, "USERNAME_TAKEN", "Username is already reserved")
		}
	}
	var security, lockedGeneration int64
	var lockedHash, state, recovery, deletion string
	var pending *string
	if e = tx.QueryRow(r.Context(), `SELECT security_version,credential_generation,password_hash,state,recovery_state,deletion_state,pending_operation_id FROM auth.users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&security, &lockedGeneration, &lockedHash, &state, &recovery, &deletion, &pending); e != nil {
		return e
	}
	if lockedGeneration != generation || lockedHash != hash || pending != nil || recovery != "normal" || deletion != "none" || (!register && state != "active") {
		return fail(401, "INVALID_CREDENTIALS", "Credentials changed during authentication")
	}
	var retired bool
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.authentication_retirements WHERE user_id=$1 AND request_id=$2)`, u.ID, q.RequestID).Scan(&retired); e != nil {
		return e
	}
	if retired {
		return fail(401, "AUTHENTICATION_RETIRED", "Original authentication intent was retired by verified recovery; do not resubmit it")
	}
	retiredByJournal, e := a.authenticationRetirement(r.Context(), u.ID, q.RequestID)
	if e != nil {
		return e
	}
	if retiredByJournal != "" {
		return fail(401, "AUTHENTICATION_RETIRED", "Original authentication intent was retired by verified recovery")
	}
	if reauthenticate {
		oldSessions := map[string]bool{}
		if q.PreviousFamilyID != "" {
			var oldID string
			if e = tx.QueryRow(r.Context(), `SELECT id::text FROM auth.sessions WHERE family_id=$1 AND user_id=$2 AND source_context_id='personal' FOR UPDATE`, q.PreviousFamilyID, u.ID).Scan(&oldID); e != nil {
				if e == pgx.ErrNoRows {
					return fail(403, "PREVIOUS_FAMILY_INVALID", "Original family does not belong to this verified account and instance")
				}
				return e
			}
			oldSessions[oldID] = true
		}
		if e = a.retireAuthenticationClosure(r.Context(), tx, u.ID, q.RequestID, predecessors, oldSessions); e != nil {
			return e
		}
		for oldID := range oldSessions {
			if _, e = tx.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, oldID); e != nil {
				return e
			}
		}
	}
	original, e := a.issueSession(r.Context(), tx, u, q.DeviceName, generation, security, epoch, web)
	if e != nil {
		return e
	}
	encoded, e := json.Marshal(original)
	if e != nil {
		return e
	}
	sealed, e := a.seal(string(encoded), a.Config.DeploymentID+":"+purpose+":"+q.RequestID+":"+u.ID+":"+original.Result.DeviceID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO auth.local_auth_requests(request_id,purpose,request_digest,user_id,device_id,delivery_ciphertext,expires_at,delivery_version,session_id,family_id) VALUES($1,$2,$3,$4,$5,$6,now()+interval '300 seconds','refresh-auth-v1',$7,$8)`, q.RequestID, purpose, digest, u.ID, original.Result.DeviceID, sealed, original.SessionID, original.Result.FamilyID)
	if e != nil {
		return fail(409, "REQUEST_IN_PROGRESS", "Retry the original request")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if register {
		if e = a.completeRegistration(r.Context(), u.ID, name, q.RequestID); e != nil {
			return e
		}
	}
	return a.deliverSession(w, r, original)
}
func (a *App) clearCookie(w http.ResponseWriter) {
	for _, name := range []string{"__Host-jobsim_session", "jobsim_dev_session"} {
		http.SetCookie(w, &http.Cookie{Name: name, Path: "/", HttpOnly: true, Secure: !a.Config.InsecureLoopback, MaxAge: -1})
	}
}
func (a *App) getUser(ctx context.Context, id string) (User, error) {
	var u User
	e := a.DB.QueryRow(ctx, `SELECT id::text,username,display_name,admin FROM auth.users WHERE id=$1 AND state='active'`, id).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Admin)
	if e != nil {
		if e == pgx.ErrNoRows {
			return u, fail(404, "USER_NOT_FOUND", "User unavailable")
		}
		return u, e
	}
	return u, nil
}
func (a *App) internalUsers(w http.ResponseWriter, r *http.Request) error {
	var q struct {
		IDs                []string `json:"ids"`
		IncludeUnavailable bool     `json:"includeUnavailable,omitempty"`
		SkipUnavailable    bool     `json:"skipUnavailable,omitempty"`
	}
	if e := decode(r, &q); e != nil {
		return e
	}
	items := []User{}
	if len(q.IDs) > 200 {
		return fail(400, "TOO_MANY_USERS", "Limit 200 users")
	}
	for _, id := range q.IDs {
		u, e := a.getUser(r.Context(), id)
		if e != nil && q.IncludeUnavailable {
			e = a.DB.QueryRow(r.Context(), `SELECT id::text,username,display_name,state FROM auth.users WHERE id=$1 AND state IN('banned','tombstoned')`, id).Scan(&u.ID, &u.Username, &u.DisplayName, &u.State)
			if u.State == "tombstoned" {
				u.Username = ""
				u.DisplayName = ""
			}
		}
		if e != nil {
			var unavailable *apiError
			if q.SkipUnavailable && (e == pgx.ErrNoRows || errors.As(e, &unavailable) && unavailable.Status == 404) {
				continue
			}
			return e
		}
		items = append(items, u)
	}
	if e := a.hydrateProfiles(r.Context(), items); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"items": items})
	return nil
}
func (a *App) users(w http.ResponseWriter, r *http.Request) error {
	if _, e := a.current(r); e != nil {
		return e
	}
	q := strings.ToLower(r.URL.Query().Get("q"))
	rows, e := a.DB.Query(r.Context(), `SELECT id::text,username,display_name FROM auth.users WHERE state='active' AND username LIKE $1 ESCAPE '\' ORDER BY username LIMIT 30`, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(q, "\\", "\\\\"), "%", "\\%"), "_", "\\_")+"%")
	if e != nil {
		return e
	}
	items := []User{}
	for rows.Next() {
		var u User
		if e = rows.Scan(&u.ID, &u.Username, &u.DisplayName); e != nil {
			return e
		}
		items = append(items, u)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	if e = a.hydrateProfiles(r.Context(), items); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"items": items})
	return nil
}

func (a *App) hydrateProfiles(ctx context.Context, users []User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	var profiles struct {
		Items []struct {
			ID                 string `json:"id"`
			DisplayName        string `json:"displayName"`
			ProfileVersion     string `json:"profileVersion"`
			AvatarURL          string `json:"avatarURL"`
			AvatarThumbnailURL string `json:"avatarThumbnailURL"`
		} `json:"items"`
	}
	if e := a.rpc(ctx, "directory", "/internal/profiles", map[string]any{"ids": ids}, &profiles); e != nil {
		return e
	}
	byID := map[string]AvatarProfile{}
	for _, p := range profiles.Items {
		byID[p.ID] = AvatarProfile{DisplayName: p.DisplayName, ProfileVersion: p.ProfileVersion, AvatarURL: p.AvatarURL, AvatarThumbnailURL: p.AvatarThumbnailURL}
	}
	for i := range users {
		if users[i].State == "tombstoned" {
			continue
		}
		if profile, ok := byID[users[i].ID]; ok {
			applyUserProfile(&users[i], profile)
		} else {
			users[i].ProfileVersion = "1"
		}
	}

	return nil
}
func (a *App) sessions(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	rows, e := a.DB.Query(r.Context(), `SELECT device_id::text,device_name,created_at,expires_at FROM auth.sessions WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC`, u.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var created, expires time.Time
		if e = rows.Scan(&id, &name, &created, &expires); e != nil {
			return e
		}
		items = append(items, map[string]any{"deviceId": id, "name": name, "createdAt": created, "expiresAt": expires})
	}
	respond(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func (a *App) revokeDevice(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	payload := map[string]any{"userId": u.ID, "deviceId": r.PathValue("deviceId")}
	op := "revoke-device:" + u.ID + ":" + r.PathValue("deviceId")
	if e = a.protect(r.Context(), op, "intent", "auth.revoke_device", payload); e != nil {
		return e
	}
	if e = a.freezeDeviceExports(r.Context(), u.ID, r.PathValue("deviceId"), op); e != nil {
		return e
	}
	_, e = a.DB.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=now() WHERE user_id=$1 AND device_id=$2`, u.ID, r.PathValue("deviceId"))
	if e != nil {
		return e
	}
	if e = a.protect(r.Context(), op, "result", "auth.revoke_device", payload); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"status": "revoked"})
	return nil
}
func (a *App) recoveryCodes(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	var q struct {
		Password  string `json:"password"`
		RequestID string `json:"requestId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 {
		return fail(400, "REQUEST_ID_REQUIRED", "Stable requestId required")
	}
	var hash string
	if e = a.DB.QueryRow(r.Context(), `SELECT password_hash FROM auth.users WHERE id=$1`, u.ID).Scan(&hash); e != nil {
		return e
	}
	if e = a.passwordSlot(r.Context()); e != nil {
		return e
	}
	valid := verifyPassword(hash, q.Password)
	<-a.hashSlots
	if !valid {
		return fail(403, "REAUTH_REQUIRED", "Current password required")
	}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "recovery-batch:"+u.ID, raw)
	batch := ""
	var generation, credential int64
	codes := []string{}
	created := false
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = tx.QueryRow(r.Context(), `SELECT credential_generation FROM auth.users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&credential); e != nil {
		return e
	}
	var previous, original []byte
	e = tx.QueryRow(r.Context(), `SELECT result,request_digest FROM auth.recovery_operations WHERE request_id=$1 AND user_id=$2 AND kind='generate'`, q.RequestID, u.ID).Scan(&previous, &original)
	if e == nil {
		if !bytes.Equal(original, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original batch request differs")
		}
		var result struct {
			BatchID    string `json:"batchId"`
			Generation int64  `json:"generation"`
			Status     string `json:"status"`
			Credential int64  `json:"credential"`
		}
		if e = json.Unmarshal(previous, &result); e != nil {
			return e
		}
		if result.Status == "ready" {
			respond(w, 200, map[string]any{"status": "already_generated", "batchId": result.BatchID, "codes": []string{}})
			return nil
		}
		batch = result.BatchID
		generation = result.Generation
		if result.Credential != credential {
			return fail(409, "CREDENTIAL_CHANGED", "Reauthenticate and explicitly generate a new batch")
		}
	} else if e == pgx.ErrNoRows {
		batch = newID()
		created = true
		if e = tx.QueryRow(r.Context(), `SELECT GREATEST(COALESCE(max(generation),0),(SELECT recovery_generation FROM auth.users WHERE id=$1))+1 FROM auth.recovery_code_batches WHERE user_id=$1`, u.ID).Scan(&generation); e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO auth.recovery_code_batches(id,user_id,generation,active) VALUES($1,$2,$3,false)`, batch, u.ID, generation)
		if e != nil {
			return e
		}
		for i := 0; i < 10; i++ {
			code := randomToken(20)
			codes = append(codes, code)
			_, e = tx.Exec(r.Context(), `INSERT INTO auth.recovery_codes(id,batch_id,code_digest) VALUES($1,$2,$3)`, newID(), batch, keyedDigest(a.Config.Key, "recovery", []byte(code)))
			if e != nil {
				return e
			}
		}
		result, _ := json.Marshal(map[string]any{"batchId": batch, "generation": generation, "credential": credential, "status": "prepared"})
		_, e = tx.Exec(r.Context(), `INSERT INTO auth.recovery_operations(request_id,user_id,request_digest,kind,result,created_at,state) VALUES($1,$2,$3,'generate',$4,now(),'prepared')`, q.RequestID, u.ID, digest, result)
		if e != nil {
			return e
		}
	} else {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	payload := map[string]any{"userId": u.ID, "batchId": batch, "generation": str(generation)}
	operation := "recovery-batch:" + q.RequestID
	if e = a.protect(r.Context(), operation, "intent", "auth.recovery_batch", payload); e != nil {
		return e
	}
	tx, e = a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var current, latestCredential int64
	if e = tx.QueryRow(r.Context(), `SELECT recovery_generation,credential_generation FROM auth.users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&current, &latestCredential); e != nil {
		return e
	}
	if current > generation || latestCredential != credential {
		return fail(409, "BATCH_SUPERSEDED", "Newer batch or credential already exists")
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1 AND active`, u.ID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_code_batches SET active=true WHERE id=$1 AND user_id=$2`, batch, u.ID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.users SET recovery_generation=$2 WHERE id=$1`, u.ID, generation)
	if e != nil {
		return e
	}
	result, _ := json.Marshal(map[string]any{"batchId": batch, "generation": generation, "credential": credential, "status": "ready"})
	_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_operations SET state='completed',result=$2 WHERE request_id=$1`, q.RequestID, result)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if e = a.protect(r.Context(), operation, "result", "auth.recovery_batch", payload); e != nil {
		return e
	}
	if !created {
		codes = []string{}
	}
	respond(w, 200, map[string]any{"batchId": batch, "codes": codes})
	return nil
}
func (a *App) recoverAccount(w http.ResponseWriter, r *http.Request) error {
	return a.recoverWithCredential(w, r, false)
}
func (a *App) recoverWithCredential(w http.ResponseWriter, r *http.Request, reset bool) error {
	if e := a.anonymousAuthOrigin(r); e != nil {
		return e
	}
	epoch, e := a.currentIdentityEpoch(r.Context())
	if e != nil {
		return e
	}
	kind := "recover"
	if reset {
		kind = "reset-password"
	}
	var q struct {
		Grant     string `json:"grant,omitempty"`
		Username  string `json:"username"`
		Code      string `json:"code"`
		Password  string `json:"password"`
		RequestID string `json:"requestId"`
	}
	if e := decode(r, &q); e != nil {
		return e
	}
	name, e := normalizeUsername(q.Username)
	if e != nil {
		return e
	}
	password, e := normalizePassword(q.Password)
	if e != nil {
		return e
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 {
		return fail(400, "REQUEST_ID_REQUIRED", "Stable recovery requestId required")
	}
	if e = a.rate(r.Context(), "recovery:"+name, 5); e != nil {
		return e
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if e = a.rate(r.Context(), "recovery-ip:"+host, 20); e != nil {
		return e
	}
	q.Username = name
	q.Password = password
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, kind, raw)
	if e = a.passwordSlot(r.Context()); e != nil {
		return e
	}
	hash := hashPassword(password)
	<-a.hashSlots
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var user string
	var generation, security int64
	var pending *string
	e = tx.QueryRow(r.Context(), `SELECT id::text,credential_generation,security_version,pending_operation_id FROM auth.users WHERE username=$1 AND deletion_state='none' AND (($2=false AND state='active' AND recovery_state='normal') OR ($2=true AND state IN('active','banned') AND recovery_state IN('normal','reset_required','recovery_required'))) FOR UPDATE`, name, reset).Scan(&user, &generation, &security, &pending)
	if e != nil {
		return fail(403, "RECOVERY_REJECTED", "Recovery proof rejected")
	}
	var oldDigest, result, planned []byte
	var state string
	e = tx.QueryRow(r.Context(), `SELECT request_digest,state,result,planned_metadata FROM auth.recovery_operations WHERE request_id=$1 AND user_id=$2 AND kind=$3 FOR UPDATE`, q.RequestID, user, kind).Scan(&oldDigest, &state, &result, &planned)
	exists := e == nil
	if e != nil && e != pgx.ErrNoRows {
		return e
	}
	if exists {
		if !bytes.Equal(digest, oldDigest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original recovery operation differs")
		}
		if state == "failed" {
			return fail(410, "RECOVERY_EXPIRED", "Original credential expired; request a new controlled reset")
		}
		if state == "completed" {
			tx.Rollback(r.Context())
			if len(planned) > 0 {
				var original map[string]string
				if e = json.Unmarshal(planned, &original); e != nil {
					return e
				}
				if e = a.protect(r.Context(), kind+":"+q.RequestID, "result", "auth.invalidate_user", map[string]any{"userId": user, "generation": original["generation"], "securityVersion": original["securityVersion"]}); e != nil {
					return e
				}
			}
			respond(w, 200, map[string]string{"status": "recovered", "result": "original_operation"})
			return nil
		}
	}
	if pending != nil && *pending != q.RequestID {
		return fail(409, "OPERATION_PENDING", "Another safety operation already owns the account")
	}
	var code string
	if reset {
		if q.Code != "" || q.Grant == "" {
			return fail(400, "RESET_GRANT_REQUIRED", "Use only the password-reset grant for this endpoint")
		}
		e = tx.QueryRow(r.Context(), `SELECT id::text FROM auth.reset_grants WHERE user_id=$1 AND purpose='password_reset' AND state='active' AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>now() AND restore_epoch=$2 AND issued_security_version=$3 AND credential_digest=$4 FOR UPDATE`, user, epoch, security, keyedDigest(a.Config.Key, "password-reset-grant", []byte(q.Grant))).Scan(&code)
	} else {
		if q.Grant != "" {
			return fail(400, "RECOVERY_PURPOSE_MISMATCH", "Password-reset grants are not account recovery codes")
		}
		e = tx.QueryRow(r.Context(), `SELECT c.id::text FROM auth.recovery_codes c JOIN auth.recovery_code_batches b ON b.id=c.batch_id JOIN auth.users u ON u.id=b.user_id WHERE b.user_id=$1 AND b.active AND b.generation=u.recovery_generation AND c.consumed_at IS NULL AND c.code_digest=$2 FOR UPDATE OF b,c`, user, keyedDigest(a.Config.Key, "recovery", []byte(q.Code))).Scan(&code)
	}
	if e != nil {
		return fail(403, "RECOVERY_REJECTED", "Recovery proof rejected")
	}
	metadata := map[string]string{"codeId": code, "generation": str(generation + 1), "securityVersion": str(security + 1)}
	if exists {
		if e = json.Unmarshal(result, &metadata); e != nil {
			return e
		}
		if metadata["codeId"] != code || metadata["generation"] != str(generation+1) || metadata["securityVersion"] != str(security+1) {
			return fail(409, "RECOVERY_SUPERSEDED", "Original recovery context no longer matches")
		}
	} else {
		result, _ = json.Marshal(metadata)
		_, e = tx.Exec(r.Context(), `INSERT INTO auth.recovery_operations(request_id,user_id,request_digest,kind,result,state,planned_metadata) VALUES($1,$2,$3,$5,$4,'prepared',$4)`, q.RequestID, user, digest, result, kind)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.users SET pending_operation_id=$2 WHERE id=$1`, user, q.RequestID)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	payload := map[string]any{"userId": user, "generation": metadata["generation"], "securityVersion": metadata["securityVersion"]}
	operation := kind + ":" + q.RequestID
	if e = a.protect(r.Context(), operation, "intent", "auth.invalidate_user", payload); e != nil {
		return e
	}
	if e = a.freezeUserExports(r.Context(), user, operation); e != nil {
		return e
	}
	tx, e = a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var currentPending *string
	if e = tx.QueryRow(r.Context(), `SELECT pending_operation_id FROM auth.users WHERE id=$1 AND deletion_state='none' AND (($2=false AND state='active' AND recovery_state='normal') OR ($2=true AND state IN('active','banned') AND recovery_state IN('normal','reset_required','recovery_required'))) FOR UPDATE`, user, reset).Scan(&currentPending); e != nil {
		return fail(403, "RECOVERY_REJECTED", "Account state changed")
	}
	if currentPending == nil || *currentPending != q.RequestID {
		return fail(409, "RECOVERY_SUPERSEDED", "Original operation no longer owns the account")
	}
	query := `UPDATE auth.recovery_codes SET consumed_at=now() WHERE id=$1 AND consumed_at IS NULL`
	if reset {
		query = `UPDATE auth.reset_grants SET consumed_at=now(),state='consumed' WHERE id=$1 AND state='active' AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>now()`
	}
	tag, e := tx.Exec(r.Context(), query, code)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		if reset {
			// A process may pause beyond the grant TTL after the denial intent.
			// Close that original operation while retaining the denial generation.
			_, e = tx.Exec(r.Context(), `UPDATE auth.users SET credential_generation=GREATEST(credential_generation,$2::bigint),security_version=GREATEST(security_version,$3::bigint),recovery_state='recovery_required',pending_operation_id=NULL,row_version=row_version+1 WHERE id=$1`, user, metadata["generation"], metadata["securityVersion"])
			if e != nil {
				return e
			}
			_, e = tx.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=now() WHERE user_id=$1`, user)
			if e != nil {
				return e
			}
			_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1`, user)
			if e != nil {
				return e
			}
			_, e = tx.Exec(r.Context(), `UPDATE auth.reset_grants SET revoked_at=now(),state='revoked' WHERE id=$1`, code)
			if e != nil {
				return e
			}
			_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_operations SET state='failed',result='{"status":"expired"}' WHERE request_id=$1`, q.RequestID)
			if e != nil {
				return e
			}
			if e = tx.Commit(r.Context()); e != nil {
				return e
			}
			if e = a.protect(r.Context(), operation, "result", "auth.invalidate_user", payload); e != nil {
				return e
			}
			return fail(410, "RECOVERY_EXPIRED", "Original credential expired; request a new controlled reset")
		}
		return fail(409, "RECOVERY_CONSUMED", "Recovery proof already consumed")
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.users SET password_hash=$2,credential_generation=$3,security_version=$4,recovery_state='normal',pending_operation_id=NULL,row_version=row_version+1 WHERE id=$1`, user, hash, metadata["generation"], metadata["securityVersion"])
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.sessions SET revoked_at=now() WHERE user_id=$1`, user)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1`, user)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.reset_grants SET revoked_at=now(),state='revoked' WHERE user_id=$1 AND state='active'`, user)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE auth.recovery_operations SET state='completed',result='{"status":"recovered"}' WHERE request_id=$1`, q.RequestID)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if e = a.protect(r.Context(), operation, "result", "auth.invalidate_user", payload); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"status": "recovered"})
	return nil
}
func (a *App) currentIdentityEpoch(ctx context.Context) (int64, error) {
	epoch, e := a.epoch(ctx)
	if e != nil {
		return 0, e
	}
	var baseline int64
	if e = a.DB.QueryRow(ctx, `SELECT restore_epoch FROM auth.instance WHERE singleton`).Scan(&baseline); e != nil {
		return 0, e
	}
	if baseline != epoch {
		return 0, fail(503, "RESTORE_GATE_CLOSED", "Business database has not passed the current restore gate")
	}
	return epoch, nil
}
func (a *App) completeRegistration(ctx context.Context, user, name, request string) error {
	payload := map[string]any{"userId": user, "username": name}
	operation := "register:" + request
	if e := a.protect(ctx, operation, "intent", "auth.reserve_username", payload); e != nil {
		return e
	}
	if _, e := a.DB.Exec(ctx, `UPDATE auth.users SET state='active' WHERE id=$1 AND username=$2 AND state='pending'`, user, name); e != nil {
		return e
	}
	return a.protect(ctx, operation, "result", "auth.reserve_username", payload)
}

// Anonymous auth writes have no authenticated CSRF cookie yet. Require the Web
// custom header, same Origin and JSON; no cross-origin CORS permission is issued.
func (a *App) anonymousAuthOrigin(r *http.Request) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return fail(415, "JSON_REQUIRED", "Authentication requires application/json")
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != a.Config.PublicURL {
		return fail(403, "ORIGIN_REJECTED", "Cross-origin authentication rejected")
	}
	if origin != "" && r.Header.Get("X-JobsIM-Client") != "web" {
		return fail(403, "BROWSER_CLIENT_REQUIRED", "Browser authentication must use the sealed BFF flow")
	}
	if r.Header.Get("X-JobsIM-Client") == "web" && origin != a.Config.PublicURL {
		return fail(403, "ORIGIN_REQUIRED", "Web authentication requires the same Origin")
	}
	return nil
}

func applyUserProfile(u *User, p AvatarProfile) {
	if p.DisplayName != "" {
		u.DisplayName = p.DisplayName
	}
	u.Signature = p.Signature
	u.ProfileVersion = p.ProfileVersion
	u.AvatarURL = p.AvatarURL
	u.AvatarThumbnailURL = p.AvatarThumbnailURL
}
