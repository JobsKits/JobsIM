package jobsim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type deploymentRestoreTestEnvironment struct{ Main, Journal, Root, PGBin, OpenSSL, NATS, Repository string }

type deploymentRestoreTestContext struct {
	A, B                                               map[string]any
	BToken, Conversation, Media, Password, NewPassword string
	Snapshot, Keep, Erase                              map[string]any
	Body                                               []byte
}
type deploymentSourceRuntime struct {
	apps      map[string]*App
	servers   []*httptest.Server
	public    *httptest.Server
	cancel    context.CancelFunc
	broker    *exec.Cmd
	brokerLog *os.File
}

func (d *deploymentSourceRuntime) stop() {
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	for _, server := range d.servers {
		server.Close()
	}
	d.servers = nil
	for _, a := range d.apps {
		a.Close()
	}
	d.apps = nil
	if d.broker != nil {
		_ = d.broker.Process.Signal(os.Interrupt)
		_ = d.broker.Wait()
		d.broker = nil
	}
	if d.brokerLog != nil {
		d.brokerLog.Close()
		d.brokerLog = nil
	}
}
func deploymentTestExec(t *testing.T, ctx context.Context, path string, args ...string) {
	t.Helper()
	if err := deploymentTool(ctx, path, args, nil); err != nil {
		t.Fatal(err)
	}
}
func deploymentSourceStart(t *testing.T, ctx context.Context, e deploymentRestoreTestEnvironment, state string, sources map[string]string) *deploymentSourceRuntime {
	t.Helper()
	runtime := &deploymentSourceRuntime{apps: map[string]*App{}}
	t.Cleanup(runtime.stop)
	token := strings.TrimSpace(string(mustDeploymentRead(t, filepath.Join(state, "Secrets", "nats.token"))))
	configuration := fmt.Sprintf("listen: \"127.0.0.1:59222\"\nauthorization {token: %q}\njetstream {store_dir: %q,max_file_store: 536870912,max_memory_store: 33554432}\n", token, filepath.Join(state, "Application", "JetStream"))
	if err := privateRestoreBytes(filepath.Join(state, "Secrets", "nats.conf"), []byte(configuration)); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(state, "Logs", "source-nats.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	runtime.brokerLog = file
	runtime.broker = exec.CommandContext(ctx, e.NATS, "-c", filepath.Join(state, "Secrets", "nats.conf"))
	runtime.broker.Stdout, runtime.broker.Stderr = file, file
	if err = runtime.broker.Start(); err != nil {
		t.Fatal("owned source broker unavailable")
	}
	urls := map[string]string{}
	cfg, err := PrepareConfig(Config{DatabaseURL: sources["identity"], JournalDatabaseURL: sources["journal"], StateDir: filepath.Join(state, "Application"), RoleURLs: urls, NATSURL: "nats://127.0.0.1:59222", NATSTokenFile: filepath.Join(state, "Secrets", "nats.token")})
	if err != nil {
		t.Fatal(err)
	}
	running, cancel := context.WithCancel(ctx)
	runtime.cancel = cancel
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance", "sync"} {
		local := cfg
		local.DatabaseURL = sources[role]
		if role == "governance" || role == "journal" {
			local.DatabaseURL = sources["identity"]
		}
		a, err := NewApp(ctx, local, role)
		if err != nil {
			t.Fatalf("source %s unavailable: %v", role, err)
		}
		runtime.apps[role] = a
		server := httptest.NewServer(a.Handler())
		runtime.servers = append(runtime.servers, server)
		urls[role] = server.URL
	}
	gateway, err := NewApp(ctx, cfg, "gateway")
	if err != nil {
		t.Fatal(err)
	}
	runtime.apps["gateway"] = gateway
	public := httptest.NewTLSServer(gateway.Handler())
	public.Client().Timeout = 10 * time.Second
	runtime.servers = append(runtime.servers, public)
	runtime.public = public
	for _, a := range runtime.apps {
		a.StartSyncWorkers(running)
	}
	deadline := time.Now().Add(15 * time.Second)
	for !runtime.apps["sync"].syncPublishConnected.Load() || !runtime.apps["sync"].syncConsumeConnected.Load() {
		if time.Now().After(deadline) {
			t.Fatal("real source JetStream publish/consume readiness missing")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return runtime
}
func mustDeploymentRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("private owned restore file unavailable")
	}
	return raw
}
func deploymentHTTP(t *testing.T, client *http.Client, base, method, path string, body any, token string, want int) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	request, _ := http.NewRequest(method, base+path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("production %s request failed; private transport detail suppressed", path)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("production %s returned %d, expected %d; private body suppressed", path, response.StatusCode, want)
	}
	var result map[string]any
	_ = json.NewDecoder(response.Body).Decode(&result)
	return result
}
func deploymentArchiveFixture(t *testing.T, ctx context.Context, e deploymentRestoreTestEnvironment, state, base string, sources map[string]string) (string, string, string) {
	t.Helper()
	stage := filepath.Join(base, "ColdStage")
	for _, dir := range []string{stage, filepath.Join(base, "Backups"), filepath.Join(base, "Recovery")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"broker-manifest.txt"}
	if err := privateRestoreBytes(filepath.Join(stage, "broker-manifest.txt"), []byte("component=NATS JetStream\nstate=stopped-before-archive\nstore=Application/JetStream\n")); err != nil {
		t.Fatal(err)
	}
	for _, role := range deploymentPhysicalDomains {
		if err := pgCommand(ctx, e.PGBin, sources[role], "pg_dump", "--format=custom", "--file="+filepath.Join(stage, role+".dump")); err != nil {
			t.Fatal(err)
		}
		files = append(files, role+".dump")
	}
	archive := filepath.Join(base, "Backups", "source.cms")
	tarball := filepath.Join(base, "source.tar.gz")
	args := []string{"-czf", tarball, "-C", state, "owner", "Application", "TLS", "Secrets", "-C", stage}
	args = append(args, files...)
	deploymentTestExec(t, ctx, "tar", args...)
	key, cert := filepath.Join(base, "Recovery", "key.pem"), filepath.Join(base, "Recovery", "cert.pem")
	deploymentTestExec(t, ctx, e.OpenSSL, "req", "-x509", "-newkey", "rsa:3072", "-nodes", "-days", "365", "-keyout", key, "-out", cert, "-subj", "/CN=Owned restore regression recovery")
	if err := os.Chmod(key, 0600); err != nil {
		t.Fatal(err)
	}
	deploymentTestExec(t, ctx, e.OpenSSL, "cms", "-encrypt", "-aes-256-gcm", "-binary", "-outform", "DER", "-in", tarball, "-out", archive, cert)
	digest, err := fileDigest(archive)
	if err != nil {
		t.Fatal(err)
	}
	return archive, key, digest.Digest
}
func TestDeploymentRestorePGEightDomainsCurrentLedgerAndNativeStartup(t *testing.T) {
	input := os.Getenv("JOBS_IM_DEPLOYMENT_RESTORE_TEST_ENV")
	if input == "" {
		t.Skip("explicit owned eight-domain restore/native-start fixture required")
	}
	var e deploymentRestoreTestEnvironment
	if json.Unmarshal(mustDeploymentRead(t, input), &e) != nil {
		t.Fatal("private restore fixture invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	if resume := os.Getenv("JOBS_IM_DEPLOYMENT_RESTORE_RESUME_CASE"); resume != "" {
		base := filepath.Join(e.Root, "RestoreCases", resume)
		var saved deploymentRestoreTestContext
		if strings.ContainsAny(resume, "/\\") || json.Unmarshal(mustDeploymentRead(t, filepath.Join(base, "test-context.json")), &saved) != nil {
			t.Fatal("owned prepared restore case unavailable")
		}
		deploymentCheckPreparedCase(t, ctx, e, base, saved)
		return
	}
	base := filepath.Join(e.Root, "RestoreCases", newID())
	sourceState := filepath.Join(base, "Source")
	for _, dir := range []string{base, sourceState, filepath.Join(sourceState, "Application"), filepath.Join(sourceState, "Secrets"), filepath.Join(sourceState, "TLS"), filepath.Join(sourceState, "Logs")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := privateRestoreBytes(filepath.Join(sourceState, "owner"), []byte(e.Repository+"\n"+strconv.Itoa(os.Getuid())+"\n")); err != nil {
		t.Fatal(err)
	}
	for _, role := range append([]string{"operator"}, deploymentPhysicalDomains...) {
		token := randomToken(32)
		digest := sha256.Sum256([]byte(token))
		if err := privateRestoreBytes(filepath.Join(sourceState, "Secrets", role+".password"), []byte(hex.EncodeToString(digest[:])+"\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := privateRestoreBytes(filepath.Join(sourceState, "Secrets", "nats.token"), []byte(strings.Repeat("a", 64)+"\n")); err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, role := range deploymentPhysicalDomains {
		address := e.Main
		if role == "journal" {
			address = e.Journal
		}
		admin, err := operatorConnect(ctx, address)
		if err != nil {
			t.Fatal("owned maintenance PG unavailable")
		}
		name := "jobsim_deployment_" + role + "_" + strings.ReplaceAll(newID(), "-", "")
		if _, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Fatal("owned restore source DB could not be created")
		}
		admin.Close(ctx)
		parsed, _ := url.Parse(address)
		parsed.Path = "/" + name
		sources[role] = parsed.String()
	}
	// Exercise the current production TLS function without invoking its main.
	sourceScript := string(mustDeploymentRead(t, filepath.Join(e.Repository, "部署", "MacOS", "部署运行.command", "部署运行.command")))
	begin := strings.Index(sourceScript, "prepare_tls() {")
	end := strings.Index(sourceScript[begin:], "\n}\n")
	tlsScript := "#!/bin/zsh\nset -eu\nSTATE=\"$1\"\nOPENSSL=\"$2\"\nPUBLIC_HOST=127.0.0.1\n" + sourceScript[begin:begin+end+3] + "\nprepare_tls\n"
	tlsPath := filepath.Join(base, "tls.zsh")
	if err := privateRestoreBytes(tlsPath, []byte(tlsScript)); err != nil {
		t.Fatal(err)
	}
	deploymentTestExec(t, ctx, "zsh", tlsPath, sourceState, e.OpenSSL)
	runtime := deploymentSourceStart(t, ctx, e, sourceState, sources)
	suffix := strings.ReplaceAll(newID(), "-", "")[:8]
	password := "Owned restore original phrase " + suffix
	register := func(prefix string) map[string]any {
		return deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "POST", "/api/auth/register", map[string]string{"username": prefix + suffix, "password": password, "requestId": newID(), "deviceName": "owned-restore-source"}, "", 200)
	}
	a, b := register("restore_a_"), register("restore_b_")
	aid, bid := a["user"].(map[string]any)["id"].(string), b["user"].(map[string]any)["id"].(string)
	at, bt := a["token"].(string), b["token"].(string)
	friend := deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "POST", "/api/contacts/requests", map[string]string{"userId": bid, "requestId": newID()}, at, 200)
	deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "POST", "/api/contacts/requests/"+friend["id"].(string)+"/accept", map[string]string{}, bt, 200)
	conversation := deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "POST", "/api/conversations", map[string]any{"kind": "direct", "memberIds": []string{bid}}, at, 200)["id"].(string)
	snapshot := deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "GET", "/api/conversations", nil, at, 200)["items"].([]any)[0].(map[string]any)
	send := func(text string) map[string]any {
		return deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "POST", "/api/conversations/"+conversation+"/messages", map[string]string{"clientMsgId": newID(), "type": "text", "text": text, "restoreEpoch": "1", "sendAdmission": snapshot["sendAdmission"].(string)}, at, 200)
	}
	keep, erase := send("owned preserved text"), send("must disappear after snapshot replay")
	media, message := newID(), newID()
	body := []byte("owned restore original bytes")
	digest := sha256.Sum256(body)
	m := runtime.apps["message"]
	prepared, err := m.prepareMessageRetention(ctx, User{ID: aid, RestoreEpoch: "1"}, message, conversation)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := m.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seq, err := nextSequence(ctx, tx, conversation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,attachment_id,sequence) VALUES($1,$2,$3,$4,$5,'file','',$6,$7)`, message, conversation, aid, newID(), []byte("fixture-binding"), media, seq); err != nil {
		t.Fatal(err)
	}
	if err = m.persistMessageRetention(ctx, tx, message, prepared); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.apps["media"].DB.Exec(ctx, `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,conversation_id,message_id) VALUES($1,$2,'owned.txt','text/plain',$3,$4,$5,$6,$7)`, media, aid, len(body), digest[:], media, conversation, message); err != nil {
		t.Fatal(err)
	}
	if err = privateRestoreBytes(filepath.Join(sourceState, "Application", "Media", media), body); err != nil {
		t.Fatal(err)
	}
	runtime.stop()
	archive, key, archiveDigest := deploymentArchiveFixture(t, ctx, e, sourceState, base, sources)
	// Real post-snapshot retraction and password rotation create newer independent
	// facts that the archived journal dump cannot authorize by itself.
	runtime = deploymentSourceStart(t, ctx, e, sourceState, sources)
	deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "DELETE", "/api/messages/"+erase["id"].(string), nil, at, 200)
	newPassword := "Owned restore changed phrase " + suffix
	deploymentHTTP(t, runtime.public.Client(), runtime.public.URL, "POST", "/api/auth/password-change", map[string]string{"requestId": newID(), "username": a["user"].(map[string]any)["username"].(string), "deviceId": a["deviceId"].(string), "restoreEpoch": "1", "currentPassword": password, "newPassword": newPassword}, at, 200)
	runtime.stop()
	spec := DeploymentRestoreSpec{Version: 1, Operation: newID(), Backup: "Backups/source.cms", BackupSHA: archiveDigest, RecoveryKey: "Recovery/key.pem", SourceState: "Source", TargetState: "Target", Repository: e.Repository, PGBin: e.PGBin, OpenSSL: e.OpenSSL, Sources: sources, MainPort: 62481, JournalPort: 62482, HTTPSPort: 59543, RolePort: 58101, NATSPort: 59322, PublicHost: "127.0.0.1"}
	specPath := filepath.Join(base, "spec.json")
	if err = writePrivateJSON(specPath, spec); err != nil {
		t.Fatal(err)
	}
	saved := deploymentRestoreTestContext{A: a, B: b, BToken: bt, Conversation: conversation, Media: media, Password: password, NewPassword: newPassword, Snapshot: snapshot, Keep: keep, Erase: erase, Body: body}
	if err = writePrivateJSON(filepath.Join(base, "test-context.json"), saved); err != nil {
		t.Fatal(err)
	}
	_ = archive
	_ = key
	deploymentCheckPreparedCase(t, ctx, e, base, saved)
}
func deploymentCheckPreparedCase(t *testing.T, ctx context.Context, e deploymentRestoreTestEnvironment, base string, saved deploymentRestoreTestContext) {
	t.Helper()
	sourceState := filepath.Join(base, "Source")
	specPath := filepath.Join(base, "spec.json")
	spec, err := loadDeploymentRestoreSpec(specPath)
	if err != nil {
		t.Fatal(err)
	}
	sources := spec.Sources
	a, b := saved.A, saved.B
	bt, conversation, media, password, newPassword := saved.BToken, saved.Conversation, saved.Media, saved.Password, saved.NewPassword
	snapshot, keep, erase, body := saved.Snapshot, saved.Keep, saved.Erase, saved.Body
	if _, err = DeploymentRestore(ctx, specPath, true); err != nil {
		t.Fatal("native restore plan:", err)
	}
	result, err := DeploymentRestore(ctx, specPath, false)
	if err != nil {
		t.Fatal("native eight-domain restore:", err)
	}
	if result["restoreEpoch"] != "2" {
		t.Fatal("restore did not allocate exactly one new epoch")
	}
	target := filepath.Join(base, "Target")
	if _, err = os.Stat(filepath.Join(target, "Application", "JetStream")); !os.IsNotExist(err) && result["resultIsHistorical"] != true {
		t.Fatal("old broker data became an active runtime directory")
	}
	if strings.TrimSpace(string(mustDeploymentRead(t, filepath.Join(target, "Secrets", "nats.token")))) == strings.Repeat("a", 64) {
		t.Fatal("restored runtime reused old broker credentials")
	}
	config, err := PrepareConfig(Config{DatabaseURL: sources["identity"], JournalDatabaseURL: sources["journal"], StateDir: filepath.Join(sourceState, "Application")})
	if err != nil {
		t.Fatal(err)
	}
	if old, err := NewApp(ctx, config, "journal"); err == nil {
		old.Close()
		t.Fatal("source ledger ordinary startup remained possible after handoff")
	}
	// Start the restored target through the real shipping native entrypoint. Ports
	// come from restore-layout.conf, without passing a second manual layout.
	log, err := os.OpenFile(filepath.Join(base, "target-native.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	entry := exec.CommandContext(ctx, "zsh", filepath.Join(e.Repository, "部署", "MacOS", "部署运行.command", "部署运行.command"), "run", "--yes", "--state", target)
	entry.Env = append(os.Environ(), "JOBS_IM_GO="+os.Getenv("GO_BINARY"), "JOBS_IM_PG_BIN="+e.PGBin, "JOBS_IM_OPENSSL="+e.OpenSSL, "JOBS_IM_NATS_BIN="+e.NATS, "JOBS_IM_BACKUP_DIR="+filepath.Join(base, "TargetBackups"), "JOBS_IM_RECOVERY_DIR="+filepath.Join(base, "TargetRecovery"))
	entry.Stdout, entry.Stderr = log, log
	if err = entry.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			stop := exec.Command("zsh", filepath.Join(e.Repository, "部署", "MacOS", "Undeployment", "【MacOS】反部署JobsIM.command", "【MacOS】反部署JobsIM.command"), "stop", "--yes", "--no-backup", "--state", target)
			stop.Env = entry.Env
			_ = stop.Run()
		}
		_ = entry.Wait()
		log.Close()
	}()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(mustDeploymentRead(t, filepath.Join(target, "TLS", "ca.crt"))) {
		t.Fatal("original target trust root missing")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}, Timeout: 5 * time.Second}
	public := "https://127.0.0.1:59543"
	deadline := time.Now().Add(90 * time.Second)
	for {
		response, e := client.Get(public + "/api/health")
		if e == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("canonical restored target did not become strict HTTPS ready; see private native log")
		}
		time.Sleep(100 * time.Millisecond)
	}
	deploymentHTTP(t, client, public, "GET", "/api/me", nil, bt, 401)
	fresh := deploymentHTTP(t, client, public, "POST", "/api/auth/login", map[string]string{"username": b["user"].(map[string]any)["username"].(string), "password": password, "requestId": newID(), "deviceName": "restored-current-epoch"}, "", 200)
	token := fresh["token"].(string)
	deploymentHTTP(t, client, public, "POST", "/api/conversations/"+conversation+"/messages", map[string]string{"clientMsgId": newID(), "type": "text", "text": "old frozen unsent intent", "restoreEpoch": "1", "sendAdmission": snapshot["sendAdmission"].(string)}, token, 409)
	history := deploymentHTTP(t, client, public, "GET", "/api/conversations/"+conversation+"/messages", nil, token, 200)
	kept, removed := false, false
	for _, raw := range history["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == keep["id"] {
			kept = item["text"] == "owned preserved text"
		}
		if item["id"] == erase["id"] {
			removed = item["deleted"] == true && item["text"] == ""
		}
	}
	if !kept || !removed {
		t.Fatal("newer deletion facts or preserved primary text did not survive domain restore")
	}
	request, _ := http.NewRequest("GET", public+"/api/media/"+media, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("restored media request failed")
	}
	actual, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !bytes.Equal(actual, body) {
		t.Fatal("restored original media bytes are not normally usable")
	}
	for _, candidate := range []string{password, newPassword} {
		raw, _ := json.Marshal(map[string]string{"username": a["user"].(map[string]any)["username"].(string), "password": candidate, "requestId": newID(), "deviceName": "must-recover"})
		r, _ := http.NewRequest("POST", public+"/api/auth/login", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		res, e := client.Do(r)
		if e != nil {
			t.Fatal("restored changed credential verification unavailable")
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal("older business snapshot resurrected a replaced credential")
		}
	}
	stop := exec.CommandContext(ctx, "zsh", filepath.Join(e.Repository, "部署", "MacOS", "Undeployment", "【MacOS】反部署JobsIM.command", "【MacOS】反部署JobsIM.command"), "stop", "--yes", "--no-backup", "--state", target)
	stop.Env = entry.Env
	if err = stop.Run(); err != nil {
		t.Fatal("canonical restored target did not stop cleanly")
	}
	stopped = true
	if err = writePrivateJSON(filepath.Join(base, "verified-result.json"), map[string]any{"status": "passed", "restoreEpoch": "2", "sourceRetained": true, "latestLedgerHandedOff": true, "oldJournalDumpAuthority": false, "canonicalNativeStartup": true, "strictHTTPS": true, "oldJWTRejected": true, "oldUnsentEpochRejected": true, "oldPasswordsRejected": true, "newDeletionReplayed": true, "primaryTextAndMediaKept": true, "brokerRebuilt": true}); err != nil {
		t.Fatal(err)
	}
	t.Log("Owned private source/target retained under the explicit restore test root; all target native resources stopped")
}

func TestJournalHandoffDecisionKeepsRetiredSourceSealedAndAllowsCanonicalSuccessor(t *testing.T) {
	for _, test := range []struct{ current, op, binding, target, want string }{{"source-A", "op-1", "binding-1", "target-B", "resume"}, {"source-A", "op-2", "binding-2", "target-C", "deny"}, {"source-A", "op-2", "binding-2", "target-B", "deny"}, {"target-B", "op-1", "different", "target-C", "deny"}, {"target-B", "op-2", "binding-2", "target-C", "next"}} {
		if actual := journalHandoffDecision("op-1", "binding-1", "target-B", test.current, test.op, test.binding, test.target); actual != test.want {
			t.Fatal("handoff could fork a retired source or silently replace the original operation")
		}
	}
}
