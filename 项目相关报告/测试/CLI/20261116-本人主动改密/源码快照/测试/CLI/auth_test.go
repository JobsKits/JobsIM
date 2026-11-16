// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testCredentialVault struct {
	mu          sync.Mutex
	raw         []byte
	fail        bool
	beforeWrite func()
}

func (v *testCredentialVault) Read() ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.raw) == 0 {
		return nil, errVaultNotFound
	}
	return append([]byte{}, v.raw...), nil
}
func (v *testCredentialVault) Write(raw []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.beforeWrite != nil {
		v.beforeWrite()
	}
	if v.fail {
		return errVaultUnavailable
	}
	v.raw = append([]byte{}, raw...)
	return nil
}
func (v *testCredentialVault) Delete() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	erase(v.raw)
	v.raw = nil
	return nil
}

func authTestPair() authPair {
	p := authPair{Token: "test-access-original", RefreshToken: "test-refresh-original", FamilyID: "family-original", RefreshGeneration: "1", DeviceID: "device-original", RestoreEpoch: "1", SourceContextID: "personal", AccessExpiresAt: time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339Nano), IdleExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339Nano), AbsoluteExpiresAt: time.Now().UTC().Add(90 * 24 * time.Hour).Format(time.RFC3339Nano)}
	p.User.ID, p.User.Username = "user-original", "test_user"
	return p
}

func newTestAuth(t *testing.T, handler http.HandlerFunc) (*authManager, *testCredentialVault) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "auth.sqlite")+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("CREATE TABLE authentication_state(singleton INTEGER PRIMARY KEY,payload TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := &client{base: server.URL, http: server.Client(), epoch: "1", deployment: "test-deployment"}
	v := &testCredentialVault{}
	m := &authManager{c: c, db: db, vault: v, username: "test_user", refresh: true, chain: true}
	c.auth = m
	if err = m.accept(authTestPair(), "initial-original"); err != nil {
		t.Fatal(err)
	}
	return m, v
}

func readTestAuthState(t *testing.T, m *authManager) authState {
	t.Helper()
	var raw string
	if err := m.db.QueryRow("SELECT payload FROM authentication_state WHERE singleton=1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "test-access-") || strings.Contains(raw, "test-refresh-") || strings.Contains(raw, "test-password") {
		t.Fatal("secret reached ordinary authentication SQLite")
	}
	var state authState
	if json.Unmarshal([]byte(raw), &state) != nil {
		t.Fatal("authentication metadata malformed")
	}
	return state
}

func TestAuthRefreshSingleFlightCommitsPairBeforePublishing(t *testing.T) {
	var count atomic.Int32
	var m *authManager
	var absolute string
	m, _ = newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/refresh" {
			count.Add(1)
			if readTestAuthState(t, m).Status != "refresh_unknown" {
				t.Error("HTTP preceded durable unknown marker")
			}
			time.Sleep(25 * time.Millisecond)
			p := authTestPair()
			p.Token, p.RefreshToken, p.RefreshGeneration = "test-access-next", "test-refresh-next", "2"
			p.AbsoluteExpiresAt = absolute
			json.NewEncoder(w).Encode(p)
			return
		}
		if r.Header.Get("Authorization") == "Bearer test-access-original" {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":{"code":"ACCESS_EXPIRED"}}`)
			return
		}
		io.WriteString(w, `{"status":"current"}`)
	})
	absolute = m.pair.AbsoluteExpiresAt
	start := make(chan struct{})
	errs := make(chan error, 8)
	for range 8 {
		go func() { <-start; var result any; errs <- m.c.call("GET", "/api/me", nil, &result) }()
	}
	close(start)
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if count.Load() != 1 || m.state.RefreshGeneration != "2" {
		t.Fatal("parallel callers consumed multiple refresh generations")
	}
	raw, _ := m.vault.Read()
	defer erase(raw)
	var protected authPair
	json.Unmarshal(raw, &protected)
	if protected.Token != m.pair.Token || protected.RefreshToken != m.pair.RefreshToken {
		t.Fatal("published pair differs from atomic secure record")
	}
	readTestAuthState(t, m)
}

