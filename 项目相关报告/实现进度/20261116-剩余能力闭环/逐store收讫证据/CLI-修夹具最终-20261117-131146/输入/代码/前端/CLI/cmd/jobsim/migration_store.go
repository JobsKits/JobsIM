// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"time"
)

type cliMigrationSelection struct {
	ID      string `json:"messageId"`
	Content string `json:"contentVersion"`
	Mode    string `json:"objectMode"`
}
type cliMigrationIntent struct {
	ID           string                  `json:"requestId"`
	Kind         string                  `json:"kind"`
	Binding      dataExportBinding       `json:"binding"`
	Created      string                  `json:"createdAt"`
	Selection    []cliMigrationSelection `json:"selection,omitempty"`
	Pairing      string                  `json:"pairingId,omitempty"`
	TargetDevice string                  `json:"targetDeviceId,omitempty"`
	TargetStore  string                  `json:"targetStoreGeneration,omitempty"`
	SourceDigest string                  `json:"sourceDigest,omitempty"`
	Migration    string                  `json:"migrationId,omitempty"`
	Digest       string                  `json:"manifestDigest,omitempty"`
	State        string                  `json:"state"`
}

func (s *localStore) cliMigrationControl(q syncSQL, b dataExportBinding, current func() bool) error {
	if err := ownDeletionStoreGate(s.path, s.identity); err != nil {
		return err
	}
	return s.dataExportControl(q, b, current)
}

