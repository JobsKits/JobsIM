package jobsim

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type personalExportBufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
	err    error
}

func (w *personalExportBufferedResponse) Header() http.Header { return w.header }
func (w *personalExportBufferedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *personalExportBufferedResponse) Write(p []byte) (int, error) {
	if w.body.Len()+len(p) > 1<<20 {
		w.err = fmt.Errorf("personal export source response capacity exceeded")
		return 0, w.err
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

// Must be placed inside withAdmissionFence. The EX boundary first drains old
// formal downloads and blocks new grants. The owner transaction then persists
// source changes and revoking holders together. Only after all owner locks have
// been released can cross-owner cleanup finish and the source response flush.
func (a *App) withPersonalExportSourceDrain(pattern string, fn func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
	_, exclusive := a.personalExportOwnerFence(pattern)
	internalDraft := a.Role == "sync" && pattern == "POST /internal/drafts/revoke-reply"
	internalOwner := a.Role == "conversation" && pattern == "POST /internal/owner-unavailable"
	if !exclusive && !internalDraft && !internalOwner {
		return fn
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		staged := &personalExportBufferedResponse{header: make(http.Header)}
		defer func() {
			raw := staged.body.Bytes()
			for i := range raw {
				raw[i] = 0
			}
		}()
		cause := fn(staged, r)
		drainError := a.drainPersonalExportInvalidations(r.Context())
		migrationError := a.drainMigrations(r.Context())
		if cause != nil {
			return cause
		}
		if drainError != nil {
			return fail(503, "EXPORT_CLEANUP_PENDING", "Source result is committed or unknown; retry its original intent while private export cleanup completes")
		}
		if migrationError != nil {
			return fail(503, "MIGRATION_CLEANUP_PENDING", "Source result is committed or unknown; retry its original operation while controlled migration cleanup completes")
		}
		if staged.err != nil {
			return staged.err
		}
		for key, values := range staged.header {
			w.Header()[key] = append([]string(nil), values...)
		}
		status := staged.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, err := w.Write(staged.body.Bytes())
		return err
	}
}

func (a *App) drainPersonalExportInvalidations(ctx context.Context) error {
	owner := personalExportOwner(a.Role)
	if owner == "" {
		return nil
	}
	var exists bool
	if err := a.DB.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, owner+".personal_export_cleanups").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	for batch := 0; batch < 20; batch++ {
		var pending bool
		if err := a.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+owner+".personal_export_cleanups WHERE state='pending')").Scan(&pending); err != nil {
			return err
		}
		if !pending {
			return nil
		}
		if err := a.resumePersonalExportInvalidations(ctx); err != nil {
			return err
		}
	}
	return fail(503, "EXPORT_CLEANUP_PENDING", "Personal export source cleanup has more durable batches")
}

func personalExportGatewayMode(r *http.Request) string {
	path, method := r.URL.Path, r.Method
	if method == "PUT" && (path == "/api/sync/device" || path == "/api/sync/storage") {
		return "exclusive"
	}
	if method == "POST" && strings.HasPrefix(path, "/api/sync/migrations/") && strings.HasSuffix(path, "/cancel") {
		return "exclusive"
	}
	if method == "POST" && path == "/api/auth/password-change-results" {
		return ""
	}
	if path == "/api/me/data-export-results" || path == "/api/me/data-exports" || strings.HasPrefix(path, "/api/me/data-exports/") {
		return "shared"
	}
	if method == "GET" || method == "HEAD" {
		return ""
	}
	if method == "POST" && path == "/api/reports" || method == "PATCH" && strings.HasPrefix(path, "/api/admin/reports/") {
		return "exclusive"
	}
	if selfStatePublicPath(path) {
		if strings.HasSuffix(path, "-results") {
			return "shared"
		}
		return "exclusive"
	}
	if method == "PATCH" && path == "/api/me/preferences" {
		return "exclusive"
	}
	if path == "/api/me" && method == "PATCH" || path == "/api/me/avatar" && method == "POST" || strings.HasPrefix(path, "/api/contacts/") || strings.HasPrefix(path, "/api/blocks/") {
		return "exclusive"
	}
	if strings.HasPrefix(path, "/api/conversations") && !strings.HasSuffix(path, "/messages") && !strings.HasSuffix(path, "/message-results") {
		return "exclusive"
	}
	for _, prefix := range []string{"/api/auth/", "/api/devices/", "/api/recovery/", "/api/admin/users/", "/api/admin/operations/"} {
		if strings.HasPrefix(path, prefix) {
			return "exclusive"
		}
	}
	return ""
}

// These override the earlier broad SH classifications only for source writes.
func (a *App) personalExportOwnerFence(pattern string) (shared, exclusive bool) {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return false, false
	}
	request := &http.Request{Method: parts[0]}
	request.URL = &url.URL{Path: parts[1]}
	mode := personalExportGatewayMode(request)
	if a.Role == "governance" && strings.HasPrefix(parts[1], "/api/me/data-export") {
		return true, false
	}
	if mode == "exclusive" && personalExportOwner(a.Role) != "" {
		return false, true
	}
	return false, false
}

// Called on each metadata owner. Cleanup is durable; ticker passage alone never
// acknowledges a holder. Source actions still perform their synchronous hook.
func (a *App) StartPersonalExportOwnerWorkers(ctx context.Context) {
	if personalExportOwner(a.Role) == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
				_ = a.drainPersonalExportInvalidations(attempt)
				cancel()
			}
		}
	}()
}
