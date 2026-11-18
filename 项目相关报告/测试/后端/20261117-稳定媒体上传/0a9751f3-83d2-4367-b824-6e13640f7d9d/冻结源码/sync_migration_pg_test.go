package jobsim

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Actual HTTPS gateway and nine production owner handlers, one isolated test
// PostgreSQL realm. No native qualification or separate-PG/broker claim.
type migrationPGFixture struct {
	base                                  *deletionPGFixture
	app                                   *App
	target                                User
	targetToken, sourceStore, targetStore string
	client                                *http.Client
	url                                   string
	message                               Message
}

func newMigrationPG(t *testing.T, original ...[]byte) *migrationPGFixture {
	t.Helper()
	f := newDeletionPG(t, 0)
	a := f.nodes["sync"]
	for _, node := range f.nodes {
		node.Config.NATSURL = "nats://127.0.0.1:1"
		server := httptest.NewServer(node.Handler())
		t.Cleanup(server.Close)
		a.Config.RoleURLs[node.Role] = server.URL
	}
	for _, node := range f.nodes {
		node.Client = &http.Client{Timeout: 5 * time.Second}
	}
	tx, err := f.nodes["identity"].DB.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := f.nodes["identity"].issueSession(f.ctx, tx, f.user, "isolated migration target", 1, 1, 1, false)
	if err != nil || tx.Commit(f.ctx) != nil {
		t.Fatal("target production family", err)
	}
	target := f.user
	target.DeviceID = delivery.Result.DeviceID
	target.RestoreEpoch = "1"
	f.user.RestoreEpoch = "1"
	gateway := &App{Config: a.Config, Role: "gateway"}
	server := httptest.NewTLSServer(gateway.Handler())
	t.Cleanup(server.Close)
	m := &migrationPGFixture{base: f, app: a, target: target, targetToken: delivery.Result.Token, sourceStore: newID(), targetStore: newID(), client: server.Client(), url: server.URL}
	for _, tuple := range []struct{ token, store string }{{f.token, m.sourceStore}, {m.targetToken, m.targetStore}} {
		m.call(t, "PUT", "/api/sync/device", tuple.token, tuple.store, map[string]string{"requestId": newID(), "storeGeneration": tuple.store}, 200)
	}
	conv, id := newID(), newID()
	deletionExec(t, f.ctx, f.nodes["conversation"], `INSERT INTO conversation.conversations(id,kind,title,owner_id) VALUES($1,'self','owned private fixture',$2);`, conv, f.user.ID)
	deletionExec(t, f.ctx, f.nodes["conversation"], `INSERT INTO conversation.members(conversation_id,user_id,role,state) VALUES($1,$2,'owner','active')`, conv, f.user.ID)
	typ := "text"
	var attachment any
	if len(original) > 0 {
		typ = "file"
		object := newID()
		attachment = object
		data := original[0]
		dir := filepath.Join(a.Config.StateDir, "Media")
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, object), data, 0600); err != nil {
			t.Fatal(err)
		}
		digest := tokenDigest(string(data))
		deletionExec(t, f.ctx, f.nodes["media"], `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,message_id,purpose) VALUES($1::uuid,$2,'original.txt','text/plain',$3,$4,$1::uuid::text,$5,'chat_attachment')`, object, f.user.ID, len(data), digest, id)
	}
	deletionExec(t, f.ctx, f.nodes["message"], `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,attachment_id,sequence) VALUES($1,$2,$3,$4,$5,$6,'lawful original personal history',$7,1)`, id, conv, f.user.ID, newID(), []byte("non-secret fixture"), typ, attachment)
	m.message, err = scanMessage(a.DB.QueryRow(f.ctx, `SELECT `+messageColumns+` FROM message.messages WHERE id=$1`, id))
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func (m *migrationPGFixture) call(t *testing.T, method, path, token, store string, body any, status int) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	r, err := http.NewRequest(method, m.url+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-JobsIM-Store-Generation", store)
	res, err := m.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode != status {
		var e struct {
			Error struct {
				Code string `json:"code"`
			}
		}
		_ = json.Unmarshal(data, &e)
		t.Fatalf("%s %s status %d want %d code %s", method, path, res.StatusCode, status, e.Error.Code)
	}
	var out map[string]any
	if len(data) > 0 && json.Unmarshal(data, &out) != nil {
		t.Fatal("expected bounded JSON")
	}
	return out
}
func (m *migrationPGFixture) request(t *testing.T) migrationCreateRequest {
	t.Helper()
	pair := m.call(t, "POST", "/api/sync/migration-pairings", m.targetToken, m.targetStore, migrationPairingRequest{Request: newID(), Store: m.targetStore, Epoch: "1", Password: m.base.q.Password}, 200)
	raw, _ := json.Marshal(m.message)
	return migrationCreateRequest{Request: newID(), Context: "personal", Store: m.sourceStore, Epoch: "1", Pairing: pair["pairingProof"].(string), Password: m.base.q.Password, Messages: []migrationInputMessage{{Envelope: base64.RawURLEncoding.EncodeToString(raw), ObjectMode: "none"}}, Objects: []migrationInputObject{}}
}
func (m *migrationPGFixture) create(t *testing.T) (migrationCreateRequest, map[string]any) {
	t.Helper()
	q := m.request(t)
	out := m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 200)
	if out["status"] != "ready" {
		t.Fatal("bounded build did not publish ready")
	}
	return q, out
}

