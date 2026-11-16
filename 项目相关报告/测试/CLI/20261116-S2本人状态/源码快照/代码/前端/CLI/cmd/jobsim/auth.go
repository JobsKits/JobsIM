// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

var errVaultUnavailable = errors.New("system credential storage unavailable")
var errVaultNotFound = errors.New("system credential entry absent")

type credentialVault interface {
	Read() ([]byte, error)
	Write([]byte) error
	Delete() error
}

type authPair struct {
	Token             string `json:"token"`
	RefreshToken      string `json:"refreshToken"`
	FamilyID          string `json:"familyId"`
	RefreshGeneration string `json:"refreshGeneration"`
	DeviceID          string `json:"deviceId"`
	RestoreEpoch      string `json:"restoreEpoch"`
	SourceContextID   string `json:"sourceContextId"`
	AccessExpiresAt   string `json:"accessExpiresAt"`
	IdleExpiresAt     string `json:"idleExpiresAt"`
	AbsoluteExpiresAt string `json:"absoluteExpiresAt"`
	StoreGeneration   string `json:"storeGeneration,omitempty"`
	MessageStore      string `json:"messageStore,omitempty"`
	User              struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

// This structure is the only authentication data allowed in ordinary SQLite.
type authState struct {
	Status            string       `json:"status"`
	UserID            string       `json:"userId,omitempty"`
	FamilyID          string       `json:"familyId,omitempty"`
	DeviceID          string       `json:"deviceId,omitempty"`
	RefreshGeneration string       `json:"refreshGeneration,omitempty"`
	RefreshRequestID  string       `json:"refreshRequestId,omitempty"`
	InitialRequestID  string       `json:"initialRequestId,omitempty"`
	Pending           []authIntent `json:"pending,omitempty"`
	StoreGeneration   string       `json:"storeGeneration,omitempty"`
	MessageStore      string       `json:"messageStore,omitempty"`
}

type authIntent struct {
	Mode               string   `json:"mode"`
	Username           string   `json:"username"`
	RequestID          string   `json:"requestId"`
	DeviceName         string   `json:"deviceName"`
	PreviousFamilyID   string   `json:"previousFamilyId,omitempty"`
	PreviousRequestID  string   `json:"previousRequestId,omitempty"`
	PreviousRequestIDs []string `json:"previousRequestIds,omitempty"`
	DeliveryExpired    bool     `json:"deliveryExpired,omitempty"`
}

type authManager struct {
	mu        sync.Mutex
	c         *client
	db        *sql.DB
	vault     credentialVault
	release   func()
	state     authState
	pair      authPair
	username  string
	profile   string
	root      string
	temporary bool
	refresh   bool
	chain     bool
}

func openAuthManager(c *client, root, profile, username string, refresh, chain bool, reader *bufio.Reader) (*authManager, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, fmt.Errorf("username required")
	}
	directory := filepath.Join(root, "JobsIM", "CLI", "Authentication", profile, fingerprint(username))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	release, err := lockAuthDirectory(directory)
	if err != nil {
		return nil, err
	}
	m := &authManager{c: c, root: root, profile: profile, username: username, refresh: refresh, chain: chain, release: release}
	failed := true
	defer func() {
		if failed {
			m.close()
		}
	}()
	path := filepath.Join(directory, "authentication.sqlite")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("authentication state must be a private regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := uri.Query()
	for _, value := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)"} {
		q.Add("_pragma", value)
	}
	uri.RawQuery = q.Encode()
	m.db, err = sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	m.db.SetMaxOpenConns(1)
	var mode string
	var synchronous int
	if m.db.QueryRow("PRAGMA journal_mode").Scan(&mode) != nil || m.db.QueryRow("PRAGMA synchronous").Scan(&synchronous) != nil || mode != "wal" || synchronous != 2 {
		return nil, fmt.Errorf("durable authentication metadata storage unavailable")
	}
	if _, err = m.db.Exec("CREATE TABLE IF NOT EXISTS authentication_state(singleton INTEGER PRIMARY KEY CHECK(singleton=1),payload TEXT NOT NULL)"); err != nil {
		return nil, err
	}
	var raw string
	err = m.db.QueryRow("SELECT payload FROM authentication_state WHERE singleton=1").Scan(&raw)
	if err == nil {
		if json.Unmarshal([]byte(raw), &m.state) != nil {
			return nil, fmt.Errorf("authentication metadata damaged; original retained")
		}
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	m.vault, err = openCredentialVault(fingerprint(profile + ":" + username + ":personal"))
	if err == nil {
		probe, readErr := m.vault.Read()
		if errors.Is(readErr, errVaultNotFound) {
			// A missing readable item does not prove the store accepts writes.
			err = m.vault.Write([]byte("JobsIM nonsecret availability probe"))
			if err == nil {
				err = m.vault.Delete()
			}
		} else if readErr == nil {
			err = m.vault.Write(probe)
		} else {
			err = readErr
		}
		erase(probe)
	}
	if err != nil {
		if !errors.Is(err, errVaultUnavailable) {
			return nil, err
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return nil, fmt.Errorf("system credentials unavailable; unattended login is refused")
		}
		fmt.Print("System credential storage unavailable. Use this process only and reauthenticate next time? Type yes: ")
		answer, readErr := reader.ReadString('\n')
		if readErr != nil || strings.TrimSpace(answer) != "yes" {
			return nil, fmt.Errorf("temporary session not selected")
		}
		m.temporary = true
	}
	failed = false
	return m, nil
}

