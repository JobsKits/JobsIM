// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func (c *client) cliMigrationHistory(archive string, b dataExportBinding, current func() bool) ([]json.RawMessage, error) {
	if !syncUUIDPattern.MatchString(archive) {
		return nil, syncRefuse("MIGRATION_ARCHIVE_INVALID")
	}
	if err := c.store.cliMigrationControl(c.store.db, b, current); err != nil {
		return nil, err
	}
	var present int
	if err := c.store.db.QueryRow("SELECT count(*) FROM migration_archives WHERE epoch=? AND id=?", b.Epoch, archive).Scan(&present); err != nil || present != 1 {
		return nil, syncRefuse("MIGRATION_ARCHIVE_NOT_FOUND")
	}
	rows, err := c.store.db.Query("SELECT payload FROM migration_messages WHERE epoch=? AND archive=? ORDER BY rowid", b.Epoch, archive)
	if err != nil {
		return nil, err
	}
	type item struct {
		id, raw   string
		forwarded bool
	}
	items := []item{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var message cliMigrationMessage
		if json.Unmarshal([]byte(raw), &message) != nil {
			err = syncRefuse("MIGRATION_ARCHIVE_INVALID")
			break
		}
		items = append(items, item{message.ID, raw, message.Forwarded})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []json.RawMessage{}
	for _, row := range items {
		var original cliMigrationMessage
		_ = json.Unmarshal([]byte(row.raw), &original)
		var live map[string]json.RawMessage
		allowed := true
		if row.forwarded {
			err = c.call("GET", "/api/messages/"+row.id, nil, &live)
			var observed struct {
				ID                 string `json:"id"`
				Content            string `json:"contentVersion"`
				Forwarded          bool   `json:"forwarded"`
				Deleted            bool   `json:"deleted"`
				Unavailable        bool   `json:"contentUnavailable"`
				ForwardUnavailable bool   `json:"forwardUnavailable"`
				Text               string `json:"text"`
				Type               string `json:"type"`
			}
			_ = json.Unmarshal(cliMigrationJSONBytes(live), &observed)
			allowed = err == nil && observed.ID == row.id && observed.Content == original.Content && observed.Forwarded && !observed.Deleted && !observed.Unavailable && !observed.ForwardUnavailable && observed.Text == original.Text && observed.Type == original.Type
			if !allowed {
				tx, e := c.store.db.Begin()
				if e != nil {
					return nil, e
				}
				e = cliMigrationRedactForwardTx(tx, b.Epoch, row.id)
				if e == nil {
					e = tx.Commit()
				} else {
					_ = tx.Rollback()
				}
				if e != nil {
					return nil, e
				}
			}
		}
		// Re-read after each network await. A concurrent global or projection
		// invalidation must win over the earlier RAM copy.
		tx, e := c.store.db.Begin()
		if e != nil {
			return nil, e
		}
		if e = c.store.cliMigrationControl(tx, b, current); e != nil {
			_ = tx.Rollback()
			return nil, e
		}
		var count int
		e = tx.QueryRow("SELECT count(*) FROM sync_terminal WHERE epoch=? AND id=?", b.Epoch, row.id).Scan(&count)
		var durable string
		if e == nil {
			e = tx.QueryRow("SELECT payload FROM migration_messages WHERE epoch=? AND archive=? AND id=?", b.Epoch, archive, row.id).Scan(&durable)
		}
		_ = tx.Rollback()
		if e == sql.ErrNoRows || count > 0 {
			continue
		}
		if e != nil {
			return nil, e
		}
		if row.forwarded && (!allowed || durable != row.raw) {
			var placeholder map[string]any
			_ = json.Unmarshal([]byte(durable), &placeholder)
			maskForward(placeholder)
			durable = string(cliMigrationJSONBytes(placeholder))
		}
		result = append(result, json.RawMessage(durable))
	}
	tx, err := c.store.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = c.store.cliMigrationControl(tx, b, current); err != nil {
		return nil, err
	}
	final := []json.RawMessage{}
	for _, raw := range result {
		var message cliMigrationMessage
		_ = json.Unmarshal(raw, &message)
		var durable string
		err = tx.QueryRow("SELECT m.payload FROM migration_messages m LEFT JOIN sync_terminal t ON t.epoch=m.epoch AND t.id=m.id WHERE m.epoch=? AND m.archive=? AND m.id=? AND t.id IS NULL", b.Epoch, archive, message.ID).Scan(&durable)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		if message.Forwarded && !syncEqual(raw, []byte(durable)) {
			var placeholder map[string]any
			_ = json.Unmarshal([]byte(durable), &placeholder)
			maskForward(placeholder)
			durable = string(cliMigrationJSONBytes(placeholder))
		}
		final = append(final, json.RawMessage(durable))
	}
	return final, nil
}
func (c *client) cliMigrationSaveObject(archive, object, destination string, b dataExportBinding, current func() bool) error {
	if !syncUUIDPattern.MatchString(archive) || !syncUUIDPattern.MatchString(object) {
		return syncRefuse("MIGRATION_ARCHIVE_INVALID")
	}
	path, err := dataExportDestination(destination)
	if err != nil {
		return err
	}
	tx, err := c.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Acquire the writer fence before taking a snapshot or publishing bytes.
	// A concurrent global transaction must wait until this delivery ends.
	fence, err := tx.Exec("UPDATE migration_objects SET digest=digest WHERE epoch=? AND archive=? AND id=?", b.Epoch, archive, object)
	if err != nil {
		return err
	}
	if count, err := fence.RowsAffected(); err != nil || count != 1 {
		return syncRefuse("MIGRATION_ORIGINAL_REQUIRED")
	}
	if err = c.store.cliMigrationControl(tx, b, current); err != nil {
		return err
	}
	var message, digest string
	var data []byte
	if err = tx.QueryRow("SELECT message_id,bytes,digest FROM migration_objects WHERE epoch=? AND archive=? AND id=?", b.Epoch, archive, object).Scan(&message, &data, &digest); err != nil {
		return err
	}
	defer erase(data)
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM sync_terminal WHERE epoch=? AND id=?", b.Epoch, message).Scan(&count); err != nil || count > 0 {
		return syncRefuse("MIGRATION_GLOBAL_FLOOR")
	}
	if len(data) > 64<<20 || fingerprint(string(data)) != digest {
		return syncRefuse("MIGRATION_ORIGINAL_CHANGED")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	published := false
	owned, statErr := file.Stat()
	defer func() {
		_ = file.Close()
		if !published && owned != nil {
			if named, err := os.Lstat(path); err == nil && os.SameFile(owned, named) {
				_ = os.Remove(path)
			}
		}
	}()
	if statErr != nil {
		return statErr
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if !current() {
		return syncRefuse("MIGRATION_CONTEXT_CHANGED")
	}
	if err = receiptSyncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	published = true
	return printJSON(map[string]any{"path": path, "sha256": digest, "sizeBytes": fmt.Sprint(len(data)), "sourcePreserved": true, "qualifiedAck": false})
}