func TestSyncMigrationPGHTTPSBoundEncryptedWholeBatchAndNoQualification(t *testing.T) {
	m := newMigrationPG(t)
	_, out := m.create(t)
	id, digest := out["id"].(string), out["manifestDigest"].(string)
	deletionExec(t, m.base.ctx, m.app, `UPDATE message.messages SET version=version+1,updated_at=now() WHERE id=$1`, m.message.ID)
	envelope, err := base64.RawURLEncoding.Strict().DecodeString(out["manifestEnvelope"].(string))
	if err != nil || syncDigest(envelope) != digest || bytes.Contains(envelope, []byte("sourceReceipt")) {
		t.Fatal("public exact manifest digest/provenance boundary")
	}
	r, _ := http.NewRequest("GET", m.url+"/api/sync/migrations/"+id+"/capsule", nil)
	r.Header.Set("Authorization", "Bearer "+m.targetToken)
	r.Header.Set("X-JobsIM-Store-Generation", m.targetStore)
	res, err := m.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	capsule, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || syncDigest(capsule) != out["capsuleSHA256"] || bytes.Contains(capsule, []byte(m.message.Text)) {
		t.Fatal("encrypted original capsule")
	}
	key := m.call(t, "POST", "/api/sync/migrations/"+id+"/key", m.targetToken, m.targetStore, map[string]string{"manifestDigest": digest, "password": m.base.q.Password}, 200)
	keyBytes, _ := base64.RawURLEncoding.DecodeString(key["key"].(string))
	aad, _ := base64.RawURLEncoding.DecodeString(key["aad"].(string))
	block, _ := aes.NewCipher(keyBytes)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, capsule[8:20], capsule[20:], aad)
	if err != nil || string(capsule[:8]) != migrationMagic {
		t.Fatal("strict original AES-GCM format")
	}
	var pack migrationPackage
	if json.Unmarshal(plain, &pack) != nil || pack.Format != migrationFormat || pack.ManifestDigest != digest || len(pack.Messages) != 1 || pack.Messages[0].Text != m.message.Text || bytes.Contains(plain, []byte("sourceReceipt")) || bytes.Contains(plain, []byte("ClientMsg")) {
		t.Fatal("ordinary history whitelist")
	}
	capsule[len(capsule)-1] ^= 1
	if _, err = gcm.Open(nil, capsule[8:20], capsule[20:], aad); err == nil {
		t.Fatal("tampered package accepted")
	}
	valid := m.call(t, "POST", "/api/sync/migrations/"+id+"/validate", m.targetToken, m.targetStore, map[string]string{"manifestDigest": digest}, 200)
	done := m.call(t, "POST", "/api/sync/migrations/"+id+"/installed", m.targetToken, m.targetStore, map[string]string{"manifestDigest": digest, "installToken": valid["installToken"].(string)}, 200)
	if done["qualifiedAck"] != false || done["serverVerifiedLocalDurability"] != false || done["sourcePreserved"] != true {
		t.Fatal("installation report fabricated qualification/durability")
	}
	var messages, qualified int
	if err = m.app.DB.QueryRow(m.base.ctx, `SELECT (SELECT count(*) FROM message.messages),(SELECT count(*) FROM sync.storage_qualifications)`).Scan(&messages, &qualified); err != nil || messages != 1 || qualified != 0 {
		t.Fatal("migration sent a new message or copied source qualification", err)
	}
}

