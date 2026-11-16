// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type passwordChangeTransport struct {
	next http.RoundTripper
	drop bool
}

func (t *passwordChangeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.drop && r.URL.Path == "/api/auth/password-change" {
		t.drop = false
		return nil, errors.New("injected pre-delivery loss")
	}
	return t.next.RoundTrip(r)
}

type cliPasswordFixture struct {
	*cliDataExportFixture
	db           *sql.DB
	vault        *testCredentialVault
	proof        passwordChangeProof
	result       passwordChangeResult
	recorded     bool
	feature      bool
	writeStatus  int
	resultStatus int
	authHeaders  []string
	beforeWrite  func()
}

func cliPasswordSetup(t *testing.T) *cliPasswordFixture {
	f := cliDataExportSetup(t)
	m := f.client.auth
	m.username = "password_user"
	m.profile = f.identity.ConnectionProfile
	m.root = f.root
	m.pair.User.Username = m.username
	identity, _ := json.Marshal(f.identity)
	m.state = authState{Status: "active", UserID: f.identity.UserID, FamilyID: m.pair.FamilyID, DeviceID: f.identity.DeviceID, StoreGeneration: f.identity.StoreGeneration, MessageStore: filepath.Join(fingerprint(string(identity)), "client.sqlite")}
	m.pair.MessageStore = m.state.MessageStore
	m.pair.StoreGeneration = m.state.StoreGeneration
	directory := filepath.Join(f.root, "JobsIM", "CLI", "Authentication", m.profile, fingerprint(m.username))
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "authentication.sqlite")+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec("CREATE TABLE authentication_state(singleton INTEGER PRIMARY KEY,payload TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	m.db = db
	if err = m.persist(); err != nil {
		t.Fatal(err)
	}
	vault := &testCredentialVault{}
	pair, _ := json.Marshal(m.pair)
	vault.Write(pair)
	m.vault = vault
	if err = passwordChangeMigrate(db); err != nil {
		t.Fatal(err)
	}
	yes := true
	r := &cliPasswordFixture{cliDataExportFixture: f, db: db, vault: vault, feature: true, writeStatus: 200, resultStatus: 404, proof: passwordChangeProof{Current: []byte("Original-password-123"), New: []byte("Replacement-password-456")}, result: passwordChangeResult{Status: "changed", Epoch: "1", Credential: "9007199254740993", Security: "9007199254740994", Current: &yes, Login: &yes}}
	f.setHook(r.servePassword)
	return r
}
func (r *cliPasswordFixture) servePassword(w http.ResponseWriter, q *http.Request) bool {
	switch q.URL.Path {
	case "/api/bootstrap":
		cliSyncJSON(w, 200, map[string]any{"deploymentId": r.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{"own_password_change": r.feature}})
	case "/api/auth/password-change-results", "/api/auth/password-change":
		raw, _ := io.ReadAll(q.Body)
		var body map[string]string
		if json.Unmarshal(raw, &body) != nil {
			r.t.Error("invalid proof envelope")
		}
		if q.URL.Path == "/api/auth/password-change-results" {
			if q.Header.Get("Authorization") != "" || q.Header.Get("Cookie") != "" {
				r.t.Error("pure result lookup must be anonymous")
			}
			if r.recorded && r.resultStatus == 200 {
				r.result.ID = body["requestId"]
				cliSyncJSON(w, 200, r.result)
			} else {
				cliSyncJSON(w, r.resultStatus, map[string]any{"error": map[string]string{"code": "PASSWORD_CHANGE_OPERATION_NOT_FOUND"}})
			}
			return true
		}
		r.authHeaders = append(r.authHeaders, q.Header.Get("Authorization"))
		if r.beforeWrite != nil {
			r.beforeWrite()
		}
		if !r.recorded && q.Header.Get("Authorization") == "" {
			cliSyncJSON(w, 401, map[string]any{"error": map[string]string{"code": "UNAUTHORIZED"}})
			return true
		}
		if r.writeStatus != 200 {
			cliSyncJSON(w, r.writeStatus, map[string]any{"error": map[string]string{"code": "PASSWORD_CHANGE_REJECTED"}})
			return true
		}
		r.recorded = true
		r.resultStatus = 200
		r.result.ID = body["requestId"]
		cliSyncJSON(w, 200, r.result)
	default:
		return false
	}
	return true
}
func (r *cliPasswordFixture) prepare() passwordChangeIntent {
	o, err := r.client.preparePasswordChange()
	if err != nil {
		r.t.Fatal(err)
	}
	return o
}
func (r *cliPasswordFixture) load(id string) passwordChangeIntent {
	o, err := loadPasswordChange(r.db, id, r.client.base, r.client.deployment, r.client.auth.profile, "password_user")
	if err != nil {
		r.t.Fatal(err)
	}
	return o
}
func TestPasswordChangeMetadataDurableAndNoProofHashOrPasswordStored(t *testing.T) {
	r := cliPasswordSetup(t)
	o := r.prepare()
	if _, err := r.client.preparePasswordChange(); err == nil {
		t.Fatal("unresolved original replaced")
	}
	r.beforeWrite = func() {
		row := r.load(o.ID)
		if row.State != "unknown" || row.Attempts != 1 || r.client.auth.state.Status != "password_change_unknown" {
			t.Error("responsibility not durable before HTTP")
		}
	}
	result, err := r.client.performPasswordChange(r.db, o, &r.proof, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "changed" || r.client.auth.state.Status != "password_changed" || r.client.token != "" {
		t.Fatal("old family not stopped")
	}
	var raw string
	r.db.QueryRow("SELECT payload FROM password_change_intents WHERE id=?", o.ID).Scan(&raw)
	for _, secret := range []string{string(r.proof.Current), string(r.proof.New), "currentPassword", "newPassword", "otp", "bodyHash", "requestDigest"} {
		if strings.Contains(raw, secret) {
			t.Fatal("secret or proof digest in metadata")
		}
	}
	if len(r.vault.raw) != 0 {
		t.Fatal("old credential item not removed")
	}
	if len(r.calls("/api/auth/refresh")) != 0 {
		t.Fatal("password change must not auto-refresh")
	}
}
func TestPasswordChangeLostFirstRequestSameFamilyExplicitAuthorization(t *testing.T) {
	r := cliPasswordSetup(t)
	o := r.prepare()
	transport := r.client.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	r.client.http.Transport = &passwordChangeTransport{next: transport, drop: true}
	if _, err := r.client.performPasswordChange(r.db, o, &r.proof, false, true); err == nil {
		t.Fatal("network loss became success")
	}
	if len(r.calls("/api/auth/password-change")) != 0 || r.client.auth.state.Status != "password_change_unknown" {
		t.Fatal("drop fixture or sticky gate")
	}
	row := r.load(o.ID)
	if _, err := r.client.performPasswordChange(r.db, row, &r.proof, false, false); err == nil {
		t.Fatal("404 accepted")
	}
	if len(r.calls("/api/auth/password-change")) != 0 {
		t.Fatal("pure lookup first-created")
	}
	result, err := r.client.performPasswordChange(r.db, row, &r.proof, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != o.ID || len(r.authHeaders) != 1 || r.authHeaders[0] == "" {
		t.Fatal("same original family retry did not authorize original nonce")
	}
	if r.load(o.ID).Attempts != 2 {
		t.Fatal("attempt metadata lost")
	}
}
func TestPasswordChangeSuccessorFamilyNeverAuthorizesOldUnknown(t *testing.T) {
	r := cliPasswordSetup(t)
	o := r.prepare()
	o, err := savePasswordChange(r.db, o, func(row *passwordChangeIntent) error { row.State = "unknown"; row.Attempts = 1; return nil })
	if err != nil {
		t.Fatal(err)
	}
	m := r.client.auth
	m.mu.Lock()
	m.pair.FamilyID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	m.state.FamilyID = m.pair.FamilyID
	m.state.Status = "active"
	pair, _ := json.Marshal(m.pair)
	m.vault.Write(pair)
	m.mu.Unlock()
	before := append([]byte{}, r.vault.raw...)
	if _, err = r.client.performPasswordChange(r.db, o, &r.proof, true, false); err == nil {
		t.Fatal("anonymous unseen change accepted")
	}
	if len(r.authHeaders) != 1 || r.authHeaders[0] != "" {
		t.Fatal("later family leaked to old intention")
	}
	if string(r.vault.raw) != string(before) || m.state.Status != "active" {
		t.Fatal("later family altered")
	}
}
func TestPasswordChangeOldChangedResultCannotClearLaterFamily(t *testing.T) {
	r := cliPasswordSetup(t)
	o := r.prepare()
	m := r.client.auth
	m.mu.Lock()
	m.pair.FamilyID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	m.state.FamilyID = m.pair.FamilyID
	pair, _ := json.Marshal(m.pair)
	m.vault.Write(pair)
	m.mu.Unlock()
	r.recorded = true
	r.resultStatus = 200
	no := false
	r.result.Current = &no
	before := append([]byte{}, r.vault.raw...)
	if _, err := r.client.performPasswordChange(r.db, o, &r.proof, false, false); err != nil {
		t.Fatal(err)
	}
	if m.state.Status != "active" || r.client.token == "" || string(r.vault.raw) != string(before) {
		t.Fatal("historical result cleared active successor")
	}
}
func TestPasswordChangeCrashAfterIntentBeforeAuthPauseBlocksRestore(t *testing.T) {
	r := cliPasswordSetup(t)
	o := r.prepare()
	if _, err := savePasswordChange(r.db, o, func(row *passwordChangeIntent) error { row.Attempts = 1; row.State = "unknown"; return nil }); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.client.auth.restore(); err != nil || ok {
		t.Fatal("unknown original was restored", err)
	}
	if r.client.auth.state.Status != "password_change_unknown" || len(r.calls("/api/auth/context")) != 0 {
		t.Fatal("restore gate did not stop business activation")
	}
}
func TestPasswordChangeAnonymousOriginalJournalReopen(t *testing.T) {
	r := cliPasswordSetup(t)
	o := r.prepare()
	r.db.Close()
	db, close, err := openPasswordChangeMetadata(r.root, r.identity.ConnectionProfile, "password_user")
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	loaded, err := loadPasswordChange(db, o.ID, r.client.base, r.client.deployment, r.identity.ConnectionProfile, "password_user")
	if err != nil {
		t.Fatal(err)
	}
	r.recorded = true
	r.resultStatus = 200
	r.client.auth = nil
	r.client.store = nil
	if _, err = r.client.performPasswordChange(db, loaded, &r.proof, false, false); err != nil {
		t.Fatal(err)
	}
	if len(r.calls("/api/auth/password-change")) != 0 {
		t.Fatal("reopen lookup wrote")
	}
}
func TestPasswordChangeCapabilityWithdrawalAndMalformedResultsRetainOriginal(t *testing.T) {
	for _, kind := range []string{"cap", "bad-counter", "missing-current", "epoch", "rate"} {
		t.Run(kind, func(t *testing.T) {
			r := cliPasswordSetup(t)
			o := r.prepare()
			r.recorded = true
			r.resultStatus = 200
			switch kind {
			case "cap":
				r.feature = false
			case "bad-counter":
				r.result.Security = "01"
			case "missing-current":
				r.result.Current = nil
			case "epoch":
				r.result.Epoch = "2"
			case "rate":
				r.resultStatus = 429
			}
			_, err := r.client.performPasswordChange(r.db, o, &r.proof, false, false)
			if (err == nil) != (kind == "cap") {
				t.Fatal("unexpected validation", kind, err)
			}
			if len(r.calls("/api/auth/password-change")) != 0 {
				t.Fatal("invalid/pure result wrote")
			}
		})
	}
}
func TestPasswordChangeSQLFailureAndRejectionNeverReuseOTPOrRefresh(t *testing.T) {
	r := cliPasswordSetup(t)
	if _, err := r.db.Exec("CREATE TRIGGER fail_password_intent BEFORE INSERT ON password_change_intents BEGIN SELECT RAISE(FAIL,'injected persistence failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.preparePasswordChange(); err == nil {
		t.Fatal("SQL failure accepted")
	}
	if len(r.calls("/api/auth/password-change")) != 0 {
		t.Fatal("proof before durable intent")
	}
	r.db.Exec("DROP TRIGGER fail_password_intent")
	o := r.prepare()
	r.writeStatus = 403
	r.proof.OTP = []byte("123456")
	if _, err := r.client.performPasswordChange(r.db, o, &r.proof, false, true); err == nil {
		t.Fatal("fresh rejection accepted")
	}
	if r.client.auth.state.Status != "active" || r.load(o.ID).State != "rejected" {
		t.Fatal("definite first rejection stuck active account")
	}
	if len(r.calls("/api/auth/refresh")) != 0 {
		t.Fatal("automatic refresh")
	}
	r.proof.clear()
	for _, b := range r.proof.OTP {
		if b != 0 {
			t.Fatal("OTP buffer not erased")
		}
	}
}
