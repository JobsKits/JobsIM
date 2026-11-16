// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
)

// Only managed receipt files participate; user-selected import/download files
// remain outside this private holder and are never unlinked by cloud events.
func receiptEraseObjectsTx(tx *sql.Tx, epoch, message string) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO receipt_object_cleanup SELECT request_id,descriptor,descriptor_hash FROM receipt_objects WHERE epoch=? AND message_id=? AND state!='erased'`, epoch, message); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE receipt_objects SET state='erasing' WHERE epoch=? AND message_id=? AND state!='erased'", epoch, message); err != nil {
		return err
	}
	_, err := tx.Exec("UPDATE receipt_outbox SET state='frozen' WHERE epoch=? AND message_id=? AND state='pending' AND json_extract(payload,'$.kind')='object_received'", epoch, message)
	return err
}
func (s *localStore) drainReceiptObjectCleanup() error {
	rows, err := s.db.Query("SELECT request_id,descriptor,descriptor_hash FROM receipt_object_cleanup ORDER BY request_id")
	if err != nil {
		return err
	}
	type item struct{ id, raw, hash string }
	var work []item
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.raw, &v.hash); err != nil {
			rows.Close()
			return err
		}
		work = append(work, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range work {
		var d receiptLocalObject
		if fingerprint(v.raw) != v.hash || json.Unmarshal([]byte(v.raw), &d) != nil || d.Request != v.id || d.Epoch != s.epoch {
			return syncRefuse("RECEIPT_OBJECT_CLEANUP_INVALID")
		}
		path, err := s.receiptObjectFile(d, false)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			file, openErr := s.openReceiptObject(d)
			if openErr != nil && !os.IsNotExist(openErr) {
				return openErr
			}
			if file != nil {
				opened, e := file.Stat()
				latest, pathErr := os.Lstat(path)
				if e != nil || pathErr != nil || !os.SameFile(opened, latest) {
					file.Close()
					return syncRefuse("RECEIPT_OBJECT_FILE_CHANGED")
				}
				file.Close()
				if err = os.Remove(path); err != nil {
					return err
				}
				if err = receiptSyncDirectory(filepath.Dir(path)); err != nil {
					return err
				}
			}
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE receipt_objects SET state='erased' WHERE request_id=? AND descriptor_hash=? AND state='erasing'", v.id, v.hash); err == nil {
			_, err = tx.Exec("DELETE FROM receipt_object_cleanup WHERE request_id=? AND descriptor=? AND descriptor_hash=?", v.id, v.raw, v.hash)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