func TestSyncMigrationPGOriginalResultsNeverCreateAndTargetCannotSwap(t *testing.T) {
	m := newMigrationPG(t)
	m.call(t, "POST", "/api/sync/migration-results", m.base.token, m.sourceStore, map[string]string{"requestId": newID(), "sourceStoreGeneration": m.sourceStore}, 200)
	q, out := m.create(t)
	q.Password = ""
	same := m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 200)
	if same["id"] != out["id"] {
		t.Fatal("original retry duplicated transfer")
	}
	q.Messages[0].ObjectMode = "metadata_only"
	m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 409)
	id := out["id"].(string)
	m.call(t, "GET", "/api/sync/migrations/"+id+"/capsule", m.base.token, m.sourceStore, nil, 403)
	m.call(t, "GET", "/api/sync/migrations/"+id, m.targetToken, newID(), nil, 403)
	m.call(t, "POST", "/api/sync/migrations/"+id+"/key", m.targetToken, m.targetStore, map[string]string{"manifestDigest": out["manifestDigest"].(string), "password": "wrong non-secret fixture"}, 403)
}

func TestSyncMigrationPGCurrentGlobalMembershipAndSecurityRechecked(t *testing.T) {
	for _, kind := range []string{"global", "membership", "security", "store"} {
		t.Run(kind, func(t *testing.T) {
			m := newMigrationPG(t)
			_, out := m.create(t)
			id := out["id"].(string)
			deletionFence(t, m.app, func(ctx context.Context) error {
				switch kind {
				case "global":
					_, err := m.app.DB.Exec(ctx, `INSERT INTO message.account_erasure_floors(user_id,deletion_id,request_version) VALUES($1,$2,1)`, m.base.user.ID, newID())
					if err != nil {
						return err
					}
				case "membership":
					_, err := m.app.DB.Exec(ctx, `UPDATE conversation.members SET membership_generation=membership_generation+1 WHERE conversation_id=$1`, m.message.ConversationID)
					if err != nil {
						return err
					}
				case "security":
					_, err := m.app.DB.Exec(ctx, `UPDATE auth.users SET security_version=security_version+1 WHERE id=$1`, m.base.user.ID)
					if err != nil {
						return err
					}
				case "store":
					_, err := m.app.DB.Exec(ctx, `DELETE FROM sync.device_stores WHERE user_id=$1 AND device_id=$2`, m.base.user.ID, m.base.user.DeviceID)
					if err != nil {
						return err
					}
				}
				role := map[string]string{"global": "message", "membership": "conversation", "security": "identity", "store": "sync"}[kind]
				return m.base.nodes[role].drainMigrations(ctx)
			})
			rec, err := m.app.readMigration(m.base.ctx, id)
			if err != nil || rec.State != "retired" || len(rec.Key) != 0 {
				t.Fatal("source invalidation retained a deliverable key", err)
			}
			path, _ := m.app.migrationPath(id)
			if _, err = os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("original capsule was not drained")
			}
		})
	}
}