func TestAuthRefreshUnknownNeverReusesOldRefreshAfterReopen(t *testing.T) {
	var count atomic.Int32
	m, v := newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/refresh" {
			count.Add(1)
			connection, _, _ := w.(http.Hijacker).Hijack()
			connection.Close()
			return
		}
		w.WriteHeader(401)
		io.WriteString(w, `{"error":{"code":"ACCESS_EXPIRED"}}`)
	})
	var out any
	if m.c.call("GET", "/api/me", nil, &out) == nil {
		t.Fatal("lost response looked successful")
	}
	if m.c.call("GET", "/api/me", nil, &out) == nil || count.Load() != 1 {
		t.Fatal("old refresh was reused after unknown")
	}
	state := readTestAuthState(t, m)
	reopened := &authManager{c: m.c, db: m.db, vault: v, state: state, username: m.username, refresh: true, chain: true}
	if resumed, err := reopened.restore(); err != nil || resumed {
		t.Fatal("secure record cleared a sticky unknown after reopen")
	}
	intent, err := reopened.prepareInitial("login")
	if err != nil || intent.Mode != "reauthenticate" || intent.PreviousFamilyID != "family-original" {
		t.Fatal("new proof did not retain original family responsibility")
	}
}

func TestAuthSecurePairFailureDoesNotPublishOrRetry(t *testing.T) {
	var count atomic.Int32
	var m *authManager
	var absolute string
	m, v := newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/refresh" {
			count.Add(1)
			p := authTestPair()
			p.Token, p.RefreshToken, p.RefreshGeneration = "test-access-next", "test-refresh-next", "2"
			p.AbsoluteExpiresAt = absolute
			json.NewEncoder(w).Encode(p)
			return
		}
		w.WriteHeader(401)
		io.WriteString(w, `{"error":{"code":"ACCESS_EXPIRED"}}`)
	})
	absolute = m.pair.AbsoluteExpiresAt
	v.fail = true
	var out any
	if m.c.call("GET", "/api/me", nil, &out) == nil || m.pair.Token != "" || m.c.token != "" {
		t.Fatal("uncommitted secure pair was published")
	}
	if m.c.call("GET", "/api/me", nil, &out) == nil || count.Load() != 1 || readTestAuthState(t, m).Status != "refresh_unknown" {
		t.Fatal("failed secure storage retried old refresh")
	}
}

func TestAuthInitialNonceAndCompleteRetirementClosureRemainImmutable(t *testing.T) {
	var m *authManager
	m, _ = newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/bootstrap" {
			io.WriteString(w, `{"deploymentId":"test-deployment","restoreEpoch":"1","features":{"auth_refresh":true,"auth_retirement_chain":true}}`)
			return
		}
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		state := readTestAuthState(t, m)
		if len(state.Pending) != 1 || state.Pending[0].RequestID != q["requestId"] {
			t.Error("initial nonce was not durable before HTTP")
		}
		connection, _, _ := w.(http.Hijacker).Hijack()
		connection.Close()
	})
	m.state = authState{}
	m.pair = authPair{}
	m.c.token = ""
	m.persist()
	if m.authenticate("login", bufio.NewReader(strings.NewReader("test-password-proof\n"))) == nil {
		t.Fatal("lost initial response did not remain unknown")
	}
	first := m.state.Pending[0]
	resume, err := m.prepareInitial("login")
	if err != nil || resume.RequestID != first.RequestID {
		t.Fatal("unknown initial request silently changed ID")
	}
	second, err := m.prepareInitial("reauthenticate")
	if err != nil || second.PreviousRequestID != first.RequestID {
		t.Fatal("verified successor lost its original nonce")
	}
	third, err := m.prepareInitial("reauthenticate")
	if err != nil || len(third.PreviousRequestIDs) != 2 || third.PreviousRequestIDs[0] != first.RequestID || third.PreviousRequestIDs[1] != second.RequestID {
		t.Fatal("multi-unknown successor dropped an ancestor")
	}
	if m.state.Pending[0].RequestID != first.RequestID || len(m.state.Pending[0].PreviousRequestIDs) != 0 {
		t.Fatal("old initial payload was amended")
	}
	m.chain = false
	if _, err = m.prepareInitial("reauthenticate"); err == nil {
		t.Fatal("old server accepted an unproved complete retirement closure")
	}
	readTestAuthState(t, m)
}