func (m *authManager) close() {
	m.pair = authPair{}
	if m.db != nil {
		m.db.Close()
	}
	if m.release != nil {
		m.release()
	}
}

func (m *authManager) persist() error {
	raw, err := json.Marshal(m.state)
	if err != nil {
		return err
	}
	_, err = m.db.Exec("INSERT INTO authentication_state(singleton,payload) VALUES(1,?) ON CONFLICT(singleton) DO UPDATE SET payload=excluded.payload", string(raw))
	return err
}

func (m *authManager) validPair(pair authPair) bool {
	if pair.Token == "" || pair.User.ID == "" || pair.User.Username != m.username || pair.DeviceID == "" || pair.RestoreEpoch != m.c.epoch {
		return false
	}
	if !m.refresh {
		return pair.RefreshToken == ""
	}
	if pair.RefreshToken == "" || pair.FamilyID == "" || !decimal.MatchString(pair.RefreshGeneration) || pair.SourceContextID != "personal" {
		return false
	}
	for _, value := range []string{pair.AccessExpiresAt, pair.IdleExpiresAt, pair.AbsoluteExpiresAt} {
		deadline, err := time.Parse(time.RFC3339Nano, value)
		if err != nil || deadline.Location() != time.UTC {
			return false
		}
	}
	return true
}

func (m *authManager) publish(pair authPair) {
	m.pair = pair
	m.c.token, m.c.user, m.c.serverDevice = pair.Token, pair.User.ID, pair.DeviceID
}

func (m *authManager) accept(pair authPair, initial string) error {
	if !m.validPair(pair) {
		return fmt.Errorf("authentication response is incomplete; original intent remains unknown")
	}
	if m.state.FamilyID == pair.FamilyID && m.state.DeviceID == pair.DeviceID {
		pair.StoreGeneration, pair.MessageStore = m.state.StoreGeneration, m.state.MessageStore
	}
	if !m.temporary {
		raw, err := json.Marshal(pair)
		if err != nil {
			return err
		}
		err = m.vault.Write(raw)
		erase(raw)
		if err != nil {
			return fmt.Errorf("secure credential replacement failed; original result remains unknown")
		}
	}
	previous := m.state
	m.state = authState{Status: "active", UserID: pair.User.ID, FamilyID: pair.FamilyID, DeviceID: pair.DeviceID, RefreshGeneration: pair.RefreshGeneration, InitialRequestID: initial, StoreGeneration: pair.StoreGeneration, MessageStore: pair.MessageStore}
	if err := m.persist(); err != nil {
		m.state = previous
		return fmt.Errorf("authentication handoff failed; protected unknown marker retained")
	}
	m.publish(pair)
	return nil
}

