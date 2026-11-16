// Created by Jobs on 2026年11月14日，星期六.
package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type localIdentity struct {
	Origin            string `json:"origin"`
	DeploymentID      string `json:"deploymentId"`
	UserID            string `json:"userId"`
	DeviceID          string `json:"deviceId"`
	Space             string `json:"space"`
	ConnectionProfile string `json:"connectionProfile,omitempty"`
	StoreGeneration   string `json:"storeGeneration,omitempty"`
}
type localStore struct {
	db       *sql.DB
	path     string
	identity localIdentity
	epoch    string
	fileInfo os.FileInfo
}
type pending struct {
	DeploymentID         string   `json:"deploymentId"`
	UserID               string   `json:"userId"`
	ConversationID       string   `json:"conversationId"`
	ClientMsgID          string   `json:"clientMsgId"`
	Type                 string   `json:"type"`
	Text                 string   `json:"text"`
	AttachmentID         string   `json:"attachmentId,omitempty"`
	ReplyToID            string   `json:"replyToId,omitempty"`
	ForwardOfID          string   `json:"forwardOfId,omitempty"`
	ForwardSourceVersion string   `json:"forwardSourceVersion,omitempty"`
	ForwardAdmission     string   `json:"forwardAdmission,omitempty"`
	RestoreEpoch         string   `json:"restoreEpoch"`
	State                string   `json:"state"`
	SendAdmission        string   `json:"sendAdmission,omitempty"`
	FreezeReason         string   `json:"freezeReason,omitempty"`
	MentionedUserIDs     []string `json:"mentionedUserIds,omitempty"`
	MentionAll           bool     `json:"mentionAll,omitempty"`
	RoleGeneration       string   `json:"roleGeneration,omitempty"`
}

func (p pending) payload() map[string]any {
	if p.Type == "forward" {
		return map[string]any{"clientMsgId": p.ClientMsgID, "type": "forward", "forwardOfId": p.ForwardOfID, "forwardSourceVersion": p.ForwardSourceVersion, "forwardAdmission": p.ForwardAdmission, "restoreEpoch": p.RestoreEpoch, "sendAdmission": p.SendAdmission}
	}
	q := map[string]any{"clientMsgId": p.ClientMsgID, "type": p.Type, "text": p.Text, "restoreEpoch": p.RestoreEpoch}
	if p.SendAdmission != "" {
		q["sendAdmission"] = p.SendAdmission
	}
	if p.AttachmentID != "" {
		q["attachmentId"] = p.AttachmentID
	}
	if p.ReplyToID != "" {
		q["replyToId"] = p.ReplyToID
	}
	if p.ForwardOfID != "" {
		q["forwardOfId"] = p.ForwardOfID
	}
	if len(p.MentionedUserIDs) > 0 {
		q["mentionedUserIds"] = p.MentionedUserIDs
	}
	if p.MentionAll {
		q["mentionAll"] = true
	}
	return q
}
func operationDigest(p pending) string {
	envelope := map[string]any{"conversationId": p.ConversationID, "payload": p.payload()}
	if p.hasMentions() && p.RoleGeneration != "" {
		envelope["originalRoleGeneration"] = p.RoleGeneration
	}
	raw, _ := json.Marshal(envelope)
	return fingerprint(string(raw))
}
func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *localStore) freezeAdmission(p pending) error {
	_, err := s.db.Exec("UPDATE outbox SET state='frozen',freeze_reason=CASE WHEN freeze_reason='' THEN 'SEND_ADMISSION_CHANGED' ELSE freeze_reason END,updated_at=? WHERE client_msg_id=? AND state!='confirmed'", time.Now().UTC().Format(time.RFC3339Nano), p.ClientMsgID)
	return err
}

var decimal = regexp.MustCompile(`^[1-9][0-9]*$`)

