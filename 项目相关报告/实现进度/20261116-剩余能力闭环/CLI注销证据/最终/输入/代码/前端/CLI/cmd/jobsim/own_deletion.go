// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"
)

type ownDeletionResult struct {
	ID      string  `json:"deletionId"`
	Request string  `json:"requestId"`
	State   string  `json:"status"`
	Version string  `json:"requestVersion"`
	Epoch   string  `json:"restoreEpoch"`
	Until   *string `json:"cancelUntil"`
	Cancel  string  `json:"cancelRequestId"`
	Login   *bool   `json:"requiresNewLogin"`
}
type ownDeletionIntent struct {
	Meta          passwordChangeIntent `json:"original"`
	Attempts      int                  `json:"attempts"`
	State         string               `json:"state"`
	Result        *ownDeletionResult   `json:"result,omitempty"`
	Cancel        string               `json:"cancelRequestId,omitempty"`
	CancelVersion string               `json:"cancelVersion,omitempty"`
	CancelState   string               `json:"cancelState,omitempty"`
	Cleared       bool                 `json:"localCleared"`
}
type ownDeletionSecret struct{ Password, Recovery, OTP []byte }

func (s ownDeletionSecret) String() string { return "ownDeletionSecret(redacted)" }
func (p ownDeletionProof) String() string  { return "ownDeletionProof(redacted)" }
func (s *ownDeletionSecret) clear()        { erase(s.Password); erase(s.Recovery); erase(s.OTP) }

type ownDeletionProof struct {
	Token   string `json:"proof"`
	ID      string `json:"deletionId"`
	Version string `json:"requestVersion"`
	Purpose string `json:"purpose"`
	Expires string `json:"expiresAt"`
	Epoch   string `json:"restoreEpoch"`
}