func (m *authManager) restore() (bool, error) {
	if m.temporary || m.state.Status != "active" || len(m.state.Pending) > 0 {
		return false, nil
	}
	raw, err := m.vault.Read()
	if errors.Is(err, errVaultNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("system credential entry cannot be read; retain protected state and unlock the system store")
	}
	defer erase(raw)
	var pair authPair
	if json.Unmarshal(raw, &pair) != nil || !m.validPair(pair) || pair.FamilyID != m.state.FamilyID || pair.DeviceID != m.state.DeviceID || pair.RefreshGeneration != m.state.RefreshGeneration {
		m.state.Status = "reauth_required"
		return false, m.persist()
	}
	if !m.existingStoreMatches(pair) {
		m.state.Status = "reauth_required"
		return false, m.persist()
	}
	m.publish(pair)
	var context authPair
	if err := m.c.call("GET", "/api/auth/context", nil, &context); err != nil {
		var api *apiFailure
		if !errors.As(err, &api) || api.Status != 401 {
			return false, err
		}
		m.pair = authPair{}
		m.c.token = ""
		m.state.Status = "reauth_required"
		return false, m.persist()
	}
	if context.FamilyID != m.pair.FamilyID || context.DeviceID != m.pair.DeviceID || context.User.ID != m.pair.User.ID || context.RestoreEpoch != m.c.epoch {
		return false, fmt.Errorf("authenticated context changed; no local namespace opened")
	}
	return true, nil
}

func (m *authManager) existingStoreMatches(pair authPair) bool {
	if m.state.MessageStore == "" || m.state.StoreGeneration == "" || pair.MessageStore != m.state.MessageStore || pair.StoreGeneration != m.state.StoreGeneration || filepath.IsAbs(pair.MessageStore) || filepath.Clean(pair.MessageStore) != pair.MessageStore {
		return false
	}
	base := filepath.Join(m.root, "JobsIM", "CLI", m.profile)
	path := filepath.Join(base, pair.MessageStore)
	relative, err := filepath.Rel(base, path)
	if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return false
	}
	defer db.Close()
	var raw, epoch string
	var retired int
	if db.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&raw) != nil || db.QueryRow("SELECT value FROM metadata WHERE key='epoch'").Scan(&epoch) != nil || db.QueryRow("SELECT count(*) FROM metadata WHERE key='auth_retired' AND value='1'").Scan(&retired) != nil || retired != 0 || epoch != m.c.epoch {
		return false
	}
	var identity localIdentity
	return json.Unmarshal([]byte(raw), &identity) == nil && identity.ConnectionProfile == m.profile && identity.DeploymentID == m.c.deployment && identity.UserID == pair.User.ID && identity.DeviceID == pair.DeviceID && identity.Space == "personal" && identity.StoreGeneration == pair.StoreGeneration
}

func (m *authManager) bindMessageStore(relative, generation string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pair := m.pair
	pair.MessageStore, pair.StoreGeneration = relative, generation
	if !m.temporary {
		raw, err := json.Marshal(pair)
		if err != nil {
			return err
		}
		err = m.vault.Write(raw)
		erase(raw)
		if err != nil {
			return fmt.Errorf("secure local-store binding failed; authentication remains unactivated locally")
		}
	}
	m.state.MessageStore, m.state.StoreGeneration = relative, generation
	if err := m.persist(); err != nil {
		return err
	}
	m.publish(pair)
	return nil
}

