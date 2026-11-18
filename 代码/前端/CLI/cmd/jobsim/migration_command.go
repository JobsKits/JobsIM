// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type cliMigrationPairing struct {
	ID         string `json:"pairingId"`
	Scope      string `json:"scopeId"`
	User       string `json:"userId"`
	Device     string `json:"targetDeviceId"`
	Store      string `json:"targetStoreGeneration"`
	Epoch      string `json:"restoreEpoch"`
	Security   string `json:"securityVersion"`
	Generation string `json:"sourceGeneration"`
	Expires    int64  `json:"expires"`
}
type cliMigrationInput struct {
	Envelope string `json:"bodyEnvelope"`
	Mode     string `json:"objectMode"`
	Receipt  string `json:"sourceReceiptId,omitempty"`
}

type cliMigrationSourceObject struct {
	ID      string `json:"objectId"`
	Message string `json:"messageId"`
	Bytes   []byte `json:"bytes"`
	Receipt string `json:"sourceObjectReceiptId,omitempty"`
}

func cliMigrationPair(token string, b dataExportBinding) (cliMigrationPairing, error) {
	var p cliMigrationPairing
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "mp1" || len(token) > 4096 {
		return p, syncRefuse("MIGRATION_PAIRING_INVALID")
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	signature, se := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if e != nil || se != nil || len(signature) != 32 || cliMigrationDecode(raw, &p) != nil || p.Scope != b.Identity.DeploymentID || p.User != b.Identity.UserID || p.Epoch != b.Epoch || p.Expires <= time.Now().Unix() || p.Expires > time.Now().Add(305*time.Second).Unix() || !cliMigrationPositive(p.Security) || !cliMigrationPositive(p.Generation) {
		return p, syncRefuse("MIGRATION_PAIRING_INVALID")
	}
	for _, id := range []string{p.ID, p.Device, p.Store} {
		if !syncUUIDPattern.MatchString(id) {
			return p, syncRefuse("MIGRATION_PAIRING_INVALID")
		}
	}
	return p, nil
}
func (c *client) cliMigrationContext() (*localStore, dataExportBinding, syncControl, func() bool, error) {
	s, b, current, e := c.dataExportContext()
	var control syncControl
	if e != nil {
		return s, b, control, current, e
	}
	control, e = s.receiptControl(s.db, nil, current, true)
	if e == nil {
		e = receiptDurability(s.db)
	}
	return s, b, control, current, e
}
func (c *client) cliMigrationHTTP(b dataExportBinding, current func() bool, method, path string, body any, binary bool, limit int64) ([]byte, error) {
	if !current() || c.store.cliMigrationControl(c.store.db, b, current) != nil {
		return nil, syncRefuse("MIGRATION_CONTEXT_CHANGED")
	}
	u, e := url.Parse(c.base)
	if e != nil || u.Scheme != "https" || !strings.HasPrefix(path, "/api/") || strings.ContainsAny(path, "?#\\") || strings.Contains(path, "..") {
		return nil, syncRefuse("MIGRATION_HTTPS_REQUIRED")
	}
	raw, e := json.Marshal(body)
	if e != nil {
		return nil, e
	}
	if body == nil {
		raw = nil
	}
	if len(raw) > cliMigrationLimit {
		return nil, syncRefuse("MIGRATION_REQUEST_TOO_LARGE")
	}
	pair, e := c.auth.snapshot()
	if e != nil {
		return nil, e
	}
	if pair.FamilyID != b.Family {
		return nil, syncRefuse("MIGRATION_CONTEXT_CHANGED")
	}
	req, e := http.NewRequest(method, c.base+path, bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+pair.Token)
	req.Header.Set("X-JobsIM-Store-Generation", b.Physical)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	h := *c.http
	h.Jar = nil
	h.Timeout = 90 * time.Second
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, e := h.Do(req)
	if e != nil {
		return nil, fmt.Errorf("original migration retained; query it before retrying: %w", e)
	}
	defer response.Body.Close()
	data, e := readLimited(response.Body, limit)
	if e != nil {
		return nil, e
	}
	if !current() || c.store.cliMigrationControl(c.store.db, b, current) != nil {
		return nil, syncRefuse("MIGRATION_CONTEXT_CHANGED")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var p struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &p)
		return nil, &apiFailure{Status: response.StatusCode, Code: p.Error.Code, Message: p.Error.Message}
	}
	if !binary {
		var object map[string]json.RawMessage
		if cliMigrationDecode(data, &object) != nil || object == nil {
			return nil, syncRefuse("MIGRATION_INVALID_RESPONSE")
		}
	}
	return data, nil
}
func (c *client) cliMigrationJSON(b dataExportBinding, current func() bool, method, path string, body, out any) error {
	raw, e := c.cliMigrationHTTP(b, current, method, path, body, false, 2<<20)
	if e != nil {
		return e
	}
	return cliMigrationDecode(raw, out)
}
func (c *client) cliMigrationCapability(b dataExportBinding, current func() bool) error {
	var cap struct {
		Enabled   bool   `json:"enabled"`
		Format    string `json:"format"`
		Context   string `json:"sourceContextId"`
		Messages  int    `json:"maxMessages"`
		Objects   int    `json:"maxObjects"`
		Request   int    `json:"maxRequestBytes"`
		Object    int    `json:"maxObjectBytes"`
		Lifetime  int    `json:"maxLifetimeSeconds"`
		Pairing   int    `json:"pairingLifetimeSeconds"`
		Install   int    `json:"installLifetimeSeconds"`
		Qualified *bool  `json:"qualifiedAck"`
		Target    *bool  `json:"targetQualified"`
	}
	if e := c.cliMigrationJSON(b, current, "GET", "/api/sync/migration-capabilities", nil, &cap); e != nil {
		return e
	}
	if !cap.Enabled || cap.Format != cliMigrationFormat || cap.Context != "personal" || cap.Messages != 256 || cap.Objects != 64 || cap.Request != cliMigrationLimit || cap.Object != 64<<20 || cap.Lifetime != 600 || cap.Pairing != 300 || cap.Install != 5 || !receiptFalse(cap.Qualified) || !receiptFalse(cap.Target) {
		return syncRefuse("MIGRATION_CAPABILITY_REQUIRED")
	}
	return nil
}
func (c *client) cliMigrationProof() (string, string, error) {
	if c.reader == nil {
		return "", "", fmt.Errorf("interactive password input required")
	}
	password, e := secretInput(c.reader, "Current password (RAM only): ")
	if e != nil {
		return "", "", e
	}
	defer erase(password)
	otp, e := secretInput(c.reader, "Current administrator code (ordinary user: empty): ")
	if e != nil {
		return "", "", e
	}
	defer erase(otp)
	if len(password) == 0 || len(password) > 1024 || len(otp) != 0 && len(otp) != 6 {
		return "", "", syncRefuse("MIGRATION_PROOF_INVALID")
	}
	for _, v := range otp {
		if v < '0' || v > '9' {
			return "", "", syncRefuse("MIGRATION_PROOF_INVALID")
		}
	}
	return string(password), string(otp), nil
}
func (c *client) cliMigrationPairingIntent(o cliMigrationIntent, b dataExportBinding, current func() bool, write bool) (any, error) {
	var result map[string]json.RawMessage
	e := c.cliMigrationJSON(b, current, "POST", "/api/sync/migration-pairing-results", map[string]string{"requestId": o.ID, "storeGeneration": b.Physical}, &result)
	if e != nil {
		return nil, e
	}
	var status string
	_ = json.Unmarshal(result["status"], &status)
	if status != "not_found" {
		var token string
		_ = json.Unmarshal(result["pairingProof"], &token)
		p, err := cliMigrationPair(token, b)
		if err != nil || p.Device != b.Identity.DeviceID || p.Store != b.Physical {
			return nil, syncRefuse("MIGRATION_PAIRING_INVALID")
		}
		return result, nil
	}
	if !write {
		return result, nil
	}
	if e = c.cliMigrationCapability(b, current); e != nil {
		return nil, e
	}
	password, otp, e := c.cliMigrationProof()
	if e != nil {
		return nil, e
	}
	defer func() { password = ""; otp = "" }()
	_, e = c.store.cliMigrationChange(b, current, o.ID, nil, func(row *cliMigrationIntent) error { row.State = "unknown"; return nil })
	if e != nil {
		return nil, e
	}
	e = c.cliMigrationJSON(b, current, "POST", "/api/sync/migration-pairings", map[string]string{"requestId": o.ID, "storeGeneration": b.Physical, "restoreEpoch": b.Epoch, "password": password, "otp": otp}, &result)
	if e != nil {
		return nil, e
	}
	var token string
	_ = json.Unmarshal(result["pairingProof"], &token)
	p, e := cliMigrationPair(token, b)
	if e != nil || p.Device != b.Identity.DeviceID || p.Store != b.Physical {
		return nil, syncRefuse("MIGRATION_PAIRING_INVALID")
	}
	_, e = c.store.cliMigrationChange(b, current, o.ID, nil, func(row *cliMigrationIntent) error { row.State = "ready"; return nil })
	return result, e
}
func (c *client) cliMigrationSourceInputs(o cliMigrationIntent, b dataExportBinding, current func() bool) ([]cliMigrationInput, []cliMigrationSourceObject, string, error) {
	inputs := []cliMigrationInput{}
	objects := []cliMigrationSourceObject{}
	for _, selection := range o.Selection {
		var live map[string]json.RawMessage
		if e := c.call("GET", "/api/messages/"+selection.ID, nil, &live); e != nil {
			return nil, nil, "", e
		}
		get := func(k string) string { var v string; _ = json.Unmarshal(live[k], &v); return v }
		truth := func(k string) bool { var v bool; _ = json.Unmarshal(live[k], &v); return v }
		if !current() || get("id") != selection.ID || get("contentVersion") != selection.Content || truth("deleted") || truth("forwardUnavailable") {
			return nil, nil, "", syncRefuse("MIGRATION_SOURCE_CHANGED")
		}
		if truth("contentUnavailable") {
			if get("deleteScope") != "cloud_only" {
				return nil, nil, "", syncRefuse("MIGRATION_CLASSIFICATION_UNKNOWN")
			}
			original, err := c.localQualifiedMigrationMessage(selection.ID, selection.Content)
			if err != nil {
				return nil, nil, "", err
			}
			if original.Original != nil {
				defer original.Original.Close()
			}
			inputs = append(inputs, cliMigrationInput{Envelope: original.Envelope, Mode: selection.Mode, Receipt: original.ContentReceiptID})
			if selection.Mode == "full" {
				if original.Object == nil || original.Original == nil || original.ObjectReceiptID == "" || len(objects) >= 64 {
					return nil, nil, "", syncRefuse("MIGRATION_ORIGINAL_REQUIRED")
				}
				data, err := readLimited(original.Original, 64<<20)
				if err != nil || fmt.Sprint(len(data)) != original.Object.Size || fingerprint(string(data)) != original.Object.Digest || !current() {
					return nil, nil, "", syncRefuse("MIGRATION_ORIGINAL_CHANGED")
				}
				objects = append(objects, cliMigrationSourceObject{ID: original.Object.Object, Message: selection.ID, Bytes: data, Receipt: original.ObjectReceiptID})
			}
			continue
		}
		normal := map[string]json.RawMessage{}
		for _, k := range []string{"id", "conversationId", "senderId", "type", "text", "replyToId", "attachmentId", "createdAt", "contentVersion", "creationEpoch", "creationSequence", "forwarded"} {
			if value, ok := live[k]; ok {
				normal[k] = value
			}
		}
		raw, _ := json.Marshal(normal)
		inputs = append(inputs, cliMigrationInput{Envelope: base64.RawURLEncoding.EncodeToString(raw), Mode: selection.Mode})
		if selection.Mode == "full" {
			object := get("attachmentId")
			if truth("forwarded") || !syncUUIDPattern.MatchString(object) || len(objects) >= 64 {
				return nil, nil, "", syncRefuse("MIGRATION_ORIGINAL_REQUIRED")
			}
			data, e := c.cliMigrationHTTP(b, current, "GET", "/api/media/"+object, nil, true, 64<<20)
			if e != nil {
				return nil, nil, "", e
			}
			objects = append(objects, cliMigrationSourceObject{ID: object, Message: selection.ID, Bytes: data})
		}
	}
	return inputs, objects, fingerprint(string(cliMigrationJSONBytes(struct {
		Messages []cliMigrationInput
		Objects  []cliMigrationSourceObject
	}{inputs, objects}))), nil
}
func (c *client) cliMigrationSource(o cliMigrationIntent, b dataExportBinding, current func() bool, write bool) (any, error) {
	var result map[string]json.RawMessage
	if e := c.cliMigrationJSON(b, current, "POST", "/api/sync/migration-results", map[string]string{"requestId": o.ID, "sourceStoreGeneration": b.Physical}, &result); e != nil {
		return nil, e
	}
	var status string
	_ = json.Unmarshal(result["status"], &status)
	if status != "not_found" {
		return c.cliMigrationRememberSource(o, b, current, result)
	}
	if !write {
		return result, nil
	}
	if e := c.cliMigrationCapability(b, current); e != nil {
		return nil, e
	}
	proofBytes, e := secretInput(c.reader, "Original target pairing proof (RAM only): ")
	if e != nil {
		return nil, e
	}
	defer erase(proofBytes)
	proof := strings.TrimSpace(string(proofBytes))
	pairing, e := cliMigrationPair(proof, b)
	if e != nil || pairing.ID != o.Pairing || pairing.Device != o.TargetDevice || pairing.Store != o.TargetStore || pairing.Device == b.Identity.DeviceID {
		return nil, syncRefuse("MIGRATION_TARGET_CHANGED")
	}
	inputs, objects, digest, e := c.cliMigrationSourceInputs(o, b, current)
	if e != nil {
		return nil, e
	}
	for i := range objects {
		defer erase(objects[i].Bytes)
	}
	if o.SourceDigest != "" && digest != o.SourceDigest {
		return nil, syncRefuse("MIGRATION_ORIGINAL_CHANGED")
	}
	password, otp, e := c.cliMigrationProof()
	if e != nil {
		return nil, e
	}
	defer func() { password = ""; otp = ""; proof = "" }()
	_, e = c.store.cliMigrationChange(b, current, o.ID, nil, func(row *cliMigrationIntent) error { row.SourceDigest = digest; row.State = "unknown"; return nil })
	if e != nil {
		return nil, e
	}
	request := map[string]any{"requestId": o.ID, "sourceContextId": "personal", "sourceStoreGeneration": b.Physical, "restoreEpoch": b.Epoch, "targetPairingProof": proof, "password": password, "otp": otp, "messages": inputs, "objects": objects}
	e = c.cliMigrationJSON(b, current, "POST", "/api/sync/migrations", request, &result)
	if e != nil {
		return nil, e
	}
	return c.cliMigrationRememberSource(o, b, current, result)
}

