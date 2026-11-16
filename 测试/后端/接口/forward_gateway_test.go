package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGatewayProjectionModeCoverage(t *testing.T) {
	cases := []struct{ method, path, mode string }{
		{"GET", "/api/search", "shared"}, {"GET", "/api/favorites", "shared"},
		{"GET", "/api/conversations/c/messages", "shared"}, {"POST", "/api/conversations/c/messages", "shared"},
		{"POST", "/api/conversations/c/message-results", "shared"}, {"GET", "/api/conversations/c/announcement", "shared"},
		{"GET", "/api/messages/m", "shared"}, {"HEAD", "/api/messages/m/attachment", "shared"},
		{"PATCH", "/api/messages/m", "exclusive"}, {"DELETE", "/api/messages/m", "exclusive"},
		{"POST", "/api/messages/m/reactions", "shared"}, {"DELETE", "/api/messages/m/reactions/ok", "shared"},
		{"PUT", "/api/messages/m/favorite", "shared"}, {"DELETE", "/api/messages/m/pin", "shared"},
		{"POST", "/api/auth/recover", ""}, {"PUT", "/api/conversations/c/announcement", ""},
	}
	for _, item := range cases {
		r := httptest.NewRequest(item.method, "https://example.test"+item.path, nil)
		if mode := gatewayProjectionMode(r); mode != item.mode {
			t.Fatalf("projection mode %s %s: %s", item.method, item.path, mode)
		}
	}
}

type temporaryForwardPrincipalTransport struct{ base http.RoundTripper }

func (v temporaryForwardPrincipalTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/internal/forwards/principal" {
		return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"TEST_PRINCIPAL_UNAVAILABLE","message":"test-only transient dependency failure"}}`)), Request: r}, nil
	}
	return v.base.RoundTrip(r)
}

func TestForwardTemporaryProjectionRecoversAtSameVersion(t *testing.T) {
	path := os.Getenv("JOBS_IM_FORWARD_JSON_FIXTURE")
	if path == "" {
		t.Skip("explicit private isolated forward fixture required")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("private fixture unreadable")
	}
	var fixture forwardJSONFixture
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("private fixture invalid")
	}
	environment, e := os.ReadFile(fixture.EnvironmentFile)
	if e != nil {
		t.Fatal("private environment unreadable")
	}
	var info struct {
		ForwardTestRealm bool   `json:"forwardTestRealm"`
		StateDir         string `json:"stateDir"`
		BusinessURL      string `json:"businessURL"`
		JournalURL       string `json:"journalURL"`
		InternalBasePort int    `json:"internalBasePort"`
	}
	if json.Unmarshal(environment, &info) != nil || !info.ForwardTestRealm {
		t.Fatal("isolated forward realm required")
	}
	roles := map[string]string{}
	for i, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
		roles[role] = fmt.Sprintf("http://127.0.0.1:%d", info.InternalBasePort+i)
	}
	configuration, e := PrepareConfig(Config{StateDir: info.StateDir, DatabaseURL: info.BusinessURL, JournalDatabaseURL: info.JournalURL, RoleURLs: roles})
	if e != nil {
		t.Fatal("private message configuration invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	message, e := NewApp(ctx, configuration, "message")
	if e != nil {
		t.Fatal("message role could not initialize")
	}
	defer message.Close()
	request := func(method, path, token string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://public.test"+path, bytes.NewReader(body)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		message.Handler().ServeHTTP(w, r)
		return w
	}
	read := request("GET", "/api/messages/"+fixture.MessageID, fixture.RecipientToken, nil)
	var original Message
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &original) != nil || original.ForwardUnavailable || original.Text == "" {
		t.Fatal("fresh active fixture required")
	}
	transport := http.DefaultTransport
	http.DefaultTransport = temporaryForwardPrincipalTransport{transport}
	defer func() { http.DefaultTransport = transport }()
	checks := 0
	for _, item := range []struct {
		method, path, token string
		body                []byte
		list                bool
	}{
		{"GET", "/api/messages/" + fixture.MessageID, fixture.RecipientToken, nil, false},
		{"GET", "/api/conversations/" + fixture.TargetID + "/messages?after=0", fixture.RecipientToken, nil, true},
		{"POST", "/api/conversations/" + fixture.TargetID + "/message-results", fixture.ActorToken, fixture.OriginalBody, false},
	} {
		response := request(item.method, item.path, item.token, item.body)
		if response.Code != 200 {
			t.Fatal("temporary projection must remain HTTP200 current placeholder")
		}
		var current Message
		if item.list {
			var page struct {
				Items []Message `json:"items"`
			}
			if json.Unmarshal(response.Body.Bytes(), &page) != nil {
				t.Fatal("history response invalid")
			}
			for _, m := range page.Items {
				if m.ID == original.ID {
					current = m
				}
			}
		} else if json.Unmarshal(response.Body.Bytes(), &current) != nil {
			t.Fatal("message response invalid")
		}
		if current.ID != original.ID || !current.Forwarded || !current.ForwardUnavailable || current.Text != "" || current.AttachmentID != "" || current.AttachmentURL != "" || current.ForwardAdmission != "" || current.Version != original.Version {
			t.Fatal("temporary placeholder exposed content or fabricated permanent content version")
		}
		checks++
	}
	for _, path := range []string{"/api/search?q=" + fixture.Query, "/api/favorites"} {
		if request("GET", path, fixture.RecipientToken, nil).Code != 503 {
			t.Fatal("uncertain search/favorite pagination must reject whole page")
		}
		checks++
	}
	http.DefaultTransport = transport
	restored := request("GET", "/api/messages/"+fixture.MessageID, fixture.RecipientToken, nil)
	var current Message
	if restored.Code != 200 || json.Unmarshal(restored.Body.Bytes(), &current) != nil || current.ForwardUnavailable || current.Text != original.Text || current.Version != original.Version || current.ContentVersion != original.ContentVersion {
		t.Fatal("fresh exact GET must restore current projection at the same version after dependency recovery")
	}
	var active bool
	if message.DB.QueryRow(ctx, `SELECT state='active' FROM message.forward_refs WHERE message_id=$1`, fixture.MessageID).Scan(&active) != nil || !active {
		t.Fatal("temporary projection must not mutate durable reference qualification")
	}
	if destination := os.Getenv("JOBS_IM_FORWARD_TEMPORARY_REPORT"); destination != "" {
		report := map[string]any{"status": "passed", "temporaryRoutes": checks, "messageHistoryResultHTTP200Placeholder": true, "searchFavoriteWholePage503": true, "sameVersionExactGETRecovers": true, "noDurableRefStateChanged": true, "scope": "Current message handler with real isolated PG and authenticated owner RPCs; only the forward principal RPC is process-locally fault-injected, not a deployed outage"}
		raw, _ := json.MarshalIndent(report, "", "  ")
		if os.WriteFile(destination, append(raw, '\n'), 0600) != nil {
			t.Fatal("structured report write failed")
		}
	}
}

type forwardJSONFixture struct {
	EnvironmentFile string          `json:"environmentFile"`
	RecipientToken  string          `json:"recipientToken"`
	ActorToken      string          `json:"actorToken"`
	TargetID        string          `json:"targetId"`
	MessageID       string          `json:"messageId"`
	Query           string          `json:"query"`
	OriginalBody    json.RawMessage `json:"originalBody"`
}

type projectionGateWriter struct {
	header  http.Header
	started chan struct{}
	ctx     context.Context
	once    sync.Once
	status  int
}

func (w *projectionGateWriter) Header() http.Header  { return w.header }
func (w *projectionGateWriter) WriteHeader(code int) { w.status = code }
func (w *projectionGateWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.ctx.Done()
	return 0, w.ctx.Err()
}

type projectionMarkerTransport struct {
	base   http.RoundTripper
	marker string
	once   sync.Once
}

func (v *projectionMarkerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Header.Get("X-JobsIM-Projection-Fence") != "" {
		v.once.Do(func() {
			// Deliberately appears after the outer fence's marker check. The
			// authenticated inner role must not promote a second nested lock.
			_ = os.WriteFile(v.marker, []byte(`{"testOnly":"projection-promotion-race"}`), 0600)
		})
	}
	return v.base.RoundTrip(r)
}

func TestGatewayJSONFlushCancellationAndUntrustedProof(t *testing.T) {
	path := os.Getenv("JOBS_IM_FORWARD_JSON_FIXTURE")
	if path == "" {
		t.Skip("explicit private isolated forward fixture required")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("private fixture unreadable")
	}
	var fixture forwardJSONFixture
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("private fixture invalid")
	}
	environment, e := os.ReadFile(fixture.EnvironmentFile)
	if e != nil {
		t.Fatal("private environment unreadable")
	}
	var info struct {
		ForwardTestRealm bool   `json:"forwardTestRealm"`
		StateDir         string `json:"stateDir"`
		BusinessURL      string `json:"businessURL"`
		JournalURL       string `json:"journalURL"`
		InternalBasePort int    `json:"internalBasePort"`
	}
	if json.Unmarshal(environment, &info) != nil || !info.ForwardTestRealm {
		t.Fatal("isolated forward realm required")
	}
	roles := map[string]string{}
	for i, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
		roles[role] = fmt.Sprintf("http://127.0.0.1:%d", info.InternalBasePort+i)
	}
	configuration, e := PrepareConfig(Config{StateDir: info.StateDir, DatabaseURL: info.BusinessURL, JournalDatabaseURL: info.JournalURL, RoleURLs: roles})
	if e != nil {
		t.Fatal("private gateway configuration invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	gateway, e := NewApp(ctx, configuration, "gateway")
	if e != nil {
		t.Fatal("gateway could not initialize")
	}
	defer gateway.Close()
	cases := []struct {
		name, method, path, token string
		body                      []byte
	}{
		{"history", "GET", "/api/conversations/" + fixture.TargetID + "/messages?after=1:0&limit=200", fixture.RecipientToken, nil},
		{"search", "GET", "/api/search?q=" + fixture.Query, fixture.RecipientToken, nil},
		{"favorites", "GET", "/api/favorites", fixture.RecipientToken, nil},
		{"announcement", "GET", "/api/conversations/" + fixture.TargetID + "/announcement", fixture.RecipientToken, nil},
		{"original-result", "POST", "/api/conversations/" + fixture.TargetID + "/message-results", fixture.ActorToken, fixture.OriginalBody},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			requestCtx, disconnect := context.WithCancel(ctx)
			defer disconnect()
			r := httptest.NewRequest(item.method, "https://public.test"+item.path, bytes.NewReader(item.body)).WithContext(requestCtx)
			r.Header.Set("Authorization", "Bearer "+item.token)
			r.Header.Set("Content-Type", "application/json")
			// Even a cryptographically valid but user-supplied internal header
			// cannot bypass the public boundary or select its locking mode.
			r.Header.Set("X-JobsIM-Projection-Fence-Mode", "exclusive")
			r.Header.Set("X-JobsIM-Projection-Fence", gateway.gatewayProjectionProof(r, "exclusive"))
			writer := &projectionGateWriter{header: http.Header{}, started: make(chan struct{}), ctx: requestCtx}
			held, e := gateway.admissionFence(ctx, true)
			if e != nil {
				t.Fatal("test EX fence unavailable")
			}
			marker := filepath.Join(configuration.StateDir, "Admission", "Pending", "projection-test-"+newID()+".json")
			done := make(chan struct{})
			originalTransport := http.DefaultTransport
			http.DefaultTransport = &projectionMarkerTransport{base: originalTransport, marker: marker}
			defer func() {
				_ = os.Remove(marker)
				disconnect()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
				}
				http.DefaultTransport = originalTransport
			}()
			go func() { defer close(done); gateway.Handler().ServeHTTP(writer, r) }()
			select {
			case <-writer.started:
				closeExportFence(held)
				t.Fatal("untrusted public proof bypassed the existing EX fence")
			case <-time.After(80 * time.Millisecond):
			}
			closeExportFence(held)
			select {
			case <-writer.started:
			case <-time.After(12 * time.Second):
				t.Fatal("public projection did not reach flush; possible nested marker-promotion lock")
			}
			if writer.status != 200 {
				t.Fatal("fixture public response was not authorized HTTP200")
			}
			blocked, stop := context.WithTimeout(ctx, 100*time.Millisecond)
			contender, e := gateway.admissionFence(blocked, true)
			stop()
			if e == nil {
				closeExportFence(contender)
				t.Fatal("public JSON flush released its source fence before response completion")
			}
			// This is this test's inert marker, not a business commit record. It
			// is removed while our public SH is still held, before cancellation.
			_ = os.Remove(marker)
			disconnect()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("client disconnect did not drain proxy")
			}
			resumed, e := gateway.admissionFence(ctx, true)
			if e != nil {
				t.Fatal("cancelled public JSON response retained its fence")
			}
			closeExportFence(resumed)
		})
	}
	if t.Failed() {
		return
	}
	if destination := os.Getenv("JOBS_IM_FORWARD_JSON_REPORT"); destination != "" {
		report := map[string]any{"status": "passed", "cases": len(cases), "realRoleHTTP": true, "publicGatewayWriterBlocked": true, "clientCancellationDrains": true, "publicForgedProofCannotBypass": true, "markerInjectedAfterOuterSH": true, "markerFixtureRemovedBeforeRelease": true, "scope": "Current gateway handler and real isolated HTTP roles/PG; separate real TLS slow-client check required"}
		raw, _ := json.MarshalIndent(report, "", "  ")
		if e := os.WriteFile(destination, append(raw, '\n'), 0600); e != nil {
			t.Fatal("structured report write failed")
		}
	}
}