func ownDeletionDecode(raw []byte, value any) error {
	var checked any
	if e := syncDecode(raw, &checked); e != nil {
		return e
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(value); e != nil {
		return syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
	}
	return nil
}
func ownDeletionMigrate(db *sql.DB) error {
	_, e := db.Exec("CREATE TABLE IF NOT EXISTS own_deletion_intents(id TEXT PRIMARY KEY,payload TEXT NOT NULL)")
	return e
}
func validateOwnDeletion(o ownDeletionIntent, origin, scope, profile, username string) error {
	if validatePasswordChangeIntent(o.Meta, origin, scope, profile, username) != nil || o.Meta.State != "unsent" || o.Meta.Attempts != 0 || o.Meta.Result != nil || o.Attempts < 0 || !oneOf(o.State, "unsent", "unknown", "pending", "requested", "cancel_pending", "processing", "completed", "cancelled", "retired", "rejected") || o.Cleared && o.State != "completed" {
		return syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
	}
	if o.Cancel != "" {
		if !syncUUIDPattern.MatchString(o.Cancel) || !oneOf(o.CancelState, "prepared", "unknown", "confirmed") {
			return syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
		}
		if n, e := syncCounter(o.CancelVersion); e != nil || n == 0 {
			return syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
		}
	} else if o.CancelVersion != "" || o.CancelState != "" {
		return syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
	}
	if o.Result != nil {
		if o.State != o.Result.State && o.State != "unknown" {
			return syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
		}
		return validateOwnDeletionResult(*o.Result, o)
	}
	return nil
}
func validateOwnDeletionResult(v ownDeletionResult, o ownDeletionIntent) error {
	if !syncUUIDPattern.MatchString(v.ID) || v.Request != o.Meta.ID || v.Epoch != o.Meta.Binding.Epoch || !oneOf(v.State, "pending", "requested", "cancel_pending", "processing", "completed", "cancelled", "retired") || v.Login == nil || !*v.Login {
		return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
	}
	n, e := syncCounter(v.Version)
	if e != nil || n == 0 {
		return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
	}
	if o.Result != nil {
		old, _ := syncCounter(o.Result.Version)
		if v.ID != o.Result.ID || n < old {
			return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
		}
	}
	if v.Cancel != "" && (!syncUUIDPattern.MatchString(v.Cancel) || o.Cancel != "" && v.Cancel != o.Cancel) {
		return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
	}
	if v.Until != nil {
		if _, e = time.Parse(time.RFC3339Nano, *v.Until); e != nil {
			return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
		}
	}
	return nil
}
func loadOwnDeletion(db *sql.DB, id, origin, scope, profile, username string) (ownDeletionIntent, error) {
	var o ownDeletionIntent
	var raw string
	e := db.QueryRow("SELECT payload FROM own_deletion_intents WHERE id=?", id).Scan(&raw)
	if e != nil {
		return o, e
	}
	if ownDeletionDecode([]byte(raw), &o) != nil || o.Meta.ID != id || validateOwnDeletion(o, origin, scope, profile, username) != nil {
		return o, syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
	}
	return o, nil
}
func saveOwnDeletion(db *sql.DB, before, after ownDeletionIntent) (ownDeletionIntent, error) {
	a, _ := json.Marshal(before.Meta)
	b, _ := json.Marshal(after.Meta)
	if !bytes.Equal(a, b) || before.Cancel != "" && (before.Cancel != after.Cancel || before.CancelVersion != after.CancelVersion) || after.Attempts < before.Attempts || before.Cleared && !after.Cleared {
		return before, syncRefuse("ACCOUNT_DELETION_ORIGINAL_CHANGED")
	}
	if e := validateOwnDeletion(after, before.Meta.Binding.Identity.Origin, before.Meta.Binding.Identity.DeploymentID, before.Meta.Binding.Identity.ConnectionProfile, before.Meta.Username); e != nil {
		return before, e
	}
	if after.Result != nil {
		if e := validateOwnDeletionResult(*after.Result, before); e != nil {
			return before, e
		}
	}
	old, _ := json.Marshal(before)
	raw, _ := json.Marshal(after)
	tx, e := db.Begin()
	if e != nil {
		return before, e
	}
	defer tx.Rollback()
	if e = receiptDurability(tx); e != nil {
		return before, e
	}
	r, e := tx.Exec("UPDATE own_deletion_intents SET payload=? WHERE id=? AND payload=?", string(raw), before.Meta.ID, string(old))
	if e != nil {
		return before, e
	}
	count, e := r.RowsAffected()
	if e != nil || count != 1 {
		return before, syncRefuse("ACCOUNT_DELETION_ORIGINAL_CHANGED")
	}
	return after, tx.Commit()
}
func (c *client) ownDeletionHTTP(route string, body any, token string, out any) error {
	if !oneOf(route, "/api/auth/deletion-requests", "/api/auth/deletion-results", "/api/auth/data-rights/proofs", "/api/auth/data-rights/query", "/api/auth/data-rights/cancel") {
		return syncRefuse("ACCOUNT_DELETION_ROUTE_INVALID")
	}
	raw, e := json.Marshal(body)
	if e != nil {
		return e
	}
	defer erase(raw)
	q, e := http.NewRequest("POST", c.base+route, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	q.Header.Set("Content-Type", "application/json")
	if token != "" {
		q.Header.Set("Authorization", "Bearer "+token)
	}
	h := *c.http
	h.Jar = nil
	h.Timeout = 20 * time.Second
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r, e := h.Do(q)
	if e != nil {
		return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
	}
	defer r.Body.Close()
	data, e := readLimited(r.Body, 32<<10)
	if e != nil {
		return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
	}
	defer erase(data)
	if r.StatusCode != 200 {
		var rejection struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if syncDecode(data, &rejection) != nil || !dataExportFailure.MatchString(rejection.Error.Code) {
			return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
		}
		return &apiFailure{Status: r.StatusCode, Code: rejection.Error.Code, Message: "original account-deletion responsibility retained"}
	}
	if ownDeletionDecode(data, out) != nil {
		return syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
	}
	return nil
}
func (c *client) ownDeletionCapability(o ownDeletionIntent, required bool) (bool, error) {
	var b struct {
		Scope    string          `json:"deploymentId"`
		Epoch    string          `json:"restoreEpoch"`
		Features map[string]bool `json:"features"`
	}
	if e := c.callWithToken("GET", "/api/bootstrap", nil, &b, ""); e != nil {
		return false, e
	}
	if c.base != o.Meta.Binding.Identity.Origin || c.deployment != o.Meta.Binding.Identity.DeploymentID || b.Scope != c.deployment {
		return false, syncRefuse("ACCOUNT_DELETION_CONTEXT_CHANGED")
	}
	ok := b.Features["own_account_deletion"] && b.Epoch == o.Meta.Binding.Epoch
	if required && !ok {
		return false, syncRefuse("ACCOUNT_DELETION_CAPABILITY_REQUIRED")
	}
	return ok, nil
}
func (c *client) ownDeletionOriginalToken(o ownDeletionIntent) string {
	if c.auth == nil || c.store == nil {
		return ""
	}
	m := c.auth
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pair
	b := o.Meta.Binding
	if !oneOf(m.state.Status, "active", "account_deletion_unknown") || p.FamilyID != b.Family || m.state.FamilyID != b.Family || p.DeviceID != b.Identity.DeviceID || m.state.DeviceID != b.Identity.DeviceID || p.User.ID != b.Identity.UserID || m.state.UserID != b.Identity.UserID || p.User.Username != o.Meta.Username || p.RestoreEpoch != b.Epoch || p.SourceContextID != "personal" || m.profile != b.Identity.ConnectionProfile || m.username != o.Meta.Username || filepath.ToSlash(p.MessageStore) != o.Meta.Namespace || filepath.ToSlash(m.state.MessageStore) != o.Meta.Namespace || p.StoreGeneration != b.Identity.StoreGeneration || m.state.StoreGeneration != b.Identity.StoreGeneration || c.token != p.Token || !c.syncCurrent(c.store, nil) {
		return ""
	}
	if c.store.dataExportControl(c.store.db, b, func() bool { return c.syncCurrent(c.store, nil) }) != nil {
		return ""
	}
	return p.Token
}
func (c *client) pauseOwnDeletion(o ownDeletionIntent) error {
	if c.auth == nil {
		return nil
	}
	m := c.auth
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pair.FamilyID == o.Meta.Binding.Family && m.pair.DeviceID == o.Meta.Binding.Identity.DeviceID && m.profile == o.Meta.Binding.Identity.ConnectionProfile {
		m.state.Status = "account_deletion_unknown"
		return m.persist()
	}
	return nil
}
func (c *client) stopOwnDeletion(o ownDeletionIntent) error {
	if c.auth == nil {
		return nil
	}
	m := c.auth
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pair.FamilyID != o.Meta.Binding.Family || m.pair.DeviceID != o.Meta.Binding.Identity.DeviceID || m.pair.User.ID != o.Meta.Binding.Identity.UserID || m.profile != o.Meta.Binding.Identity.ConnectionProfile {
		return nil
	}
	m.state.Status = "account_deletion_stopped"
	e := m.persist()
	m.pair = authPair{}
	c.token = ""
	if m.vault != nil && !m.temporary {
		raw, read := m.vault.Read()
		if read == nil {
			var pair authPair
			if json.Unmarshal(raw, &pair) == nil && pair.FamilyID == o.Meta.Binding.Family {
				e = errors.Join(e, m.vault.Delete())
			}
			erase(raw)
		} else if !errors.Is(read, errVaultNotFound) {
			e = errors.Join(e, read)
		}
	}
	return e
}
func (c *client) applyOwnDeletion(db *sql.DB, o ownDeletionIntent, v ownDeletionResult) (ownDeletionResult, error) {
	if e := validateOwnDeletionResult(v, o); e != nil {
		return v, e
	}
	next := o
	next.Result = &v
	next.State = v.State
	if o.Cancel != "" && v.Cancel == o.Cancel && v.State == "cancelled" {
		next.CancelState = "confirmed"
	}
	saved, e := saveOwnDeletion(db, o, next)
	if e != nil {
		return v, e
	}
	e = c.stopOwnDeletion(o)
	if v.State == "completed" && !saved.Cleared && c.auth != nil {
		if clean := c.clearOwnDeletionLocal(c.auth, saved); clean != nil {
			return v, errors.Join(e, clean)
		}
		done := saved
		done.Cleared = true
		_, clean := saveOwnDeletion(db, saved, done)
		e = errors.Join(e, clean)
	}
	return v, e
}
func (c *client) performOwnDeletion(db *sql.DB, o ownDeletionIntent, s *ownDeletionSecret, retry, fresh bool) (ownDeletionResult, error) {
	var empty ownDeletionResult
	available, e := c.ownDeletionCapability(o, fresh)
	if e != nil {
		return empty, e
	}
	body := map[string]string{"requestId": o.Meta.ID, "username": o.Meta.Username, "deviceId": o.Meta.Binding.Identity.DeviceID, "restoreEpoch": o.Meta.Binding.Epoch, "password": string(s.Password)}
	var v ownDeletionResult
	e = c.ownDeletionHTTP("/api/auth/deletion-results", body, "", &v)
	unseen := false
	if e == nil {
		if v.State != "pending" || !retry {
			return c.applyOwnDeletion(db, o, v)
		}
		if _, e = c.applyOwnDeletion(db, o, v); e != nil {
			return empty, e
		}
		o, e = loadOwnDeletion(db, o.Meta.ID, c.base, c.deployment, o.Meta.Binding.Identity.ConnectionProfile, o.Meta.Username)
		if e != nil {
			return empty, e
		}
	} else {
		var api *apiFailure
		if !errors.As(e, &api) || api.Status != 404 || api.Code != "ACCOUNT_DELETION_NOT_FOUND" || !retry && !fresh {
			return empty, e
		}
		unseen = true
	}
	if !available {
		return empty, syncRefuse("ACCOUNT_DELETION_CAPABILITY_REQUIRED")
	}
	token := ""
	if unseen {
		token = c.ownDeletionOriginalToken(o)
		if token == "" {
			return empty, syncRefuse("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED")
		}
	}
	if _, e = c.ownDeletionCapability(o, true); e != nil {
		return empty, e
	}
	if token != "" && c.ownDeletionOriginalToken(o) != token {
		return empty, syncRefuse("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED")
	}
	before := o
	o.Attempts++
	o.State = "unknown"
	o, e = saveOwnDeletion(db, before, o)
	if e != nil {
		return empty, e
	}
	if e = c.pauseOwnDeletion(o); e != nil {
		return empty, e
	}
	if token != "" && c.ownDeletionOriginalToken(o) != token {
		return empty, syncRefuse("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED")
	}
	if len(s.OTP) > 0 {
		body["otp"] = string(s.OTP)
	}
	e = c.ownDeletionHTTP("/api/auth/deletion-requests", body, token, &v)
	if e != nil {
		var api *apiFailure
		if fresh && before.Attempts == 0 && errors.As(e, &api) && (api.Status == 400 && api.Code == "INVALID_ACCOUNT_DELETION" || api.Status == 403 && api.Code == "DATA_RIGHTS_REJECTED" || api.Status == 429 && api.Code == "RATE_LIMITED") {
			rejected := o
			rejected.State = "rejected"
			if _, save := saveOwnDeletion(db, o, rejected); save == nil && c.auth != nil {
				m := c.auth
				m.mu.Lock()
				if m.state.FamilyID == o.Meta.Binding.Family && m.pair.FamilyID == o.Meta.Binding.Family && m.state.Status == "account_deletion_unknown" {
					m.state.Status = "active"
					_ = m.persist()
				}
				m.mu.Unlock()
			}
		}
		return empty, e
	}
	return c.applyOwnDeletion(db, o, v)
}
func (c *client) ownDeletionRestricted(db *sql.DB, o ownDeletionIntent, s *ownDeletionSecret, cancel, retry bool) (ownDeletionResult, error) {
	var empty ownDeletionResult
	if o.Result == nil {
		return empty, syncRefuse("ACCOUNT_DELETION_RESULT_REQUIRED")
	}
	if _, e := c.ownDeletionCapability(o, true); e != nil {
		return empty, e
	}
	seen := *o.Result
	if cancel {
		if seen.State != "requested" && seen.State != "cancel_pending" {
			return empty, syncRefuse("ACCOUNT_DELETION_CANCEL_UNAVAILABLE")
		}
		if o.CancelState == "unknown" && !retry {
			return empty, syncRefuse("ACCOUNT_DELETION_CANCEL_QUERY_FIRST")
		}
		if o.Cancel != "" && o.CancelVersion != seen.Version {
			return empty, syncRefuse("ACCOUNT_DELETION_CANCEL_VERSION_CHANGED")
		}
	}
	query := func() (ownDeletionResult, error) {
		p, e := c.issueOwnDeletionProof(o, s, "query")
		if e != nil {
			return empty, e
		}
		defer func() { p.Token = "" }()
		var v ownDeletionResult
		e = c.ownDeletionHTTP("/api/auth/data-rights/query", map[string]string{"proof": p.Token}, "", &v)
		if e != nil {
			return empty, e
		}
		if v.ID != p.ID || v.Version != p.Version || v.Epoch != p.Epoch {
			return empty, syncRefuse("DATA_RIGHTS_PROOF_INVALID")
		}
		return c.applyOwnDeletion(db, o, v)
	}
	if !cancel {
		return query()
	}
	if o.CancelState == "unknown" {
		v, e := query()
		if e != nil {
			return empty, e
		}
		if v.State == "cancelled" {
			return v, nil
		}
		if v.Version != seen.Version || v.State != "requested" {
			return empty, syncRefuse("ACCOUNT_DELETION_CANCEL_VERSION_CHANGED")
		}
		o, e = loadOwnDeletion(db, o.Meta.ID, c.base, c.deployment, o.Meta.Binding.Identity.ConnectionProfile, o.Meta.Username)
		if e != nil {
			return empty, e
		}
	}
	if seen.Until == nil {
		return empty, syncRefuse("ACCOUNT_DELETION_CANCEL_UNAVAILABLE")
	}
	until, e := time.Parse(time.RFC3339Nano, *seen.Until)
	if e != nil || !until.After(time.Now()) {
		return empty, syncRefuse("ACCOUNT_DELETION_CANCEL_UNAVAILABLE")
	}
	if o.Cancel == "" {
		next := o
		next.Cancel, e = syncNewUUID()
		if e != nil {
			return empty, e
		}
		next.CancelVersion = seen.Version
		next.CancelState = "prepared"
		o, e = saveOwnDeletion(db, o, next)
		if e != nil {
			return empty, e
		}
	}
	p, e := c.issueOwnDeletionProof(o, s, "cancel")
	if e != nil {
		return empty, e
	}
	defer func() { p.Token = "" }()
	before := o
	o.CancelState = "unknown"
	o, e = saveOwnDeletion(db, before, o)
	if e != nil {
		return empty, e
	}
	var v ownDeletionResult
	e = c.ownDeletionHTTP("/api/auth/data-rights/cancel", map[string]string{"proof": p.Token, "requestId": o.Cancel}, "", &v)
	if e != nil {
		return empty, e
	}
	if v.Cancel != o.Cancel {
		return empty, syncRefuse("ACCOUNT_DELETION_RESULT_UNKNOWN")
	}
	return c.applyOwnDeletion(db, o, v)
}
func (c *client) issueOwnDeletionProof(o ownDeletionIntent, s *ownDeletionSecret, purpose string) (ownDeletionProof, error) {
	var p ownDeletionProof
	if o.Result == nil || !oneOf(purpose, "query", "cancel") || (len(s.Password) == 0) == (len(s.Recovery) == 0) {
		return p, syncRefuse("DATA_RIGHTS_PROOF_REQUIRED")
	}
	body := map[string]string{"username": o.Meta.Username, "deletionId": o.Result.ID, "requestVersion": o.Result.Version, "purpose": purpose}
	if len(s.Password) > 0 {
		body["password"] = string(s.Password)
	} else {
		body["recoveryCode"] = string(s.Recovery)
	}
	if len(s.OTP) > 0 {
		body["otp"] = string(s.OTP)
	}
	if e := c.ownDeletionHTTP("/api/auth/data-rights/proofs", body, "", &p); e != nil {
		return p, e
	}
	n, e := syncCounter(p.Version)
	old, _ := syncCounter(o.Result.Version)
	expiry, e2 := time.Parse(time.RFC3339Nano, p.Expires)
	if e != nil || e2 != nil || n < old || purpose == "cancel" && p.Version != o.Result.Version || p.ID != o.Result.ID || p.Epoch != o.Meta.Binding.Epoch || p.Purpose != purpose || len(p.Token) < 32 || len(p.Token) > 128 || !expiry.After(time.Now()) || expiry.After(time.Now().Add(6*time.Minute)) {
		p.Token = ""
		return p, syncRefuse("DATA_RIGHTS_PROOF_INVALID")
	}
	return p, nil
}