func (c *client) cliMigrationRememberSource(o cliMigrationIntent, b dataExportBinding, current func() bool, result map[string]json.RawMessage) (any, error) {
	var view cliMigrationView
	if cliMigrationDecode(cliMigrationJSONBytes(result), &view) != nil || view.Request != o.ID || view.Manifest.TargetDevice != o.TargetDevice || view.Manifest.TargetStore != o.TargetStore {
		return nil, syncRefuse("MIGRATION_ORIGINAL_CHANGED")
	}
	if !oneOf(view.Status, "building", "ready", "installed_reported", "revoking", "retired") {
		return nil, syncRefuse("MIGRATION_INVALID_RESPONSE")
	}
	// Historical queries do not acquire keys or republish a retired package.
	// Validate its original manifest at its deadline, retaining the real status.
	check := view
	check.Status = "ready"
	if check.Size == "0" && check.CapsuleDigest == "" {
		check.Size, check.CapsuleDigest = "36", strings.Repeat("0", 64)
	}
	observedAt := time.Now()
	expiry, parseErr := time.Parse(time.RFC3339Nano, view.Expires)
	if parseErr != nil {
		return nil, syncRefuse("MIGRATION_INVALID_RESPONSE")
	}
	if !expiry.After(observedAt) {
		observedAt = expiry.Add(-time.Nanosecond)
	}
	if err := cliMigrationCheckView(check, b, false, observedAt); err != nil {
		return nil, err
	}
	_, err := c.store.cliMigrationChange(b, current, o.ID, nil, func(row *cliMigrationIntent) error {
		row.Migration, row.Digest, row.State = view.ID, view.Digest, "ready"
		if view.Status == "retired" || view.Status == "revoking" || !expiry.After(time.Now()) {
			row.State = "retired"
		}
		return nil
	})
	return result, err
}
func (c *client) cliMigrationImport(o cliMigrationIntent, b dataExportBinding, control syncControl, current func() bool) (any, error) {
	var view cliMigrationView
	if e := c.cliMigrationJSON(b, current, "GET", "/api/sync/migrations/"+o.Migration, nil, &view); e != nil {
		return nil, e
	}
	if o.State == "installed" {
		return map[string]any{"localCommitted": true, "id": o.Migration, "manifestDigest": o.Digest, "serverStatus": view.Status, "sourcePreserved": true, "qualifiedAck": false}, nil
	}
	if e := cliMigrationCheckView(view, b, true, time.Now()); e != nil {
		return nil, e
	}
	if view.Status != "ready" {
		return nil, syncRefuse("MIGRATION_ALREADY_REPORTED")
	}
	var e error
	o, e = c.store.cliMigrationChange(b, current, o.ID, nil, func(row *cliMigrationIntent) error { row.Digest = view.Digest; row.State = "unknown"; return nil })
	if e != nil {
		return nil, e
	}
	var capsule []byte
	e = c.store.db.QueryRow("SELECT capsule FROM migration_stages WHERE epoch=? AND id=? AND digest=?", b.Epoch, o.Migration, view.Digest).Scan(&capsule)
	if e != nil {
		capsule, e = c.cliMigrationHTTP(b, current, "GET", "/api/sync/migrations/"+o.Migration+"/capsule", nil, true, cliMigrationLimit+4096)
		if e != nil {
			return nil, e
		}
		if fingerprint(string(capsule)) != view.CapsuleDigest {
			return nil, syncRefuse("MIGRATION_CAPSULE_INVALID")
		}
		tx, err := c.store.db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err = c.store.receiptControl(tx, &control, current, true); err != nil {
			return nil, err
		}
		for _, ref := range view.Manifest.Messages {
			var n int
			if err = tx.QueryRow("SELECT count(*) FROM sync_terminal WHERE epoch=? AND id=?", b.Epoch, ref.ID).Scan(&n); err != nil || n > 0 {
				return nil, syncRefuse("MIGRATION_GLOBAL_FLOOR")
			}
			if _, err = tx.Exec("INSERT OR IGNORE INTO migration_stage_messages VALUES(?,?,?)", b.Epoch, o.Migration, ref.ID); err != nil {
				return nil, err
			}
		}
		if _, err = tx.Exec("INSERT INTO migration_stages VALUES(?,?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET capsule=excluded.capsule WHERE digest=excluded.digest", b.Epoch, o.Migration, view.Digest, capsule); err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
	}
	if len(capsule) < 36 || len(capsule) > cliMigrationLimit+4096 || string(capsule[:8]) != "JIMMIGR1" || fmt.Sprint(len(capsule)) != view.Size || fingerprint(string(capsule)) != view.CapsuleDigest {
		return nil, syncRefuse("MIGRATION_CAPSULE_INVALID")
	}
	password, otp, e := c.cliMigrationProof()
	if e != nil {
		return nil, e
	}
	defer func() { password = ""; otp = "" }()
	var key cliMigrationKey
	if e = c.cliMigrationJSON(b, current, "POST", "/api/sync/migrations/"+o.Migration+"/key", map[string]string{"manifestDigest": view.Digest, "password": password, "otp": otp}, &key); e != nil {
		return nil, e
	}
	packet, e := cliMigrationOpen(view, key, capsule, b, time.Now())
	key.Key = ""
	if e != nil {
		return nil, e
	}
	for i := range packet.Objects {
		defer erase(packet.Objects[i].Bytes)
	}
	var validation struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Digest    string `json:"manifestDigest"`
		Token     string `json:"installToken"`
		Before    string `json:"installBefore"`
		Atomic    *bool  `json:"requiresAtomicLocalCommit"`
		Qualified *bool  `json:"qualifiedAck"`
		Target    *bool  `json:"targetQualified"`
	}
	if e = c.cliMigrationJSON(b, current, "POST", "/api/sync/migrations/"+o.Migration+"/validate", map[string]string{"manifestDigest": view.Digest}, &validation); e != nil {
		return nil, e
	}
	deadline, de := time.Parse(time.RFC3339Nano, validation.Before)
	if de != nil || !deadline.After(time.Now()) || deadline.After(time.Now().Add(6*time.Second)) || validation.ID != o.Migration || validation.Status != "validated" || validation.Digest != view.Digest || len(validation.Token) < 16 || len(validation.Token) > 4096 || validation.Atomic == nil || !*validation.Atomic || !receiptFalse(validation.Qualified) || !receiptFalse(validation.Target) {
		return nil, syncRefuse("MIGRATION_INSTALL_INVALID")
	}
	if e = c.store.cliMigrationCommit(b, control, current, o, view, packet, deadline); e != nil {
		return nil, e
	}
	var result map[string]json.RawMessage
	e = c.cliMigrationJSON(b, current, "POST", "/api/sync/migrations/"+o.Migration+"/installed", map[string]string{"manifestDigest": view.Digest, "installToken": validation.Token}, &result)
	if e != nil {
		return map[string]any{"id": o.Migration, "localCommitted": true, "reportUnknown": true, "qualifiedAck": false, "sourcePreserved": true}, e
	}
	return result, nil
}

