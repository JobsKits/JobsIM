// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type ownDeletionFloor struct{ Origin, Deployment, Profile, User, Deletion string }

func ownDeletionFloorPath(base, user string) string {
	return filepath.Join(base, "AccountErasure-"+user+".json")
}
func ownDeletionStoreGate(path string, identity localIdentity) error {
	raw, e := os.ReadFile(ownDeletionFloorPath(filepath.Dir(filepath.Dir(path)), identity.UserID))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var f ownDeletionFloor
	if syncDecode(raw, &f) != nil || f.Origin != identity.Origin || f.Deployment != identity.DeploymentID || f.Profile != identity.ConnectionProfile || f.User != identity.UserID {
		return syncRefuse("ACCOUNT_ERASURE_FLOOR_INVALID")
	}
	return syncRefuse("ACCOUNT_ERASURE_LOCAL_TERMINAL")
}
func (m *authManager) ownDeletionRestoreGate() error {
	var table int
	if e := m.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='own_deletion_intents'").Scan(&table); e != nil {
		return e
	}
	if table == 0 {
		return nil
	}
	rows, e := m.db.Query("SELECT payload FROM own_deletion_intents")
	if e != nil {
		return e
	}
	var all []ownDeletionIntent
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			break
		}
		var o ownDeletionIntent
		if ownDeletionDecode([]byte(raw), &o) != nil || validateOwnDeletion(o, m.c.base, m.c.deployment, m.profile, m.username) != nil {
			e = syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
			break
		}
		all = append(all, o)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	for _, o := range all {
		if o.Attempts > 0 && o.State != "rejected" && o.Meta.Binding.Family == m.state.FamilyID && m.state.Status == "active" {
			m.state.Status = "account_deletion_stopped"
			if e = m.persist(); e != nil {
				return e
			}
		}
		if o.State == "completed" && !o.Cleared {
			if e = m.c.clearOwnDeletionLocal(m, o); e != nil {
				return e
			}
			next := o
			next.Cleared = true
			if _, e = saveOwnDeletion(m.db, o, next); e != nil {
				return e
			}
		}
	}
	return nil
}
func (c *client) clearOwnDeletionLocal(m *authManager, o ownDeletionIntent) error {
	if o.State != "completed" || o.Result == nil || m.profile != o.Meta.Binding.Identity.ConnectionProfile || m.username != o.Meta.Username {
		return syncRefuse("ACCOUNT_DELETION_CLEANUP_INVALID")
	}
	b := o.Meta.Binding.Identity
	base := filepath.Join(m.root, "JobsIM", "CLI", m.profile)
	if e := receiptDirectory(base, false); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	floor := ownDeletionFloor{b.Origin, b.DeploymentID, b.ConnectionProfile, b.UserID, o.Result.ID}
	raw, _ := json.Marshal(floor)
	path := ownDeletionFloorPath(base, b.UserID)
	if old, e := os.ReadFile(path); e == nil {
		if !json.Valid(old) || !bytesEqualFloor(old, raw) {
			return syncRefuse("ACCOUNT_ERASURE_FLOOR_INVALID")
		}
	} else if !os.IsNotExist(e) {
		return e
	} else {
		file, e := os.CreateTemp(base, ".account-erasure-*.part")
		if e != nil {
			return e
		}
		temp := file.Name()
		defer os.Remove(temp)
		if e = file.Chmod(0600); e == nil {
			_, e = file.Write(raw)
		}
		if e == nil {
			e = file.Sync()
		}
		e2 := file.Close()
		if e != nil {
			return e
		}
		if e2 != nil {
			return e2
		}
		if e = os.Rename(temp, path); e != nil {
			return e
		}
		if e = receiptSyncDirectory(base); e != nil {
			return e
		}
	}
	entries, e := os.ReadDir(base)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		name := entry.Name()
		if len(name) != 64 || strings.Trim(name, "0123456789abcdef") != "" {
			continue
		}
		dir := filepath.Join(base, name)
		if e = receiptDirectory(dir, false); e != nil {
			return e
		}
		dbpath := filepath.Join(dir, "client.sqlite")
		info, e := os.Lstat(dbpath)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return syncRefuse("ACCOUNT_ERASURE_UNSAFE_STORE")
		}
		u := url.URL{Scheme: "file", Path: filepath.ToSlash(dbpath)}
		q := u.Query()
		q.Set("mode", "rw")
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "synchronous(FULL)")
		u.RawQuery = q.Encode()
		db, e := sql.Open("sqlite", u.String())
		if e != nil {
			return e
		}
		db.SetMaxOpenConns(1)
		var identityRaw string
		var identity localIdentity
		e = db.QueryRow("SELECT value FROM metadata WHERE key='identity'").Scan(&identityRaw)
		if e != nil {
			db.Close()
			return e
		}
		if syncDecode([]byte(identityRaw), &identity) != nil {
			db.Close()
			return syncRefuse("ACCOUNT_ERASURE_UNSAFE_STORE")
		}
		if identity.Origin != b.Origin || identity.DeploymentID != b.DeploymentID || identity.ConnectionProfile != b.ConnectionProfile || identity.UserID != b.UserID || identity.Space != "personal" {
			db.Close()
			continue
		}
		if fingerprint(identityRaw) != name {
			db.Close()
			return syncRefuse("ACCOUNT_ERASURE_UNSAFE_STORE")
		}
		if c.store != nil && c.store.path == dbpath {
			c.store.db.Close()
		}
		if e = eraseOwnDeletionStore(db, dir); e != nil {
			db.Close()
			return e
		}
		if e = db.Close(); e != nil {
			return e
		}
	}
	return nil
}
func bytesEqualFloor(a, b []byte) bool {
	var x, y ownDeletionFloor
	return syncDecode(a, &x) == nil && syncDecode(b, &y) == nil && x == y
}
func eraseOwnDeletionStore(db *sql.DB, dir string) error {
	if _, e := db.Exec("PRAGMA secure_delete=ON"); e != nil {
		return e
	}
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = receiptDurability(tx); e != nil {
		return e
	}
	rows, e := tx.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name!='metadata'")
	if e != nil {
		return e
	}
	var tables []string
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			break
		}
		tables = append(tables, name)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	for _, name := range tables {
		if _, e = tx.Exec(`DROP TABLE "` + strings.ReplaceAll(name, `"`, `""`) + `"`); e != nil {
			return e
		}
	}
	if _, e = tx.Exec("DELETE FROM metadata WHERE key NOT IN('identity','epoch'); INSERT OR REPLACE INTO metadata VALUES('auth_retired','1'),('account_erased','1')"); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	if _, e = db.Exec("VACUUM"); e != nil {
		return e
	}
	var busy, log, done int
	if e = db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &done); e != nil {
		return e
	}
	if busy != 0 {
		return syncRefuse("ACCOUNT_ERASURE_CHECKPOINT_PENDING")
	}
	managed := filepath.Join(dir, "ReceiptObjects")
	if info, e := os.Lstat(managed); os.IsNotExist(e) {
		return receiptSyncDirectory(dir)
	} else if e != nil {
		return e
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return syncRefuse("ACCOUNT_ERASURE_UNSAFE_PRIVATE_PATH")
	}
	// Only the program's UUID .bin receipt holders are managed local originals.
	return filepath.WalkDir(managed, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return syncRefuse("ACCOUNT_ERASURE_UNSAFE_PRIVATE_PATH")
		}
		if entry.IsDir() {
			if path != managed {
				if n, e := syncCounter(entry.Name()); e != nil || n == 0 {
					return syncRefuse("ACCOUNT_ERASURE_UNSAFE_PRIVATE_PATH")
				}
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".bin") || !syncUUIDPattern.MatchString(strings.TrimSuffix(entry.Name(), ".bin")) {
			return fmt.Errorf("unrecognized file in managed holder; cleanup retained")
		}
		info, e := entry.Info()
		if e != nil || !info.Mode().IsRegular() {
			return syncRefuse("ACCOUNT_ERASURE_UNSAFE_PRIVATE_PATH")
		}
		if e = os.Remove(path); e != nil {
			return e
		}
		return receiptSyncDirectory(filepath.Dir(path))
	})
}