func openStore(path string, identity localIdentity, epoch string) (*localStore, error) {
	if !decimal.MatchString(epoch) {
		return nil, fmt.Errorf("invalid restore epoch")
	}
	if identity.Origin == "" || identity.DeploymentID == "" || identity.UserID == "" || identity.DeviceID == "" || identity.Space == "" {
		return nil, fmt.Errorf("local namespace incomplete")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("local database must be a regular private file")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	openedInfo, err := os.Lstat(path)
	if err != nil || !openedInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("local SQLite file identity unavailable")
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := uri.Query()
	for _, pragma := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)", "foreign_keys(ON)"} {
		query.Add("_pragma", pragma)
	}
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	failed := true
	defer func() {
		if failed {
			db.Close()
		}
	}()
	var mode string
	var sync int
	var schemaVersion int
	if err = db.QueryRow("PRAGMA user_version").Scan(&schemaVersion); err != nil {
		return nil, err
	}
	if schemaVersion > 4 {
		return nil, fmt.Errorf("newer local schema detected; original retained")
	}
	if err = db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return nil, err
	}
	if err = db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
		return nil, err
	}
	if strings.ToLower(mode) != "wal" || sync != 2 {
		return nil, fmt.Errorf("durable SQLite WAL/FULL unavailable")
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS outbox(client_msg_id TEXT PRIMARY KEY,conversation_id TEXT NOT NULL,payload TEXT NOT NULL,payload_hash TEXT NOT NULL,restore_epoch TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','confirmed','frozen')),server_id TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,freeze_reason TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS messages(epoch TEXT NOT NULL,id TEXT NOT NULL,conversation_id TEXT NOT NULL,created_at TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
 CREATE INDEX IF NOT EXISTS message_history ON messages(epoch,conversation_id,created_at,id);
 CREATE TABLE IF NOT EXISTS cursors(epoch TEXT NOT NULL,conversation_id TEXT NOT NULL,cursor TEXT NOT NULL,PRIMARY KEY(epoch,conversation_id));
 CREATE TABLE IF NOT EXISTS drafts(epoch TEXT NOT NULL,conversation_id TEXT NOT NULL,text TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(epoch,conversation_id));
 CREATE TABLE IF NOT EXISTS conversations(epoch TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
 CREATE TABLE IF NOT EXISTS imports(sha256 TEXT PRIMARY KEY,source TEXT NOT NULL,imported_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS governance_operations(id TEXT PRIMARY KEY,kind TEXT NOT NULL,peer_id TEXT NOT NULL,restore_epoch TEXT NOT NULL,payload TEXT NOT NULL,payload_hash TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','completed','failed','frozen')),result TEXT,error_code TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS governance_pending ON governance_operations(kind,peer_id) WHERE state='pending';
 CREATE TABLE IF NOT EXISTS announcement_operations(id TEXT PRIMARY KEY,conversation_id TEXT NOT NULL,restore_epoch TEXT NOT NULL,method TEXT NOT NULL,payload TEXT NOT NULL,payload_hash TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','completed','frozen')),result TEXT,error_code TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS announcement_pending ON announcement_operations(conversation_id) WHERE state='pending';`)
	if err != nil {
		return nil, err
	}
	namespace, _ := json.Marshal(identity)
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if schemaVersion < 2 {
		columns, e := tx.Query("PRAGMA table_info(outbox)")
		if e != nil {
			return nil, e
		}
		hasReason := false
		for columns.Next() {
			var index, required, primary int
			var name, kind string
			var defaultValue any
			if e = columns.Scan(&index, &name, &kind, &required, &defaultValue, &primary); e != nil {
				columns.Close()
				return nil, e
			}
			hasReason = hasReason || name == "freeze_reason"
		}
		e = columns.Err()
		columns.Close()
		if e != nil {
			return nil, e
		}
		if !hasReason {
			if _, e = tx.Exec("ALTER TABLE outbox ADD COLUMN freeze_reason TEXT NOT NULL DEFAULT ''"); e != nil {
				return nil, e
			}
		}
	}
	var existing string
	err = tx.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&existing)
	if err == sql.ErrNoRows {
		_, err = tx.Exec("INSERT INTO metadata(key,value) VALUES('identity',?),('epoch',?)", string(namespace), epoch)
	} else if err == nil && existing != string(namespace) {
		return nil, fmt.Errorf("local database identity mismatch; original data retained")
	}
	if err != nil {
		return nil, err
	}
	var previous string
	if err = tx.QueryRow("SELECT value FROM metadata WHERE key='epoch'").Scan(&previous); err != nil {
		return nil, err
	}
	if previous != epoch {
		if _, err = tx.Exec("UPDATE outbox SET state='frozen',freeze_reason='RESTORE_EPOCH_CHANGED',updated_at=? WHERE state='pending'", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return nil, err
		}
		if _, err = tx.Exec("UPDATE governance_operations SET state='frozen',error_code='RESTORE_EPOCH_CHANGED',updated_at=? WHERE state='pending'", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return nil, err
		}
		if _, err = tx.Exec("UPDATE announcement_operations SET state='frozen',error_code='RESTORE_EPOCH_CHANGED',updated_at=? WHERE state='pending'", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return nil, err
		}
		if _, err = tx.Exec("UPDATE metadata SET value=? WHERE key='epoch'", epoch); err != nil {
			return nil, err
		}
	}
	if err = syncMigrate(tx, schemaVersion); err != nil {
		return nil, err
	}
	if _, err = tx.Exec("PRAGMA user_version=4"); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	failed = false
	return &localStore{db: db, path: path, identity: identity, epoch: epoch, fileInfo: openedInfo}, nil
}
func (s *localStore) close() error { return s.db.Close() }
func (s *localStore) enqueue(p pending) error {
	if p.DeploymentID != s.identity.DeploymentID || p.UserID != s.identity.UserID || p.RestoreEpoch != s.epoch {
		return fmt.Errorf("pending operation belongs to a different account or restore epoch")
	}
	digest := operationDigest(p)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldHash, state string
	var retired int
	if err = tx.QueryRow("SELECT count(*) FROM metadata WHERE key='auth_retired' AND value='1'").Scan(&retired); err != nil {
		return err
	}
	if retired != 0 {
		return fmt.Errorf("retired device namespace refuses late enqueue")
	}
	err = tx.QueryRow("SELECT payload_hash,state FROM outbox WHERE client_msg_id=?", p.ClientMsgID).Scan(&oldHash, &state)
	if err == nil {
		if oldHash != digest {
			return fmt.Errorf("same clientMsgId has a different payload")
		}
		if state == "confirmed" {
			return fmt.Errorf("message already confirmed")
		}
		if state == "frozen" {
			return fmt.Errorf("message frozen; explicit reconcile keeps its original payload")
		}
	} else if err == sql.ErrNoRows {
		var confirmed int
		if err = tx.QueryRow("SELECT count(*) FROM sync_receipts WHERE client_id=?", p.ClientMsgID).Scan(&confirmed); err != nil {
			return err
		}
		if confirmed != 0 {
			return fmt.Errorf("original clientMsgId already has a readonly confirmed receipt")
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		p.State = "pending"
		row, _ := json.Marshal(p)
		if _, err = tx.Exec("INSERT INTO outbox(client_msg_id,conversation_id,payload,payload_hash,restore_epoch,state,created_at,updated_at) VALUES(?,?,?,?,?,'pending',?,?)", p.ClientMsgID, p.ConversationID, string(row), digest, p.RestoreEpoch, now, now); err != nil {
			return err
		}
	} else {
		return err
	}
	if p.Type != "forward" {
		if _, err = tx.Exec("DELETE FROM drafts WHERE epoch=? AND conversation_id=?", s.epoch, p.ConversationID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *localStore) outbox() (map[string]pending, error) {
	rows, err := s.db.Query("SELECT payload,state,freeze_reason,payload_hash FROM outbox ORDER BY created_at,client_msg_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]pending{}
	for rows.Next() {
		var raw, state, reason, digest string
		if err = rows.Scan(&raw, &state, &reason, &digest); err != nil {
			return nil, err
		}
		var p pending
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, fmt.Errorf("local outbox integrity error")
		}
		p.State = state
		if reason != "" {
			p.FreezeReason = reason
		}
		if operationDigest(p) != digest {
			return nil, fmt.Errorf("local outbox payload hash mismatch; nothing sent")
		}
		if p.DeploymentID != s.identity.DeploymentID || p.UserID != s.identity.UserID {
			return nil, fmt.Errorf("local outbox identity mismatch")
		}
		result[p.ClientMsgID] = p
	}
	return result, rows.Err()
}

type cacheMessage struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	CreatedAt      string `json:"createdAt"`
	ClientMsgID    string `json:"clientMsgId"`
	SenderID       string `json:"senderId"`
	Version        string `json:"version"`
}

func writeCached(tx *sql.Tx, epoch string, raw json.RawMessage) (cacheMessage, error) {
	// Cache only a placeholder for references; each live read rechecks the source.
	var projection map[string]any
	if err := json.Unmarshal(raw, &projection); err != nil {
		return cacheMessage{}, err
	}
	global := projection["deleted"] == true
	id, _ := projection["id"].(string)
	var known int
	if err := tx.QueryRow("SELECT count(*) FROM sync_terminal WHERE epoch=? AND id=?", epoch, id).Scan(&known); err != nil {
		return cacheMessage{}, err
	}
	global = global || known > 0
	if global {
		projection["deleted"] = true
		projection["text"] = ""
		for _, key := range []string{"attachmentId", "attachmentURL", "attachmentName", "replyToId", "reactions", "mentionedUserIds", "mentionAll", "mentionedMe", "pinned", "favorite"} {
			delete(projection, key)
		}
	}
	delete(projection, "forwardAdmission")
	if projection["forwarded"] == true {
		maskForward(projection)
	}
	var sanitizeErr error
	raw, sanitizeErr = json.Marshal(projection)
	if sanitizeErr != nil {
		return cacheMessage{}, sanitizeErr
	}
	var message cacheMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return message, err
	}
	if !decimal.MatchString(message.Version) {
		return message, fmt.Errorf("invalid authoritative version")
	}
	var previousRaw string
	previousErr := tx.QueryRow("SELECT payload FROM messages WHERE epoch=? AND id=?", epoch, message.ID).Scan(&previousRaw)
	if previousErr == nil {
		var previous cacheMessage
		if err := json.Unmarshal([]byte(previousRaw), &previous); err != nil {
			return message, err
		}
		a, aok := new(big.Int).SetString(previous.Version, 10)
		b, bok := new(big.Int).SetString(message.Version, 10)
		if !aok || !bok {
			return message, fmt.Errorf("invalid cached version")
		}
		var priorState struct {
			Deleted bool `json:"deleted"`
		}
		if err := json.Unmarshal([]byte(previousRaw), &priorState); err != nil {
			return message, err
		}
		if priorState.Deleted && projection["deleted"] != true {
			return previous, nil
		}
		if !global && a.Cmp(b) > 0 {
			return previous, nil
		}
	} else if previousErr != sql.ErrNoRows {
		return message, previousErr
	}
	created, err := time.Parse(time.RFC3339Nano, message.CreatedAt)
	if err != nil || message.ID == "" || message.ConversationID == "" {
		return message, fmt.Errorf("invalid authoritative message; cursor not advanced")
	}
	if global {
		if err = syncRecordGlobalTx(tx, epoch, message.ID); err != nil {
			return message, err
		}
	}
	_, err = tx.Exec("INSERT INTO messages(epoch,id,conversation_id,created_at,payload) VALUES(?,?,?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET conversation_id=excluded.conversation_id,created_at=excluded.created_at,payload=excluded.payload", epoch, message.ID, message.ConversationID, created.UTC().Format("2006-01-02T15:04:05.000000000Z"), string(raw))
	return message, err
}
func (s *localStore) confirm(p pending, raw json.RawMessage) error {
	responsible, err := s.syncResponsible()
	if err != nil {
		return err
	}
	if responsible {
		return syncRefuse("SYNC_READONLY_GUARD_REQUIRED")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var digest, freezeReason string
	if err = tx.QueryRow("SELECT payload_hash,freeze_reason FROM outbox WHERE client_msg_id=?", p.ClientMsgID).Scan(&digest, &freezeReason); err != nil {
		return err
	}
	if digest != operationDigest(p) {
		return fmt.Errorf("confirmation payload differs from stored original")
	}
	var message cacheMessage
	if freezeReason == "CONVERSATION_LEFT" {
		// An explicit result check must not refill history purged by a local leave intent.
		err = json.Unmarshal(raw, &message)
		if err == nil && (!decimal.MatchString(message.Version) || message.ID == "") {
			err = fmt.Errorf("invalid confirmation")
		}
	} else {
		message, err = writeCached(tx, s.epoch, raw)
	}
	if err != nil {
		return err
	}
	if message.ClientMsgID != p.ClientMsgID || message.ConversationID != p.ConversationID || message.SenderID != s.identity.UserID {
		return fmt.Errorf("confirmation differs from original operation")
	}
	if _, err = tx.Exec("UPDATE outbox SET state='confirmed',server_id=?,updated_at=? WHERE client_msg_id=? AND restore_epoch=?", message.ID, time.Now().UTC().Format(time.RFC3339Nano), p.ClientMsgID, p.RestoreEpoch); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *localStore) savePage(conversation, cursor string, items []json.RawMessage) error {
	epoch, sequence, ok := strings.Cut(cursor, ":")
	if !ok || epoch != s.epoch || sequence == "" || strings.Trim(sequence, "0123456789") != "" {
		return fmt.Errorf("restore cursor mismatch; page not committed")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, raw := range items {
		m, err := writeCached(tx, s.epoch, raw)
		if err != nil {
			return err
		}
		if m.ConversationID != conversation {
			return fmt.Errorf("message outside requested conversation")
		}
		// A history projection does not prove that the original route and body matched.
	}
	var previousCursor string
	cursorErr := tx.QueryRow("SELECT cursor FROM cursors WHERE epoch=? AND conversation_id=?", s.epoch, conversation).Scan(&previousCursor)
	if cursorErr == nil {
		_, oldSequence, _ := strings.Cut(previousCursor, ":")
		a, aok := new(big.Int).SetString(oldSequence, 10)
		b, bok := new(big.Int).SetString(sequence, 10)
		if !aok || !bok {
			return fmt.Errorf("invalid cached cursor")
		}
		if a.Cmp(b) > 0 {
			cursor = previousCursor
		}
	} else if cursorErr != sql.ErrNoRows {
		return cursorErr
	}
	if _, err = tx.Exec("INSERT INTO cursors(epoch,conversation_id,cursor) VALUES(?,?,?) ON CONFLICT(epoch,conversation_id) DO UPDATE SET cursor=excluded.cursor", s.epoch, conversation, cursor); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *localStore) cursor(conversation string) (string, error) {
	var cursor string
	err := s.db.QueryRow("SELECT cursor FROM cursors WHERE epoch=? AND conversation_id=?", s.epoch, conversation).Scan(&cursor)
	if err == sql.ErrNoRows {
		return "0", nil
	}
	return cursor, err
}
func (s *localStore) history(conversation string) ([]json.RawMessage, error) {
	if err := s.syncReady(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query("SELECT m.payload FROM messages m LEFT JOIN sync_terminal g ON g.epoch=m.epoch AND g.id=m.id WHERE m.epoch=? AND m.conversation_id=? AND g.id IS NULL ORDER BY m.created_at,m.id", s.epoch, conversation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []json.RawMessage{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		result = append(result, json.RawMessage(raw))
	}
	return result, rows.Err()
}
func (s *localStore) draft(conversation, text string) error {
	_, err := s.db.Exec("INSERT INTO drafts(epoch,conversation_id,text,updated_at) VALUES(?,?,?,?) ON CONFLICT(epoch,conversation_id) DO UPDATE SET text=excluded.text,updated_at=excluded.updated_at", s.epoch, conversation, text, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *localStore) drafts() (map[string]string, error) {
	rows, err := s.db.Query("SELECT conversation_id,text FROM drafts WHERE epoch=? ORDER BY conversation_id", s.epoch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var id, text string
		if err = rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		result[id] = text
	}
	return result, rows.Err()
}
func (s *localStore) importLegacy(path, confirmedDeployment string) error {
	if confirmedDeployment != s.identity.DeploymentID {
		return fmt.Errorf("explicit source deployment confirmation required")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	items := map[string]pending{}
	for scanner.Scan() {
		line := scanner.Bytes()
		hash.Write(line)
		hash.Write([]byte{'\n'})
		var p pending
		if err = json.Unmarshal(line, &p); err != nil {
			return fmt.Errorf("legacy integrity error; source retained")
		}
		if p.DeploymentID != s.identity.DeploymentID || p.UserID != s.identity.UserID || p.ClientMsgID == "" {
			return fmt.Errorf("legacy identity mismatch")
		}
		if p.RestoreEpoch == "" {
			p.RestoreEpoch = "1"
		}
		if old, ok := items[p.ClientMsgID]; ok {
			a, _ := json.Marshal(old.payload())
			b, _ := json.Marshal(p.payload())
			if string(a) != string(b) || old.ConversationID != p.ConversationID {
				return fmt.Errorf("legacy same ID has different payload")
			}
		}
		items[p.ClientMsgID] = p
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var one int
	err = tx.QueryRow("SELECT 1 FROM imports WHERE sha256=?", digest).Scan(&one)
	if err == nil {
		return fmt.Errorf("legacy source already imported")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	for _, p := range items {
		state := p.State
		if state != "confirmed" && state != "frozen" {
			if p.RestoreEpoch != s.epoch {
				state = "frozen"
			} else {
				state = "pending"
			}
		}
		p.State = state
		row, _ := json.Marshal(p)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err = tx.Exec("INSERT INTO outbox(client_msg_id,conversation_id,payload,payload_hash,restore_epoch,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", p.ClientMsgID, p.ConversationID, string(row), operationDigest(p), p.RestoreEpoch, state, now, now); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT INTO imports(sha256,source,imported_at) VALUES(?,?,?)", digest, filepath.Base(path), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