func appendAuthKey(keys []string, key string) []string {
	if key == "" {
		return keys
	}
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func (m *authManager) prepareInitial(mode string) (authIntent, error) {
	if len(m.state.Pending) > 0 && mode != "reauthenticate" {
		return m.state.Pending[len(m.state.Pending)-1], nil
	}
	intent := authIntent{Mode: mode, Username: m.username, RequestID: id(), DeviceName: "JobsIM CLI"}
	if len(m.state.Pending) > 0 {
		last := m.state.Pending[len(m.state.Pending)-1]
		if last.Mode == "register" && !last.DeliveryExpired && m.state.FamilyID == "" {
			return intent, fmt.Errorf("unknown registration must continue its original ID until the server confirms delivery expiry; local elapsed time proves nothing")
		}
		var keys []string
		for _, previous := range m.state.Pending {
			for _, key := range previous.PreviousRequestIDs {
				keys = appendAuthKey(keys, key)
			}
			keys = appendAuthKey(keys, previous.PreviousRequestID)
			keys = appendAuthKey(keys, previous.RequestID)
		}
		if len(keys) > 64 || len(keys) > 1 && !m.chain {
			return intent, fmt.Errorf("server does not prove this complete authentication retirement chain; continue the original intent")
		}
		intent.Mode, intent.PreviousRequestIDs = "reauthenticate", keys
		intent.PreviousRequestID = keys[len(keys)-1]
	}
	if m.state.FamilyID != "" && mode != "register" {
		intent.Mode, intent.PreviousFamilyID = "reauthenticate", m.state.FamilyID
	}
	if intent.Mode == "reauthenticate" && intent.PreviousFamilyID == "" && len(intent.PreviousRequestIDs) == 0 {
		return intent, fmt.Errorf("no original family or unknown initial intent is recorded; use login")
	}
	m.state.Pending = append(m.state.Pending, intent)
	m.state.Status = "initial_pending"
	if err := m.persist(); err != nil {
		return intent, err
	}
	return intent, nil
}

func (m *authManager) authenticate(mode string, reader *bufio.Reader) error {
	intent, err := m.prepareInitial(mode)
	if err != nil {
		return err
	}
	var capability struct {
		DeploymentID string `json:"deploymentId"`
		RestoreEpoch string `json:"restoreEpoch"`
		Features     struct {
			Refresh bool `json:"auth_refresh"`
			Chain   bool `json:"auth_retirement_chain"`
		} `json:"features"`
	}
	if err = m.c.callWithToken("GET", "/api/bootstrap", nil, &capability, ""); err != nil {
		return err
	}
	if capability.DeploymentID != m.c.deployment || capability.RestoreEpoch != m.c.epoch || m.refresh && !capability.Features.Refresh || len(intent.PreviousRequestIDs) > 1 && !capability.Features.Chain {
		return fmt.Errorf("server no longer proves the original authentication context or complete retirement protocol; original intent retained")
	}
	password, err := secretInput(reader, "Password: ")
	if err != nil {
		return err
	}
	defer erase(password)
	payload := map[string]any{"username": intent.Username, "password": string(password), "requestId": intent.RequestID, "deviceName": intent.DeviceName}
	if intent.PreviousFamilyID != "" {
		payload["previousFamilyId"] = intent.PreviousFamilyID
	}
	if intent.PreviousRequestID != "" {
		payload["previousRequestId"] = intent.PreviousRequestID
	}
	if len(intent.PreviousRequestIDs) > 0 {
		payload["previousRequestIds"] = intent.PreviousRequestIDs
	}
	defer delete(payload, "password")
	var pair authPair
	err = m.c.callWithToken("POST", "/api/auth/"+intent.Mode, payload, &pair, "")
	var api *apiFailure
	if errors.As(err, &api) && api.Code == "MFA_REQUIRED" {
		otp, readErr := secretInput(reader, "Authenticator code: ")
		if readErr != nil {
			return readErr
		}
		payload["otp"] = string(otp)
		err = m.c.callWithToken("POST", "/api/auth/"+intent.Mode, payload, &pair, "")
		erase(otp)
		delete(payload, "otp")
	}
	if err != nil {
		if errors.As(err, &api) && api.Code == "DELIVERY_EXPIRED" {
			m.state.Pending[len(m.state.Pending)-1].DeliveryExpired = true
			if persistErr := m.persist(); persistErr != nil {
				return persistErr
			}
		}
		return fmt.Errorf("%w; original authentication intent retained; use its original proof or explicit reauthenticate", err)
	}
	return m.accept(pair, intent.RequestID)
}

func (m *authManager) snapshot() (authPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Status != "active" || m.pair.Token == "" {
		return authPair{}, fmt.Errorf("authentication is stopped; verify the account and retire the original unknown family")
	}
	return m.pair, nil
}

func (m *authManager) rotate(original authPair) (authPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Status != "active" || m.pair.FamilyID != original.FamilyID || m.pair.DeviceID != original.DeviceID || m.pair.User.ID != original.User.ID {
		return authPair{}, fmt.Errorf("original authentication context is no longer active")
	}
	if m.pair.RefreshGeneration != original.RefreshGeneration {
		return m.pair, nil
	}
	if !m.refresh || original.RefreshToken == "" {
		return authPair{}, fmt.Errorf("this server/session has no refresh protocol; authenticate again")
	}
	m.state.Status, m.state.RefreshRequestID = "refresh_unknown", id()
	if err := m.persist(); err != nil {
		return authPair{}, err
	}
	body := map[string]string{"requestId": m.state.RefreshRequestID, "refreshToken": original.RefreshToken, "familyId": original.FamilyID, "refreshGeneration": original.RefreshGeneration}
	var fresh authPair
	if err := m.c.callWithToken("POST", "/api/auth/refresh", body, &fresh, ""); err != nil {
		m.pair = authPair{}
		m.c.token = ""
		return authPair{}, fmt.Errorf("%w; refresh outcome is stopped, old refresh credential will not be reused", err)
	}
	next, ok := new(big.Int).SetString(original.RefreshGeneration, 10)
	if !ok {
		return authPair{}, fmt.Errorf("original refresh generation invalid")
	}
	next.Add(next, big.NewInt(1))
	if !m.validPair(fresh) || fresh.FamilyID != original.FamilyID || fresh.DeviceID != original.DeviceID || fresh.User.ID != original.User.ID || fresh.RefreshGeneration != next.String() || fresh.AbsoluteExpiresAt != original.AbsoluteExpiresAt {
		m.pair, m.c.token = authPair{}, ""
		return authPair{}, fmt.Errorf("refresh response changed the original identity or deadline; outcome remains unknown")
	}
	if err := m.accept(fresh, m.state.InitialRequestID); err != nil {
		m.pair, m.c.token = authPair{}, ""
		return authPair{}, err
	}
	return fresh, nil
}

func (m *authManager) logout() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Status == "signed_out" && m.pair.Token == "" {
		return nil
	}
	original := m.pair
	m.state.Status = "logout_pending"
	persistErr := m.persist()
	m.pair, m.c.token = authPair{}, ""
	var deleteErr error
	if m.vault != nil && !m.temporary {
		deleteErr = m.vault.Delete()
	}
	if original.Token == "" {
		if persistErr != nil {
			return fmt.Errorf("local credentials stopped; durable revocation record and server completion remain unconfirmed")
		}
		return fmt.Errorf("original family remains pending verified revocation")
	}
	err := m.c.callWithToken("POST", "/api/auth/logout", map[string]string{}, nil, original.Token)
	if err == nil {
		m.state.Status = "signed_out"
		if persistErr := m.persist(); persistErr != nil {
			return persistErr
		}
	}
	if deleteErr != nil {
		return fmt.Errorf("local session stopped; system credential deletion requires retry")
	}
	if persistErr != nil {
		return fmt.Errorf("runtime and system credentials cleared; durable revocation record could not be updated")
	}
	return err
}

