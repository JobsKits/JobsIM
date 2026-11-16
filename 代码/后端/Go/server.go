package jobsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	ReceiptProfiles     []ReceiptProfile
	ExperimentalExports bool
	WebDir              string
	JournalDatabaseURL  string
	DatabaseURL         string
	StateDir            string
	DeploymentID        string
	Name                string
	Key                 []byte
	PublicURL           string
	InsecureLoopback    bool
	RoleURLs            map[string]string
	AdminUsername       string
	AdminPassword       string
	NATSURL             string
	NATSTokenFile       string
}
type App struct {
	Config                 Config
	Role                   string
	DB                     *pgxpool.Pool
	Client                 *http.Client
	hashSlots              chan struct{}
	syncTransportConnected atomic.Bool
	syncPublishConnected   atomic.Bool
	syncConsumeConnected   atomic.Bool
	syncTransportMu        sync.Mutex
}

func PrepareConfig(c Config) (Config, error) {
	if err := validateReceiptProfiles(c.ReceiptProfiles); err != nil {
		return c, err
	}
	if c.StateDir == "" {
		root, e := os.UserConfigDir()
		if e != nil {
			return c, e
		}
		c.StateDir = filepath.Join(root, "JobsIM", "Server")
	}
	if err := os.MkdirAll(c.StateDir, 0700); err != nil {
		return c, err
	}
	if _, e := os.Stat(filepath.Join(c.StateDir, "restore.pending")); e == nil {
		return c, fmt.Errorf("isolated restore is incomplete; runtime remains closed")
	} else if !os.IsNotExist(e) {
		return c, e
	}
	file := filepath.Join(c.StateDir, "instance.json")
	var stored struct {
		DeploymentID string `json:"deploymentId"`
		Key          string `json:"key"`
	}
	b, err := os.ReadFile(file)
	if err == nil {
		if err = json.Unmarshal(b, &stored); err != nil {
			return c, err
		}
	} else if os.IsNotExist(err) {
		stored.DeploymentID = newID()
		stored.Key = randomToken(32)
		b, _ = json.Marshal(stored)
		f, e := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return c, e
		}
		_, err = f.Write(b)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return c, err
		}
		if closeErr != nil {
			return c, closeErr
		}
	} else {
		return c, err
	}
	c.DeploymentID = stored.DeploymentID
	c.Key, err = base64.RawURLEncoding.DecodeString(stored.Key)
	if err != nil || len(c.Key) != 32 {
		return c, fmt.Errorf("invalid instance secret")
	}
	if c.Name == "" {
		c.Name = "JobsIM"
	}
	if c.RoleURLs == nil {
		c.RoleURLs = map[string]string{}
		for i, r := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance", "sync"} {
			c.RoleURLs[r] = fmt.Sprintf("http://127.0.0.1:%d", 8101+i)
		}
	}
	return c, nil
}
func NewApp(ctx context.Context, c Config, role string) (*App, error) {
	var prepareError error
	c, prepareError = normalizeAdmissionState(c)
	if prepareError != nil {
		return nil, prepareError
	}
	a := &App{Config: c, Role: role, Client: &http.Client{Timeout: 5 * time.Second}, hashSlots: make(chan struct{}, 2)}
	if prepareError = a.verifyAdmissionFence(ctx); prepareError != nil {
		return nil, prepareError
	}
	if role != "gateway" {
		if role == "journal" {
			if err := VerifyIndependentJournal(ctx, c.DatabaseURL, c.JournalDatabaseURL); err != nil {
				return nil, err
			}
			c.DatabaseURL = c.JournalDatabaseURL
			a.Config.DatabaseURL = c.DatabaseURL
		}
		if c.DatabaseURL == "" {
			return nil, fmt.Errorf("JOBS_IM_DATABASE_URL is required; no in-memory database fallback")
		}
		pc, err := pgxpool.ParseConfig(c.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid database configuration")
		}
		pc.MaxConns = 8
		pc.ConnConfig.RuntimeParams["application_name"] = "jobsim-" + role
		pc.ConnConfig.RuntimeParams["synchronous_commit"] = "on"
		pc.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
		pc.ConnConfig.RuntimeParams["lock_timeout"] = "5000"
		a.DB, err = pgxpool.NewWithConfig(ctx, pc)
		if err != nil {
			return nil, err
		}
		if err = a.DB.Ping(ctx); err != nil {
			a.DB.Close()
			return nil, fmt.Errorf("PostgreSQL unavailable for %s", role)
		}
		var fsync, fullPages string
		if err = a.DB.QueryRow(ctx, `SELECT current_setting('fsync'),current_setting('full_page_writes')`).Scan(&fsync, &fullPages); err != nil || fsync != "on" || fullPages != "on" {
			a.DB.Close()
			return nil, fmt.Errorf("PostgreSQL fsync/full_page_writes durability configuration is required")
		}
		if err = a.migrate(ctx); err != nil {
			a.DB.Close()
			return nil, err
		}
	}
	return a, nil
}
func (a *App) Close() {
	if a.DB != nil {
		a.DB.Close()
	}
}
func (a *App) rpc(ctx context.Context, role, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "POST", a.Config.RoleURLs[role]+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-JobsIM-Service-Key", base64.RawURLEncoding.EncodeToString(a.Config.Key))
	r.Header.Set("X-JobsIM-Caller", a.Role)
	if held, ok := ctx.Value(heldAdmissionContextKey{}).(heldAdmissionContext); ok {
		if held.Scope == nil {
			return fail(503, "ADMISSION_FENCE_EXPIRED", "Private RPC has no live admission scope")
		}
		held.Scope.mu.RLock()
		defer held.Scope.mu.RUnlock()
		if !held.Scope.active || ctx.Err() != nil || held.Identity != a.admissionFenceIdentity() || (held.Mode != "shared" && held.Mode != "exclusive") {
			return fail(503, "ADMISSION_FENCE_EXPIRED", "Private RPC no longer owns its live admission scope")
		}
		r.Header.Set("X-JobsIM-Projection-Fence-Mode", held.Mode)
		r.Header.Set("X-JobsIM-Projection-Fence", a.gatewayProjectionProof(r, held.Mode))
	}
	res, err := a.Client.Do(r)
	if err != nil {
		return fail(503, "DEPENDENCY_UNAVAILABLE", role+" service unavailable")
	}
	defer res.Body.Close()
	if path == "/internal/member" || path == "/internal/relationship" || path == "/internal/admission/recover" || path == "/internal/read-scopes" || path == "/internal/read-boundaries" || path == "/internal/read-summary" || path == "/internal/announcement-source" || path == "/internal/mentions/snapshot" || strings.HasPrefix(path, "/internal/forwards/") || strings.HasPrefix(path, "/internal/sync/") || strings.HasPrefix(path, "/internal/drafts/") || strings.HasPrefix(path, "/internal/receipts/") || strings.HasPrefix(path, "/internal/personal-exports/") {
		if !hmac.Equal([]byte(res.Header.Get("X-JobsIM-Admission-Fence")), []byte(a.admissionFenceIdentity())) {
			return fail(503, "ADMISSION_FENCE_MISMATCH", "Roles must share the same canonical local instance directory")
		}
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		if e.Error.Code == "" {
			e.Error.Code = "DEPENDENCY_FAILED"
			e.Error.Message = role + " request failed"
		}
		return fail(res.StatusCode, e.Error.Code, e.Error.Message)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
	}
	return nil
}
func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	for _, name := range []string{"__Host-jobsim_session", "jobsim_dev_session"} {
		if c, e := r.Cookie(name); e == nil {
			return c.Value
		}
	}
	return ""
}
func (a *App) current(r *http.Request) (User, error) {
	var out User
	body := map[string]string{"token": bearer(r), "method": r.Method, "origin": r.Header.Get("Origin"), "csrf": r.Header.Get("X-CSRF-Token"), "authorization": r.Header.Get("Authorization"), "path": r.URL.Path, "authFamily": r.Header.Get("X-JobsIM-Auth-Family"), "contextLearn": r.Header.Get("X-JobsIM-Context-Learn")}
	err := a.rpc(r.Context(), "identity", "/internal/authorize", body, &out)
	return out, err
}
func (a *App) member(ctx context.Context, user, conv string) (map[string]any, error) {
	var out map[string]any
	err := a.rpc(ctx, "conversation", "/internal/member", map[string]string{"userId": user, "conversationId": conv}, &out)
	return out, err
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern string, fn func(http.ResponseWriter, *http.Request) error) {
		fn = a.withPersonalExportSourceDrain(pattern, fn)
		fn = a.withAdmissionFence(pattern, fn)
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := fn(w, r); err != nil {
				writeError(w, r, err)
			}
		})
	}
	handle("GET /api/health", func(w http.ResponseWriter, r *http.Request) error {
		if a.DB != nil {
			if err := a.DB.Ping(r.Context()); err != nil {
				return fail(503, "DATABASE_UNAVAILABLE", "Database unavailable")
			}
		}
		respond(w, 200, map[string]string{"status": "ok", "service": a.Role})
		return nil
	})
	if a.Role == "gateway" {
		for _, prefix := range []string{"/api/", "/internal/"} {
			mux.Handle(prefix, a.gateway())
		}
		if a.Config.WebDir != "" {
			mux.Handle("/", a.webHandler())
		}
		return a.securityHeaders(mux)
	}
	internal := func(fn func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
		return func(w http.ResponseWriter, r *http.Request) error {
			want := base64.RawURLEncoding.EncodeToString(a.Config.Key)
			if !hmac.Equal([]byte(r.Header.Get("X-JobsIM-Service-Key")), []byte(want)) {
				return fail(403, "SERVICE_AUTH_REQUIRED", "Internal service authentication required")
			}
			return fn(w, r)
		}
	}
	if syncOwnerSchema(a.Role) != "" {
		a.syncOwnerRoutes(handle, internal)
		a.syncValidationOwnerRoute(handle, internal)
		a.syncHistoricalAccessRoute(handle, internal)
	}
	a.personalExportOwnerRoutes(handle, internal)
	switch a.Role {
	case "sync":
		a.syncRoutes(handle)
		a.syncReceiptRoutes(handle)
		a.selfViewRoutes(handle)
		a.selfReadRoutes(handle)
		a.selfDraftRoutes(handle, internal)
	case "identity":
		a.identityRoutes(handle, internal)
	case "directory":
		a.directoryRoutes(handle, internal)
		a.sourcePreferencesRoutes(handle)
	case "conversation":
		a.conversationRoutes(handle, internal)
		a.searchScopeRoutes(handle, internal)
	case "message":
		a.messageRoutes(handle, internal)
		a.searchRoutes(handle, internal)
		a.readPointMessageRoutes(handle, internal)
		a.readSummaryMessageRoutes(handle, internal)
		a.draftReplyHolderRoutes(handle, internal)
	case "media":
		a.mediaRoutes(handle, internal)
		a.personalExportMediaRoutes(handle, internal)
	case "control":
		a.controlRoutes(handle, internal)
	case "journal":
		a.journalRoutes(handle, internal)
	case "governance":
		a.governanceRoutes(handle, internal)
		a.exportInternalRoutes(handle, internal)
		a.personalExportRoutes(handle)
		a.personalExportGovernanceRoutes(handle, internal)
		if a.Config.ExperimentalExports {
			a.exportRoutes(handle)
		}
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, fail(404, "NOT_FOUND", "API route unavailable"))
	})
	return a.securityHeaders(mux)
}
func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/") {
			w.Header().Set("X-JobsIM-Admission-Fence", a.admissionFenceIdentity())
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		defer func() {
			if recover() != nil {
				writeError(w, r, fmt.Errorf("handler panic"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (a *App) gateway() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/") {
			http.NotFound(w, r)
			return
		}
		role := "identity"
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/api/sync/"):
			role = "sync"
		case selfStatePublicPath(p):
			role = "sync"
		case strings.HasPrefix(p, "/api/me/exports"), p == "/api/me/data-exports", p == "/api/me/data-export-results", strings.HasPrefix(p, "/api/me/data-exports/"):
			role = "governance"
		case strings.HasPrefix(p, "/api/reports"), strings.HasPrefix(p, "/api/admin/reports"), strings.HasPrefix(p, "/api/admin/report-operations"):
			role = "governance"
		case strings.HasPrefix(p, "/api/me/avatar"):
			role = "directory"
		case p == "/api/me/preferences" || p == "/api/me/preferences-results" || strings.HasPrefix(p, "/api/me/preferences/operations/"):
			role = "directory"
		case strings.HasPrefix(p, "/api/avatars/"):
			role = "media"
		case strings.HasPrefix(p, "/api/admin/users"), strings.HasPrefix(p, "/api/admin/operations"):
			role = "identity"
		case p == "/api/me" && r.Method == "PATCH":
			role = "directory"
		case strings.HasPrefix(p, "/api/contacts"), strings.HasPrefix(p, "/api/blocks"):
			role = "directory"
		case strings.HasPrefix(p, "/api/media"):
			role = "media"
		case strings.Contains(p, "/messages"), strings.HasSuffix(p, "/message-results"), strings.HasPrefix(p, "/api/search"), p == "/api/favorites":
			role = "message"
		case strings.HasPrefix(p, "/api/conversations"):
			role = "conversation"
		case p == "/api/bootstrap", strings.HasPrefix(p, "/api/capabilities"), strings.HasPrefix(p, "/api/admin/"):
			role = "control"
		}
		u, e := url.Parse(a.Config.RoleURLs[role])
		if e != nil {
			writeError(w, r, e)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(u)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
			writeError(w, r, fail(503, "DEPENDENCY_UNAVAILABLE", role+" unavailable"))
		}
		r.Header.Del("X-JobsIM-Service-Key")
		r.Header.Del("X-JobsIM-Caller")
		r.Header.Del("X-JobsIM-Forward-Stream-Fence")
		r.Header.Del("X-JobsIM-Projection-Fence")
		r.Header.Del("X-JobsIM-Projection-Fence-Mode")
		if mode := gatewayProjectionMode(r); mode != "" {
			projection := a.withAdmissionFence("gateway:"+mode+"-projection", func(w http.ResponseWriter, request *http.Request) error {
				forward := request.Clone(request.Context())
				forward.Header.Set("X-JobsIM-Projection-Fence-Mode", mode)
				forward.Header.Set("X-JobsIM-Projection-Fence", a.gatewayProjectionProof(forward, mode))
				proxy.ServeHTTP(w, forward)
				return nil
			})
			if e = projection(w, r); e != nil {
				writeError(w, r, e)
			}
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
