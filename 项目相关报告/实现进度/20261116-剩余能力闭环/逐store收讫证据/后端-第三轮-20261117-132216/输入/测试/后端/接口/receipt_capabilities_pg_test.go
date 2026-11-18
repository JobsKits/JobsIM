package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReceiptQualificationRotationDrainsOriginalSourceReaders(t *testing.T) {
	a := &App{Role: "sync", Config: Config{StateDir: t.TempDir(), DeploymentID: newID(), Key: bytes.Repeat([]byte{0x38}, 32)}}
	readerEntered, releaseReader, writerEntered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	readerDone, writerDone := make(chan error, 1), make(chan error, 1)
	reader := a.withAdmissionFence("POST /api/sync/receipts", func(http.ResponseWriter, *http.Request) error { close(readerEntered); <-releaseReader; return nil })
	writer := a.withAdmissionFence("PUT /api/sync/storage", func(http.ResponseWriter, *http.Request) error { close(writerEntered); return nil })
	go func() {
		readerDone <- reader(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/sync/receipts", nil))
	}()
	select {
	case <-readerEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("original receipt source fence did not open")
	}
	defer func() {
		select {
		case <-releaseReader:
		default:
			close(releaseReader)
		}
	}()
	go func() {
		writerDone <- writer(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/sync/storage", nil))
	}()
	select {
	case <-writerEntered:
		t.Fatal("qualification rotation raced an original receipt commit")
	case err := <-writerDone:
		t.Fatal("qualification fence failed before the old reader drained", err)
	case <-time.After(80 * time.Millisecond):
	}
	close(releaseReader)
	for _, done := range []chan error{readerDone, writerDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("original receipt/rotation fence did not finish")
		}
	}
	for _, item := range []struct{ method, path, mode string }{{"PUT", "/api/sync/storage", "exclusive"}, {"GET", "/api/sync/storage", "shared"}, {"POST", "/api/sync/storage-results", "shared"}, {"POST", "/api/sync/receipts", "shared"}} {
		if gatewayProjectionMode(httptest.NewRequest(item.method, item.path, nil)) != item.mode {
			t.Fatal("gateway and owner receipt fence modes differ")
		}
	}
}

// This optional positive window adds a second physical PG and a private real
// JetStream to the existing production-handler/strict-HTTPS fixture. Test-only
// profile approval does not attest any actual client/native disk.
func receiptIndependentJournalPG(t *testing.T, m *migrationPGFixture) {
	t.Helper()
	if os.Getenv("JOBS_IM_RECEIPT_RUNTIME_TEST") != "1" {
		t.Skip("explicit independent receipt PG/JetStream window required")
	}
	raw := os.Getenv("JOBS_IM_RECEIPT_JOURNAL_ADMIN_URL")
	cfg, err := pgx.ParseConfig(raw)
	home, homeErr := os.UserHomeDir()
	owner, owned := s2PGPassfileOwner(os.Getenv("PGPASSFILE"), home)
	if raw == "" || err != nil || homeErr != nil || !owned || cfg.Host != "127.0.0.1" || cfg.Port != 62382 || cfg.User != owner || cfg.Database != "postgres" {
		t.Fatal("independent receipt journal maintenance configuration rejected; values suppressed")
	}
	for _, fallback := range cfg.Fallbacks {
		if fallback == nil || fallback.Host != "127.0.0.1" || fallback.Port != 62382 {
			t.Fatal("independent receipt journal fallback rejected")
		}
	}
	admin, err := pgx.ConnectConfig(m.base.ctx, cfg)
	if err != nil {
		t.Fatal("independent owned journal unavailable")
	}
	name := "jobsim_receipt_runtime_" + strings.ReplaceAll(newID(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(m.base.ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		admin.Close(context.Background())
		t.Fatal("independent owned journal creation failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted); err != nil {
			t.Error("independent owned journal cleanup failed; no foreign sessions terminated")
		}
		admin.Close(ctx)
	})
	child := cfg.Copy()
	child.Database = name
	poolCfg, err := pgxpool.ParseConfig("")
	if err != nil {
		t.Fatal("independent receipt pool configuration unavailable")
	}
	poolCfg.ConnConfig, poolCfg.MaxConns = child, 8
	pool, err := pgxpool.NewWithConfig(m.base.ctx, poolCfg)
	if err != nil {
		t.Fatal("independent receipt pool unavailable")
	}
	t.Cleanup(pool.Close)
	journal := m.base.nodes["journal"]
	journal.DB = pool
	journalURL := (&url.URL{Scheme: "postgresql", User: url.User(child.User), Host: net.JoinHostPort(child.Host, fmt.Sprint(child.Port)), Path: "/" + name, RawQuery: "sslmode=disable"}).String()
	for _, node := range m.base.nodes {
		node.Config.JournalDatabaseURL = journalURL
	}
	if err = journal.migrate(m.base.ctx); err != nil {
		t.Fatal("independent production journal migration failed")
	}
}

func receiptRuntimeBroker(t *testing.T, m *migrationPGFixture) func() {
	t.Helper()
	binary := os.Getenv("JOBS_IM_RECEIPT_NATS_BINARY")
	if binary == "" || !filepath.IsAbs(binary) {
		t.Fatal("explicit private receipt NATS binary required")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("private broker endpoint unavailable")
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	root := t.TempDir()
	token := strings.ReplaceAll(newID()+newID(), "-", "")
	tokenFile := filepath.Join(root, "token")
	if err = os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"host": "127.0.0.1", "port": port, "authorization": map[string]string{"token": token}, "jetstream": map[string]any{"store_dir": filepath.Join(root, "Broker"), "max_file_store": 512 << 20, "max_memory_store": 32 << 20}})
	configPath := filepath.Join(root, "nats.json")
	if err = os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(filepath.Join(root, "broker.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-c", configPath)
	cmd.Stdout, cmd.Stderr = log, log
	if err = cmd.Start(); err != nil {
		log.Close()
		t.Fatal("private receipt broker failed to start")
	}
	var once sync.Once
	stop := func() {
		once.Do(func() { _ = cmd.Process.Signal(syscall.SIGTERM); _ = cmd.Wait(); _ = log.Close() })
	}
	t.Cleanup(stop)
	ctx, cancel := context.WithCancel(m.base.ctx)
	for _, node := range m.base.nodes {
		node.Config.NATSURL = fmt.Sprintf("nats://127.0.0.1:%d", port)
		node.Config.NATSTokenFile = tokenFile
		node.StartSyncWorkers(ctx)
	}
	t.Cleanup(func() {
		cancel()
		stop()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if !m.app.syncPublishConnected.Load() && !m.app.syncConsumeConnected.Load() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("owned receipt transport workers did not stop before database cleanup")
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if m.app.receiptRuntime(ctx, "1").Ready {
			return stop
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("actual independent PG/JetStream receipt readiness never became ready")
	return stop
}

func TestReceiptCapabilityPGIndependentJournalAndRealJetStream(t *testing.T) {
	if os.Getenv("JOBS_IM_RECEIPT_RUNTIME_TEST") != "1" {
		t.Skip("explicit independent receipt PG/JetStream window required")
	}
	m := newMigrationPG(t, []byte("receipt original file\x00bytes"))
	receiptIndependentJournalPG(t, m)
	stopBroker := receiptRuntimeBroker(t, m)
	parent, _ := m.qualifiedOriginal(t)
	view := m.call(t, "GET", "/api/sync/receipt-context", m.base.token, m.sourceStore, nil, 200)
	if view["qualifiedAck"] != true || view["afterReceiptEnabled"] != true || view["afterObjectReceiptEnabled"] != true || view["qualification"] != "qualified_content_and_objects" {
		t.Fatal("actual qualified store remained permanently disabled")
	}
	legacy := m.call(t, "GET", "/api/sync/storage", m.base.token, m.sourceStore, nil, 200)
	oldContext := m.call(t, "GET", "/api/sync/context", m.base.token, m.sourceStore, nil, 200)
	bootstrap := m.call(t, "GET", "/api/bootstrap", m.base.token, m.sourceStore, nil, 200)
	if legacy["afterReceiptEnabled"] != false || oldContext["qualifiedAck"] != false || oldContext["qualification"] != "transport_only" || bootstrap["features"].(map[string]any)["after_receipt"] != false {
		t.Fatal("qualified negotiation broke the legacy transport contract")
	}
	target := m.call(t, "GET", "/api/sync/receipt-targets/"+m.message.ID, m.base.token, m.sourceStore, nil, 200)
	if target["originalTarget"] != true || target["contentRecorded"] != true || target["objectRecorded"] != false {
		t.Fatal("own original receipt target view was not exact")
	}
	objectChallenge := m.call(t, "POST", "/api/sync/object-receipt-challenges", m.base.token, m.sourceStore, map[string]string{"parentReceiptId": parent}, 200)
	proof, err := m.app.decodeObjectReceiptProof(objectChallenge["challenge"].(string), time.Now())
	if err != nil || proof.DeliveryGeneration != target["deliveryGeneration"] || proof.ManifestDigest != target["manifestDigest"] || proof.DeliveryGeneration == "" {
		t.Fatal("original object proof omitted the parent target tuple", err)
	}
	object := objectChallenge["object"].(map[string]any)
	m.call(t, "POST", "/api/sync/object-receipts", m.base.token, m.sourceStore, objectReceiptRequest{RequestID: newID(), Store: m.sourceStore, Epoch: "1", Qualification: proof.Qualification, Kind: "object_received", Parent: parent, Challenge: objectChallenge["challenge"].(string), Object: object["objectId"].(string), Size: object["sizeBytes"].(string), Digest: object["sha256"].(string)}, 200)
	target = m.call(t, "GET", "/api/sync/receipt-targets/"+m.message.ID, m.base.token, m.sourceStore, nil, 200)
	if target["objectRecorded"] != true || target["deletionEligible"] != false {
		t.Fatal("original object fact was not independently recorded")
	}
	zero := int64(0)
	policy := defaultRetentionPolicy()
	policy.Mode, policy.Delay = "after_receipt", &zero
	request := map[string]any{"requestId": newID(), "expectedVersion": "1", "policy": policy}
	m.call(t, "PUT", "/api/conversations/"+m.message.ConversationID+"/retention", m.base.token, m.sourceStore, request, 200)
	conversations := m.call(t, "GET", "/api/conversations", m.base.token, m.sourceStore, nil, 200)
	admission := ""
	for _, item := range conversations["items"].([]any) {
		conv := item.(map[string]any)
		if conv["id"] == m.message.ConversationID {
			admission, _ = conv["sendAdmission"].(string)
		}
	}
	if admission == "" {
		t.Fatal("production conversation omitted actual send admission")
	}
	deletionExec(t, m.base.ctx, m.base.nodes["message"], `INSERT INTO message.heads(conversation_id,sequence) VALUES($1,1) ON CONFLICT DO NOTHING`, m.message.ConversationID)
	sent := m.call(t, "POST", "/api/conversations/"+m.message.ConversationID+"/messages", m.base.token, m.sourceStore, sendPayload{ClientMsgID: newID(), Type: "text", Text: "new frozen receipt-policy original", RestoreEpoch: "1", SendAdmission: admission}, 200)
	id := sent["id"].(string)
	challenge := m.call(t, "POST", "/api/sync/receipt-challenges", m.base.token, m.sourceStore, map[string]string{"messageId": id}, 200)
	m.call(t, "POST", "/api/sync/receipts", m.base.token, m.sourceStore, contentReceiptRequest{RequestID: newID(), Store: m.sourceStore, Epoch: "1", Qualification: challenge["qualificationId"].(string), Kind: "message_content", Challenge: challenge["challenge"].(string), BodyDigest: challenge["bodyDigest"].(string)}, 200)
	if err = m.app.refreshReceiptDeliveries(m.base.ctx, 32); err != nil {
		t.Fatal("actual original receipt accumulation", err)
	}
	if err = m.base.nodes["message"].resumeRetention(m.base.ctx, 32); err != nil {
		t.Fatal("actual protected receipt cleanup", err)
	}
	var text, scope string
	var unavailable bool
	if err = m.app.DB.QueryRow(m.base.ctx, `SELECT text,content_unavailable,delete_scope FROM message.messages WHERE id=$1`, id).Scan(&text, &unavailable, &scope); err != nil || text != "" || !unavailable || scope != "cloud_only" {
		t.Fatal("zero delay did not complete the protected cloud-only cleanup", err)
	}
	var protected bool
	if err = m.base.nodes["journal"].DB.QueryRow(m.base.ctx, `SELECT EXISTS(SELECT 1 FROM journal.entries WHERE kind='message.retention_delete')`).Scan(&protected); err != nil || !protected {
		t.Fatal("independent current journal lacks the protected deletion fact", err)
	}
	originalQualification := challenge["qualificationId"].(string)
	profile := m.app.Config.ReceiptProfiles[0]
	rotatedProfile := profile
	rotatedProfile.ID += "-rotated"
	rotatedProfile.VerificationDigest = syncDigest([]byte("second explicit fixture approval, no native attestation"))
	m.app.Config.ReceiptProfiles = []ReceiptProfile{profile, rotatedProfile}
	m.call(t, "PUT", "/api/sync/storage", m.base.token, m.sourceStore, storageDeclaration{RequestID: newID(), StoreGeneration: m.sourceStore, RevisionEpoch: "1", ProfileID: rotatedProfile.ID, BuildDigest: rotatedProfile.BuildDigest, VerificationDigest: rotatedProfile.VerificationDigest}, 200)
	// Successful pre-negotiation declarations carried the approved immutable
	// tuple without explicit booleans. Do not erase those old historical facts.
	deletionExec(t, m.base.ctx, m.app, `UPDATE sync.storage_declarations SET result=result-'qualified'-'objectQualified' WHERE result->>'qualificationId'=$1`, originalQualification)
	historyPath := "/api/sync/receipt-targets/" + id + "?qualificationId=" + originalQualification
	target = m.call(t, "GET", historyPath, m.base.token, m.sourceStore, nil, 200)
	if target["qualificationId"] != originalQualification || target["contentRecorded"] != true || target["messageCurrent"] != true || target["contentTarget"] != false {
		t.Fatal("same-store re-declaration lost a historical fact or inherited its ACK target")
	}
	m.call(t, "GET", "/api/sync/receipt-targets/"+id+"?qualificationId="+newID(), m.base.token, m.sourceStore, nil, 404)
	m.call(t, "GET", historyPath, m.targetToken, m.targetStore, nil, 404)
	m.app.Config.ReceiptProfiles = nil
	target = m.call(t, "GET", historyPath, m.base.token, m.sourceStore, nil, 200)
	if target["contentRecorded"] != true || target["messageCurrent"] != true || target["contentTarget"] != false {
		t.Fatal("approval withdrawal erased the old qualified-at-commit fact or enabled a new ACK")
	}
	m.app.Config.ReceiptProfiles = []ReceiptProfile{profile, rotatedProfile}
	owner := m.base.nodes["message"]
	tx, err := owner.DB.Begin(m.base.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.insertRetentionJob(m.base.ctx, tx, id, "1", "2", "1", "target-view-global-pending", "global", time.Now().Add(-time.Second)); err != nil {
		tx.Rollback(m.base.ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(m.base.ctx); err != nil {
		t.Fatal(err)
	}
	target = m.call(t, "GET", historyPath, m.base.token, m.sourceStore, nil, 200)
	if target["messageCurrent"] != false || target["current"] != false || target["contentRecorded"] != true {
		t.Fatal("due pending global job released an old qualified body or hid its historical receipt")
	}
	stopBroker()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (m.app.syncPublishConnected.Load() || m.app.syncConsumeConnected.Load()) {
		time.Sleep(20 * time.Millisecond)
	}
	view = m.call(t, "GET", "/api/sync/receipt-context", m.base.token, m.sourceStore, nil, 200)
	if view["qualifiedAck"] != true || view["afterReceiptEnabled"] != false {
		t.Fatal("disconnected broker preserved lifecycle readiness or erased historical qualification")
	}
	// The committed original policy remains queryable without authorizing a new one.
	m.call(t, "PUT", "/api/conversations/"+m.message.ConversationID+"/retention", m.base.token, m.sourceStore, request, 200)
	request["requestId"], request["expectedVersion"] = newID(), "2"
	m.call(t, "PUT", "/api/conversations/"+m.message.ConversationID+"/retention", m.base.token, m.sourceStore, request, 409)
}

func TestReceiptCapabilityPGProfilesOriginalDeclarationAndSharedClusterRefusal(t *testing.T) {
	m := newMigrationPG(t)
	view := m.call(t, "GET", "/api/sync/receipt-context", m.base.token, m.sourceStore, nil, 200)
	if view["qualifiedAck"] != false || view["afterReceiptEnabled"] != false || view["qualification"] != "transport_only" {
		t.Fatal("ordinary registered store acquired qualification without an approved declaration")
	}
	m.call(t, "GET", "/api/sync/receipt-context", m.base.token, newID(), nil, 409)
	profile := receiptTestProfile()
	profiles := m.call(t, "GET", "/api/sync/receipt-profiles?buildDigest="+profile.BuildDigest, m.base.token, m.sourceStore, nil, 200)
	if len(profiles["profiles"].([]any)) != 0 || profiles["qualifiedAck"] != false {
		t.Fatal("profile inspection approved an unconfigured build")
	}
	m.app.Config.ReceiptProfiles = []ReceiptProfile{profile}
	q := storageDeclaration{RequestID: newID(), StoreGeneration: m.sourceStore, RevisionEpoch: "1", ProfileID: profile.ID, BuildDigest: profile.BuildDigest, VerificationDigest: profile.VerificationDigest}
	m.call(t, "POST", "/api/sync/storage-results", m.base.token, m.sourceStore, q, 404)
	var count int
	if m.app.DB.QueryRow(m.base.ctx, `SELECT count(*) FROM sync.storage_declarations`).Scan(&count) != nil || count != 0 {
		t.Fatal("readonly declaration query created a new qualification")
	}
	m.call(t, "PUT", "/api/sync/storage", m.base.token, m.sourceStore, q, 200)
	// Negative fixture: even optimistic transport flags cannot turn a second DB
	// on this same physical PG into an independent protection journal.
	m.app.syncPublishConnected.Store(true)
	m.app.syncConsumeConnected.Store(true)
	m.base.nodes["message"].Config.JournalDatabaseURL = "explicit same-physical-cluster negative fixture"
	view = m.call(t, "GET", "/api/sync/receipt-context", m.base.token, m.sourceStore, nil, 200)
	if view["qualifiedAck"] != true || view["objectReceiptEnabled"] != false || view["afterReceiptEnabled"] != false || view["readiness"].(map[string]any)["journalIndependent"] != false {
		t.Fatal("body-only approval or same-cluster journal was upgraded")
	}
	zero := int64(0)
	policy := defaultRetentionPolicy()
	policy.Mode, policy.Delay = "after_receipt", &zero
	m.call(t, "PUT", "/api/conversations/"+m.message.ConversationID+"/retention", m.base.token, m.sourceStore, map[string]any{"requestId": newID(), "expectedVersion": "1", "policy": policy}, 409)
	m.app.Config.ReceiptProfiles = nil
	original := m.call(t, "POST", "/api/sync/storage-results", m.base.token, m.sourceStore, q, 200)
	if original["current"] != false || original["storage"].(map[string]any)["qualified"] != false || original["result"].(map[string]any)["qualified"] != true {
		t.Fatal("withdrawn approval hid the original fact or restored current qualification")
	}
	q.VerificationDigest = strings.Repeat("f", 64)
	m.call(t, "POST", "/api/sync/storage-results", m.base.token, m.sourceStore, q, 409)
	q.RequestID = newID()
	m.call(t, "POST", "/api/sync/storage-results", m.base.token, m.sourceStore, q, 404)
}
