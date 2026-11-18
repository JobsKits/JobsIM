// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"golang.org/x/term"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func (c *client) prepareOwnDeletion() (ownDeletionIntent, error) {
	var o ownDeletionIntent
	_, b, _, e := c.dataExportContext()
	if e != nil {
		return o, e
	}
	m := c.auth
	m.mu.Lock()
	username, namespace := m.username, filepath.ToSlash(m.state.MessageStore)
	m.mu.Unlock()
	id, e := syncNewUUID()
	if e != nil {
		return o, e
	}
	o = ownDeletionIntent{Meta: passwordChangeIntent{ID: id, Username: username, Binding: b, Namespace: namespace, Created: time.Now().UTC().Format(time.RFC3339Nano), State: "unsent"}, State: "unsent"}
	if e = validateOwnDeletion(o, c.base, c.deployment, m.profile, username); e != nil {
		return o, e
	}
	if _, e = c.ownDeletionCapability(o, true); e != nil {
		return o, e
	}
	if e = ownDeletionMigrate(m.db); e != nil {
		return o, e
	}
	tx, e := m.db.Begin()
	if e != nil {
		return o, e
	}
	defer tx.Rollback()
	if e = receiptDurability(tx); e != nil {
		return o, e
	}
	var pending int
	if e = tx.QueryRow("SELECT count(*) FROM own_deletion_intents WHERE json_extract(payload,'$.original.binding.familyId')=? AND json_extract(payload,'$.state') IN('unsent','unknown','pending','requested','cancel_pending','processing')", b.Family).Scan(&pending); e != nil {
		return o, e
	}
	if pending != 0 {
		return o, syncRefuse("ACCOUNT_DELETION_ORIGINAL_PENDING")
	}
	if c.ownDeletionOriginalToken(o) == "" {
		return o, syncRefuse("ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED")
	}
	raw, _ := json.Marshal(o)
	if _, e = tx.Exec("INSERT INTO own_deletion_intents VALUES(?,?)", id, string(raw)); e != nil {
		return o, e
	}
	return o, tx.Commit()
}
func listOwnDeletions(db *sql.DB, origin, scope, profile, username string) ([]ownDeletionIntent, error) {
	rows, e := db.Query("SELECT payload FROM own_deletion_intents ORDER BY rowid DESC")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	all := []ownDeletionIntent{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		var o ownDeletionIntent
		if ownDeletionDecode([]byte(raw), &o) != nil || validateOwnDeletion(o, origin, scope, profile, username) != nil {
			return nil, syncRefuse("ACCOUNT_DELETION_LOCAL_INVALID")
		}
		all = append(all, o)
	}
	return all, rows.Err()
}
func readOwnDeletionSecret(reader *bufio.Reader, restricted bool) (ownDeletionSecret, error) {
	var s ownDeletionSecret
	var e error
	recovery := false
	if restricted {
		fmt.Print("Use password or recovery code? Enter password or recovery: ")
		kind, read := reader.ReadString('\n')
		if read != nil {
			return s, read
		}
		kind = strings.TrimSpace(kind)
		if !oneOf(kind, "password", "recovery") {
			return s, syncRefuse("DATA_RIGHTS_PROOF_REQUIRED")
		}
		recovery = kind == "recovery"
	}
	if recovery {
		s.Recovery, e = secretInput(reader, "Current one-use recovery code: ")
	} else {
		s.Password, e = secretInput(reader, "Current password (original password for original-result lookup): ")
	}
	if e != nil {
		s.clear()
		return s, e
	}
	if len(s.Password) > 512 || len(s.Recovery) > 128 || len(s.Password)+len(s.Recovery) == 0 || !utf8.Valid(s.Password) || !utf8.Valid(s.Recovery) {
		s.clear()
		return s, syncRefuse("DATA_RIGHTS_PROOF_INPUT_INVALID")
	}
	s.OTP, e = secretInput(reader, "Current authenticator code (Enter if not required): ")
	if e != nil {
		s.clear()
	}
	return s, e
}
func ownDeletionConfirmation(reader *bufio.Reader, label string) error {
	fmt.Println(label)
	fmt.Print("Type confirm to proceed; any other text cancels: ")
	line, e := reader.ReadString('\n')
	if e != nil {
		return e
	}
	if strings.TrimSpace(line) != "confirm" {
		return syncRefuse("ACCOUNT_DELETION_CANCELLED_LOCALLY")
	}
	return nil
}
func (c *client) ownDeletionCommand(cmd, args string) (bool, error) {
	if !oneOf(cmd, "account-delete", "account-deletions", "deletion-reconcile", "deletion-retry", "deletion-query", "deletion-cancel", "deletion-cancel-retry") {
		return false, nil
	}
	if c.auth == nil || c.reader == nil || !term.IsTerminal(int(os.Stdin.Fd())) {
		return true, syncRefuse("ACCOUNT_DELETION_INTERACTIVE_TTY_REQUIRED")
	}
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	m := c.auth
	if e := ownDeletionMigrate(m.db); e != nil {
		return true, e
	}
	if cmd == "account-deletions" {
		if strings.TrimSpace(args) != "" {
			return true, fmt.Errorf("account-deletions takes no arguments")
		}
		rows, e := listOwnDeletions(m.db, c.base, c.deployment, m.profile, m.username)
		if e != nil {
			return true, e
		}
		return true, printJSON(map[string]any{"items": rows})
	}
	var o ownDeletionIntent
	var e error
	if cmd == "account-delete" {
		if strings.TrimSpace(args) != "" {
			return true, fmt.Errorf("account-delete takes no password arguments")
		}
		if e = ownDeletionConfirmation(c.reader, "Request deletion of this account. All current families will be revoked; cancellation requires a new restricted proof during the cooldown."); e != nil {
			return true, e
		}
		o, e = c.prepareOwnDeletion()
	} else {
		o, e = loadOwnDeletion(m.db, strings.TrimSpace(args), c.base, c.deployment, m.profile, m.username)
	}
	if e != nil {
		return true, e
	}
	cancel := cmd == "deletion-cancel" || cmd == "deletion-cancel-retry"
	restricted := cancel || cmd == "deletion-query"
	if cancel {
		if o.Result == nil {
			return true, syncRefuse("ACCOUNT_DELETION_RESULT_REQUIRED")
		}
		if e = ownDeletionConfirmation(c.reader, "Cancel the original deletion "+o.Result.ID+" at observed version "+o.Result.Version+"; original cancellation nonce is preserved."); e != nil {
			return true, e
		}
	}
	fmt.Println("Original request:", o.Meta.ID)
	s, e := readOwnDeletionSecret(c.reader, restricted)
	if e != nil {
		return true, e
	}
	defer s.clear()
	var v ownDeletionResult
	if restricted {
		v, e = c.ownDeletionRestricted(m.db, o, &s, cancel, cmd == "deletion-cancel-retry")
	} else {
		v, e = c.performOwnDeletion(m.db, o, &s, cmd == "deletion-retry", cmd == "account-delete")
	}
	if e != nil {
		return true, fmt.Errorf("original deletion %s retained: %w", o.Meta.ID, e)
	}
	return true, printJSON(v)
}
func openOwnDeletionMetadata(root, profile, username string) (*sql.DB, func(), error) {
	dir := filepath.Join(root, "JobsIM", "CLI", "Authentication", profile, fingerprint(username))
	if e := receiptDirectory(dir, false); e != nil {
		return nil, nil, e
	}
	release, e := lockAuthDirectory(dir)
	if e != nil {
		return nil, nil, e
	}
	failed := true
	defer func() {
		if failed {
			release()
		}
	}()
	path := filepath.Join(dir, "authentication.sqlite")
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, syncRefuse("ACCOUNT_DELETION_UNSAFE_METADATA")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	q.Set("mode", "rw")
	for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)"} {
		q.Add("_pragma", p)
	}
	u.RawQuery = q.Encode()
	db, e := sql.Open("sqlite", u.String())
	if e != nil {
		return nil, nil, e
	}
	db.SetMaxOpenConns(1)
	if e = receiptDurability(db); e != nil {
		db.Close()
		return nil, nil, e
	}
	if e = ownDeletionMigrate(db); e != nil {
		db.Close()
		return nil, nil, e
	}
	failed = false
	return db, func() { db.Close(); release() }, nil
}
func (c *client) runOwnDeletionResume(dataDirectory, ca string, args []string, reader *bufio.Reader) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return syncRefuse("ACCOUNT_DELETION_INTERACTIVE_TTY_REQUIRED")
	}
	if len(args) != 2 && !(len(args) == 1 && args[0] == "account-deletions") {
		return fmt.Errorf("deletion-reconcile|deletion-retry|deletion-query|deletion-cancel|deletion-cancel-retry ORIGINAL_REQUEST_ID; account-deletions")
	}
	fmt.Print("Username: ")
	username, e := reader.ReadString('\n')
	if e != nil {
		return e
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return syncRefuse("USERNAME_REQUIRED")
	}
	root, e := os.UserConfigDir()
	if dataDirectory != "" {
		root, e = filepath.Abs(dataDirectory)
	}
	if e != nil {
		return e
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
	db, close, e := openOwnDeletionMetadata(root, profile, username)
	if e != nil {
		return e
	}
	defer close()
	c.reader = reader
	c.auth = &authManager{c: c, db: db, root: root, profile: profile, username: username}
	_, e = c.ownDeletionCommand(args[0], strings.Join(args[1:], " "))
	return e
}