func TestSyncMigrationPGCloudOnlyNeedsOriginalQualifiedDigest(t *testing.T) {
	m := newMigrationPG(t)
	q := m.request(t)
	deletionExec(t, m.base.ctx, m.app, `UPDATE message.messages SET content_unavailable=true,text='',delete_scope='cloud_only' WHERE id=$1`, m.message.ID)
	m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 409)
	q.Messages[0].Receipt = "invented-receipt"
	m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 409)
	var created int
	if err := m.app.DB.QueryRow(m.base.ctx, `SELECT count(*) FROM sync.migrations`).Scan(&created); err != nil || created != 0 {
		t.Fatal("unqualified local-only source created an artifact", err)
	}
}

func TestSyncMigrationPGCancellationDrainsAndCannotRepublish(t *testing.T) {
	m := newMigrationPG(t)
	_, out := m.create(t)
	id, digest := out["id"].(string), out["manifestDigest"].(string)
	m.call(t, "POST", "/api/sync/migrations/"+id+"/cancel", m.base.token, m.sourceStore, map[string]string{"manifestDigest": digest}, 200)
	m.call(t, "POST", "/api/sync/migrations/"+id+"/validate", m.targetToken, m.targetStore, map[string]string{"manifestDigest": digest}, 409)
	tag, err := m.app.DB.Exec(m.base.ctx, `UPDATE sync.migrations SET state='ready' WHERE id=$1 AND state='building'`, id)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatal("old builder republished a cancelled migration")
	}
}

