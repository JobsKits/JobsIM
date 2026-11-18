package jobsim

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The dedicated harness attaches the not-yet-public export routes explicitly.
// It verifies real PG/journal/filesystem protocol primitives without claiming
// that production mutation hooks or the complete account export are delivered.
func TestExportRealProtocolAndWriterFence(t *testing.T) {
	path := os.Getenv("JOBS_IM_EXPORT_TEST_ENV")
	if path == "" {
		t.Skip("explicit isolated export test environment required")
	}
	var env struct {
		BusinessURL string `json:"businessURL"`
		JournalURL  string `json:"journalURL"`
		StateDir    string `json:"stateDir"`
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(data, &env); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	urls := map[string]string{}
	cfg, e := PrepareConfig(Config{DatabaseURL: env.BusinessURL, JournalDatabaseURL: env.JournalURL, StateDir: env.StateDir, RoleURLs: urls})
	if e != nil {
		t.Fatal(e)
	}
	apps := map[string]*App{}
	servers := []*httptest.Server{}
	var gate atomic.Bool
	started := make(chan exportHolderRequest, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		cancel()
		for _, s := range servers {
			s.Close()
		}
		for _, a := range apps {
			a.Close()
		}
	}()
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
		a, e := NewApp(ctx, cfg, role)
		if e != nil {
			t.Fatal(e)
		}
		apps[role] = a
		mux := http.NewServeMux()
		h := func(pattern string, fn func(http.ResponseWriter, *http.Request) error) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				if e := fn(w, r); e != nil {
					writeError(w, r, e)
				}
			})
		}
		internal := func(fn func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
			return func(w http.ResponseWriter, r *http.Request) error {
				if !hmac.Equal([]byte(r.Header.Get("X-JobsIM-Service-Key")), []byte(base64.RawURLEncoding.EncodeToString(a.Config.Key))) {
					return fail(403, "SERVICE_AUTH_REQUIRED", "Internal authentication required")
				}
				return fn(w, r)
			}
		}
		switch role {
		case "identity":
			a.exportIdentityRoutes(h, internal)
		case "message":
			a.exportMessageRoutes(h, internal)
		case "media":
			a.exportMediaRoutes(h, internal)
		case "governance":
			a.exportInternalRoutes(h, internal)
			a.exportRoutes(h)
		}
		mux.Handle("/", a.Handler())
		handler := http.Handler(mux)
		if role == "message" {
			original := handler
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/internal/exports/read-message" && gate.Load() {
					raw, _ := io.ReadAll(r.Body)
					r.Body = io.NopCloser(bytes.NewReader(raw))
					var q exportHolderRequest
					_ = json.Unmarshal(raw, &q)
					select {
					case started <- q:
					default:
					}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				original.ServeHTTP(w, r)
			})
		}
		server := httptest.NewServer(handler)
		servers = append(servers, server)
		urls[role] = server.URL
	}
	gov := apps["governance"]
	gov.StartExportWorkers(ctx)
	gateway := &App{Config: cfg, Role: "gateway"}
	mux := http.NewServeMux()
	target, _ := url.Parse(urls["governance"])
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux.Handle("/api/me/exports", proxy)
	mux.Handle("/api/me/exports/", proxy)
	mux.Handle("/", gateway.Handler())
	https := httptest.NewTLSServer(mux)
	defer https.Close()
	client := https.Client()
	client.Timeout = 20 * time.Second
	assertions := 0
	tokens := []string{}
	request := func(method, path string, body any, token string, status int) map[string]any {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, https.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := client.Do(req)
		if e != nil {
			t.Fatal("request transport failed")
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != status {
			var err struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(data, &err)
			t.Fatalf("%s %s status=%d code=%s (body redacted)", method, path, response.StatusCode, err.Error.Code)
		}
		assertions++
		var result map[string]any
		if len(data) > 0 {
			if e = json.Unmarshal(data, &result); e != nil {
				t.Fatal("JSON response malformed")
			}
		}
		return result
	}
	defer func() {
		for _, token := range tokens {
			req, _ := http.NewRequest("POST", https.URL+"/api/auth/logout", strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			if response, e := client.Do(req); e == nil {
				response.Body.Close()
			}
		}
	}()
	suffix := strings.ReplaceAll(newID(), "-", "")[:8]
	password := "Export protocol fixture phrase " + suffix
	register := func(prefix string) map[string]any {
		v := request("POST", "/api/auth/register", map[string]string{"username": prefix + suffix, "password": password, "requestId": newID(), "deviceName": "export-protocol"}, "", 200)
		tokens = append(tokens, v["token"].(string))
		return v
	}
	a, b := register("export_a_"), register("export_b_")
	at, bt := a["token"].(string), b["token"].(string)
	aid := a["user"].(map[string]any)["id"].(string)
	bid := b["user"].(map[string]any)["id"].(string)
	friend := request("POST", "/api/contacts/requests", map[string]string{"userId": bid, "requestId": newID()}, at, 200)
	request("POST", "/api/contacts/requests/"+friend["id"].(string)+"/accept", map[string]any{}, bt, 200)
	conversation := request("POST", "/api/conversations", map[string]any{"kind": "direct", "memberIds": []string{bid}}, at, 200)
	conv := conversation["id"].(string)
	boot := request("GET", "/api/bootstrap", nil, "", 200)
	epoch := boot["restoreEpoch"].(string)
	text := "Selected private text from a real PostgreSQL source " + suffix
	message := request("POST", "/api/conversations/"+conv+"/messages", map[string]string{"clientMsgId": newID(), "type": "text", "text": text, "restoreEpoch": epoch, "sendAdmission": conversation["sendAdmission"].(string)}, at, 200)
	blob := bytes.Repeat([]byte("Fixture bytes for a controlled attachment."), 40000)
	var upload bytes.Buffer
	multi := multipart.NewWriter(&upload)
	part, _ := multi.CreateFormFile("file", "fixture.bin")
	part.Write(blob)
	multi.Close()
	up, _ := http.NewRequest("POST", https.URL+"/api/media", &upload)
	up.Header.Set("Content-Type", multi.FormDataContentType())
	up.Header.Set("Authorization", "Bearer "+at)
	response, e := client.Do(up)
	if e != nil {
		t.Fatal(e)
	}
	var media map[string]any
	_ = json.NewDecoder(response.Body).Decode(&media)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("media upload failed; body redacted")
	}
	object := media["id"].(string)
	request("POST", "/api/conversations/"+conv+"/messages", map[string]string{"clientMsgId": newID(), "type": "file", "text": "selected file", "attachmentId": object, "restoreEpoch": epoch, "sendAdmission": conversation["sendAdmission"].(string)}, at, 200)
	op := newID()
	created := request("POST", "/api/me/exports", map[string]string{"requestId": op}, at, 202)
	job := created["job"].(map[string]any)
	jobID := job["id"].(string)
	keyText := created["decryptionKey"].(string)
	repeat := request("POST", "/api/me/exports", map[string]string{"requestId": op}, at, 200)
	if _, present := repeat["decryptionKey"]; present || repeat["job"].(map[string]any)["id"] != jobID {
		t.Fatal("one-time key or original job retry contract failed")
	}
	waitReady := func(id, token string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			v := request("GET", "/api/me/exports/"+id, nil, token, 200)
			if v["state"] == "ready" {
				return
			}
			if v["state"] != "requested" && v["state"] != "building" {
				t.Fatalf("export build entered %v", v["state"])
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("export did not become ready")
	}
	waitReady(jobID, at)
	request("GET", "/api/me/exports/"+jobID, nil, bt, 404)
	ticket := request("POST", "/api/me/exports/"+jobID+"/download-ticket", map[string]any{}, at, 200)
	download, _ := http.NewRequest("GET", https.URL+"/api/me/exports/"+jobID+"/download", nil)
	download.Header.Set("Authorization", "Bearer "+at)
	download.Header.Set("X-JobsIM-Download-Token", ticket["downloadToken"].(string))
	response, e = client.Do(download)
	if e != nil {
		t.Fatal(e)
	}
	ciphertext, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || bytes.Contains(ciphertext, []byte(text)) {
		t.Fatal("encrypted package download failed")
	}
	key, _ := base64.RawURLEncoding.DecodeString(keyText)
	var plain bytes.Buffer
	if e = decryptArchive(bytes.NewReader(ciphertext), &plain, key); e != nil {
		t.Fatal(e)
	}
	archive := tar.NewReader(&plain)
	foundText, foundFile := false, false
	for {
		head, e := archive.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(archive)
		if e != nil {
			t.Fatal(e)
		}
		if strings.HasSuffix(head.Name, message["id"].(string)+".json") {
			foundText = bytes.Contains(data, []byte(text))
		}
		if head.Name == "Media/"+object {
			foundFile = bytes.Equal(data, blob)
		}
	}
	if !foundText || !foundFile {
		t.Fatal("export lost a selected message or actual attachment bytes")
	}
	operation := "retract:" + message["id"].(string)
	if e = apps["message"].freezeMessageExports(ctx, message["id"].(string), operation); e != nil {
		t.Fatal(e)
	}
	request("POST", "/api/me/exports/"+jobID+"/download-ticket", map[string]any{}, at, 409)
	old, e := gov.exportJob(ctx, jobID)
	if e != nil {
		t.Fatal(e)
	}
	prefix, _ := gov.exportPrefix(old)
	if _, e = os.Stat(prefix); !os.IsNotExist(e) {
		t.Fatal("cancelled artifact prefix remains")
	}
	if e = gov.runExportBuild(ctx, jobID); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(prefix); !os.IsNotExist(e) {
		t.Fatal("late worker recreated a cleared prefix")
	}
	if _, e = apps["message"].DB.Exec(ctx, `UPDATE message.messages SET deleted=true,text='',version=version+1,export_pending_operation_id=NULL WHERE id=$1 AND export_pending_operation_id=$2`, message["id"].(string), operation); e != nil {
		t.Fatal(e)
	}
	gate.Store(true)
	second := request("POST", "/api/me/exports", map[string]string{"requestId": newID()}, at, 202)
	secondID := second["job"].(map[string]any)["id"].(string)
	var paused exportHolderRequest
	select {
	case paused = <-started:
	case <-time.After(15 * time.Second):
		t.Fatal("worker did not reach registered-source gate")
	}
	var registered bool
	if e = apps["message"].DB.QueryRow(ctx, `SELECT state='active' AND journal_seq IS NOT NULL FROM message.export_holders WHERE job_id=$1 AND message_id=$2`, paused.JobID, paused.MessageID).Scan(&registered); e != nil || !registered {
		t.Fatal("body was requested before independent holder confirmation")
	}
	short, done := context.WithTimeout(ctx, 150*time.Millisecond)
	e = apps["message"].freezeMessageExports(short, paused.MessageID, "retract:"+paused.MessageID)
	done()
	if e == nil {
		t.Fatal("cleanup falsely completed while old worker held its writer fence")
	}
	short, done = context.WithTimeout(ctx, 150*time.Millisecond)
	e = gov.invalidateUserExports(short, aid, "account-state-fixture")
	done()
	if e == nil {
		t.Fatal("user retry skipped an already-terminal task whose old writer had not drained")
	}
	state, e := gov.exportJob(ctx, secondID)
	if e != nil {
		t.Fatal(e)
	}
	if state.CleanupState == "cleared" {
		t.Fatal("busy worker was declared drained")
	}
	releaseOnce.Do(func() { close(release) })
	gate.Store(false)
	deadline := time.Now().Add(10 * time.Second)
	for {
		e = apps["message"].freezeMessageExports(ctx, paused.MessageID, "retract:"+paused.MessageID)
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("original pending cleanup did not resume")
		}
		time.Sleep(30 * time.Millisecond)
	}
	if e = gov.invalidateUserExports(ctx, aid, "account-state-fixture"); e != nil {
		t.Fatal("original user cancellation did not finish after worker drain")
	}
	state, e = gov.exportJob(ctx, secondID)
	if e != nil {
		t.Fatal(e)
	}
	prefix, _ = gov.exportPrefix(state)
	if _, e = os.Stat(prefix); !os.IsNotExist(e) {
		t.Fatal("old worker left bytes after cleanup acknowledgement")
	}
	if e = gov.runExportBuild(ctx, secondID); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(prefix); !os.IsNotExist(e) {
		t.Fatal("late old build regained write authority")
	}
	report := map[string]any{"status": "passed", "httpAssertions": assertions, "realPostgreSQLAndIndependentJournal": true, "realMediaBytesEncryptedAndDecoded": true, "mediaByteCount": len(blob), "userRetryDoesNotSkipTerminalPending": true, "oneTimeKeyAndStableTask": true, "holderConfirmedBeforeBody": true, "foreignOwnerDenied": true, "busyWriterCleanupStaysPending": true, "originalCleanupResumesAfterDrain": true, "lateWorkerCannotRecreateClearedPrefix": true, "scope": "explicit private protocol harness; production mutation hooks, full account snapshot and public capability remain unaccepted"}
	encoded, _ := json.MarshalIndent(report, "", "  ")
	if out := os.Getenv("JOBS_IM_EXPORT_TEST_REPORT"); out != "" {
		if e = os.WriteFile(filepath.Clean(out), append(encoded, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Log(string(encoded))
}
