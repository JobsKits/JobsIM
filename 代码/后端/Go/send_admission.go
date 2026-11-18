package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func init() {
	migrations["directory"] += `ALTER TABLE directory.contacts ADD COLUMN IF NOT EXISTS permission_generation bigint NOT NULL DEFAULT 1 CHECK(permission_generation>0);`
}

// The current deployment uses loopback roles and one local StateDir. This OS
// lock spans independent PostgreSQL databases; it is not a distributed lease.
func normalizeAdmissionState(c Config) (Config, error) {
	if c.StateDir == "" {
		return c, fmt.Errorf("shared local instance state directory required")
	}
	absolute, e := filepath.Abs(c.StateDir)
	if e != nil {
		return c, e
	}
	if e = os.MkdirAll(absolute, 0700); e != nil {
		return c, e
	}
	c.StateDir, e = filepath.EvalSymlinks(absolute)
	if e != nil {
		return c, e
	}
	for _, address := range c.RoleURLs {
		u, e := url.Parse(address)
		if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
			return c, fmt.Errorf("admission fencing requires same-host loopback role URLs")
		}
	}
	return c, nil
}
func (a *App) admissionFenceIdentity() string {
	return base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "same-host-admission-fence-v1", []byte(a.Config.StateDir)))
}

type heldAdmissionContextKey struct{}
type admissionScopeLease struct {
	mu     sync.RWMutex
	active bool
}
type heldAdmissionContext struct {
	Identity, Mode string
	Scope          *admissionScopeLease
}