func (c *client) call(method, path string, body any, out any) error {
	stamp, guardErr := c.syncHTTPGuard(method, path)
	if guardErr != nil {
		return guardErr
	}
	var observed json.RawMessage
	target := out
	observe := c.store != nil && syncProtectedPath(method, path) && !strings.HasSuffix(path, "/message-results")
	if observe {
		target = &observed
	}
	finish := func(e error) error {
		if e != nil {
			var api *apiFailure
			if c.store != nil && errors.As(e, &api) && (api.Status == 401 || api.Code == "RESTORE_EPOCH_CHANGED" || api.Code == "DEPLOYMENT_CHANGED") {
				retireErr := c.store.retireSyncIdentity("SYNC_REAUTH_REQUIRED")
				c.lookup = nil
				c.token = ""
				if retireErr != nil {
					return fmt.Errorf("%w; retiring local identity failed: %v", e, retireErr)
				}
			}
			return e
		}
		if observe {
			if e = c.observeMessageGlobals(method, path, body, observed); e != nil {
				return e
			}
			if out != nil && len(observed) != 0 {
				return syncDecode(observed, out)
			}
		}
		return nil
	}
	if c.auth == nil {
		err := c.callWithToken(method, path, body, target, c.token)
		if guardErr := c.syncHTTPAfter(stamp); guardErr != nil {
			return guardErr
		}
		return finish(err)
	}
	original, err := c.auth.snapshot()
	if err != nil {
		return err
	}
	var frozen any
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		frozen = json.RawMessage(raw)
	}
	err = c.callWithToken(method, path, frozen, target, original.Token)
	if guardErr := c.syncHTTPAfter(stamp); guardErr != nil {
		return guardErr
	}
	if !c.auth.owns(original) {
		return fmt.Errorf("late response belongs to a stopped authentication context")
	}
	var api *apiFailure
	if !errors.As(err, &api) || api.Status != 401 || api.Code != "ACCESS_EXPIRED" {
		return finish(err)
	}
	fresh, err := c.auth.rotate(original)
	if err != nil {
		return err
	}
	err = c.callWithToken(method, path, frozen, target, fresh.Token)
	if guardErr := c.syncHTTPAfter(stamp); guardErr != nil {
		return guardErr
	}
	if !c.auth.owns(fresh) {
		return fmt.Errorf("late retried response belongs to a stopped authentication context")
	}
	return finish(err)
}

