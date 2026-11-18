package jobsim

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAdmissionLockChild(t *testing.T) {
	path := os.Getenv("JOBS_IM_ADMISSION_LOCK_CHILD")
	if path == "" {
		return
	}
	a := &App{Config: Config{StateDir: path}}
	f, e := a.admissionFence(context.Background(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer closeExportFence(f)
	if os.Getenv("JOBS_IM_ADMISSION_MARKER_CHILD") == "1" {
		raw, e := os.ReadFile(os.Getenv("JOBS_IM_ADMISSION_TEST_ENV"))
		if e != nil {
			t.Fatal("private fixture unavailable")
		}
		var env struct {
			BusinessURL string `json:"businessURL"`
		}
		if json.Unmarshal(raw, &env) != nil {
			t.Fatal("invalid fixture")
		}
		db, e := pgxpool.New(context.Background(), env.BusinessURL)
		if e != nil {
			t.Fatal(e)
		}
		defer db.Close()
		a.Config.DatabaseURL = env.BusinessURL
		a.Config.Key = []byte("isolated-admission-key-for-testing")
		a.DB = db
		tx, e := db.Begin(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(context.Background())
		_, e = a.markAdmissionCommit(context.Background(), tx, admissionMarker{SenderID: newID(), ConversationID: newID(), ClientMsgID: newID(), Digest: "crash-before-commit"})
		if e != nil {
			t.Fatal(e)
		}
	}
	fmt.Println("locked")
	for {
		time.Sleep(time.Hour)
	}
}
func TestAdmissionLockCancellationCrashAndStateIdentity(t *testing.T) {
	dir := t.TempDir()
	cfg, e := normalizeAdmissionState(Config{StateDir: dir, RoleURLs: map[string]string{"message": "http://127.0.0.1:1"}})
	if e != nil {
		t.Fatal(e)
	}
	a := &App{Config: cfg}
	if e = a.verifyAdmissionFence(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = normalizeAdmissionState(Config{StateDir: dir, RoleURLs: map[string]string{"message": "http://192.0.2.1:1"}}); e == nil {
		t.Fatal("cross-host roles admitted")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestAdmissionLockChild$")
	child.Env = append(os.Environ(), "JOBS_IM_ADMISSION_LOCK_CHILD="+dir)
	out, e := child.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if text, e := bufio.NewReader(out).ReadString('\n'); e != nil || text != "locked\n" {
		t.Fatal("child lock not acquired")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if f, e := a.admissionFence(ctx, false); !errors.Is(e, context.DeadlineExceeded) {
		if f != nil {
			closeExportFence(f)
		}
		t.Fatal("cancelled reader bypassed exclusive child")
	}
	if e = child.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	_ = child.Wait()
	ctx2, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	f, e := a.admissionFence(ctx2, true)
	if e != nil {
		t.Fatal("kernel retained crashed process lock")
	}
	closeExportFence(f)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-JobsIM-Admission-Fence", "other-canonical-state-dir")
		fmt.Fprint(w, `{"kind":"group"}`)
	}))
	defer peer.Close()
	a.Role = "message"
	a.Client = peer.Client()
	a.Config.RoleURLs = map[string]string{"conversation": peer.URL}
	if e = a.rpc(context.Background(), "conversation", "/internal/member", map[string]string{}, nil); e == nil {
		t.Fatal("different role state directory bypassed fence")
	}
}
func TestAdmissionUnknownCommitMarkerRequiresPGProof(t *testing.T) {
	file := os.Getenv("JOBS_IM_ADMISSION_TEST_ENV")
	if file == "" {
		t.Skip("isolated private PG configuration required")
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal("private test config unavailable")
	}
	var env struct {
		BusinessURL string `json:"businessURL"`
	}
	if json.Unmarshal(raw, &env) != nil {
		t.Fatal("invalid private test config")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, env.BusinessURL)
	if e != nil {
		t.Fatal("private database unavailable")
	}
	defer db.Close()
	a := &App{Config: Config{StateDir: t.TempDir(), DatabaseURL: env.BusinessURL, Key: []byte("isolated-admission-key-for-testing")}, DB: db}
	if e = a.verifyAdmissionFence(ctx); e != nil {
		t.Fatal(e)
	}
	marker := admissionMarker{SenderID: newID(), ConversationID: newID(), ClientMsgID: newID(), Digest: base64.RawURLEncoding.EncodeToString([]byte("original-digest"))}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	path, e := a.markAdmissionCommit(ctx, tx, marker)
	if e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = a.recoverAdmissionMarkers(ctx); e == nil {
		t.Fatal("in-progress transaction marker cleared")
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("unknown marker lost")
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if e = a.recoverAdmissionMarkers(ctx); e != nil {
		t.Fatal("proven abort not recoverable", e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("settled abort marker retained")
	}
	// A commit is proven only when its original immutable message fact agrees.
	tx, e = db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	message := newID()
	_, e = tx.Exec(ctx, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text','isolated marker fixture',1)`, message, marker.ConversationID, marker.SenderID, marker.ClientMsgID, []byte("original-digest"))
	if e != nil {
		tx.Rollback(ctx)
		t.Fatal("fixture creation failed")
	}
	defer db.Exec(ctx, `DELETE FROM message.messages WHERE id=$1`, message)
	path, e = a.markAdmissionCommit(ctx, tx, marker)
	if e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = a.recoverAdmissionMarkers(ctx); e != nil {
		t.Fatal("committed original fact not recoverable", e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("proven commit marker retained")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestAdmissionLockChild$")
	child.Env = append(os.Environ(), "JOBS_IM_ADMISSION_LOCK_CHILD="+a.Config.StateDir, "JOBS_IM_ADMISSION_MARKER_CHILD=1")
	out, e := child.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	if text, e := bufio.NewReader(out).ReadString('\n'); e != nil || text != "locked\n" {
		child.Process.Kill()
		child.Wait()
		t.Fatal("child marker was not durable")
	}
	entries, e := a.admissionMarkers()
	if e != nil || len(entries) != 1 {
		child.Process.Kill()
		child.Wait()
		t.Fatal("crash marker not retained")
	}
	child.Process.Kill()
	child.Wait()
	for attempts := 0; ; attempts++ {
		e = a.recoverAdmissionMarkers(ctx)
		if e == nil {
			break
		}
		if attempts > 100 {
			t.Fatal("crashed backend outcome never became provable", e)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if entries, e = a.admissionMarkers(); e != nil || len(entries) != 0 {
		t.Fatal("crashed original abort did not reconcile")
	}
	// A copied marker from a different database must survive and close writes.
	other := filepath.Join(a.Config.StateDir, "Admission", "Pending", "foreign.json")
	if e = os.WriteFile(other, []byte(`{"database":"different"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e = a.recoverAdmissionMarkers(ctx); e == nil {
		t.Fatal("foreign marker was discarded")
	}
	if _, e = os.Stat(other); e != nil {
		t.Fatal("foreign marker lost")
	}
	if f, e := maintenanceAdmissionGuard(ctx, a.Config.StateDir); e == nil {
		closeExportFence(f)
		t.Fatal("backup/restore ignored unresolved markers")
	}
}

func TestSendAdmissionWithIndependentRoleDatabases(t *testing.T) {
	file := os.Getenv("JOBS_IM_ADMISSION_TEST_ENV")
	if file == "" {
		t.Skip("explicit private PG fixture required")
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal("private fixture unavailable")
	}
	var env struct {
		BusinessURL string `json:"businessURL"`
		JournalURL  string `json:"journalURL"`
	}
	if json.Unmarshal(raw, &env) != nil {
		t.Fatal("invalid fixture")
	}
	ctx := context.Background()
	urls := map[string]string{}
	cfg, e := PrepareConfig(Config{StateDir: t.TempDir(), RoleURLs: urls})
	if e != nil {
		t.Fatal(e)
	}
	apps := []*App{}
	servers := []*httptest.Server{}
	databases := map[string]string{}
	administrators := map[string]*pgx.Conn{}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
		for _, a := range apps {
			a.Close()
		}
		for role, address := range databases {
			u, _ := url.Parse(address)
			name := u.Path[1:]
			admin := administrators[role]
			_, err := admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize())
			admin.Close(ctx)
			if err != nil {
				t.Error("isolated domain database cleanup failed")
			}
		}
	}()
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
		source := env.BusinessURL
		if role == "journal" {
			source = env.JournalURL
		}
		admin, e := pgx.Connect(ctx, source)
		if e != nil {
			t.Fatal("database operator unavailable")
		}
		administrators[role] = admin
		name := "jobsim_admission_" + role + "_" + base64.RawURLEncoding.EncodeToString([]byte(newID()))[:8]
		if _, e = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); e != nil {
			t.Fatal("isolated domain database creation failed")
		}
		u, _ := url.Parse(source)
		u.Path = "/" + name
		databases[role] = u.String()
	}
	cfg.DatabaseURL = databases["identity"]
	cfg.JournalDatabaseURL = databases["journal"]
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
		local := cfg
		if role != "journal" {
			local.DatabaseURL = databases[role]
		}
		app, e := NewApp(ctx, local, role)
		if e != nil {
			t.Fatalf("isolated %s startup failed: %v", role, e)
		}
		apps = append(apps, app)
		server := httptest.NewServer(app.Handler())
		servers = append(servers, server)
		urls[role] = server.URL
	}
	// Use production initialization so the gateway and role proofs bind the same
	// canonical StateDir, including on hosts whose temporary directory is a link.
	gateway, e := NewApp(ctx, cfg, "gateway")
	if e != nil {
		t.Fatal("isolated gateway initialization failed")
	}
	apps = append(apps, gateway)
	public := httptest.NewTLSServer(gateway.Handler())
	public.Client().Timeout = 15 * time.Second
	servers = append(servers, public)
	request := func(method, path string, body any, token string, status int) map[string]any {
		b, _ := json.Marshal(body)
		r, _ := http.NewRequest(method, public.URL+path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := public.Client().Do(r)
		if e != nil {
			t.Fatal("strict TLS request failed")
		}
		defer response.Body.Close()
		var data map[string]any
		json.NewDecoder(response.Body).Decode(&data)
		if response.StatusCode != status {
			t.Fatalf("%s %s returned %d; private response redacted", method, path, response.StatusCode)
		}
		return data
	}
	suffix := base64.RawURLEncoding.EncodeToString([]byte(newID()))[:8]
	register := func(prefix string) map[string]any {
		return request("POST", "/api/auth/register", map[string]string{"username": prefix + suffix, "password": "Independent domain database phrase " + suffix, "requestId": newID(), "deviceName": "role-db-admission"}, "", 200)
	}
	a, b := register("split_a_"), register("split_b_")
	at, bt := a["token"].(string), b["token"].(string)
	bid := b["user"].(map[string]any)["id"].(string)
	friend := request("POST", "/api/contacts/requests", map[string]string{"userId": bid, "requestId": newID()}, at, 200)
	request("POST", "/api/contacts/requests/"+friend["id"].(string)+"/accept", map[string]string{}, bt, 200)
	group := request("POST", "/api/conversations", map[string]any{"kind": "group", "title": "split owners", "memberIds": []string{bid}}, at, 200)
	conversation := group["id"].(string)
	list := request("GET", "/api/conversations", nil, bt, 200)["items"].([]any)
	snapshot := list[0].(map[string]any)
	original := map[string]string{"clientMsgId": newID(), "type": "text", "text": "original member admission", "restoreEpoch": "1", "sendAdmission": snapshot["sendAdmission"].(string)}
	request("DELETE", "/api/conversations/"+conversation+"/members/"+bid+"?membershipGeneration="+snapshot["membershipGeneration"].(string), nil, at, 200)
	request("POST", "/api/conversations/"+conversation+"/members", map[string]string{"userId": bid}, at, 200)
	result := request("POST", "/api/conversations/"+conversation+"/messages", original, bt, 409)
	if result["error"].(map[string]any)["code"] != "SEND_ADMISSION_CHANGED" {
		t.Fatal("split-owner old admission was not permanently rejected")
	}
	fresh := request("GET", "/api/conversations", nil, bt, 200)["items"].([]any)[0].(map[string]any)
	request("POST", "/api/conversations/"+conversation+"/messages", map[string]string{"clientMsgId": newID(), "type": "text", "text": "new explicit intent", "restoreEpoch": "1", "sendAdmission": fresh["sendAdmission"].(string)}, bt, 200)
	request("POST", "/api/auth/logout", map[string]string{}, at, 200)
	request("POST", "/api/auth/logout", map[string]string{}, bt, 200)
}
