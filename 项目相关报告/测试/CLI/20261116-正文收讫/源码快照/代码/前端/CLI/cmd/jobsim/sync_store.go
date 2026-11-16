// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type syncControl struct {
	syncBinding
	SchemaVersion         string          `json:"schemaVersion,omitempty"`
	RegistrationRequestID string          `json:"registrationRequestId"`
	Guard                 string          `json:"guard"`
	Position              string          `json:"position"`
	Cursor                string          `json:"cursor,omitempty"`
	Permission            string          `json:"permission,omitempty"`
	Classification        string          `json:"classification"`
	Restriction           string          `json:"restriction,omitempty"`
	SnapshotRequest       json.RawMessage `json:"snapshotRequest,omitempty"`
	Manifest              json.RawMessage `json:"manifest,omitempty"`
}

func (v syncControl) schema() string {
	if v.SchemaVersion == "2" {
		return "2"
	}
	return "1"
}

type syncProof struct {
	Original pending
	Result   json.RawMessage
	Guard    string
}
type syncSQL interface {
	Exec(string, ...any) (sql.Result, error)
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

func syncMigrate(tx *sql.Tx, version int) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS sync_refs(epoch TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_objects(epoch TEXT NOT NULL,id TEXT NOT NULL,kind TEXT NOT NULL,state TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_pages(epoch TEXT NOT NULL,snapshot TEXT NOT NULL,page INTEGER NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,snapshot,page));
CREATE TABLE IF NOT EXISTS sync_events(epoch TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_terminal(epoch TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_cleanup(epoch TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_retained(epoch TEXT NOT NULL,id TEXT NOT NULL,conversation_id TEXT NOT NULL,created_at TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_eligibility(epoch TEXT NOT NULL,id TEXT NOT NULL,version TEXT NOT NULL,body_hash TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_receipts(epoch TEXT NOT NULL,client_id TEXT NOT NULL,conversation_id TEXT NOT NULL,payload_hash TEXT NOT NULL,server_id TEXT NOT NULL,PRIMARY KEY(epoch,client_id));
CREATE TABLE IF NOT EXISTS sync_retained_conversations(epoch TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS sync_stream_archive(epoch TEXT NOT NULL,stream TEXT NOT NULL,kind TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,stream,kind,id));
CREATE TABLE IF NOT EXISTS self_operations(id TEXT PRIMARY KEY,kind TEXT NOT NULL,conversation TEXT NOT NULL,epoch TEXT NOT NULL,method TEXT NOT NULL,route TEXT NOT NULL,body TEXT NOT NULL,body_hash TEXT NOT NULL,state TEXT NOT NULL,result TEXT NOT NULL DEFAULT '',created TEXT NOT NULL);`)
	if err != nil {
		return err
	}
	if err = receiptMigrate(tx); err != nil {
		return err
	}
	var physical string
	err = tx.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical)
	if errors.Is(err, sql.ErrNoRows) {
		var responsibility int
		if err = tx.QueryRow("SELECT count(*) FROM metadata WHERE key='sync-responsibility' OR key LIKE 'sync-control:%'").Scan(&responsibility); err != nil {
			return err
		}
		if version >= 4 || responsibility != 0 {
			return syncRefuse("SYNC_STORE_INVALID")
		}
		physical, err = syncNewUUID()
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO metadata VALUES('sync-physical-generation',?)", physical)
	} else if err == nil && !syncUUIDPattern.MatchString(physical) {
		return syncRefuse("SYNC_STORE_INVALID")
	}
	return err
}
func syncLoadControl(q syncSQL, epoch string) (syncControl, bool, error) {
	var value syncControl
	var raw string
	err := q.QueryRow("SELECT value FROM metadata WHERE key=?", "sync-control:"+epoch).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	if syncDecode([]byte(raw), &value) != nil || !syncUUIDPattern.MatchString(value.Guard) || !syncUUIDPattern.MatchString(value.RegistrationRequestID) || value.SchemaVersion != "" && value.SchemaVersion != "1" && value.SchemaVersion != "2" {
		return value, true, syncRefuse("SYNC_STORE_INVALID")
	}
	if _, err = syncCounter(value.Position); err != nil {
		return value, true, err
	}
	if _, err = syncCounter(value.Classification); err != nil {
		return value, true, err
	}
	if err = value.syncBinding.check(value.syncBinding, value.StreamID != ""); err != nil {
		return value, true, err
	}
	return value, true, nil
}
func (s *localStore) syncResponsible() (bool, error) {
	var n int
	err := s.db.QueryRow("SELECT count(*) FROM metadata WHERE key='sync-responsibility' OR key LIKE 'sync-control:%'").Scan(&n)
	return n > 0, err
}
func syncSaveControl(tx *sql.Tx, epoch string, v syncControl) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO metadata VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "sync-control:"+epoch, string(raw))
	return err
}
func syncCheckControl(actual, expected syncControl) error {
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(expected)
	if !bytes.Equal(a, b) {
		return syncRefuse("SYNC_LOCAL_GUARD_CHANGED")
	}
	return nil
}
func (s *localStore) ensureSync() (syncControl, error) {
	var empty syncControl
	if s.identity.ConnectionProfile == "" || s.identity.Space != "personal" || !syncUUIDPattern.MatchString(s.identity.DeploymentID) || !syncUUIDPattern.MatchString(s.identity.UserID) || !syncUUIDPattern.MatchString(s.identity.DeviceID) {
		return empty, syncRefuse("SYNC_NATIVE_IDENTITY_REQUIRED")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	control, present, err := syncLoadControl(tx, s.epoch)
	if err != nil {
		return control, err
	}
	var generation string
	if err = tx.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&generation); err != nil || !syncUUIDPattern.MatchString(generation) {
		return control, syncRefuse("SYNC_STORE_INVALID")
	}
	expected := syncBinding{ScopeID: s.identity.DeploymentID, PrincipalID: s.identity.UserID, SourceContextID: s.identity.Space, DeviceID: s.identity.DeviceID, StoreGeneration: generation, RestoreEpoch: s.epoch}
	if present {
		if err = control.syncBinding.check(expected, false); err != nil {
			return control, err
		}
	} else {
		request, e := syncNewUUID()
		if e != nil {
			return control, e
		}
		guard, e := syncNewUUID()
		if e != nil {
			return control, e
		}
		control = syncControl{syncBinding: expected, RegistrationRequestID: request, Guard: guard, Position: "0", Classification: "0", Restriction: "SYNC_BASELINE_REQUIRED"}
		if err = archiveSyncCloud(tx, s.epoch); err != nil {
			return control, err
		}
		if err = syncSaveControl(tx, s.epoch, control); err != nil {
			return control, err
		}
	}
	if _, err = tx.Exec("INSERT INTO metadata VALUES('sync-responsibility','1') ON CONFLICT(key) DO NOTHING"); err != nil {
		return control, err
	}
	return control, tx.Commit()
}
func (s *localStore) updateSync(expected, next syncControl, page *syncPage) (syncControl, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return expected, err
	}
	defer tx.Rollback()
	current, ok, err := syncLoadControl(tx, s.epoch)
	if err != nil {
		return expected, err
	}
	if !ok {
		return expected, syncRefuse("SYNC_STORE_INVALID")
	}
	if err = syncCheckControl(current, expected); err != nil {
		return expected, err
	}
	if next.schema() != expected.schema() || next.RegistrationRequestID != expected.RegistrationRequestID {
		return expected, syncRefuse("SYNC_SCHEMA_TRANSITION_REQUIRED")
	}
	if err = next.syncBinding.check(expected.syncBinding, false); err != nil {
		return expected, err
	}
	if page != nil {
		var manifest syncManifest
		if syncDecode(next.Manifest, &manifest) != nil || !syncUUIDPattern.MatchString(manifest.SnapshotID) {
			return expected, syncRefuse("SYNC_STAGE_INVALID")
		}
		raw, e := json.Marshal(page)
		if e != nil {
			return expected, e
		}
		if _, err = tx.Exec("INSERT INTO sync_pages VALUES(?,?,?,?) ON CONFLICT(epoch,snapshot,page) DO UPDATE SET payload=excluded.payload", s.epoch, manifest.SnapshotID, page.Index, string(raw)); err != nil {
			return expected, err
		}
	}
	next.Guard, err = syncNewUUID()
	if err != nil {
		return expected, err
	}
	if err = syncSaveControl(tx, s.epoch, next); err != nil {
		return expected, err
	}
	return next, tx.Commit()
}
func syncPersistable(raw json.RawMessage, global bool) (json.RawMessage, error) {
	var body map[string]any
	if err := syncDecode(raw, &body); err != nil || body == nil {
		return nil, syncRefuse("SYNC_INVALID_OBJECT")
	}
	delete(body, "forwardAdmission")
	delete(body, "replyAdmission")
	delete(body, "draftAdmission")
	if body["forwarded"] == true {
		maskForward(body)
	}
	if global {
		body["deleted"], body["text"], body["contentUnavailable"] = true, "", false
		for _, key := range []string{"attachmentId", "attachmentURL", "attachmentName", "replyToId", "reactions", "mentionedUserIds", "mentionAll", "mentionedMe", "pinned", "favorite"} {
			delete(body, key)
		}
	}
	return json.Marshal(body)
}
func archiveSyncCloud(tx *sql.Tx, epoch string) error {
	rows, err := tx.Query("SELECT id,conversation_id,created_at,payload FROM messages WHERE epoch=?", epoch)
	if err != nil {
		return err
	}
	type row struct{ id, conversation, created, raw string }
	var values []row
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.id, &r.conversation, &r.created, &r.raw); err != nil {
			rows.Close()
			return err
		}
		values = append(values, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range values {
		var state struct {
			Forwarded   bool `json:"forwarded"`
			Deleted     bool `json:"deleted"`
			Unavailable bool `json:"contentUnavailable"`
		}
		if err = json.Unmarshal([]byte(r.raw), &state); err != nil {
			return err
		}
		if state.Forwarded || state.Deleted || state.Unavailable {
			continue
		}
		if _, err = tx.Exec("INSERT INTO sync_retained VALUES(?,?,?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET payload=excluded.payload,created_at=excluded.created_at", epoch, r.id, r.conversation, r.created, r.raw); err != nil {
			return err
		}
	}
	rows, err = tx.Query("SELECT id,payload FROM conversations WHERE epoch=?", epoch)
	if err != nil {
		return err
	}
	var conversations []row
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.id, &r.raw); err != nil {
			rows.Close()
			return err
		}
		conversations = append(conversations, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range conversations {
		var value map[string]any
		if err = syncDecode([]byte(r.raw), &value); err != nil {
			return err
		}
		raw, _ := json.Marshal(map[string]any{"id": r.id, "kind": value["kind"], "title": value["title"], "readOnly": true, "localHistory": true})
		if _, err = tx.Exec("INSERT INTO sync_retained_conversations VALUES(?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET payload=excluded.payload", epoch, r.id, string(raw)); err != nil {
			return err
		}
	}
	for _, table := range []string{"messages", "conversations", "cursors"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE epoch=?", epoch); err != nil {
			return err
		}
	}
	_, err = tx.Exec("DELETE FROM sync_objects WHERE epoch=?", epoch)
	return err
}
func (s *localStore) restrictSync(reason string, obsolete bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	control, exists, err := syncLoadControl(tx, s.epoch)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err = archiveSyncCloud(tx, s.epoch); err != nil {
		return err
	}
	control.Restriction = reason
	control.Guard, err = syncNewUUID()
	if err != nil {
		return err
	}
	if obsolete {
		control.SnapshotRequest = nil
		control.Manifest = nil
		if _, err = tx.Exec("DELETE FROM sync_pages WHERE epoch=?", s.epoch); err != nil {
			return err
		}
	}
	if err = syncSaveControl(tx, s.epoch, control); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *localStore) syncReady() error {
	var retired int
	if err := s.db.QueryRow("SELECT count(*) FROM metadata WHERE key='auth_retired' AND value='1'").Scan(&retired); err != nil {
		return err
	}
	if retired != 0 {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	responsible, err := s.syncResponsible()
	if err != nil {
		return err
	}
	if !responsible {
		return nil
	}
	if err = s.checkSyncFile(); err != nil {
		return err
	}
	control, ok, err := syncLoadControl(s.db, s.epoch)
	if err != nil {
		return err
	}
	if !ok || control.Restriction != "" || control.Cursor == "" {
		return syncRefuse("SYNC_NOT_READY")
	}
	if err = s.checkSyncBinding(control); err != nil {
		return err
	}
	return nil
}
func (s *localStore) retireSyncIdentity(reason string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO metadata VALUES('auth_retired','1') ON CONFLICT(key) DO UPDATE SET value='1'"); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE outbox SET state='frozen',freeze_reason=CASE WHEN freeze_reason='' THEN 'AUTH_CONTEXT_CHANGED' ELSE freeze_reason END WHERE state='pending'"); err != nil {
		return err
	}
	control, present, err := syncLoadControl(tx, s.epoch)
	if err != nil {
		return err
	}
	if present {
		if err = archiveSyncCloud(tx, s.epoch); err != nil {
			return err
		}
		control.Restriction = reason
		control.Guard, err = syncNewUUID()
		if err != nil {
			return err
		}
		if err = syncSaveControl(tx, s.epoch, control); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *localStore) checkSyncFile() error {
	current, err := os.Lstat(s.path)
	if err != nil || s.fileInfo == nil || !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(s.fileInfo, current) {
		return syncRefuse("SYNC_STORE_INVALID")
	}
	return nil
}
func (s *localStore) checkSyncBinding(control syncControl) error {
	if err := s.checkSyncFile(); err != nil {
		return err
	}
	var physical string
	if err := s.db.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); err != nil {
		return syncRefuse("SYNC_STORE_INVALID")
	}
	expected := syncBinding{ScopeID: s.identity.DeploymentID, PrincipalID: s.identity.UserID, SourceContextID: s.identity.Space, DeviceID: s.identity.DeviceID, StoreGeneration: physical, RestoreEpoch: s.epoch}
	return control.syncBinding.check(expected, false)
}
func (s *localStore) localHistory(conversation string) ([]json.RawMessage, error) {
	var retired int
	if err := s.db.QueryRow("SELECT count(*) FROM metadata WHERE key='auth_retired' AND value='1'").Scan(&retired); err != nil {
		return nil, err
	}
	if retired != 0 {
		return nil, syncRefuse("SYNC_REVALIDATION_REQUIRED")
	}
	control, present, err := syncLoadControl(s.db, s.epoch)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, syncRefuse("SYNC_REVALIDATION_REQUIRED")
	}
	if err = s.checkSyncBinding(control); err != nil {
		return nil, err
	}
	switch control.Restriction {
	case "UNAUTHORIZED", "REFRESH_REAUTH_REQUIRED", "RESTORE_EPOCH_CHANGED", "DEPLOYMENT_CHANGED", "SYNC_CONTEXT_CHANGED", "STORE_GENERATION_CHANGED", "SYNC_STORE_INVALID":
		return nil, syncRefuse("SYNC_REVALIDATION_REQUIRED")
	}
	rows, err := s.db.Query(`SELECT r.payload,e.version,e.body_hash FROM sync_retained r JOIN sync_eligibility e ON e.epoch=r.epoch AND e.id=r.id LEFT JOIN sync_terminal g ON g.epoch=r.epoch AND g.id=r.id WHERE r.epoch=? AND r.conversation_id=? AND g.id IS NULL ORDER BY r.created_at,r.id`, s.epoch, conversation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []json.RawMessage{}
	for rows.Next() {
		var raw, version, hash string
		if err = rows.Scan(&raw, &version, &hash); err != nil {
			return nil, err
		}
		var state struct {
			Version     string `json:"version"`
			Forwarded   bool   `json:"forwarded"`
			Deleted     bool   `json:"deleted"`
			Unavailable bool   `json:"contentUnavailable"`
		}
		if syncDecode([]byte(raw), &state) != nil {
			return nil, syncRefuse("SYNC_LOCAL_INTEGRITY")
		}
		if !state.Forwarded && !state.Deleted && !state.Unavailable && state.Version == version && fingerprint(raw) == hash {
			result = append(result, json.RawMessage(raw))
		}
	}
	return result, rows.Err()
}
func syncEraseTx(tx *sql.Tx, epoch, id string) error {
	for _, table := range []string{"messages", "sync_retained", "sync_eligibility"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE epoch=? AND id=?", epoch, id); err != nil {
			return err
		}
	}
	// Only an earlier complete readonly proof permits disposal of an acknowledged original.
	if _, err := tx.Exec("DELETE FROM outbox WHERE state='confirmed' AND server_id=? AND EXISTS(SELECT 1 FROM sync_receipts r WHERE r.epoch=outbox.restore_epoch AND r.client_id=outbox.client_msg_id AND r.payload_hash=outbox.payload_hash AND r.server_id=?)", id, id); err != nil {
		return err
	}
	// Announcements contain only references; no copied source body is retained.
	return nil
}
func syncRecordGlobalTx(tx *sql.Tx, epoch, id string) error {
	fact, _ := json.Marshal(map[string]any{"id": id, "deleted": true})
	if _, err := tx.Exec("INSERT OR IGNORE INTO sync_terminal VALUES(?,?,?)", epoch, id, string(fact)); err != nil {
		return err
	}
	if err := syncEraseTx(tx, epoch, id); err != nil {
		return err
	}
	var raw string
	if err := tx.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&raw); err != nil {
		return err
	}
	var identity localIdentity
	if json.Unmarshal([]byte(raw), &identity) != nil {
		return syncRefuse("SYNC_STORE_INVALID")
	}
	if identity.ConnectionProfile == "" || !syncUUIDPattern.MatchString(id) {
		return nil
	}
	proof, _ := json.Marshal(struct {
		Identity localIdentity `json:"identity"`
		Epoch    string        `json:"epoch"`
		ID       string        `json:"id"`
	}{identity, epoch, id})
	_, err := tx.Exec("INSERT OR IGNORE INTO sync_cleanup VALUES(?,?,?)", epoch, id, string(proof))
	return err
}
func (s *localStore) applySync(tx *sql.Tx, p syncProjection, control syncControl) error {
	epoch := s.epoch
	if err := p.Ref.schemaCheck(control.schema()); err != nil {
		return err
	}
	selfBody, selfErr := validateSelfProjection(p, control)
	if selfErr != nil {
		return selfErr
	}
	p.Object = selfBody
	var terminal string
	terminalErr := tx.QueryRow("SELECT payload FROM sync_terminal WHERE epoch=? AND id=?", epoch, p.Ref.ID).Scan(&terminal)
	if terminalErr != nil && !errors.Is(terminalErr, sql.ErrNoRows) {
		return terminalErr
	}
	known := terminalErr == nil
	if p.Ref.Kind == "message" && (p.State == "global" || known) {
		fact, _ := json.Marshal(map[string]any{"id": p.Ref.ID, "deleted": true, "version": p.CurrentRevision, "ref": p.Ref})
		if _, err := tx.Exec("INSERT INTO sync_terminal VALUES(?,?,?) ON CONFLICT(epoch,id) DO NOTHING", epoch, p.Ref.ID, string(fact)); err != nil {
			return err
		}
		if err := syncEraseTx(tx, epoch, p.Ref.ID); err != nil {
			return err
		}
		proof, _ := json.Marshal(struct {
			Identity localIdentity `json:"identity"`
			Epoch    string        `json:"epoch"`
			ID       string        `json:"id"`
		}{s.identity, epoch, p.Ref.ID})
		_, err := tx.Exec("INSERT INTO sync_cleanup VALUES(?,?,?) ON CONFLICT(epoch,id) DO NOTHING", epoch, p.Ref.ID, string(proof))
		return err
	}
	var prior string
	err := tx.QueryRow("SELECT payload FROM sync_refs WHERE epoch=? AND id=?", epoch, p.Ref.key()).Scan(&prior)
	if err == nil {
		var old syncProjection
		if syncDecode([]byte(prior), &old) != nil {
			return syncRefuse("SYNC_LOCAL_INTEGRITY")
		}
		if old.CurrentRevision != "" && p.CurrentRevision != "" {
			a, _ := syncCounter(old.CurrentRevision)
			b, _ := syncCounter(p.CurrentRevision)
			if a > b {
				return nil
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	meta := p
	meta.Object = nil
	meta.BodyEnvelope = ""
	metadata, _ := json.Marshal(meta)
	if _, err = tx.Exec("INSERT INTO sync_refs VALUES(?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET payload=excluded.payload", epoch, p.Ref.key(), string(metadata)); err != nil {
		return err
	}
	if p.Ref.Kind == "message" {
		if len(p.Object) == 0 || p.State == "redacted" || p.State == "unknown" {
			return syncRefuse("SYNC_PROJECTION_UNKNOWN")
		}
		m, err := syncValidateProjectionMessage(p.Object, false)
		if err != nil {
			return err
		}
		if m.ID != p.Ref.ID || p.Ref.ConversationID != "" && m.ConversationID != p.Ref.ConversationID {
			return syncRefuse("SYNC_OBJECT_ID_MISMATCH")
		}
		raw, err := syncPersistable(p.Object, false)
		if err != nil {
			return err
		}
		if p.State == "unavailable" {
			var body map[string]any
			_ = syncDecode(raw, &body)
			body["text"] = ""
			body["contentUnavailable"] = true
			for _, key := range []string{"attachmentId", "attachmentURL", "replyToId", "reactions"} {
				delete(body, key)
			}
			raw, _ = json.Marshal(body)
		}
		if _, err = writeCached(tx, epoch, raw); err != nil {
			return err
		}
		var body struct {
			Forwarded   bool `json:"forwarded"`
			Deleted     bool `json:"deleted"`
			Unavailable bool `json:"contentUnavailable"`
		}
		_ = json.Unmarshal(raw, &body)
		if p.State == "available" && !body.Forwarded && !body.Deleted && !body.Unavailable {
			_, err = tx.Exec("INSERT INTO sync_eligibility VALUES(?,?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET version=excluded.version,body_hash=excluded.body_hash", epoch, m.ID, m.Version, fingerprint(string(raw)))
			if err != nil {
				return err
			}
		}
		return nil
	}
	var object map[string]any
	if len(p.Object) != 0 {
		if err = syncDecode(p.Object, &object); err != nil {
			return err
		}
	}
	if p.Ref.Kind == "announcement" {
		delete(object, "message")
	}
	if p.Ref.Kind == "conversation" {
		if p.State == "available" {
			id, _ := object["id"].(string)
			if id != p.Ref.ID {
				return syncRefuse("SYNC_OBJECT_ID_MISMATCH")
			}
			kind, _ := object["kind"].(string)
			if kind != "direct" && kind != "group" && kind != "self" {
				return syncRefuse("SYNC_INVALID_CONVERSATION")
			}
			members, ok := object["memberIds"].([]any)
			if !ok {
				return syncRefuse("SYNC_INVALID_CONVERSATION")
			}
			for _, member := range members {
				id, ok := member.(string)
				if !ok || !syncUUIDPattern.MatchString(id) {
					return syncRefuse("SYNC_INVALID_CONVERSATION")
				}
			}
			for _, key := range []string{"membershipGeneration", "roleGeneration"} {
				if value, ok := object[key]; ok {
					v, ok := value.(string)
					if !ok {
						return syncRefuse("SYNC_INVALID_COUNTER")
					}
					if _, err := syncCounter(v); err != nil {
						return err
					}
				}
			}
			raw, _ := json.Marshal(object)
			_, err = tx.Exec("INSERT INTO conversations VALUES(?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET payload=excluded.payload", epoch, p.Ref.ID, string(raw))
		} else {
			_, err = tx.Exec("DELETE FROM conversations WHERE epoch=? AND id=?", epoch, p.Ref.ID)
		}
		if err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(map[string]any{"ref": p.Ref, "state": p.State, "object": object})
	_, err = tx.Exec("INSERT INTO sync_objects VALUES(?,?,?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET state=excluded.state,payload=excluded.payload", epoch, p.Ref.key(), p.Ref.Kind, p.State, string(raw))
	return err
}

func (s *localStore) commitSync(expected, next syncControl, m *syncManifest, pages []syncPage, projections []syncProjection, d syncDelta, proofs []syncProof, current func() bool) error {
	if !current() {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actual, ok, err := syncLoadControl(tx, s.epoch)
	if err != nil {
		return err
	}
	if !ok {
		return syncRefuse("SYNC_STORE_INVALID")
	}
	if err = syncCheckControl(actual, expected); err != nil {
		return err
	}
	var physical string
	if err = tx.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); err != nil || physical != expected.StoreGeneration {
		return syncRefuse("SYNC_STORE_INVALID")
	}
	if err = s.checkSyncFile(); err != nil {
		return err
	}
	before, _ := syncCounter(actual.Position)
	after, err := syncCounter(next.Position)
	if err != nil || after < before {
		return syncRefuse("SYNC_REVISION_REGRESSION")
	}
	classification, err := syncCounter(next.Classification)
	oldClass, _ := syncCounter(actual.Classification)
	if err != nil || classification < oldClass {
		return syncRefuse("SYNC_REVISION_REGRESSION")
	}
	if m != nil {
		if err = m.alive(); err != nil {
			return err
		}
		if !syncEqual(expected.Manifest, m.Raw) || len(pages) != len(m.PageManifest) {
			return syncRefuse("SYNC_STAGE_INCOMPLETE")
		}
		var count int
		if err = tx.QueryRow("SELECT count(*) FROM sync_pages WHERE epoch=? AND snapshot=?", s.epoch, m.SnapshotID).Scan(&count); err != nil {
			return err
		}
		if count != len(pages) {
			return syncRefuse("SYNC_STAGE_INCOMPLETE")
		}
		for i, page := range pages {
			var saved string
			if err = tx.QueryRow("SELECT payload FROM sync_pages WHERE epoch=? AND snapshot=? AND page=?", s.epoch, m.SnapshotID, i).Scan(&saved); err != nil {
				return err
			}
			raw, _ := json.Marshal(page)
			if !syncEqual([]byte(saved), raw) {
				return syncRefuse("SYNC_STAGE_INCOMPLETE")
			}
		}
		if err = archiveSyncCloud(tx, s.epoch); err != nil {
			return err
		}
	}
	for _, item := range projections {
		if err = s.applySync(tx, item, next); err != nil {
			return err
		}
	}
	for _, event := range d.Events {
		var v struct {
			Origin string `json:"originEventId"`
			From   string `json:"from"`
			To     string `json:"to"`
		}
		if err = json.Unmarshal(event, &v); err != nil {
			return err
		}
		key := v.Origin
		if key == "" {
			key = "range:" + v.From + ":" + v.To
		}
		var old string
		err = tx.QueryRow("SELECT payload FROM sync_events WHERE epoch=? AND id=?", s.epoch, key).Scan(&old)
		if err == nil && !syncEqual([]byte(old), event) {
			return syncRefuse("SYNC_ORIGIN_CONFLICT")
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec("INSERT OR IGNORE INTO sync_events VALUES(?,?,?)", s.epoch, key, string(event)); err != nil {
			return err
		}
	}
	for _, proof := range proofs {
		if err = s.confirmSyncTx(tx, proof); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("UPDATE outbox SET state='frozen',freeze_reason='SYNC_SCOPE_UNAVAILABLE' WHERE state='pending' AND restore_epoch=? AND conversation_id NOT IN(SELECT id FROM conversations WHERE epoch=?)", s.epoch, s.epoch); err != nil {
		return err
	}
	next.Restriction = ""
	next.Guard, err = syncNewUUID()
	if err != nil {
		return err
	}
	next.SnapshotRequest = nil
	next.Manifest = nil
	if err = syncSaveControl(tx, s.epoch, next); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM sync_pages WHERE epoch=?", s.epoch); err != nil {
		return err
	}
	if !current() {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	return tx.Commit()
}
func (s *localStore) confirmSyncTx(tx *sql.Tx, proof syncProof) error {
	control, present, guardErr := syncLoadControl(tx, s.epoch)
	if guardErr != nil {
		return guardErr
	}
	if !present || proof.Guard == "" || proof.Guard != control.Guard {
		return syncRefuse("SYNC_LOCAL_GUARD_CHANGED")
	}
	m, err := syncValidateMessage(proof.Result, false)
	if err != nil {
		return err
	}
	p := proof.Original
	if m.ClientMsgID != p.ClientMsgID || m.SenderID != s.identity.UserID || m.ConversationID != p.ConversationID || p.DeploymentID != s.identity.DeploymentID || p.UserID != s.identity.UserID || p.RestoreEpoch != s.epoch {
		return syncRefuse("SYNC_OUTBOX_RESULT_INVALID")
	}
	var hash string
	err = tx.QueryRow("SELECT payload_hash FROM outbox WHERE client_msg_id=? AND conversation_id=? AND restore_epoch=?", p.ClientMsgID, p.ConversationID, s.epoch).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if hash != operationDigest(p) {
		return syncRefuse("SYNC_OUTBOX_ORIGINAL_CHANGED")
	}
	var priorID, priorHash, priorRoute string
	priorErr := tx.QueryRow("SELECT server_id,payload_hash,conversation_id FROM sync_receipts WHERE epoch=? AND client_id=?", s.epoch, p.ClientMsgID).Scan(&priorID, &priorHash, &priorRoute)
	if priorErr == nil && (priorID != m.ID || priorHash != hash || priorRoute != p.ConversationID) {
		return syncRefuse("SYNC_OUTBOX_RESULT_CONFLICT")
	}
	if priorErr != nil && !errors.Is(priorErr, sql.ErrNoRows) {
		return priorErr
	}
	_, err = tx.Exec("UPDATE outbox SET state='confirmed',server_id=?,updated_at=? WHERE client_msg_id=? AND payload_hash=?", m.ID, time.Now().UTC().Format(time.RFC3339Nano), p.ClientMsgID, hash)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO sync_receipts VALUES(?,?,?,?,?) ON CONFLICT(epoch,client_id) DO UPDATE SET server_id=excluded.server_id", s.epoch, p.ClientMsgID, p.ConversationID, hash, m.ID); err != nil {
		return err
	}
	var state struct {
		Deleted bool `json:"deleted"`
	}
	_ = json.Unmarshal(proof.Result, &state)
	if state.Deleted {
		return syncRecordGlobalTx(tx, s.epoch, m.ID)
	}
	return nil
}
func (s *localStore) confirmReadOnly(p pending, raw json.RawMessage, guard string) error {
	responsible, err := s.syncResponsible()
	if err != nil {
		return err
	}
	if !responsible {
		return s.confirm(p, raw)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.confirmSyncTx(tx, syncProof{Original: p, Result: raw, Guard: guard}); err != nil {
		return err
	}
	control, ok, err := syncLoadControl(tx, s.epoch)
	if err != nil {
		return err
	}
	if !ok {
		return syncRefuse("SYNC_STORE_INVALID")
	}
	control.Guard, err = syncNewUUID()
	if err != nil {
		return err
	}
	if err = syncSaveControl(tx, s.epoch, control); err != nil {
		return err
	}
	return tx.Commit()
}

// Cleanup is durable across databases, but never claimed as a cross-SQLite transaction.
func (s *localStore) drainSyncCleanup() error {
	rows, err := s.db.Query("SELECT id,payload FROM sync_cleanup WHERE epoch=? ORDER BY id", s.epoch)
	if err != nil {
		return err
	}
	type item struct{ id, raw string }
	var work []item
	for rows.Next() {
		var r item
		if err = rows.Scan(&r.id, &r.raw); err != nil {
			rows.Close()
			return err
		}
		work = append(work, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range work {
		var proof struct {
			Identity localIdentity `json:"identity"`
			Epoch    string        `json:"epoch"`
			ID       string        `json:"id"`
		}
		if syncDecode([]byte(r.raw), &proof) != nil || proof.Identity != s.identity || proof.Epoch != s.epoch || proof.ID != r.id || !syncUUIDPattern.MatchString(r.id) {
			return syncRefuse("SYNC_CLEANUP_CONTEXT_INVALID")
		}
		if err = s.cleanTrustedCLIStores(r.id); err != nil {
			return err
		}
		if _, err = s.db.Exec("DELETE FROM sync_cleanup WHERE epoch=? AND id=? AND payload=?", s.epoch, r.id, r.raw); err != nil {
			return err
		}
	}
	return nil
}
func (s *localStore) cleanTrustedCLIStores(id string) error {
	profileRoot := filepath.Dir(filepath.Dir(s.path))
	if s.identity.ConnectionProfile == "" || filepath.Base(profileRoot) != s.identity.ConnectionProfile {
		return syncRefuse("SYNC_CLEANUP_PROFILE_UNKNOWN")
	}
	entries, err := os.ReadDir(profileRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(profileRoot, entry.Name(), "client.sqlite")
		if filepath.Clean(path) == filepath.Clean(s.path) {
			continue
		}
		info, e := os.Lstat(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return syncRefuse("SYNC_CLEANUP_UNSAFE_PATH")
		}
		trusted, err := s.cleanCandidate(path, id, false, info)
		if err != nil {
			return err
		}
		if !trusted {
			continue
		}
		if _, err = s.cleanCandidate(path, id, true, info); err != nil {
			return err
		}
	}
	return nil
}
func (s *localStore) cleanCandidate(path, id string, write bool, original os.FileInfo) (bool, error) {
	current, err := os.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(original, current) {
		return false, syncRefuse("SYNC_CLEANUP_FILE_CHANGED")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	if write {
		q.Set("mode", "rw")
	} else {
		q.Set("mode", "ro")
	}
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return false, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var raw, epoch string
	if tx.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&raw) != nil || tx.QueryRow("SELECT value FROM metadata WHERE key='epoch'").Scan(&epoch) != nil {
		return false, nil
	}
	var identity localIdentity
	if json.Unmarshal([]byte(raw), &identity) != nil || identity.Origin != s.identity.Origin || identity.ConnectionProfile != s.identity.ConnectionProfile || identity.DeploymentID != s.identity.DeploymentID || identity.UserID != s.identity.UserID || identity.Space != "personal" || epoch != s.epoch {
		return false, nil
	}
	canonical, _ := json.Marshal(identity)
	if filepath.Base(filepath.Dir(path)) != fingerprint(string(canonical)) || !syncUUIDPattern.MatchString(identity.DeviceID) {
		return false, nil
	}
	if !write {
		return true, nil
	}
	var version int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return false, err
	}
	if version > 4 {
		return false, nil
	}
	if err = syncMigrate(tx, version); err != nil {
		return false, err
	}
	fact, _ := json.Marshal(map[string]any{"id": id, "deleted": true})
	if _, err = tx.Exec("INSERT OR IGNORE INTO sync_terminal VALUES(?,?,?)", epoch, id, string(fact)); err != nil {
		return false, err
	}
	if err = syncEraseTx(tx, epoch, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec("PRAGMA user_version=4"); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *localStore) syncSummary() (map[string]any, error) {
	control, present, err := syncLoadControl(s.db, s.epoch)
	if err != nil {
		return nil, err
	}
	var pending int
	if err = s.db.QueryRow("SELECT count(*) FROM sync_cleanup WHERE epoch=?", s.epoch).Scan(&pending); err != nil {
		return nil, err
	}
	return map[string]any{"registered": present, "schemaVersion": control.schema(), "streamId": control.StreamID, "storeGeneration": control.StoreGeneration, "position": control.Position, "restriction": control.Restriction, "cleanupPending": pending, "qualifiedAck": false, "afterReceiptEnabled": false}, nil
}