func cliMigrationMigrate(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS migration_intents(id TEXT PRIMARY KEY,epoch TEXT NOT NULL,payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS migration_stages(epoch TEXT NOT NULL,id TEXT NOT NULL,digest TEXT NOT NULL,capsule BLOB NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS migration_stage_messages(epoch TEXT NOT NULL,id TEXT NOT NULL,message_id TEXT NOT NULL,PRIMARY KEY(epoch,id,message_id));
CREATE TABLE IF NOT EXISTS migration_archives(epoch TEXT NOT NULL,id TEXT NOT NULL,digest TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,id));
CREATE TABLE IF NOT EXISTS migration_messages(epoch TEXT NOT NULL,archive TEXT NOT NULL,id TEXT NOT NULL,conversation TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(epoch,archive,id));
CREATE TABLE IF NOT EXISTS migration_objects(epoch TEXT NOT NULL,archive TEXT NOT NULL,id TEXT NOT NULL,message_id TEXT NOT NULL,bytes BLOB NOT NULL,digest TEXT NOT NULL,PRIMARY KEY(epoch,archive,id));`)
	return err
}
func cliMigrationValidateIntent(o cliMigrationIntent) error {
	if !syncUUIDPattern.MatchString(o.ID) || !oneOf(o.Kind, "pair", "source", "target") || !oneOf(o.State, "prepared", "unknown", "ready", "installed", "retired") || !syncUUIDPattern.MatchString(o.Binding.Family) || !syncUUIDPattern.MatchString(o.Binding.Physical) || o.Binding.Identity.Space != "personal" || !cliMigrationPositive(o.Binding.Epoch) {
		return syncRefuse("MIGRATION_INTENT_INVALID")
	}
	if _, err := time.Parse(time.RFC3339Nano, o.Created); err != nil {
		return err
	}
	for _, v := range []string{o.Pairing, o.TargetDevice, o.TargetStore, o.Migration} {
		if v != "" && !syncUUIDPattern.MatchString(v) {
			return syncRefuse("MIGRATION_INTENT_INVALID")
		}
	}
	for _, v := range []string{o.SourceDigest, o.Digest} {
		if v != "" && !syncDigestPattern.MatchString(v) {
			return syncRefuse("MIGRATION_INTENT_INVALID")
		}
	}
	if len(o.Selection) > 256 || o.Kind == "source" && (len(o.Selection) == 0 || o.Pairing == "" || o.TargetDevice == "" || o.TargetStore == "") || o.Kind == "target" && o.Migration == "" {
		return syncRefuse("MIGRATION_INTENT_INVALID")
	}
	seen := map[string]bool{}
	for _, r := range o.Selection {
		if seen[r.ID] || !syncUUIDPattern.MatchString(r.ID) || !cliMigrationPositive(r.Content) || !oneOf(r.Mode, "none", "metadata_only", "full") {
			return syncRefuse("MIGRATION_SELECTION_INVALID")
		}
		seen[r.ID] = true
	}
	return nil
}
func cliMigrationLoad(q syncSQL, id string) (cliMigrationIntent, error) {
	var o cliMigrationIntent
	var raw string
	err := q.QueryRow("SELECT payload FROM migration_intents WHERE id=?", id).Scan(&raw)
	if err != nil {
		return o, err
	}
	if cliMigrationDecode([]byte(raw), &o) != nil || o.ID != id || cliMigrationValidateIntent(o) != nil {
		return o, syncRefuse("MIGRATION_INTENT_INVALID")
	}
	return o, nil
}
func (s *localStore) cliMigrationChange(b dataExportBinding, current func() bool, id string, create *cliMigrationIntent, change func(*cliMigrationIntent) error) (cliMigrationIntent, error) {
	var o cliMigrationIntent
	tx, err := s.db.Begin()
	if err != nil {
		return o, err
	}
	defer tx.Rollback()
	if err = s.cliMigrationControl(tx, b, current); err != nil {
		return o, err
	}
	o, err = cliMigrationLoad(tx, id)
	if err == sql.ErrNoRows && create != nil {
		if create.State != "prepared" {
			return o, syncRefuse("MIGRATION_INTENT_INVALID")
		}
		o = *create
		err = nil
	}
	if err != nil {
		return o, err
	}
	if o.Binding != b {
		return o, syncRefuse("MIGRATION_CONTEXT_CHANGED")
	}
	before := o
	if change != nil {
		if err = change(&o); err != nil {
			return o, err
		}
	}
	if o.ID != before.ID || o.Kind != before.Kind || o.Created != before.Created || o.Binding != before.Binding || !syncEqual(cliMigrationJSONBytes(o.Selection), cliMigrationJSONBytes(before.Selection)) || o.Pairing != before.Pairing || o.TargetDevice != before.TargetDevice || o.TargetStore != before.TargetStore || before.SourceDigest != "" && o.SourceDigest != before.SourceDigest || before.Migration != "" && o.Migration != before.Migration || before.Digest != "" && o.Digest != before.Digest || before.State == "installed" && o.State != "installed" || cliMigrationValidateIntent(o) != nil {
		return o, syncRefuse("MIGRATION_ORIGINAL_CHANGED")
	}
	raw, _ := json.Marshal(o)
	_, err = tx.Exec("INSERT INTO migration_intents VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", o.ID, b.Epoch, string(raw))
	if err != nil {
		return o, err
	}
	return o, tx.Commit()
}
func cliMigrationJSONBytes(v any) []byte { raw, _ := json.Marshal(v); return raw }
func cliMigrationEraseTx(tx *sql.Tx, epoch, id string) error {
	// Delete the entire encrypted stage if any listed message became global.
	if _, err := tx.Exec("DELETE FROM migration_stages WHERE epoch=? AND id IN(SELECT id FROM migration_stage_messages WHERE epoch=? AND message_id=?)", epoch, epoch, id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM migration_stage_messages WHERE epoch=? AND id NOT IN(SELECT id FROM migration_stages WHERE epoch=?)", epoch, epoch); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM migration_objects WHERE epoch=? AND message_id=?", epoch, id); err != nil {
		return err
	}
	_, err := tx.Exec("DELETE FROM migration_messages WHERE epoch=? AND id=?", epoch, id)
	return err
}
func cliMigrationRedactForwardTx(tx *sql.Tx, epoch, id string) error {
	// These archives are reference projections, not permanently owned text.
	_, err := tx.Exec(`UPDATE migration_messages SET payload=json_set(payload,'$.text','','$.forwardUnavailable',json('true')) WHERE epoch=? AND id=? AND json_extract(payload,'$.forwarded')=1`, epoch, id)
	return err
}
func (s *localStore) cliMigrationCommit(b dataExportBinding, control syncControl, current func() bool, o cliMigrationIntent, v cliMigrationView, packet cliMigrationPacket, before time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.receiptControl(tx, &control, current, true); err != nil {
		return err
	}
	if err = s.cliMigrationControl(tx, b, current); err != nil {
		return err
	}
	if err = receiptDurability(tx); err != nil {
		return err
	}
	stored, err := cliMigrationLoad(tx, o.ID)
	if err != nil {
		return err
	}
	if !syncEqual(cliMigrationJSONBytes(stored), cliMigrationJSONBytes(o)) || o.Migration != v.ID || o.Digest != v.Digest {
		return syncRefuse("MIGRATION_ORIGINAL_CHANGED")
	}
	var existing string
	err = tx.QueryRow("SELECT digest FROM migration_archives WHERE epoch=? AND id=?", b.Epoch, v.ID).Scan(&existing)
	if err == nil {
		if existing != v.Digest {
			return syncRefuse("MIGRATION_ARCHIVE_CHANGED")
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	for i, ref := range v.Manifest.Messages {
		var terminal int
		if err = tx.QueryRow("SELECT count(*) FROM sync_terminal WHERE epoch=? AND id=?", b.Epoch, ref.ID).Scan(&terminal); err != nil || terminal != 0 {
			return syncRefuse("MIGRATION_GLOBAL_FLOOR")
		}
		var conversation string
		if err = tx.QueryRow("SELECT payload FROM conversations WHERE epoch=? AND id=?", b.Epoch, ref.Conversation).Scan(&conversation); err != nil {
			return syncRefuse("MIGRATION_MEMBERSHIP_UNKNOWN")
		}
		var member struct {
			Generation string `json:"membershipGeneration"`
			State      string `json:"state"`
			Deleted    bool   `json:"deleted"`
		}
		if syncDecode([]byte(conversation), &member) != nil || member.Generation != ref.Membership || member.Deleted || member.State != "" && !oneOf(member.State, "active", "owner_unavailable") {
			return syncRefuse("MIGRATION_MEMBERSHIP_CHANGED")
		}
		var raw string
		err = tx.QueryRow("SELECT payload FROM messages WHERE epoch=? AND id=?", b.Epoch, ref.ID).Scan(&raw)
		if err == nil {
			var m struct {
				Content string `json:"contentVersion"`
			}
			if syncDecode([]byte(raw), &m) != nil || !cliMigrationPositive(m.Content) {
				return syncRefuse("MIGRATION_LOCAL_VERSION_UNKNOWN")
			}
			a, _ := new(big.Int).SetString(m.Content, 10)
			z, _ := new(big.Int).SetString(ref.Content, 10)
			if a.Cmp(z) > 0 {
				return syncRefuse("MIGRATION_NEWER_LOCAL_VERSION")
			}
		} else if err != sql.ErrNoRows {
			return err
		}
		err = nil
		archived, queryErr := tx.Query("SELECT payload FROM migration_messages WHERE epoch=? AND id=?", b.Epoch, ref.ID)
		if queryErr != nil {
			return queryErr
		}
		for archived.Next() {
			var prior string
			if err = archived.Scan(&prior); err != nil {
				break
			}
			var message cliMigrationMessage
			if json.Unmarshal([]byte(prior), &message) != nil || !cliMigrationPositive(message.Content) {
				err = syncRefuse("MIGRATION_LOCAL_VERSION_UNKNOWN")
				break
			}
			a, _ := new(big.Int).SetString(message.Content, 10)
			z, _ := new(big.Int).SetString(ref.Content, 10)
			if a.Cmp(z) > 0 {
				err = syncRefuse("MIGRATION_NEWER_LOCAL_VERSION")
				break
			}
		}
		if err == nil {
			err = archived.Err()
		}
		archived.Close()
		if err != nil {
			return err
		}
		// Packet order is not trusted; find the original validated fragment by ID.
		var message json.RawMessage
		for _, p := range packet.Messages {
			var m cliMigrationMessage
			if json.Unmarshal(p, &m) == nil && m.ID == ref.ID {
				message = p
				break
			}
		}
		if message == nil {
			return fmt.Errorf("migration message %d missing", i)
		}
		if _, err = tx.Exec("INSERT INTO migration_messages VALUES(?,?,?,?,?)", b.Epoch, v.ID, ref.ID, ref.Conversation, string(message)); err != nil {
			return err
		}
	}
	for _, object := range packet.Objects {
		if _, err = tx.Exec("INSERT INTO migration_objects VALUES(?,?,?,?,?,?)", b.Epoch, v.ID, object.ID, object.Message, object.Bytes, fingerprint(string(object.Bytes))); err != nil {
			return err
		}
	}
	// Archive metadata contains no message bodies or object bytes, so global
	// pruning cannot leave a second hidden copy in its manifest record.
	if _, err = tx.Exec("INSERT INTO migration_archives VALUES(?,?,?,?)", b.Epoch, v.ID, v.Digest, string(cliMigrationJSONBytes(v.Manifest))); err != nil {
		return err
	}
	o.State = "installed"
	if _, err = tx.Exec("UPDATE migration_intents SET payload=? WHERE id=?", string(cliMigrationJSONBytes(o)), o.ID); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM migration_stages WHERE epoch=? AND id=?; DELETE FROM migration_stage_messages WHERE epoch=? AND id=?", b.Epoch, v.ID, b.Epoch, v.ID); err != nil {
		return err
	}
	if !before.After(time.Now()) || !cliMigrationDeadline(v.Expires, time.Now()) || !current() {
		return syncRefuse("MIGRATION_INSTALL_EXPIRED")
	}
	return tx.Commit()
}