func (c *client) migrationCommand(cmd, args string) (bool, error) {
	if !strings.HasPrefix(cmd, "migration-") {
		return false, nil
	}
	s, b, control, current, e := c.cliMigrationContext()
	if e != nil {
		return true, e
	}
	parts := strings.Fields(args)
	if cmd == "migration-capabilities" {
		return true, c.cliMigrationCapability(b, current)
	}
	if cmd == "migration-list" {
		rows, e := s.db.Query("SELECT payload FROM migration_intents ORDER BY rowid")
		if e != nil {
			return true, e
		}
		defer rows.Close()
		result := []json.RawMessage{}
		for rows.Next() {
			var raw string
			if e = rows.Scan(&raw); e != nil {
				return true, e
			}
			result = append(result, json.RawMessage(raw))
		}
		return true, printJSON(result)
	}
	if cmd == "migration-history" {
		if len(parts) != 1 || !syncUUIDPattern.MatchString(parts[0]) {
			return true, fmt.Errorf("migration-history ARCHIVE_ID")
		}
		result, e := c.cliMigrationHistory(parts[0], b, current)
		if e != nil {
			return true, e
		}
		return true, printJSON(result)
	}
	if cmd == "migration-file" {
		if len(parts) != 3 {
			return true, fmt.Errorf("migration-file ARCHIVE_ID OBJECT_ID NEW_DESTINATION")
		}
		return true, c.cliMigrationSaveObject(parts[0], parts[1], parts[2], b, current)
	}
	if cmd == "migration-pair" || cmd == "migration-create" || cmd == "migration-import" {
		id, e := syncNewUUID()
		if e != nil {
			return true, e
		}
		o := cliMigrationIntent{ID: id, Binding: b, Created: time.Now().UTC().Format(time.RFC3339Nano), State: "prepared"}
		switch cmd {
		case "migration-pair":
			if len(parts) != 0 {
				return true, fmt.Errorf("migration-pair")
			}
			o.Kind = "pair"
		case "migration-import":
			if len(parts) != 1 || !syncUUIDPattern.MatchString(parts[0]) {
				return true, fmt.Errorf("migration-import MIGRATION_ID")
			}
			o.Kind = "target"
			o.Migration = parts[0]
			var previous string
			lookup := s.db.QueryRow("SELECT id FROM migration_intents WHERE epoch=? AND json_extract(payload,'$.kind')='target' AND json_extract(payload,'$.migrationId')=? ORDER BY rowid LIMIT 1", b.Epoch, o.Migration).Scan(&previous)
			if lookup == nil {
				old, err := cliMigrationLoad(s.db, previous)
				if err != nil || old.Binding != b {
					return true, syncRefuse("MIGRATION_ORIGINAL_FAMILY_REQUIRED")
				}
				result, err := c.cliMigrationImport(old, b, control, current)
				if result != nil {
					_ = printJSON(result)
				}
				return true, err
			}
		case "migration-create":
			if len(parts) != 1 {
				return true, fmt.Errorf("migration-create SELECTION_JSON_FILE")
			}
			file, e := os.Open(parts[0])
			if e != nil {
				return true, e
			}
			raw, e := readLimited(file, 128<<10)
			file.Close()
			if e != nil || cliMigrationDecode(raw, &o.Selection) != nil {
				return true, syncRefuse("MIGRATION_SELECTION_INVALID")
			}
			proof, e := secretInput(c.reader, "Target pairing proof (RAM only; re-enter when sending): ")
			if e != nil {
				return true, e
			}
			p, e := cliMigrationPair(strings.TrimSpace(string(proof)), b)
			erase(proof)
			if e != nil || p.Device == b.Identity.DeviceID {
				return true, syncRefuse("MIGRATION_TARGET_CHANGED")
			}
			o.Kind = "source"
			o.Pairing = p.ID
			o.TargetDevice = p.Device
			o.TargetStore = p.Store
		}
		if _, e = s.cliMigrationChange(b, current, o.ID, &o, nil); e != nil {
			return true, e
		}
		fmt.Println("Original migration request:", o.ID)
		var result any
		if o.Kind == "pair" {
			result, e = c.cliMigrationPairingIntent(o, b, current, true)
		} else if o.Kind == "source" {
			result, e = c.cliMigrationSource(o, b, current, true)
		} else {
			result, e = c.cliMigrationImport(o, b, control, current)
		}
		if result != nil {
			_ = printJSON(result)
		}
		return true, e
	}
	if len(parts) != 1 || !syncUUIDPattern.MatchString(parts[0]) {
		return true, fmt.Errorf("%s ORIGINAL_REQUEST_ID", cmd)
	}
	o, e := cliMigrationLoad(s.db, parts[0])
	if e != nil {
		return true, e
	}
	if o.Binding != b {
		return true, syncRefuse("MIGRATION_ORIGINAL_FAMILY_REQUIRED")
	}
	var result any
	switch cmd {
	case "migration-status", "migration-retry":
		write := cmd == "migration-retry"
		if o.Kind == "pair" {
			result, e = c.cliMigrationPairingIntent(o, b, current, write)
		} else if o.Kind == "source" {
			result, e = c.cliMigrationSource(o, b, current, write)
		} else if write {
			result, e = c.cliMigrationImport(o, b, control, current)
		} else {
			var raw map[string]json.RawMessage
			e = c.cliMigrationJSON(b, current, "GET", "/api/sync/migrations/"+o.Migration, nil, &raw)
			result = raw
		}
	case "migration-cancel":
		if o.Migration == "" {
			return true, fmt.Errorf("query original source result first; cancel requires its migration identity")
		}
		var view cliMigrationView
		if e = c.cliMigrationJSON(b, current, "GET", "/api/sync/migrations/"+o.Migration, nil, &view); e != nil {
			return true, e
		}
		var raw map[string]json.RawMessage
		e = c.cliMigrationJSON(b, current, "POST", "/api/sync/migrations/"+o.Migration+"/cancel", map[string]string{"manifestDigest": view.Digest}, &raw)
		result = raw
		if e == nil {
			_, e = s.db.Exec("DELETE FROM migration_stages WHERE epoch=? AND id=?; DELETE FROM migration_stage_messages WHERE epoch=? AND id=?", b.Epoch, o.Migration, b.Epoch, o.Migration)
		}
	default:
		return true, fmt.Errorf("unknown migration command")
	}
	if result != nil {
		_ = printJSON(result)
	}
	return true, e
}