func TestSyncMigrationPGNoHTTPOrForgedGatewayTLS(t *testing.T) {
	m := newMigrationPG(t)
	q := m.request(t)
	raw, _ := json.Marshal(q)
	r := httptest.NewRequest("POST", "/api/sync/migrations", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+m.base.token)
	r.Header.Set("X-JobsIM-Migration-TLS-Time", str(time.Now().Unix()))
	r.Header.Set("X-JobsIM-Migration-TLS-Proof", "forged")
	w := httptest.NewRecorder()
	m.app.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("plaintext/forged public migration gate accepted")
	}
	var count int
	if err := m.app.DB.QueryRow(m.base.ctx, `SELECT count(*) FROM sync.migrations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied plaintext request created migration", err)
	}
}

func TestSyncMigrationPGRestoreRetiresKeysCapsulesAndPairings(t *testing.T) {
	m := newMigrationPG(t)
	_, out := m.create(t)
	conn, err := m.app.DB.Acquire(m.base.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if err = CancelRestoredMigrations(m.base.ctx, conn.Conn(), m.app.Config.StateDir); err != nil {
		t.Fatal(err)
	}
	rec, err := m.app.readMigration(m.base.ctx, out["id"].(string))
	if err != nil || rec.State != "retired" || len(rec.Key) != 0 {
		t.Fatal("restore retained derived authority")
	}
	files, err := os.ReadDir(filepath.Join(m.app.Config.StateDir, "Migrations"))
	if err != nil || len(files) != 0 {
		t.Fatal("restore retained delivery capsule", err)
	}
	var live bool
	if err = m.app.DB.QueryRow(m.base.ctx, `SELECT EXISTS(SELECT 1 FROM sync.migration_pairings WHERE expires_at>clock_timestamp())`).Scan(&live); err != nil || live {
		t.Fatal("restore retained old pairing", err)
	}
}

func (m *migrationPGFixture) qualifiedOriginal(t *testing.T) (string, string) {
	t.Helper()
	profile := ReceiptProfile{ID: "isolated-migration-test", BuildDigest: syncDigest([]byte("explicit test build")), VerificationDigest: syncDigest([]byte("explicit fixture approval; no native evidence")), StorageContract: receiptFileContract}
	m.app.Config.ReceiptProfiles = []ReceiptProfile{profile}
	m.call(t, "PUT", "/api/sync/storage", m.base.token, m.sourceStore, storageDeclaration{RequestID: newID(), StoreGeneration: m.sourceStore, RevisionEpoch: "1", ProfileID: profile.ID, BuildDigest: profile.BuildDigest, VerificationDigest: profile.VerificationDigest}, 200)
	owner := m.base.nodes["message"]
	prepared, err := owner.prepareMessageRetention(m.base.ctx, m.base.user, m.message.ID, m.message.ConversationID)
	if err != nil {
		t.Fatal("original delivery preparation", err)
	}
	tx, err := owner.DB.Begin(m.base.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.persistMessageRetention(m.base.ctx, tx, m.message.ID, prepared); err != nil {
		tx.Rollback(m.base.ctx)
		t.Fatal("original policy assignment", err)
	}
	if err = tx.Commit(m.base.ctx); err != nil {
		t.Fatal(err)
	}
	challenge := m.call(t, "POST", "/api/sync/receipt-challenges", m.base.token, m.sourceStore, map[string]string{"messageId": m.message.ID}, 200)
	request := newID()
	m.call(t, "POST", "/api/sync/receipts", m.base.token, m.sourceStore, contentReceiptRequest{RequestID: request, Store: m.sourceStore, Epoch: "1", Qualification: challenge["qualificationId"].(string), Kind: "message_content", Challenge: challenge["challenge"].(string), BodyDigest: challenge["bodyDigest"].(string)}, 200)
	return request, challenge["bodyEnvelope"].(string)
}

func TestSyncMigrationPGQualifiedLocalOnlySurvivesCloudCleanupWithoutNewACK(t *testing.T) {
	m := newMigrationPG(t)
	receipt, envelope := m.qualifiedOriginal(t)
	owner := m.base.nodes["message"]
	deletionFence(t, owner, func(ctx context.Context) error {
		return owner.QueueRetentionErasure(ctx, m.message.ID, "isolated-original-cloud-cleanup", "cloud_only", time.Now().Add(-time.Second))
	})
	if err := owner.resumeRetention(m.base.ctx, 2); err != nil {
		t.Fatal("real cloud lifecycle", err)
	}
	q := m.request(t)
	q.Messages[0].Envelope = envelope
	q.Messages[0].Receipt = receipt
	out := m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 200)
	if out["status"] != "ready" {
		t.Fatal("lawfully retained original body was denied")
	}
	id := out["id"].(string)
	m.call(t, "POST", "/api/sync/migrations/"+id+"/validate", m.targetToken, m.targetStore, map[string]string{"manifestDigest": out["manifestDigest"].(string)}, 200)
	deletionFence(t, m.app, func(ctx context.Context) error {
		_, err := m.app.DB.Exec(ctx, `UPDATE conversation.members SET membership_generation=membership_generation+1 WHERE conversation_id=$1`, m.message.ConversationID)
		if err != nil {
			return err
		}
		return m.base.nodes["conversation"].drainMigrations(ctx)
	})
	rec, err := m.app.readMigration(m.base.ctx, id)
	if err != nil || rec.State != "retired" {
		t.Fatal("rejoin inherited an old local-only receipt", err)
	}
	q.Request = newID()
	q.Pairing = m.request(t).Pairing
	m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 403)
}

func TestSyncMigrationPGUnprotectedUnavailableNeverBecomesLocalOnly(t *testing.T) {
	for _, kind := range []string{"unknown", "protection_pending"} {
		t.Run(kind, func(t *testing.T) {
			m := newMigrationPG(t)
			q := m.request(t)
			if kind == "unknown" {
				deletionExec(t, m.base.ctx, m.app, `UPDATE message.messages SET content_unavailable=true,text='',delete_scope='cloud_only' WHERE id=$1`, m.message.ID)
			} else {
				deletionExec(t, m.base.ctx, m.app, `INSERT INTO message.lifecycle_jobs(scope_id,id,message_id,generation,policy_version,restore_epoch,reason,delete_scope,due_at,state,before_event,after_event) VALUES($1,$2,$3,1,1,1,'history_expired','cloud_only',now()-interval '1 second','protection_pending',$4,$5)`, m.app.Config.DeploymentID, newID(), m.message.ID, newID(), newID())
			}
			m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 409)
		})
	}
}

type migrationRetireOutage struct{}

func (migrationRetireOutage) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/internal/migrations/retire" {
		return nil, errors.New("injected controlled migration retirement outage")
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestSyncMigrationPGIndependentObjectCleanupDrainsMigrationBeforeUnlink(t *testing.T) {
	data := []byte("complete original personal attachment; not a URL or thumbnail\n")
	m := newMigrationPG(t, data)
	receipt, envelope := m.qualifiedOriginal(t)
	media := m.base.nodes["media"]
	var plan retentionMediaPlan
	deletionFence(t, media, func(ctx context.Context) error {
		var err error
		plan, _, err = media.mediaRetentionPlan(ctx, m.message.ID, m.message.AttachmentID)
		return err
	})
	q := m.request(t)
	q.Messages[0] = migrationInputMessage{Envelope: envelope, Receipt: receipt, ObjectMode: "full"}
	q.Objects = []migrationInputObject{{Object: m.message.AttachmentID, Message: m.message.ID, Bytes: data}}
	out := m.call(t, "POST", "/api/sync/migrations", m.base.token, m.sourceStore, q, 200)
	id := out["id"].(string)
	deletionFence(t, media, func(ctx context.Context) error {
		_, err := media.DB.Exec(ctx, `UPDATE media.retention_floors SET due_at=clock_timestamp()-interval '1 second' WHERE object_id=$1`, m.message.AttachmentID)
		return err
	})
	media.Client = &http.Client{Transport: migrationRetireOutage{}, Timeout: 5 * time.Second}
	deletionFence(t, media, func(ctx context.Context) error {
		err := media.clearRetainedMedia(ctx, plan, m.message.AttachmentID, m.message.ID, m.message.AttachmentID, time.Now().Add(-time.Second))
		if err == nil {
			t.Fatal("source bytes were cleared while derived migration retirement was unknown")
		}
		return nil
	})
	path := filepath.Join(media.Config.StateDir, "Media", m.message.AttachmentID)
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
		t.Fatal("original bytes lost before exact migration retirement", err)
	}
	var cleared bool
	if err := media.DB.QueryRow(m.base.ctx, `SELECT bytes_cleared FROM media.retention_floors WHERE object_id=$1`, m.message.AttachmentID).Scan(&cleared); err != nil || cleared {
		t.Fatal("unknown cleanup falsely acknowledged bytes cleared", err)
	}
	media.Client = &http.Client{Timeout: 5 * time.Second}
	deletionFence(t, media, func(ctx context.Context) error {
		return media.clearRetainedMedia(ctx, plan, m.message.AttachmentID, m.message.ID, m.message.AttachmentID, time.Now().Add(-time.Second))
	})
	rec, err := m.app.readMigration(m.base.ctx, id)
	if err != nil || rec.State != "retired" || len(rec.Key) != 0 {
		t.Fatal("object cleanup left a deliverable migration", err)
	}
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("acknowledged object cleanup retained original bytes", err)
	}
}

func TestSyncMigrationPGStoredManifestTupleAndDigestFailClosed(t *testing.T) {
	m := newMigrationPG(t)
	_, out := m.create(t)
	id := out["id"].(string)
	deletionExec(t, m.base.ctx, m.app, `UPDATE sync.migrations SET manifest=jsonb_set(manifest,'{targetStoreGeneration}',to_jsonb($2::text)) WHERE id=$1`, id, newID())
	m.call(t, "POST", "/api/sync/migrations/"+id+"/validate", m.targetToken, m.targetStore, map[string]string{"manifestDigest": out["manifestDigest"].(string)}, 503)
}
