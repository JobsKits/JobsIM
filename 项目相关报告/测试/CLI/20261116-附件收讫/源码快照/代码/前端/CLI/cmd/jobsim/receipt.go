// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Approval is operator policy for an exact build/report, never runtime attestation.
type receiptStorage struct {
	Qualification   string `json:"qualificationId"`
	Store           string `json:"storeGeneration"`
	Epoch           string `json:"restoreEpoch"`
	Profile         string `json:"profileId"`
	Build           string `json:"buildDigest"`
	Verification    string `json:"verificationDigest"`
	Qualified       bool   `json:"qualified"`
	Contract        string `json:"storageContract,omitempty"`
	ObjectQualified bool   `json:"objectQualified,omitempty"`
}
type receiptStorageView struct {
	Storage  receiptStorage  `json:"storage"`
	Result   *receiptStorage `json:"result,omitempty"`
	Current  *bool           `json:"current,omitempty"`
	Deletion *bool           `json:"deletionEligible"`
	After    *bool           `json:"afterReceiptEnabled"`
}
type receiptChallenge struct {
	Challenge     string `json:"challenge"`
	Message       string `json:"messageId"`
	Conversation  string `json:"conversationId"`
	Content       string `json:"contentVersion"`
	Epoch         string `json:"restoreEpoch"`
	Store         string `json:"storeGeneration"`
	Qualification string `json:"qualificationId"`
	Envelope      string `json:"bodyEnvelope"`
	Digest        string `json:"bodyDigest"`
	Expires       string `json:"expiresAt"`
	Atomic        *bool  `json:"requiresAtomicLocalCommit"`
	Object        *bool  `json:"objectComplete"`
	Deletion      *bool  `json:"deletionEligible"`
}
type receiptProofTuple struct {
	Scope         string `json:"scope"`
	User          string `json:"user"`
	Device        string `json:"device"`
	Store         string `json:"store"`
	Epoch         string `json:"epoch"`
	Qualification string `json:"qualification"`
	Message       string `json:"message"`
	Conversation  string `json:"conversation"`
	Content       string `json:"contentVersion"`
	Digest        string `json:"bodyDigest"`
	Permission    string `json:"permission"`
	Expires       int64  `json:"expires"`
}
type receiptOperation struct {
	ID           string              `json:"id"`
	Kind         string              `json:"kind"`
	Binding      syncBinding         `json:"binding"`
	Method       string              `json:"method"`
	Route        string              `json:"route"`
	Body         string              `json:"body"`
	Hash         string              `json:"bodyHash"`
	Message      string              `json:"messageId,omitempty"`
	Conversation string              `json:"conversationId,omitempty"`
	Content      string              `json:"contentVersion,omitempty"`
	Digest       string              `json:"bodyDigest,omitempty"`
	CachedDigest string              `json:"cachedDigest,omitempty"`
	Expires      string              `json:"expiresAt,omitempty"`
	State        string              `json:"state"`
	Result       json.RawMessage     `json:"result,omitempty"`
	Object       *receiptLocalObject `json:"object,omitempty"`
}