func TestAuthUnknownRegistrationCannotInferCreatedAccount(t *testing.T) {
	m, _ := newTestAuth(t, func(http.ResponseWriter, *http.Request) {})
	m.state = authState{}
	m.pair = authPair{}
	m.c.token = ""
	m.persist()
	original, err := m.prepareInitial("register")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.prepareInitial("reauthenticate"); err == nil {
		t.Fatal("local unknown registration was treated as an existing account")
	}
	resumed, err := m.prepareInitial("register")
	if err != nil || resumed.RequestID != original.RequestID {
		t.Fatal("original registration ID lost")
	}
	m.state.Pending[0].DeliveryExpired = true
	if intent, err := m.prepareInitial("reauthenticate"); err != nil || intent.PreviousRequestID != original.RequestID {
		t.Fatal("server-confirmed delivery expiry could not enter verified recovery")
	}
}

func TestAuthLateFamilyResponseCannotEnterReplacementContext(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	m, _ := newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		io.WriteString(w, `{"id":"old-user-response"}`)
	})
	result := make(chan error, 1)
	go func() { var out any; result <- m.c.call("GET", "/api/me", nil, &out) }()
	<-entered
	m.mu.Lock()
	next := m.pair
	next.User.ID, next.FamilyID, next.DeviceID = "other-user", "other-family", "other-device"
	m.publish(next)
	m.state.Status = "active"
	m.mu.Unlock()
	close(release)
	if err := <-result; err == nil {
		t.Fatal("late old-family success was released into new context")
	}
}

func TestAuthDeviceRetirementPreservesOriginalOutboxAndRejectsLateEnqueue(t *testing.T) {
	identity := identityForTest()
	store, err := openStore(filepath.Join(t.TempDir(), "old.sqlite"), identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	p := pending{DeploymentID: identity.DeploymentID, UserID: identity.UserID, ConversationID: "conversation", ClientMsgID: "original-id", Type: "text", Text: "original body", RestoreEpoch: "1", SendAdmission: "original-admission"}
	if err = store.enqueue(p); err != nil {
		t.Fatal(err)
	}
	var payload, hash string
	store.db.QueryRow("SELECT payload,payload_hash FROM outbox").Scan(&payload, &hash)
	if err = freezePreviousCLIStore(store.db, identity.DeploymentID, identity.UserID, "new-server-device", "1"); err != nil {
		t.Fatal(err)
	}
	var after, afterHash, state string
	store.db.QueryRow("SELECT payload,payload_hash,state FROM outbox").Scan(&after, &afterHash, &state)
	if payload != after || hash != afterHash || state != "frozen" {
		t.Fatal("device replacement migrated or rewrote an original intent")
	}
	p.ClientMsgID = "late-id"
	if store.enqueue(p) == nil {
		t.Fatal("late enqueue bypassed durable device retirement")
	}
}

func TestAuthLogoutUnknownKeepsFamilyButClearsRuntimeAndSystemEntry(t *testing.T) {
	var count atomic.Int32
	m, v := newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(503)
		io.WriteString(w, `{"error":{"code":"TEMPORARY"}}`)
	})
	if m.logout() == nil {
		t.Fatal("unknown server revoke looked completed")
	}
	if m.pair.Token != "" || m.c.token != "" || len(v.raw) != 0 {
		t.Fatal("local logout retained credentials")
	}
	if state := readTestAuthState(t, m); state.Status != "logout_pending" || state.FamilyID != "family-original" {
		t.Fatal("unknown logout lost original revocation responsibility")
	}
	if _, err := m.snapshot(); err == nil {
		t.Fatal("logged out family reopened business")
	}
	if intent, err := m.prepareInitial("login"); err != nil || intent.PreviousFamilyID != "family-original" {
		t.Fatal("fresh proof did not retire unknown old family")
	}
	if count.Load() != 1 {
		t.Fatal("logout retried without original authorization")
	}
}