func (m *authManager) owns(original authPair) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.Status == "active" && m.pair.FamilyID == original.FamilyID && m.pair.DeviceID == original.DeviceID && m.pair.User.ID == original.User.ID
}

// Body bytes are replayed only after an authenticated ACCESS_EXPIRED rejection.
// A transport failure remains unknown and never enters this replay path.
func (c *client) authenticatedResponse(method, path, contentType string, body []byte, timeout time.Duration) (*http.Response, error) {
	var token string
	var original authPair
	var err error
	if c.auth != nil {
		original, err = c.auth.snapshot()
		if err != nil {
			return nil, err
		}
		token = original.Token
	} else {
		token = c.token
	}
	request := func(token string) (*http.Response, error) {
		r, err := http.NewRequest(method, c.base+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		transport := *c.http
		transport.Timeout = timeout
		return transport.Do(r)
	}
	response, err := request(token)
	if err != nil || c.auth == nil {
		return response, err
	}
	if !c.auth.owns(original) {
		response.Body.Close()
		return nil, fmt.Errorf("late media response belongs to a stopped authentication context")
	}
	if response.StatusCode != 401 {
		return response, nil
	}
	raw, readErr := readLimited(response.Body, 1<<20)
	response.Body.Close()
	var rejection struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if readErr != nil || json.Unmarshal(raw, &rejection) != nil || rejection.Error.Code != "ACCESS_EXPIRED" {
		return nil, fmt.Errorf("media authorization rejected; original operation is not replayed")
	}
	fresh, err := c.auth.rotate(original)
	if err != nil {
		return nil, err
	}
	response, err = request(fresh.Token)
	if err == nil && !c.auth.owns(fresh) {
		response.Body.Close()
		return nil, fmt.Errorf("late media retry belongs to a stopped authentication context")
	}
	return response, err
}

func freezePreviousCLIStores(root, origin, profile, deployment, user, device, epoch string) error {
	for _, bucket := range []string{profile, fingerprint(origin)} {
		base := filepath.Join(root, "JobsIM", "CLI", bucket)
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(base, entry.Name(), "client.sqlite")
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("old CLI store is not a regular private file")
			}
			u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
			db, err := sql.Open("sqlite", u.String()+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
			if err != nil {
				return err
			}
			err = freezePreviousCLIStore(db, deployment, user, device, epoch)
			db.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func freezePreviousCLIStore(db *sql.DB, deployment, user, device, epoch string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw, previousEpoch string
	if tx.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&raw) != nil || tx.QueryRow("SELECT value FROM metadata WHERE key='epoch'").Scan(&previousEpoch) != nil {
		return fmt.Errorf("old CLI namespace cannot be proved; original data retained")
	}
	var identity localIdentity
	if json.Unmarshal([]byte(raw), &identity) != nil {
		return fmt.Errorf("old CLI namespace damaged; original retained")
	}
	if identity.DeploymentID != deployment || identity.UserID != user || identity.Space != "personal" || identity.DeviceID == device && previousEpoch == epoch {
		return nil
	}
	if _, err = tx.Exec("INSERT INTO metadata(key,value) VALUES('auth_retired','1') ON CONFLICT(key) DO UPDATE SET value='1'"); err != nil {
		return err
	}
	columns, err := tx.Query("PRAGMA table_info(outbox)")
	if err != nil {
		return err
	}
	hasReason := false
	for columns.Next() {
		var ordinal, required, primary int
		var name, kind string
		var defaultValue any
		if err = columns.Scan(&ordinal, &name, &kind, &required, &defaultValue, &primary); err != nil {
			columns.Close()
			return err
		}
		hasReason = hasReason || name == "freeze_reason"
	}
	columns.Close()
	update := "UPDATE outbox SET state='frozen',updated_at=? WHERE state='pending'"
	if hasReason {
		update = "UPDATE outbox SET state='frozen',freeze_reason='AUTH_CONTEXT_CHANGED',updated_at=? WHERE state='pending'"
	}
	if _, err = tx.Exec(update, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	for _, table := range []string{"governance_operations", "announcement_operations"} {
		var present int
		if err = tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&present); err != nil {
			return err
		}
		if present > 0 {
			if _, err = tx.Exec("UPDATE "+table+" SET state='frozen',error_code='AUTH_CONTEXT_CHANGED',updated_at=? WHERE state='pending'", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