func receiptMigrate(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS receipt_outbox(id TEXT PRIMARY KEY,epoch TEXT NOT NULL,message_id TEXT NOT NULL,payload TEXT NOT NULL,payload_hash TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','completed','frozen')),result TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS storage_declarations(id TEXT PRIMARY KEY,epoch TEXT NOT NULL,payload TEXT NOT NULL,payload_hash TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','completed','frozen')),result TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS receipt_objects(request_id TEXT PRIMARY KEY,epoch TEXT NOT NULL,message_id TEXT NOT NULL,descriptor TEXT NOT NULL,descriptor_hash TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('ready','erasing','erased')));
CREATE TABLE IF NOT EXISTS receipt_object_cleanup(request_id TEXT PRIMARY KEY,descriptor TEXT NOT NULL,descriptor_hash TEXT NOT NULL);`)
	return err
}
func receiptTable(kind string) string {
	if kind == "storage" {
		return "storage_declarations"
	}
	return "receipt_outbox"
}
func receiptHash(o receiptOperation) string {
	o.Hash, o.State, o.Result = "", "", nil
	raw, _ := json.Marshal(o)
	return fingerprint(string(raw))
}
func receiptCheckIntent(o receiptOperation) error {
	var body map[string]json.RawMessage
	if !syncUUIDPattern.MatchString(o.ID) || syncDecode([]byte(o.Body), &body) != nil {
		return syncRefuse("RECEIPT_INTENT_CHANGED")
	}
	get := func(key string) string { var v string; _ = json.Unmarshal(body[key], &v); return v }
	if get("requestId") != o.ID || get("storeGeneration") != o.Binding.StoreGeneration || get("revisionEpoch") != o.Binding.RestoreEpoch {
		return syncRefuse("RECEIPT_INTENT_CHANGED")
	}
	if o.Kind == "storage" {
		if o.Method != "PUT" || o.Route != "/api/sync/storage" || len(body) != 6 || !receiptProfileID(get("profileId")) || !syncDigestPattern.MatchString(get("buildDigest")) || !syncDigestPattern.MatchString(get("verificationDigest")) {
			return syncRefuse("RECEIPT_INTENT_CHANGED")
		}
	} else if o.Kind == "message_content" {
		if o.Method != "POST" || o.Route != "/api/sync/receipts" || len(body) != 7 || get("kind") != o.Kind || get("bodyDigest") != o.Digest || !syncDigestPattern.MatchString(o.CachedDigest) || !syncUUIDPattern.MatchString(get("qualificationId")) || !strings.HasPrefix(get("challenge"), "rc1.") {
			return syncRefuse("RECEIPT_INTENT_CHANGED")
		}
	} else if o.Kind == "object_received" {
		if err := receiptCheckObjectIntent(o, body); err != nil {
			return err
		}
	} else {
		return syncRefuse("RECEIPT_INTENT_CHANGED")
	}
	return nil
}
func receiptDurability(q syncSQL) error {
	var journal string
	var full int
	if err := q.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		return err
	}
	if err := q.QueryRow("PRAGMA synchronous").Scan(&full); err != nil {
		return err
	}
	if strings.ToLower(journal) != "wal" || full != 2 {
		return syncRefuse("RECEIPT_STORAGE_DURABILITY_REQUIRED")
	}
	return nil
}
func receiptLocalBuild(build string) error {
	actual, err := receiptExecutableDigest()
	if err != nil {
		return err
	}
	if actual != build {
		return syncRefuse("RECEIPT_BUILD_MISMATCH")
	}
	return nil
}
func receiptFalse(v *bool) bool { return v != nil && !*v }
func receiptCheckStorage(v receiptStorageView, b syncBinding) error {
	if !receiptFalse(v.Deletion) || !receiptFalse(v.After) || v.Storage.Store != b.StoreGeneration || v.Storage.Epoch != b.RestoreEpoch {
		return syncRefuse("RECEIPT_STORAGE_INVALID")
	}
	if v.Storage.Qualified && (!syncUUIDPattern.MatchString(v.Storage.Qualification) || !syncDigestPattern.MatchString(v.Storage.Build) || !syncDigestPattern.MatchString(v.Storage.Verification) || !receiptProfileID(v.Storage.Profile)) {
		return syncRefuse("RECEIPT_STORAGE_INVALID")
	}
	if v.Storage.ObjectQualified && (!v.Storage.Qualified || v.Storage.Contract != receiptFileContract) {
		return syncRefuse("RECEIPT_STORAGE_INVALID")
	}
	return nil
}
func receiptProfileID(v string) bool {
	if len(v) < 8 || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.", r)) {
			return false
		}
	}
	return true
}
func receiptNormalized(raw json.RawMessage) (json.RawMessage, error) {
	var body map[string]any
	if syncDecode(raw, &body) != nil {
		return nil, syncRefuse("RECEIPT_CONTENT_INVALID")
	}
	for _, key := range []string{"forwardAdmission", "replyAdmission", "draftAdmission"} {
		delete(body, key)
	}
	return json.Marshal(body)
}
func receiptValidateChallenge(q receiptChallenge, b syncBinding, qualification, message, permission string, now time.Time) (json.RawMessage, error) {
	return receiptValidateMessageChallenge(q, b, qualification, message, permission, now, true)
}
func receiptValidateMessageChallenge(q receiptChallenge, b syncBinding, qualification, message, permission string, now time.Time, textOnly bool) (json.RawMessage, error) {
	if q.Message != message || !syncUUIDPattern.MatchString(message) || !syncUUIDPattern.MatchString(q.Conversation) || q.Epoch != b.RestoreEpoch || q.Store != b.StoreGeneration || q.Qualification != qualification || q.Atomic == nil || !*q.Atomic || !receiptFalse(q.Object) || !receiptFalse(q.Deletion) {
		return nil, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	expires, err := time.Parse(time.RFC3339Nano, q.Expires)
	if err != nil || !expires.After(now) || expires.After(now.Add(time.Hour+time.Minute)) {
		return nil, syncRefuse("RECEIPT_PROOF_EXPIRED")
	}
	parts := strings.Split(q.Challenge, ".")
	if len(q.Challenge) > 4096 || len(parts) != 3 || parts[0] != "rc1" {
		return nil, syncRefuse("RECEIPT_PROOF_INVALID")
	}
	proof, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	signature, sigErr := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	var p receiptProofTuple
	if err != nil || sigErr != nil || len(signature) != 32 || syncDecode(proof, &p) != nil {
		return nil, syncRefuse("RECEIPT_PROOF_INVALID")
	}
	// Decoding checks accidental scope substitution; only the server verifies its HMAC.
	if p.Scope != b.ScopeID || p.User != b.PrincipalID || p.Device != b.DeviceID || p.Store != b.StoreGeneration || p.Epoch != b.RestoreEpoch || p.Qualification != qualification || p.Message != message || p.Conversation != q.Conversation || p.Content != q.Content || p.Digest != q.Digest || p.Permission != permission || p.Expires != expires.Unix() {
		return nil, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	if n, e := syncCounter(q.Content); e != nil || n == 0 {
		return nil, syncRefuse("RECEIPT_CONTENT_INVALID")
	}
	raw, err := syncEnvelope(q.Envelope, q.Digest)
	if err != nil {
		return nil, err
	}
	v, err := syncValidateMessage(raw, false)
	if err != nil || v.ID != message || v.ConversationID != q.Conversation {
		return nil, syncRefuse("RECEIPT_CONTENT_INVALID")
	}
	var object struct {
		Type               string `json:"type"`
		Content            string `json:"contentVersion"`
		Attachment         string `json:"attachmentId"`
		AttachmentURL      string `json:"attachmentURL"`
		Forwarded          bool   `json:"forwarded"`
		ForwardUnavailable bool   `json:"forwardUnavailable"`
		Unavailable        bool   `json:"contentUnavailable"`
		Deleted            bool   `json:"deleted"`
	}
	if syncDecode(raw, &object) != nil || object.Forwarded || object.ForwardUnavailable || object.Unavailable || object.Deleted || object.Content != q.Content {
		return nil, syncRefuse("RECEIPT_CONTENT_UNSUPPORTED")
	}
	if textOnly && (object.Type != "text" || object.Attachment != "" || object.AttachmentURL != "") {
		return nil, syncRefuse("RECEIPT_TEXT_ONLY")
	}
	if !textOnly && (object.Type == "text" && (object.Attachment != "" || object.AttachmentURL != "") || object.Type != "text" && (!oneOf(object.Type, "image", "audio", "video", "file") || !syncUUIDPattern.MatchString(object.Attachment) || object.AttachmentURL != "")) {
		return nil, syncRefuse("RECEIPT_CONTENT_UNSUPPORTED")
	}
	return receiptNormalized(raw)
}
func (s *localStore) receiptControl(q syncSQL, expected *syncControl, current func() bool, writable bool) (syncControl, error) {
	control, present, err := syncLoadControl(q, s.epoch)
	if err != nil || !present {
		return control, syncRefuse("SYNC_DEVICE_REQUIRED")
	}
	if err = s.checkSyncFile(); err != nil {
		return control, err
	}
	var physical string
	var retired int
	if err = q.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); err != nil {
		return control, err
	}
	if err = q.QueryRow("SELECT count(*) FROM metadata WHERE key='auth_retired' AND value='1'").Scan(&retired); err != nil {
		return control, err
	}
	b := syncBinding{ScopeID: s.identity.DeploymentID, PrincipalID: s.identity.UserID, SourceContextID: s.identity.Space, DeviceID: s.identity.DeviceID, StoreGeneration: physical, RestoreEpoch: s.epoch}
	if retired != 0 || !current() || control.syncBinding.check(b, false) != nil || !syncUUIDPattern.MatchString(control.StreamID) {
		return control, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	if expected != nil {
		if err = syncCheckControl(control, *expected); err != nil {
			return control, err
		}
	}
	if writable && control.Restriction != "" {
		return control, syncRefuse("SYNC_REVALIDATION_REQUIRED")
	}
	return control, nil
}
func (s *localStore) stageReceipt(control syncControl, o receiptOperation, message json.RawMessage, current func() bool) error {
	if receiptHash(o) != o.Hash || receiptCheckIntent(o) != nil || o.State != "pending" || o.Binding.check(control.syncBinding, true) != nil {
		return syncRefuse("RECEIPT_INTENT_INVALID")
	}
	if o.Kind == "object_received" {
		file, e := s.openReceiptObject(*o.Object)
		if e != nil {
			return e
		}
		defer file.Close()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.receiptControl(tx, &control, current, true); err != nil {
		return err
	}
	if err = receiptDurability(tx); err != nil {
		return err
	}
	if o.Kind == "object_received" {
		if err = s.stageReceiptObjectTx(tx, control, o); err != nil {
			return err
		}
	}
	if o.Kind == "message_content" {
		var terminal int
		if err = tx.QueryRow("SELECT count(*) FROM sync_terminal WHERE id=?", o.Message).Scan(&terminal); err != nil {
			return err
		}
		if terminal != 0 || fingerprint(string(message)) != o.CachedDigest {
			return syncRefuse("RECEIPT_CONTENT_CHANGED")
		}
		if _, err = writeCached(tx, s.epoch, message); err != nil {
			return err
		}
		var installed string
		if err = tx.QueryRow("SELECT payload FROM messages WHERE epoch=? AND id=?", s.epoch, o.Message).Scan(&installed); err != nil {
			return err
		}
		if !syncEqual([]byte(installed), message) {
			return syncRefuse("RECEIPT_CONTENT_CHANGED")
		}
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	if o.Kind == "storage" {
		_, err = tx.Exec("INSERT INTO storage_declarations(id,epoch,payload,payload_hash,state) VALUES(?,?,?,?,?)", o.ID, s.epoch, string(raw), o.Hash, o.State)
	} else {
		_, err = tx.Exec("INSERT INTO receipt_outbox(id,epoch,message_id,payload,payload_hash,state) VALUES(?,?,?,?,?,?)", o.ID, s.epoch, o.Message, string(raw), o.Hash, o.State)
	}
	if err != nil {
		return err
	}
	if !current() {
		return syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	return tx.Commit()
}
func (s *localStore) loadReceipt(id string) (receiptOperation, error) {
	return loadReceiptQuery(s.db, id)
}
func loadReceiptQuery(q syncSQL, id string) (receiptOperation, error) {
	var o receiptOperation
	for _, table := range []string{"receipt_outbox", "storage_declarations"} {
		var raw, hash, state, result string
		err := q.QueryRow("SELECT payload,payload_hash,state,result FROM "+table+" WHERE id=?", id).Scan(&raw, &hash, &state, &result)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return o, err
		}
		if syncDecode([]byte(raw), &o) != nil || o.ID != id || receiptHash(o) != hash || o.Hash != hash || receiptCheckIntent(o) != nil || receiptTable(o.Kind) != table {
			return o, syncRefuse("RECEIPT_INTENT_CHANGED")
		}
		o.State = state
		if result != "" {
			o.Result = json.RawMessage(result)
		}
		return o, nil
	}
	return o, sql.ErrNoRows
}
func (s *localStore) finishReceipt(o receiptOperation, expected syncControl, result json.RawMessage, state string, current func() bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.receiptControl(tx, &expected, current, false); err != nil {
		return err
	}
	var hash, original string
	if err = tx.QueryRow("SELECT payload_hash,payload FROM "+receiptTable(o.Kind)+" WHERE id=?", o.ID).Scan(&hash, &original); err != nil {
		return err
	}
	var stored receiptOperation
	if hash != o.Hash || syncDecode([]byte(original), &stored) != nil || receiptHash(stored) != o.Hash {
		return syncRefuse("RECEIPT_INTENT_CHANGED")
	}
	if _, err = tx.Exec("UPDATE "+receiptTable(o.Kind)+" SET state=?,result=? WHERE id=? AND payload_hash=?", state, string(result), o.ID, o.Hash); err != nil {
		return err
	}
	if !current() {
		return syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	return tx.Commit()
}
func receiptValidateResult(raw json.RawMessage, o receiptOperation) error {
	var v struct {
		ID            string `json:"operationId"`
		Status        string `json:"status"`
		Message       string `json:"messageId"`
		Conversation  string `json:"conversationId"`
		Content       string `json:"contentVersion"`
		Epoch         string `json:"restoreEpoch"`
		Device        string `json:"deviceId"`
		Store         string `json:"storeGeneration"`
		Qualification string `json:"qualificationId"`
		Digest        string `json:"bodyDigest"`
		Kind          string `json:"receiptKind"`
		Qualified     *bool  `json:"storageQualifiedAtCommit"`
		Object        *bool  `json:"objectComplete"`
		Target        *bool  `json:"targetRangeComplete"`
		Deletion      *bool  `json:"deletionEligible"`
	}
	var body struct {
		Qualification string `json:"qualificationId"`
	}
	if syncDecode([]byte(o.Body), &body) != nil || syncDecode(raw, &v) != nil || v.ID != o.ID || v.Status != "recorded" || v.Message != o.Message || v.Conversation != o.Conversation || v.Content != o.Content || v.Epoch != o.Binding.RestoreEpoch || v.Device != o.Binding.DeviceID || v.Store != o.Binding.StoreGeneration || v.Qualification != body.Qualification || v.Digest != o.Digest || v.Kind != "message_content" || v.Qualified == nil || !*v.Qualified || !receiptFalse(v.Object) || !receiptFalse(v.Target) || !receiptFalse(v.Deletion) {
		return syncRefuse("RECEIPT_RESULT_INVALID")
	}
	return nil
}
func (c *client) receiptCapabilities() error {
	return c.receiptCapabilitiesFor(false)
}
func (c *client) receiptCapabilitiesFor(object bool) error {
	var boot struct {
		Deployment string          `json:"deploymentId"`
		Epoch      string          `json:"restoreEpoch"`
		Features   map[string]bool `json:"features"`
	}
	if err := c.call("GET", "/api/bootstrap", nil, &boot); err != nil {
		return err
	}
	if boot.Deployment != c.deployment || boot.Epoch != c.epoch {
		return syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	if !boot.Features["durable_receipt_protocol"] || !boot.Features["authoritative_sync"] || !boot.Features["sync_jetstream"] || boot.Features["after_receipt"] {
		return syncRefuse("RECEIPT_CAPABILITY_REQUIRED")
	}
	if object && !boot.Features["durable_object_receipt_protocol"] {
		return syncRefuse("OBJECT_RECEIPT_CAPABILITY_REQUIRED")
	}
	return nil
}
func (c *client) receiptContext() (*localStore, func() bool, error) {
	s := c.store
	if s == nil {
		return nil, nil, syncRefuse("SYNC_STORE_REQUIRED")
	}
	var pair *authPair
	if c.auth != nil {
		p, err := c.auth.snapshot()
		if err != nil {
			return nil, nil, err
		}
		pair = &p
	}
	current := func() bool { return c.syncCurrent(s, pair) }
	if !current() {
		return nil, nil, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	return s, current, nil
}
func (c *client) getReceiptStorage(control syncControl) (receiptStorageView, error) {
	var value receiptStorageView
	err := c.call("GET", syncSchemaPath("/api/sync/storage", control.schema()), nil, &value)
	if err == nil {
		err = receiptCheckStorage(value, control.syncBinding)
	}
	return value, err
}
func (c *client) performReceipt(id string, explicit bool, fresh bool) (json.RawMessage, error) {
	s, current, err := c.receiptContext()
	if err != nil {
		return nil, err
	}
	o, err := s.loadReceipt(id)
	if err != nil {
		return nil, err
	}
	control, err := s.receiptControl(s.db, nil, current, false)
	if err != nil {
		return nil, err
	}
	if o.Binding.check(control.syncBinding, false) != nil {
		return nil, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	if err = c.receiptCapabilities(); err != nil {
		return nil, err
	}
	if !current() {
		return nil, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	if o.Kind == "object_received" {
		return c.performObjectReceipt(s, current, control, o, explicit, fresh)
	}
	var raw json.RawMessage
	if o.Kind == "storage" {
		if !explicit || o.State != "pending" {
			v, e := c.getReceiptStorage(control)
			if e != nil {
				return nil, e
			}
			return json.Marshal(map[string]any{"requestId": o.ID, "originalStatus": o.State, "currentObservation": v, "originalResultVerified": o.State == "completed"})
		}
	} else {
		err = c.call("POST", "/api/sync/receipt-results", json.RawMessage(o.Body), &raw)
		if err == nil {
			if err = receiptValidateResult(raw, o); err != nil {
				return nil, err
			}
			return raw, s.finishReceipt(o, control, raw, "completed", current)
		}
		var api *apiFailure
		if !errors.As(err, &api) || api.Status != 404 || api.Code != "RECEIPT_OPERATION_NOT_FOUND" || (!explicit && !fresh) || o.State != "pending" {
			return nil, err
		}
	}
	if _, err = s.receiptControl(s.db, &control, current, true); err != nil {
		return nil, err
	}
	if err = receiptDurability(s.db); err != nil {
		return nil, err
	}
	if o.Kind == "message_content" {
		expires, e := time.Parse(time.RFC3339Nano, o.Expires)
		if e != nil || !expires.After(time.Now()) {
			_ = s.finishReceipt(o, control, nil, "frozen", current)
			return nil, syncRefuse("RECEIPT_PROOF_EXPIRED")
		}
		storage, e := c.getReceiptStorage(control)
		if e != nil {
			return nil, e
		}
		var body struct {
			Qualification string `json:"qualificationId"`
			Challenge     string `json:"challenge"`
		}
		if syncDecode([]byte(o.Body), &body) != nil {
			return nil, syncRefuse("RECEIPT_INTENT_CHANGED")
		}
		if !storage.Storage.Qualified || storage.Storage.Qualification != body.Qualification {
			return nil, syncRefuse("STORAGE_NOT_QUALIFIED")
		}
		if err = receiptLocalBuild(storage.Storage.Build); err != nil {
			return nil, err
		}
		var cached string
		var terminal int
		if err = s.db.QueryRow("SELECT count(*) FROM sync_terminal WHERE id=?", o.Message).Scan(&terminal); err != nil {
			return nil, err
		}
		if err = s.db.QueryRow("SELECT payload FROM messages WHERE epoch=? AND id=?", s.epoch, o.Message).Scan(&cached); err != nil {
			return nil, err
		}
		if terminal != 0 || fingerprint(cached) != o.CachedDigest {
			return nil, syncRefuse("RECEIPT_CONTENT_CHANGED")
		}
	} else {
		var body receiptStorage
		if syncDecode([]byte(o.Body), &body) != nil {
			return nil, syncRefuse("RECEIPT_INTENT_CHANGED")
		}
		if err = receiptLocalBuild(body.Build); err != nil {
			return nil, err
		}
	}
	if err = c.receiptCapabilities(); err != nil {
		return nil, err
	}
	if _, err = s.receiptControl(s.db, &control, current, true); err != nil {
		return nil, err
	}
	err = c.call(o.Method, o.Route, json.RawMessage(o.Body), &raw)
	if err != nil {
		return nil, err
	}
	if o.Kind == "storage" {
		var v receiptStorageView
		var body receiptStorage
		if syncDecode(raw, &v) != nil || syncDecode([]byte(o.Body), &body) != nil || v.Result == nil || v.Current == nil || receiptCheckStorage(v, control.syncBinding) != nil || v.Result.Store != o.Binding.StoreGeneration || v.Result.Epoch != o.Binding.RestoreEpoch || !syncUUIDPattern.MatchString(v.Result.Qualification) || v.Result.Profile != body.Profile || v.Result.Build != body.Build || v.Result.Verification != body.Verification {
			return nil, syncRefuse("RECEIPT_STORAGE_INVALID")
		}
		if *v.Current != (v.Storage.Qualified && v.Storage.Qualification == v.Result.Qualification) {
			return nil, syncRefuse("RECEIPT_STORAGE_INVALID")
		}
	} else if err = receiptValidateResult(raw, o); err != nil {
		return nil, err
	}
	return raw, s.finishReceipt(o, control, raw, "completed", current)
}
func (c *client) receiveReceiptText(message string) (json.RawMessage, error) {
	return c.receiveReceiptMessage(message, true)
}
func (c *client) receiveReceiptMessage(message string, textOnly bool) (json.RawMessage, error) {
	s, current, err := c.receiptContext()
	if err != nil {
		return nil, err
	}
	if !syncUUIDPattern.MatchString(message) {
		return nil, syncRefuse("INVALID_MESSAGE_ID")
	}
	control, err := s.receiptControl(s.db, nil, current, true)
	if err != nil {
		return nil, err
	}
	var pendingID string
	err = s.db.QueryRow("SELECT id FROM receipt_outbox WHERE epoch=? AND message_id=? AND state='pending' AND json_extract(payload,'$.kind')='message_content' ORDER BY rowid LIMIT 1", s.epoch, message).Scan(&pendingID)
	if err == nil {
		return c.performReceipt(pendingID, false, false)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err = c.receiptCapabilities(); err != nil {
		return nil, err
	}
	storage, err := c.getReceiptStorage(control)
	if err != nil {
		return nil, err
	}
	if !storage.Storage.Qualified {
		return nil, syncRefuse("STORAGE_NOT_QUALIFIED")
	}
	if err = receiptLocalBuild(storage.Storage.Build); err != nil {
		return nil, err
	}
	var challenge receiptChallenge
	if err = c.call("POST", "/api/sync/receipt-challenges", map[string]string{"messageId": message, "schemaVersion": control.schema()}, &challenge); err != nil {
		return nil, err
	}
	normalized, err := receiptValidateMessageChallenge(challenge, control.syncBinding, storage.Storage.Qualification, message, control.Permission, time.Now(), textOnly)
	if err != nil {
		return nil, err
	}
	request, err := syncNewUUID()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{"requestId": request, "storeGeneration": control.StoreGeneration, "revisionEpoch": s.epoch, "qualificationId": challenge.Qualification, "kind": "message_content", "challenge": challenge.Challenge, "bodyDigest": challenge.Digest})
	o := receiptOperation{ID: request, Kind: "message_content", Binding: control.syncBinding, Method: "POST", Route: "/api/sync/receipts", Body: string(body), Message: message, Conversation: challenge.Conversation, Content: challenge.Content, Digest: challenge.Digest, CachedDigest: fingerprint(string(normalized)), Expires: challenge.Expires, State: "pending"}
	o.Hash = receiptHash(o)
	if err = s.stageReceipt(control, o, normalized, current); err != nil {
		return nil, err
	}
	return c.performReceipt(o.ID, false, true)
}
func receiptExecutableDigest() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (c *client) receiptCommand(cmd, args string) (bool, error) {
	if !oneOf(cmd, "storage-status", "storage-declare", "receive-text", "receive-message", "receive-object", "receive-object-file", "receipt-file-status", "receipts", "reconcile-receipt", "retry-receipt") {
		return false, nil
	}
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	s, current, err := c.receiptContext()
	if err != nil {
		return true, err
	}
	if oneOf(cmd, "receive-object", "receive-object-file", "receipt-file-status") {
		return true, c.receiptObjectCommand(cmd, args, s)
	}
	if cmd == "receipts" {
		items := []map[string]string{}
		for _, table := range []string{"receipt_outbox", "storage_declarations"} {
			rows, e := s.db.Query("SELECT id,payload,state FROM " + table + " ORDER BY rowid")
			if e != nil {
				return true, e
			}
			for rows.Next() {
				var id, raw, state string
				if e = rows.Scan(&id, &raw, &state); e != nil {
					break
				}
				var o receiptOperation
				if e = syncDecode([]byte(raw), &o); e != nil {
					break
				}
				item := map[string]string{"requestId": id, "kind": o.Kind, "state": state, "messageId": o.Message, "restoreEpoch": o.Binding.RestoreEpoch}
				if o.Object != nil {
					item["objectId"], item["relativePath"] = o.Object.Object, o.Object.Path
				}
				items = append(items, item)
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				return true, e
			}
		}
		return true, printJSON(map[string]any{"items": items, "deletionEligible": false, "afterReceiptEnabled": false})
	}
	if cmd == "reconcile-receipt" || cmd == "retry-receipt" {
		raw, e := c.performReceipt(strings.TrimSpace(args), cmd == "retry-receipt", false)
		if e != nil {
			return true, e
		}
		return true, printJSON(raw)
	}
	if cmd == "receive-text" || cmd == "receive-message" {
		raw, e := c.receiveReceiptMessage(strings.TrimSpace(args), cmd == "receive-text")
		if e != nil {
			return true, e
		}
		return true, printJSON(raw)
	}
	if err = c.receiptCapabilities(); err != nil {
		return true, err
	}
	control, err := s.receiptControl(s.db, nil, current, true)
	if err != nil {
		return true, err
	}
	if cmd == "storage-status" {
		v, e := c.getReceiptStorage(control)
		if e != nil {
			return true, e
		}
		return true, printJSON(v)
	}
	parts := strings.Fields(args)
	if len(parts) != 3 || !receiptProfileID(parts[0]) || !syncDigestPattern.MatchString(parts[1]) || !syncDigestPattern.MatchString(parts[2]) {
		return true, fmt.Errorf("storage-declare PROFILE_ID BUILD_SHA256 VERIFICATION_SHA256")
	}
	build, e := receiptExecutableDigest()
	if e != nil {
		return true, e
	}
	if build != parts[1] {
		return true, syncRefuse("RECEIPT_BUILD_MISMATCH")
	}
	var journal string
	var full int
	if e = s.db.QueryRow("PRAGMA journal_mode").Scan(&journal); e != nil {
		return true, e
	}
	if e = s.db.QueryRow("PRAGMA synchronous").Scan(&full); e != nil {
		return true, e
	}
	if strings.ToLower(journal) != "wal" || full != 2 {
		return true, syncRefuse("RECEIPT_STORAGE_DURABILITY_REQUIRED")
	}
	// An unresolved declaration must be explicitly resumed, never silently replaced.
	var n int
	if e = s.db.QueryRow("SELECT count(*) FROM storage_declarations WHERE epoch=? AND state='pending'", s.epoch).Scan(&n); e != nil {
		return true, e
	}
	if n != 0 {
		return true, syncRefuse("RECEIPT_STORAGE_DECLARATION_PENDING")
	}
	request, e := syncNewUUID()
	if e != nil {
		return true, e
	}
	body, _ := json.Marshal(map[string]string{"requestId": request, "storeGeneration": control.StoreGeneration, "revisionEpoch": s.epoch, "profileId": parts[0], "buildDigest": parts[1], "verificationDigest": parts[2]})
	o := receiptOperation{ID: request, Kind: "storage", Binding: control.syncBinding, Method: "PUT", Route: "/api/sync/storage", Body: string(body), State: "pending"}
	o.Hash = receiptHash(o)
	if e = s.stageReceipt(control, o, nil, current); e != nil {
		return true, e
	}
	raw, e := c.performReceipt(request, true, true)
	if e != nil {
		return true, e
	}
	return true, printJSON(raw)
}