func TestAuthLogoutMetadataFailureStillDeletesSecureEntryAndRevokes(t *testing.T) {
	var count atomic.Int32
	m, v := newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		io.WriteString(w, `{"status":"signed_out"}`)
	})
	if _, err := m.db.Exec("CREATE TRIGGER fail_logout_metadata BEFORE UPDATE ON authentication_state BEGIN SELECT RAISE(ABORT,'fixture write failure'); END"); err != nil {
		t.Fatal(err)
	}
	if m.logout() == nil {
		t.Fatal("metadata failure was hidden")
	}
	if len(v.raw) != 0 || m.pair.Token != "" || m.c.token != "" || count.Load() != 1 {
		t.Fatal("SQLite failure short-circuited secure deletion or server revocation")
	}
}

func TestAuthMissingMessageStoreCannotResumeSurvivingSystemCredentials(t *testing.T) {
	var calls atomic.Int32
	m, v := newTestAuth(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	m.root, m.profile = t.TempDir(), "test-profile"
	path := filepath.Join(m.root, "JobsIM", "CLI", m.profile, "namespace", "client.sqlite")
	identity := localIdentity{Origin: m.c.base, DeploymentID: m.c.deployment, UserID: m.pair.User.ID, DeviceID: m.pair.DeviceID, Space: "personal", ConnectionProfile: m.profile, StoreGeneration: "store-generation"}
	store, err := openStore(path, identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.bindMessageStore(filepath.Join("namespace", "client.sqlite"), identity.StoreGeneration); err != nil {
		t.Fatal(err)
	}
	store.close()
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	reopened := &authManager{c: m.c, db: m.db, vault: v, state: readTestAuthState(t, m), username: m.username, refresh: true, chain: true, root: m.root, profile: m.profile}
	if resumed, err := reopened.restore(); err != nil || resumed || calls.Load() != 0 {
		t.Fatal("missing local message DB inherited surviving device credentials")
	}
	if intent, err := reopened.prepareInitial("login"); err != nil || intent.PreviousFamilyID != "family-original" || intent.Mode != "reauthenticate" {
		t.Fatal("lost-store path did not preserve old family retirement")
	}
}

func TestAuthMediaRefreshPreservesExactBodyBytes(t *testing.T) {
	var count atomic.Int32
	var absolute string
	want := []byte{0x00, 0xff, 0x80, 0x0d, 0x0a, 0x22, 0x7f}
	m, _ := newTestAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/refresh" {
			count.Add(1)
			p := authTestPair()
			p.Token, p.RefreshToken, p.RefreshGeneration = "test-access-next", "test-refresh-next", "2"
			p.AbsoluteExpiresAt = absolute
			json.NewEncoder(w).Encode(p)
			return
		}
		if r.Header.Get("Authorization") == "Bearer test-access-original" {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":{"code":"ACCESS_EXPIRED"}}`)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != string(want) {
			t.Error("binary media body changed across authenticated retry")
		}
		io.WriteString(w, `{"id":"authenticated-media"}`)
	})
	absolute = m.pair.AbsoluteExpiresAt
	response, err := m.c.authenticatedResponse("POST", "/api/media", "application/octet-stream", want, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || count.Load() != 1 {
		t.Fatal("media did not share the single original-family refresh coordinator")
	}
}
