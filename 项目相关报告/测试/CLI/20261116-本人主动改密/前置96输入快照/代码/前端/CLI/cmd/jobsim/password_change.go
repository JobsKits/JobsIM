// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// Only public responsibility metadata is durable: no password pair, OTP or
// password-derived/request digest is stored, even when delivery is unknown.
type passwordChangeIntent struct {
	ID        string                `json:"requestId"`
	Username  string                `json:"username"`
	Binding   dataExportBinding     `json:"binding"`
	Namespace string                `json:"namespace"`
	Created   string                `json:"createdAt"`
	Attempts  int                   `json:"attempts"`
	State     string                `json:"state"`
	Result    *passwordChangeResult `json:"result,omitempty"`
}
type passwordChangeResult struct {
	ID         string `json:"requestId"`
	Status     string `json:"status"`
	Epoch      string `json:"restoreEpoch"`
	Credential string `json:"credentialGeneration"`
	Security   string `json:"securityVersion"`
	Current    *bool  `json:"current"`
	Login      *bool  `json:"requiresNewLogin"`
}
type passwordChangeProof struct{ Current, New, OTP []byte }

func (p *passwordChangeProof) clear() { erase(p.Current); erase(p.New); erase(p.OTP) }
func passwordChangeMigrate(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS password_change_intents(id TEXT PRIMARY KEY,payload TEXT NOT NULL)")
	return err
}
func (m *authManager) passwordChangeRestoreGate() error {
	if m.db == nil || m.state.FamilyID == "" {
		return nil
	}
	var table int
	if err := m.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='password_change_intents'").Scan(&table); err != nil {
		return err
	}
	if table == 0 {
		return nil
	}
	var pending int
	if err := m.db.QueryRow("SELECT count(*) FROM password_change_intents WHERE json_extract(payload,'$.binding.familyId')=? AND json_extract(payload,'$.attempts')>0 AND json_extract(payload,'$.state') IN('unknown','pending')", m.state.FamilyID).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 && m.state.Status == "active" {
		m.state.Status = "password_change_unknown"
		return m.persist()
	}
	return nil
}
func validatePasswordChangeIntent(o passwordChangeIntent, origin, scope, profile, username string) error {
	b := o.Binding
	if !syncUUIDPattern.MatchString(o.ID) || o.Username != username || b.Identity.Origin != origin || b.Identity.DeploymentID != scope || b.Identity.ConnectionProfile != profile || b.Identity.Space != "personal" || !syncUUIDPattern.MatchString(b.Identity.UserID) || !syncUUIDPattern.MatchString(b.Identity.DeviceID) || !syncUUIDPattern.MatchString(b.Family) || !syncUUIDPattern.MatchString(b.Physical) || o.Attempts < 0 || !oneOf(o.State, "unsent", "unknown", "pending", "changed", "retired", "rejected") {
		return syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	if n, e := syncCounter(b.Epoch); e != nil || n == 0 {
		return syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	if _, e := time.Parse(time.RFC3339Nano, o.Created); e != nil {
		return syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	raw, _ := json.Marshal(b.Identity)
	if o.Namespace != filepath.ToSlash(filepath.Join(fingerprint(string(raw)), "client.sqlite")) {
		return syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	if o.Result != nil {
		return validatePasswordChangeResult(*o.Result, o)
	}
	return nil
}
func validatePasswordChangeResult(v passwordChangeResult, o passwordChangeIntent) error {
	if v.ID != o.ID || v.Epoch != o.Binding.Epoch || !oneOf(v.Status, "pending", "changed", "retired") || v.Current == nil || v.Login == nil || v.Status == "changed" && !*v.Login || *v.Current && v.Status != "changed" {
		return syncRefuse("PASSWORD_CHANGE_RESULT_INVALID")
	}
	for _, counter := range []string{v.Credential, v.Security} {
		if n, e := syncCounter(counter); e != nil || n == 0 {
			return syncRefuse("PASSWORD_CHANGE_RESULT_INVALID")
		}
	}
	return nil
}
func loadPasswordChange(db *sql.DB, id, origin, scope, profile, username string) (passwordChangeIntent, error) {
	var o passwordChangeIntent
	var raw string
	if err := db.QueryRow("SELECT payload FROM password_change_intents WHERE id=?", id).Scan(&raw); err != nil {
		return o, err
	}
	if syncDecode([]byte(raw), &o) != nil || o.ID != id || validatePasswordChangeIntent(o, origin, scope, profile, username) != nil {
		return o, syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	return o, nil
}
func samePasswordChange(a, b passwordChangeIntent) bool {
	a.State, b.State = "", ""
	a.Attempts, b.Attempts = 0, 0
	a.Result, b.Result = nil, nil
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
func savePasswordChange(db *sql.DB, o passwordChangeIntent, change func(*passwordChangeIntent) error) (passwordChangeIntent, error) {
	tx, err := db.Begin()
	if err != nil {
		return o, err
	}
	defer tx.Rollback()
	if err = receiptDurability(tx); err != nil {
		return o, err
	}
	var raw string
	var current passwordChangeIntent
	if err = tx.QueryRow("SELECT payload FROM password_change_intents WHERE id=?", o.ID).Scan(&raw); err != nil {
		return o, err
	}
	if syncDecode([]byte(raw), &current) != nil || !samePasswordChange(current, o) {
		return o, syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	if err = change(&current); err != nil {
		return o, err
	}
	if err = validatePasswordChangeIntent(current, o.Binding.Identity.Origin, o.Binding.Identity.DeploymentID, o.Binding.Identity.ConnectionProfile, o.Username); err != nil {
		return o, err
	}
	encoded, _ := json.Marshal(current)
	if _, err = tx.Exec("UPDATE password_change_intents SET payload=? WHERE id=? AND payload=?", string(encoded), o.ID, raw); err != nil {
		return o, err
	}
	return current, tx.Commit()
}
func (c *client) passwordChangeCapability(o passwordChangeIntent, required bool) (bool, error) {
	var boot struct {
		Scope    string          `json:"deploymentId"`
		Epoch    string          `json:"restoreEpoch"`
		Features map[string]bool `json:"features"`
	}
	if err := c.callWithToken("GET", "/api/bootstrap", nil, &boot, ""); err != nil {
		return false, err
	}
	if c.base != o.Binding.Identity.Origin || c.deployment != o.Binding.Identity.DeploymentID || boot.Scope != o.Binding.Identity.DeploymentID {
		return false, syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	available := boot.Features["own_password_change"] && boot.Epoch == o.Binding.Epoch
	if required && !available {
		return false, syncRefuse("PASSWORD_CHANGE_CAPABILITY_REQUIRED")
	}
	return available, nil
}
func (c *client) passwordChangeHTTP(o passwordChangeIntent, proof *passwordChangeProof, route, token string) (passwordChangeResult, error) {
	var result passwordChangeResult
	if c.base != o.Binding.Identity.Origin || c.deployment != o.Binding.Identity.DeploymentID || !oneOf(route, "/api/auth/password-change", "/api/auth/password-change-results") {
		return result, syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
	}
	body := map[string]string{"requestId": o.ID, "username": o.Username, "deviceId": o.Binding.Identity.DeviceID, "restoreEpoch": o.Binding.Epoch, "currentPassword": string(proof.Current), "newPassword": string(proof.New)}
	if route == "/api/auth/password-change" && len(proof.OTP) > 0 {
		body["otp"] = string(proof.OTP)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return result, err
	}
	defer erase(raw)
	request, err := http.NewRequest("POST", c.base+route, bytes.NewReader(raw))
	if err != nil {
		return result, err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	transport := *c.http
	transport.Timeout = 20 * time.Second
	transport.Jar = nil
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := transport.Do(request)
	if err != nil {
		return result, syncRefuse("PASSWORD_CHANGE_RESULT_UNKNOWN")
	}
	defer response.Body.Close()
	data, err := readLimited(response.Body, 32<<10)
	if err != nil {
		return result, syncRefuse("PASSWORD_CHANGE_RESULT_UNKNOWN")
	}
	defer erase(data)
	if response.StatusCode != 200 {
		var rejection struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if syncDecode(data, &rejection) != nil || !dataExportFailure.MatchString(rejection.Error.Code) {
			return result, syncRefuse("PASSWORD_CHANGE_RESULT_UNKNOWN")
		}
		return result, &apiFailure{Status: response.StatusCode, Code: rejection.Error.Code, Message: "password-change original responsibility retained"}
	}
	if syncDecode(data, &result) != nil || validatePasswordChangeResult(result, o) != nil {
		return result, syncRefuse("PASSWORD_CHANGE_RESULT_UNKNOWN")
	}
	return result, nil
}
func (c *client) passwordChangeOriginalToken(o passwordChangeIntent) string {
	if c.auth == nil || c.store == nil {
		return ""
	}
	m := c.auth
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pair
	b := o.Binding
	if !oneOf(m.state.Status, "active", "password_change_unknown") || p.FamilyID != b.Family || p.DeviceID != b.Identity.DeviceID || p.User.ID != b.Identity.UserID || p.RestoreEpoch != b.Epoch || p.SourceContextID != "personal" || m.profile != b.Identity.ConnectionProfile || m.username != o.Username || filepath.ToSlash(m.state.MessageStore) != o.Namespace || filepath.ToSlash(p.MessageStore) != o.Namespace || p.StoreGeneration != b.Identity.StoreGeneration || m.state.StoreGeneration != b.Identity.StoreGeneration || !c.syncCurrent(c.store, nil) {
		return ""
	}
	current := func() bool { return c.syncCurrent(c.store, nil) }
	if c.store.dataExportControl(c.store.db, b, current) != nil {
		return ""
	}
	return p.Token
}
func (c *client) passwordChangePause(o passwordChangeIntent) error {
	if c.auth == nil {
		return nil
	}
	m := c.auth
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pair.FamilyID != o.Binding.Family || m.pair.DeviceID != o.Binding.Identity.DeviceID || m.profile != o.Binding.Identity.ConnectionProfile {
		return nil
	}
	m.state.Status = "password_change_unknown"
	return m.persist()
}
func (c *client) passwordChangeStopOriginal(o passwordChangeIntent) error {
	if c.auth == nil {
		return nil
	}
	m := c.auth
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pair.FamilyID != o.Binding.Family || m.state.FamilyID != o.Binding.Family || m.pair.DeviceID != o.Binding.Identity.DeviceID || m.pair.User.ID != o.Binding.Identity.UserID || m.profile != o.Binding.Identity.ConnectionProfile {
		return nil
	}
	m.state.Status = "password_changed"
	err := m.persist()
	m.pair = authPair{}
	c.token = ""
	if m.vault != nil && !m.temporary {
		stored, e := m.vault.Read()
		if e == nil {
			var pair authPair
			if json.Unmarshal(stored, &pair) == nil && pair.FamilyID == o.Binding.Family {
				e = m.vault.Delete()
			}
			erase(stored)
		}
		if e != nil && !errors.Is(e, errVaultNotFound) {
			err = errors.Join(err, e)
		}
	}
	if c.store != nil && c.store.identity == o.Binding.Identity && c.store.epoch == o.Binding.Epoch {
		var physical string
		if e := c.store.db.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); e == nil && physical == o.Binding.Physical {
			err = errors.Join(err, c.store.retireSyncIdentity("PASSWORD_CHANGE_REQUIRES_LOGIN"))
		}
	}
	return err
}
func (c *client) applyPasswordChangeResult(db *sql.DB, o passwordChangeIntent, v passwordChangeResult) (passwordChangeResult, error) {
	_, err := savePasswordChange(db, o, func(row *passwordChangeIntent) error { row.State = v.Status; row.Result = &v; return nil })
	if v.Login != nil && *v.Login {
		err = errors.Join(err, c.passwordChangeStopOriginal(o))
	}
	return v, err
}
func (c *client) performPasswordChange(db *sql.DB, o passwordChangeIntent, proof *passwordChangeProof, retry, fresh bool) (passwordChangeResult, error) {
	var empty passwordChangeResult
	available, err := c.passwordChangeCapability(o, fresh)
	if err != nil {
		return empty, err
	}
	result, err := c.passwordChangeHTTP(o, proof, "/api/auth/password-change-results", "")
	unseen := false
	if err == nil {
		if result.Status != "pending" || !retry {
			return c.applyPasswordChangeResult(db, o, result)
		}
		if _, err = c.applyPasswordChangeResult(db, o, result); err != nil {
			return empty, err
		}
	} else {
		var api *apiFailure
		if !errors.As(err, &api) || api.Status != 404 || api.Code != "PASSWORD_CHANGE_OPERATION_NOT_FOUND" || (!retry && !fresh) {
			return empty, err
		}
		unseen = true
	}
	token := ""
	if available && unseen {
		token = c.passwordChangeOriginalToken(o)
	}
	if fresh && token == "" {
		return empty, syncRefuse("PASSWORD_CHANGE_ORIGINAL_FAMILY_REQUIRED")
	}
	if token != "" && len(proof.OTP) == 0 && c.reader != nil {
		proof.OTP, err = secretInput(c.reader, "Authenticator code (optional; Enter if not required): ")
		if err != nil {
			return empty, err
		}
	}
	priorAttempts := o.Attempts
	o, err = savePasswordChange(db, o, func(row *passwordChangeIntent) error { row.Attempts++; row.State = "unknown"; return nil })
	if err != nil {
		return empty, err
	}
	if err = c.passwordChangePause(o); err != nil {
		return empty, err
	}
	result, err = c.passwordChangeHTTP(o, proof, "/api/auth/password-change", token)
	if err != nil {
		var api *apiFailure
		if fresh && priorAttempts == 0 && errors.As(err, &api) && oneOf(fmt.Sprint(api.Status), "400", "401", "403", "429") {
			_, _ = savePasswordChange(db, o, func(row *passwordChangeIntent) error { row.State = "rejected"; return nil })
			if c.auth != nil {
				c.auth.mu.Lock()
				if c.auth.pair.FamilyID == o.Binding.Family && c.auth.state.Status == "password_change_unknown" {
					c.auth.state.Status = "active"
					_ = c.auth.persist()
				}
				c.auth.mu.Unlock()
			}
		}
		return empty, err
	}
	return c.applyPasswordChangeResult(db, o, result)
}
func readPasswordChangeProof(reader *bufio.Reader) (passwordChangeProof, error) {
	var proof passwordChangeProof
	var err error
	proof.Current, err = secretInput(reader, "Original current password: ")
	if err != nil {
		return proof, err
	}
	proof.New, err = secretInput(reader, "Original new password: ")
	if err != nil {
		proof.clear()
		return proof, err
	}
	confirm, err := secretInput(reader, "Confirm original new password: ")
	defer erase(confirm)
	if err != nil || !bytes.Equal(confirm, proof.New) || !utf8.Valid(proof.Current) || !utf8.Valid(proof.New) || len(proof.Current) == 0 || len(proof.Current) > 512 || len(proof.New) == 0 || len(proof.New) > 512 {
		proof.clear()
		return proof, syncRefuse("PASSWORD_CHANGE_PROOF_INPUT_INVALID")
	}
	return proof, nil
}
func (c *client) preparePasswordChange() (passwordChangeIntent, error) {
	var o passwordChangeIntent
	s, b, current, err := c.dataExportContext()
	if err != nil {
		return o, err
	}
	m := c.auth
	m.mu.Lock()
	o.Username = m.username
	o.Namespace = filepath.ToSlash(m.state.MessageStore)
	m.mu.Unlock()
	o.ID, err = syncNewUUID()
	if err != nil {
		return o, err
	}
	o.Binding = b
	o.Created = time.Now().UTC().Format(time.RFC3339Nano)
	o.State = "unsent"
	if err = validatePasswordChangeIntent(o, c.base, c.deployment, m.profile, m.username); err != nil {
		return o, err
	}
	if _, err = c.passwordChangeCapability(o, true); err != nil {
		return o, err
	}
	if err = passwordChangeMigrate(m.db); err != nil {
		return o, err
	}
	tx, err := m.db.Begin()
	if err != nil {
		return o, err
	}
	defer tx.Rollback()
	if err = receiptDurability(tx); err != nil {
		return o, err
	}
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM password_change_intents WHERE json_extract(payload,'$.binding.familyId')=? AND json_extract(payload,'$.state') IN('unsent','unknown','pending')", b.Family).Scan(&count); err != nil {
		return o, err
	}
	if count != 0 {
		return o, syncRefuse("PASSWORD_CHANGE_ORIGINAL_PENDING")
	}
	if err = s.dataExportControl(s.db, b, current); err != nil {
		return o, err
	}
	raw, _ := json.Marshal(o)
	if _, err = tx.Exec("INSERT INTO password_change_intents VALUES(?,?)", o.ID, string(raw)); err != nil {
		return o, err
	}
	if c.passwordChangeOriginalToken(o) == "" {
		return o, syncRefuse("PASSWORD_CHANGE_ORIGINAL_FAMILY_REQUIRED")
	}
	return o, tx.Commit()
}
func listPasswordChanges(db *sql.DB, origin, scope, profile, username string) ([]map[string]any, error) {
	rows, err := db.Query("SELECT payload FROM password_change_intents ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var o passwordChangeIntent
		if syncDecode([]byte(raw), &o) != nil || validatePasswordChangeIntent(o, origin, scope, profile, username) != nil {
			return nil, syncRefuse("PASSWORD_CHANGE_LOCAL_CONTEXT_CHANGED")
		}
		items = append(items, map[string]any{"requestId": o.ID, "username": o.Username, "deviceId": o.Binding.Identity.DeviceID, "restoreEpoch": o.Binding.Epoch, "state": o.State, "attempts": o.Attempts, "lastResult": o.Result})
	}
	return items, rows.Err()
}
func (c *client) passwordChangeCommand(cmd, args string) (bool, error) {
	if !oneOf(cmd, "password-change", "password-changes", "password-change-reconcile", "password-change-retry") {
		return false, nil
	}
	if c.auth == nil || c.reader == nil || !term.IsTerminal(int(os.Stdin.Fd())) {
		return true, syncRefuse("PASSWORD_CHANGE_INTERACTIVE_TTY_REQUIRED")
	}
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	m := c.auth
	if err := passwordChangeMigrate(m.db); err != nil {
		return true, err
	}
	if cmd == "password-changes" {
		items, err := listPasswordChanges(m.db, c.base, c.deployment, m.profile, m.username)
		if err != nil {
			return true, err
		}
		return true, printJSON(map[string]any{"items": items})
	}
	var o passwordChangeIntent
	var err error
	if cmd == "password-change" {
		if strings.TrimSpace(args) != "" {
			return true, fmt.Errorf("password-change takes no password arguments")
		}
		o, err = c.preparePasswordChange()
	} else {
		o, err = loadPasswordChange(m.db, strings.TrimSpace(args), c.base, c.deployment, m.profile, m.username)
	}
	if err != nil {
		return true, err
	}
	fmt.Println("Original password-change request:", o.ID, "; re-enter the exact original pair; passwords are never saved.")
	proof, err := readPasswordChangeProof(c.reader)
	if err != nil {
		return true, err
	}
	defer proof.clear()
	result, err := c.performPasswordChange(m.db, o, &proof, cmd == "password-change-retry", cmd == "password-change")
	if err != nil {
		return true, fmt.Errorf("original password-change %s retained: %w", o.ID, err)
	}
	return true, printJSON(result)
}
func openPasswordChangeMetadata(root, profile, username string) (*sql.DB, func(), error) {
	directory := filepath.Join(root, "JobsIM", "CLI", "Authentication", profile, fingerprint(username))
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, syncRefuse("PASSWORD_CHANGE_UNSAFE_METADATA")
	}
	release, err := lockAuthDirectory(directory)
	if err != nil {
		return nil, nil, err
	}
	failed := true
	defer func() {
		if failed {
			release()
		}
	}()
	path := filepath.Join(directory, "authentication.sqlite")
	info, err = os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, syncRefuse("PASSWORD_CHANGE_UNSAFE_METADATA")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	q.Set("mode", "rw")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	if err = receiptDurability(db); err != nil {
		db.Close()
		return nil, nil, err
	}
	var count int
	if db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='password_change_intents'").Scan(&count) != nil || count != 1 {
		db.Close()
		return nil, nil, syncRefuse("PASSWORD_CHANGE_ORIGINAL_NOT_FOUND")
	}
	failed = false
	return db, func() { db.Close(); release() }, nil
}
func (c *client) runPasswordChangeResume(dataDirectory, ca string, args []string, reader *bufio.Reader) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return syncRefuse("PASSWORD_CHANGE_INTERACTIVE_TTY_REQUIRED")
	}
	if len(args) != 2 && !(len(args) == 1 && args[0] == "password-changes") {
		return fmt.Errorf("password-change-reconcile|password-change-retry ORIGINAL_REQUEST_ID; password-changes")
	}
	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return syncRefuse("USERNAME_REQUIRED")
	}
	root, err := os.UserConfigDir()
	if dataDirectory != "" {
		root, err = filepath.Abs(dataDirectory)
	}
	if err != nil {
		return err
	}
	trust := "system"
	if ca != "" {
		raw, e := os.ReadFile(ca)
		if e != nil {
			return e
		}
		trust = fingerprint(string(raw))
	}
	profile := fingerprint(c.base + ":" + trust + ":" + c.deployment)
	db, close, err := openPasswordChangeMetadata(root, profile, username)
	if err != nil {
		return err
	}
	defer close()
	if args[0] == "password-changes" {
		items, e := listPasswordChanges(db, c.base, c.deployment, profile, username)
		if e != nil {
			return e
		}
		return printJSON(map[string]any{"items": items})
	}
	o, err := loadPasswordChange(db, args[1], c.base, c.deployment, profile, username)
	if err != nil {
		return err
	}
	fmt.Println("Original password-change request:", o.ID, "; anonymous original-result lookup does not log in or replace a family.")
	proof, err := readPasswordChangeProof(reader)
	if err != nil {
		return err
	}
	defer proof.clear()
	c.reader = reader
	// Without an already captured current family this path is anonymous. It may
	// finish prepared responsibility, but cannot authorize a never-seen change.
	result, err := c.performPasswordChange(db, o, &proof, args[0] == "password-change-retry", false)
	if err != nil {
		return fmt.Errorf("original password-change %s retained: %w", o.ID, err)
	}
	return printJSON(result)
}
