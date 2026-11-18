package jobsim

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
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

func newMigrationPG(t *testing.T) *migrationPGFixture {
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
	deletionExec(t, f.ctx, f.nodes["message"], `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text','lawful original personal history',1)`, id, conv, f.user.ID, newID(), []byte("non-secret fixture"))
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
				return m.app.drainMigrationRecords(ctx)
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
