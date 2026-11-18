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
	if _, err := tx.Exec("DELETE FROM receipt_bodies WHERE epoch=? AND message_id=?", epoch, message); err != nil {
		return err
	}
	if err := receiptRetirePreparationsTx(tx, epoch, "", message); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO receipt_object_cleanup SELECT request_id,descriptor,descriptor_hash FROM receipt_objects WHERE epoch=? AND message_id=? AND state!='erased'`, epoch, message); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE receipt_objects SET state='erasing' WHERE epoch=? AND message_id=? AND state!='erased'", epoch, message); err != nil {
		return err
	}
	_, err := tx.Exec("UPDATE receipt_outbox SET state='frozen' WHERE epoch=? AND message_id=? AND state='pending' AND json_extract(payload,'$.kind')='object_received'", epoch, message)
	return err
}
func receiptRetirePreparationsTx(tx *sql.Tx, epoch, request, message string) error {
	rows, err := tx.Query("SELECT payload,payload_hash FROM receipt_object_preparations WHERE epoch=? AND (?='' OR request_id=?) AND (?='' OR message_id=?)", epoch, request, request, message, message)
	if err != nil {
		return err
	}
	type entry struct{ raw, hash string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.raw, &e.hash); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	var identityRaw, physical string
	if err = tx.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&identityRaw); err != nil {
		return err
	}
	if err = tx.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); err != nil {
		return err
	}
	var identity localIdentity
	if json.Unmarshal([]byte(identityRaw), &identity) != nil {
		return syncRefuse("RECEIPT_OBJECT_CLEANUP_INVALID")
	}
	binding := syncBinding{ScopeID: identity.DeploymentID, PrincipalID: identity.UserID, SourceContextID: identity.Space, DeviceID: identity.DeviceID, StoreGeneration: physical, RestoreEpoch: epoch}
	for _, e := range entries {
		var o receiptOperation
		if json.Unmarshal([]byte(e.raw), &o) != nil || o.Hash != e.hash || receiptHash(o) != e.hash || receiptCheckIntent(o) != nil || o.Kind != "object_received" || o.Binding.check(binding, false) != nil {
			return syncRefuse("RECEIPT_OBJECT_CLEANUP_INVALID")
		}
		d := *o.Object
		d.Incomplete = true
		raw, _ := json.Marshal(d)
		hash := fingerprint(string(raw))
		if _, err = tx.Exec("INSERT OR IGNORE INTO receipt_object_cleanup VALUES(?,?,?)", o.ID, string(raw), hash); err != nil {
			return err
		}
		var existing string
		if err = tx.QueryRow("SELECT descriptor_hash FROM receipt_object_cleanup WHERE request_id=?", o.ID).Scan(&existing); err != nil {
			return err
		}
		if existing != hash {
			return syncRefuse("RECEIPT_OBJECT_CLEANUP_INVALID")
		}
		if _, err = tx.Exec("DELETE FROM receipt_object_preparations WHERE request_id=? AND payload_hash=?", o.ID, e.hash); err != nil {
			return err
		}
	}
	return nil
}
func (s *localStore) retireReceiptPreparations(request string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = receiptRetirePreparationsTx(tx, s.epoch, request, ""); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *localStore) receiptObjectWorkItems() ([]map[string]string, error) {
	items := []map[string]string{}
	for _, table := range []string{"receipt_object_preparations", "receipt_object_cleanup"} {
		fields := "request_id,payload,payload_hash"
		if table == "receipt_object_cleanup" {
			fields = "request_id,descriptor,descriptor_hash"
		}
		rows, err := s.db.Query("SELECT " + fields + " FROM " + table + " ORDER BY request_id")
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, raw, hash string
			if err = rows.Scan(&id, &raw, &hash); err != nil {
				break
			}
			var d receiptLocalObject
			state := "cleanup_pending"
			if table == "receipt_object_preparations" {
				var o receiptOperation
				if json.Unmarshal([]byte(raw), &o) != nil || receiptHash(o) != hash || o.Hash != hash || o.ID != id || receiptCheckIntent(o) != nil || o.Object == nil {
					err = syncRefuse("RECEIPT_INTENT_CHANGED")
					break
				}
				d = *o.Object
				state = "prepared"
			} else if fingerprint(raw) != hash || json.Unmarshal([]byte(raw), &d) != nil || d.Request != id {
				err = syncRefuse("RECEIPT_OBJECT_CLEANUP_INVALID")
				break
			}
			if d.Epoch == s.epoch {
				items = append(items, map[string]string{"requestId": id, "kind": "object_local_work", "state": state, "messageId": d.Message, "objectId": d.Object, "relativePath": d.Path, "restoreEpoch": d.Epoch})
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (s *localStore) drainReceiptObjectCleanup() error {
	if err := s.retireReceiptPreparations(""); err != nil {
		return err
	}
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
	var physical string
	if err = s.db.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); err != nil {
		return err
	}
	binding := syncBinding{ScopeID: s.identity.DeploymentID, PrincipalID: s.identity.UserID, SourceContextID: s.identity.Space, DeviceID: s.identity.DeviceID, StoreGeneration: physical, RestoreEpoch: s.epoch}
	for _, v := range work {
		var d receiptLocalObject
		if fingerprint(v.raw) != v.hash || json.Unmarshal([]byte(v.raw), &d) != nil || d.Request != v.id || d.Epoch != s.epoch || d.Binding.check(binding, false) != nil {
			return syncRefuse("RECEIPT_OBJECT_CLEANUP_INVALID")
		}
		path, err := s.receiptObjectFile(d, false)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			var file *os.File
			var openErr error
			if d.Incomplete {
				before, e := os.Lstat(path)
				if e != nil && !os.IsNotExist(e) {
					return e
				}
				if e == nil && !before.Mode().IsRegular() {
					return syncRefuse("RECEIPT_OBJECT_UNSAFE_PATH")
				}
				if e == nil {
					file, openErr = os.Open(path)
				}
				if openErr == nil && file != nil {
					opened, e := file.Stat()
					if e != nil || !os.SameFile(before, opened) {
						file.Close()
						return syncRefuse("RECEIPT_OBJECT_FILE_CHANGED")
					}
				}
			} else {
				file, openErr = s.openReceiptObject(d)
			}
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
			if err = receiptSyncDirectory(filepath.Dir(path)); err != nil {
				return err
			}
		} else if os.IsNotExist(err) {
			if err = receiptSyncDirectory(filepath.Dir(s.path)); err != nil {
				return err
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