func (a *App) heldAdmissionScope(ctx context.Context, mode string) (context.Context, func()) {
	scope := &admissionScopeLease{active: true}
	return context.WithValue(ctx, heldAdmissionContextKey{}, heldAdmissionContext{a.admissionFenceIdentity(), mode, scope}), func() { scope.mu.Lock(); scope.active = false; scope.mu.Unlock() }
}
func (a *App) admissionFence(ctx context.Context, exclusive bool) (*os.File, error) {
	path := filepath.Join(a.Config.StateDir, "send-admission.lock")
	if info, e := os.Lstat(path); e == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("admission lock must be a regular local file")
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lockExportFile(ctx, f, exclusive); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func (a *App) verifyAdmissionFence(ctx context.Context) error {
	dir := filepath.Join(a.Config.StateDir, "Admission", "Pending")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	for _, parent := range []string{a.Config.StateDir, filepath.Dir(dir), dir} {
		if e := syncAdmissionDirectory(parent); e != nil {
			return fmt.Errorf("admission state durability unavailable: %w", e)
		}
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	first, e := a.admissionFence(probe, true)
	if e != nil {
		return fmt.Errorf("local admission lock unavailable: %w", e)
	}
	defer closeExportFence(first)
	contender, done := context.WithTimeout(ctx, 30*time.Millisecond)
	defer done()
	second, e := a.admissionFence(contender, true)
	if e == nil {
		closeExportFence(second)
		return fmt.Errorf("filesystem does not enforce admission locks")
	}
	if contender.Err() == nil {
		return fmt.Errorf("admission lock contention validation failed: %w", e)
	}
	return nil
}
func (a *App) withAdmissionFence(pattern string, fn func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
	shared := a.Role == "message" && pattern == "POST /api/conversations/{id}/messages"
	shared = shared || a.Role == "media" && strings.Contains(pattern, "/api/") && reportMediaPath(strings.SplitN(pattern, " ", 2)[1])
	shared = shared || a.Role == "gateway" && pattern == "gateway:shared-projection"
	shared = shared || a.Role == "sync" && strings.Contains(pattern, "/api/sync/")
	shared = shared || a.Role == "media" && (pattern == "GET /api/media/{id}" || pattern == "GET /api/avatars/{id}" || pattern == "POST /internal/receipts/object")
	shared = shared || a.Role == "media" && (pattern == "POST /api/media" || pattern == "POST /api/media/upload-results" || pattern == "GET /api/media/capabilities")
	shared = shared || a.Role == "sync" && strings.Contains(pattern, "/api/conversations/{id}/")
	shared = shared || a.Role == "directory" && strings.Contains(pattern, "/api/me/preferences")
	shared = shared || a.Role == "message" && pattern == "POST /api/conversations/{id}/message-results"
	shared = shared || a.Role == "conversation" && (pattern == "GET /api/conversations/{id}/announcement" || pattern == "GET /api/conversations/{id}/announcement/operations/{requestId}")
	if a.Role == "message" {
		for _, readProjection := range []string{
			"GET /api/messages/{id}", "GET /api/conversations/{id}/messages", "GET /api/search", "GET /api/favorites", "GET /api/messages/{id}/attachment",
			"POST /api/messages/{id}/reactions", "DELETE /api/messages/{id}/reactions/{emoji}",
			"PUT /api/messages/{id}/pin", "DELETE /api/messages/{id}/pin", "PUT /api/messages/{id}/favorite", "DELETE /api/messages/{id}/favorite",
		} {
			shared = shared || pattern == readProjection
		}
	}
	exclusive := a.Role == "gateway" && pattern == "gateway:exclusive-projection"
	exclusive = exclusive || strings.Contains(pattern, " /internal/deletion/")
	exclusive = exclusive || a.Role == "identity" && (pattern == "POST /api/auth/refresh" || pattern == "POST /api/auth/reauthenticate")
	exclusive = exclusive || a.Role == "directory" && (strings.HasPrefix(pattern, "PUT /api/blocks/") || strings.HasPrefix(pattern, "DELETE /api/blocks/") || strings.HasPrefix(pattern, "DELETE /api/contacts/") || strings.HasSuffix(pattern, "/accept"))
	exclusive = exclusive || a.Role == "message" && (pattern == "PATCH /api/messages/{id}" || pattern == "DELETE /api/messages/{id}" || pattern == "POST /internal/forwards/revoke-principal" || pattern == "POST /internal/forwards/revoke-source")
	exclusive = exclusive || a.Role == "message" && pattern == "PUT /api/conversations/{id}/retention"
	exclusive = exclusive || a.Role == "sync" && pattern == "PUT /api/sync/storage"

	exclusive = exclusive || a.Role == "conversation" && (strings.HasPrefix(pattern, "POST /api/conversations") || strings.HasPrefix(pattern, "PUT /api/conversations") || strings.HasPrefix(pattern, "PATCH /api/conversations") || strings.HasPrefix(pattern, "DELETE /api/conversations") || pattern == "POST /internal/owner-unavailable")
	if personalShared, personalExclusive := a.personalExportOwnerFence(pattern); personalShared || personalExclusive {
		shared, exclusive = personalShared, personalExclusive
	}
	if !shared && !exclusive {
		return fn
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		// The authenticated gateway proof represents its still-held public response
		// fence. Reacquiring here could self-deadlock on marker promotion.
		if a.Role == "message" || a.Role == "conversation" || a.Role == "sync" || a.Role == "directory" || a.Role == "identity" || a.Role == "governance" || a.Role == "media" {
			if a.hasGatewayProjectionFence(r, true) {
				ctx, expire := a.heldAdmissionScope(r.Context(), "exclusive")
				defer expire()
				return fn(w, r.WithContext(ctx))
			}
			if a.hasGatewayProjectionFence(r, false) {
				if exclusive {
					return fail(503, "ADMISSION_FENCE_MODE_MISMATCH", "Source write requires its outer exclusive fence")
				}
				ctx, expire := a.heldAdmissionScope(r.Context(), "shared")
				defer expire()
				return fn(w, r.WithContext(ctx))
			}
		}
		writeFence := exclusive
		lock, e := a.admissionFence(r.Context(), writeFence)
		if e != nil {
			return fail(503, "ADMISSION_FENCE_UNAVAILABLE", "Local admission fence unavailable; retry only the original operation")
		}
		if !writeFence {
			pending, checkError := a.admissionMarkers()
			if checkError != nil {
				closeExportFence(lock)
				return checkError
			}
			if len(pending) > 0 {
				closeExportFence(lock)
				lock, e = a.admissionFence(r.Context(), true)
				if e != nil {
					return fail(503, "ADMISSION_FENCE_UNAVAILABLE", "Admission recovery is waiting for active writers")
				}
				writeFence = true
			}
		}
		defer closeExportFence(lock)
		if writeFence {
			pending, checkError := a.admissionMarkers()
			if checkError != nil {
				return checkError
			}
			if len(pending) > 0 {
				if a.Role == "message" {
					e = a.recoverAdmissionMarkers(r.Context())
				} else {
					e = a.rpc(r.Context(), "message", "/internal/admission/recover", map[string]string{}, nil)
				}
				if e != nil {
					return e
				}
			}
		}
		mode := "shared"
		if writeFence {
			mode = "exclusive"
		}
		ctx, expire := a.heldAdmissionScope(r.Context(), mode)
		defer expire()
		return fn(w, r.WithContext(ctx))
	}
}

type admissionMarker struct {
	SenderID       string    `json:"senderId"`
	ClientMsgID    string    `json:"clientMsgId"`
	ConversationID string    `json:"conversationId"`
	Digest         string    `json:"digest"`
	TransactionID  string    `json:"transactionId"`
	Database       string    `json:"database"`
	BackendPID     uint32    `json:"backendPid"`
	BackendStarted time.Time `json:"backendStarted"`
}

func (a *App) admissionMarkers() ([]os.DirEntry, error) {
	dir := filepath.Join(a.Config.StateDir, "Admission", "Pending")
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, fail(503, "ADMISSION_RECOVERY_REQUIRED", "Admission markers cannot be inspected")
	}
	return entries, nil
}
func syncAdmissionDirectory(dir string) error {
	f, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (a *App) markAdmissionCommit(ctx context.Context, tx pgx.Tx, marker admissionMarker) (string, error) {
	marker.Database = base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "admission-database", []byte(a.Config.DatabaseURL)))
	e := tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text,pid,backend_start FROM pg_stat_activity WHERE pid=pg_backend_pid()`).Scan(&marker.TransactionID, &marker.BackendPID, &marker.BackendStarted)
	if e != nil {
		return "", e
	}
	dir := filepath.Join(a.Config.StateDir, "Admission", "Pending")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	path := filepath.Join(dir, base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "admission-marker", []byte(marker.SenderID+":"+marker.ClientMsgID)))+".json")
	raw, _ := json.Marshal(marker)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", fail(503, "ADMISSION_RECOVERY_REQUIRED", "Original admission marker requires recovery")
	}
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	closed := f.Close()
	if e != nil {
		return "", e
	}
	if closed != nil {
		return "", closed
	}
	if e = syncAdmissionDirectory(dir); e != nil {
		return "", e
	}
	return path, nil
}
func clearAdmissionMarker(path string) error {
	if e := os.Remove(path); e != nil {
		return e
	}
	return syncAdmissionDirectory(filepath.Dir(path))
}

// Caller holds the exclusive OS fence. Never clear a marker by elapsed time.
func (a *App) recoverAdmissionMarkers(ctx context.Context) error {
	entries, e := a.admissionMarkers()
	if e != nil {
		return e
	}
	for _, entry := range entries {
		path := filepath.Join(a.Config.StateDir, "Admission", "Pending", entry.Name())
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			return fail(503, "ADMISSION_RECOVERY_REQUIRED", "Unknown admission state retained")
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var marker admissionMarker
		if json.Unmarshal(raw, &marker) != nil || marker.Database != base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "admission-database", []byte(a.Config.DatabaseURL))) || !validObjectID(marker.SenderID) || !validObjectID(marker.ConversationID) || len(marker.ClientMsgID) < 8 || len(marker.TransactionID) == 0 || strings.Trim(marker.TransactionID, "0123456789") != "" {
			return fail(503, "ADMISSION_RECOVERY_REQUIRED", "Original admission database or metadata unavailable")
		}
		var status *string
		if e = a.DB.QueryRow(ctx, `SELECT pg_xact_status($1::xid8)`, marker.TransactionID).Scan(&status); e != nil {
			return fail(503, "ADMISSION_RECOVERY_REQUIRED", "Original transaction status unavailable")
		}
		if status == nil || (*status != "committed" && *status != "aborted") {
			return fail(503, "ADMISSION_RECOVERY_REQUIRED", "Original transaction has not reached a provable outcome")
		}
		var digest []byte
		e = a.DB.QueryRow(ctx, `SELECT payload_digest FROM message.messages WHERE sender_id=$1 AND client_msg_id=$2 AND conversation_id=$3`, marker.SenderID, marker.ClientMsgID, marker.ConversationID).Scan(&digest)
		if *status == "committed" && (e != nil || base64.RawURLEncoding.EncodeToString(digest) != marker.Digest) || *status == "aborted" && e != pgx.ErrNoRows {
			return fail(503, "ADMISSION_RECOVERY_REQUIRED", "Original transaction and message fact disagree")
		}
		if e = clearAdmissionMarker(path); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) admissionRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/admission/recover", internal(func(w http.ResponseWriter, r *http.Request) error {
		if e := a.recoverAdmissionMarkers(r.Context()); e != nil {
			return e
		}
		respond(w, 200, map[string]string{"status": "settled"})
		return nil
	}))
}

func maintenanceAdmissionGuard(ctx context.Context, state string) (*os.File, error) {
	c, e := normalizeAdmissionState(Config{StateDir: state})
	if e != nil {
		return nil, e
	}
	a := &App{Config: c}
	f, e := a.admissionFence(ctx, true)
	if e != nil {
		return nil, e
	}
	entries, e := a.admissionMarkers()
	if e != nil {
		closeExportFence(f)
		return nil, e
	}
	if len(entries) > 0 {
		closeExportFence(f)
		return nil, fmt.Errorf("unresolved send-admission markers retained; reconcile-admissions against the original database before backup or restore")
	}
	return f, nil
}

// Explicit local maintenance: resolve original outcomes without sending a message.
func ReconcileSendAdmissions(ctx context.Context, c Config) error {
	var e error
	c, e = normalizeAdmissionState(c)
	if e != nil {
		return e
	}
	a := &App{Config: c, Role: "message"}
	f, e := a.admissionFence(ctx, true)
	if e != nil {
		return e
	}
	defer closeExportFence(f)
	a.DB, e = pgxpool.New(ctx, c.DatabaseURL)
	if e != nil {
		return fmt.Errorf("original message database unavailable")
	}
	defer a.DB.Close()
	var deployment string
	if e = a.DB.QueryRow(ctx, `SELECT deployment_id FROM message.instance WHERE singleton`).Scan(&deployment); e != nil || deployment != c.DeploymentID {
		return fmt.Errorf("original message database identity mismatch")
	}
	return a.recoverAdmissionMarkers(ctx)
}
func (a *App) sendAdmission(user, conversation, kind string, membership, role int64, permission string) string {
	material := strings.Join([]string{user, conversation, kind, str(membership), str(role), permission}, "\x00")
	return "sa1." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "send-admission-v1", []byte(material)))
}
func sameAdmission(a, b string) bool { return a != "" && b != "" && hmac.Equal([]byte(a), []byte(b)) }

func (a *App) relationshipAdmission(ctx context.Context, first, second string) (blocked, friends bool, generation string, err error) {
	var result struct {
		Blocked              bool   `json:"blocked"`
		Friends              bool   `json:"friends"`
		PermissionGeneration string `json:"permissionGeneration"`
	}
	err = a.rpc(ctx, "directory", "/internal/relationship", map[string]string{"first": first, "second": second}, &result)
	return result.Blocked, result.Friends, result.PermissionGeneration, err
}
