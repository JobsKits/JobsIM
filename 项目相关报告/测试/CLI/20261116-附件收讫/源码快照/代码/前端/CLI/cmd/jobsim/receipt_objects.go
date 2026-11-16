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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const receiptFileContract = "native_sqlite_full_file_fsync_v1"

type receiptObject struct {
	ID          string `json:"objectId"`
	Size        string `json:"sizeBytes"`
	Digest      string `json:"sha256"`
	ContentType string `json:"contentType"`
}
type receiptLocalObject struct {
	Object      string `json:"objectId"`
	Size        string `json:"sizeBytes"`
	Digest      string `json:"sha256"`
	ContentType string `json:"contentType"`
	Path        string `json:"relativePath"`
	Parent      string `json:"parentReceiptId"`
	Request     string `json:"requestId"`
	Message     string `json:"messageId"`
	Epoch       string `json:"restoreEpoch"`
}
type receiptObjectChallenge struct {
	Challenge     string        `json:"challenge"`
	Parent        string        `json:"parentReceiptId"`
	Message       string        `json:"messageId"`
	Conversation  string        `json:"conversationId"`
	Content       string        `json:"contentVersion"`
	Epoch         string        `json:"restoreEpoch"`
	Store         string        `json:"storeGeneration"`
	Qualification string        `json:"qualificationId"`
	Digest        string        `json:"bodyDigest"`
	Object        receiptObject `json:"object"`
	Expires       string        `json:"expiresAt"`
	Durable       *bool         `json:"requiresDurableOriginalObject"`
	Complete      *bool         `json:"objectComplete"`
	Target        *bool         `json:"targetRangeComplete"`
	Deletion      *bool         `json:"deletionEligible"`
}
type receiptObjectProof struct {
	receiptProofTuple
	Parent       string `json:"parentReceiptId"`
	Object       string `json:"objectId"`
	Size         string `json:"sizeBytes"`
	ObjectDigest string `json:"objectDigest"`
}

func receiptObjectSize(raw string) (int64, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 || n > 64<<20 || strconv.FormatInt(n, 10) != raw {
		return 0, syncRefuse("RECEIPT_OBJECT_SIZE_INVALID")
	}
	return n, nil
}
func receiptObjectPath(epoch, request string) string {
	return "ReceiptObjects/" + epoch + "/" + request + ".bin"
}
func receiptCheckObjectIntent(o receiptOperation, body map[string]json.RawMessage) error {
	get := func(key string) string { var v string; _ = json.Unmarshal(body[key], &v); return v }
	d := o.Object
	if d == nil || len(body) != 10 || o.Method != "POST" || o.Route != "/api/sync/object-receipts" || get("kind") != "object_received" || !syncUUIDPattern.MatchString(get("qualificationId")) || !strings.HasPrefix(get("challenge"), "ro1.") || get("parentReceiptId") != d.Parent || get("objectId") != d.Object || get("sizeBytes") != d.Size || get("objectDigest") != d.Digest {
		return syncRefuse("RECEIPT_INTENT_CHANGED")
	}
	if !syncUUIDPattern.MatchString(d.Parent) || !syncUUIDPattern.MatchString(d.Object) || !syncDigestPattern.MatchString(d.Digest) || !syncDigestPattern.MatchString(o.Digest) || d.ContentType == "" || d.Request != o.ID || d.Message != o.Message || d.Epoch != o.Binding.RestoreEpoch || d.Path != receiptObjectPath(d.Epoch, o.ID) {
		return syncRefuse("RECEIPT_INTENT_CHANGED")
	}
	_, err := receiptObjectSize(d.Size)
	return err
}
func receiptObjectQualification(v receiptStorageView) error {
	if !v.Storage.Qualified || !v.Storage.ObjectQualified || v.Storage.Contract != receiptFileContract {
		return syncRefuse("OBJECT_STORAGE_NOT_QUALIFIED")
	}
	return receiptLocalBuild(v.Storage.Build)
}
func receiptParent(q syncSQL, id string, binding syncBinding) (receiptOperation, error) {
	o, err := loadReceiptQuery(q, id)
	if err != nil {
		return o, err
	}
	if o.Kind != "message_content" || o.State != "completed" || o.Binding.check(binding, false) != nil || receiptValidateResult(o.Result, o) != nil {
		return o, syncRefuse("CONTENT_RECEIPT_REQUIRED")
	}
	return o, nil
}
func receiptParentQualification(o receiptOperation) string {
	var body struct {
		Qualification string `json:"qualificationId"`
	}
	_ = syncDecode([]byte(o.Body), &body)
	return body.Qualification
}
func receiptValidateObjectChallenge(v receiptObjectChallenge, p receiptOperation, permission string, now time.Time) error {
	if v.Parent != p.ID || v.Message != p.Message || v.Conversation != p.Conversation || v.Content != p.Content || v.Digest != p.Digest || v.Store != p.Binding.StoreGeneration || v.Epoch != p.Binding.RestoreEpoch || v.Qualification != receiptParentQualification(p) || v.Durable == nil || !*v.Durable || !receiptFalse(v.Complete) || !receiptFalse(v.Target) || !receiptFalse(v.Deletion) || !syncUUIDPattern.MatchString(v.Object.ID) || !syncDigestPattern.MatchString(v.Object.Digest) || strings.TrimSpace(v.Object.ContentType) == "" {
		return syncRefuse("OBJECT_RECEIPT_CONTEXT_CHANGED")
	}
	if _, err := receiptObjectSize(v.Object.Size); err != nil {
		return err
	}
	expires, err := time.Parse(time.RFC3339Nano, v.Expires)
	if err != nil || !expires.After(now) || expires.After(now.Add(time.Hour+time.Minute)) {
		return syncRefuse("RECEIPT_PROOF_EXPIRED")
	}
	parts := strings.Split(v.Challenge, ".")
	if len(v.Challenge) > 8192 || len(parts) != 3 || parts[0] != "ro1" {
		return syncRefuse("INVALID_OBJECT_RECEIPT_PROOF")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	sig, sigErr := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	var proof receiptObjectProof
	if err != nil || sigErr != nil || len(sig) != 32 || syncDecode(raw, &proof) != nil {
		return syncRefuse("INVALID_OBJECT_RECEIPT_PROOF")
	}
	want := receiptProofTuple{Scope: p.Binding.ScopeID, User: p.Binding.PrincipalID, Device: p.Binding.DeviceID, Store: v.Store, Epoch: v.Epoch, Qualification: v.Qualification, Message: v.Message, Conversation: v.Conversation, Content: v.Content, Digest: v.Digest, Permission: permission, Expires: expires.Unix()}
	// This is tuple consistency only; the server alone authenticates the ro1 signature.
	if proof.receiptProofTuple != want || proof.Parent != p.ID || proof.Object != v.Object.ID || proof.Size != v.Object.Size || proof.ObjectDigest != v.Object.Digest {
		return syncRefuse("OBJECT_RECEIPT_CONTEXT_CHANGED")
	}
	return nil
}

func receiptSyncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func receiptDirectory(path string, create bool) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) && create {
		if err = os.Mkdir(path, 0700); err != nil {
			return err
		}
		if err = receiptSyncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return syncRefuse("RECEIPT_OBJECT_UNSAFE_PATH")
	}
	return nil
}
func (s *localStore) receiptObjectFile(d receiptLocalObject, create bool) (string, error) {
	if d.Path != receiptObjectPath(d.Epoch, d.Request) || !syncUUIDPattern.MatchString(d.Request) {
		return "", syncRefuse("RECEIPT_OBJECT_UNSAFE_PATH")
	}
	if n, err := syncCounter(d.Epoch); err != nil || n == 0 {
		return "", syncRefuse("RECEIPT_OBJECT_UNSAFE_PATH")
	}
	root := filepath.Dir(s.path)
	if err := receiptDirectory(root, false); err != nil {
		return "", err
	}
	for _, part := range []string{"ReceiptObjects", d.Epoch} {
		root = filepath.Join(root, part)
		if err := receiptDirectory(root, create); err != nil {
			return "", err
		}
	}
	return filepath.Join(filepath.Dir(s.path), filepath.FromSlash(d.Path)), nil
}
func receiptHashFile(f *os.File, d receiptLocalObject) error {
	size, err := receiptObjectSize(d.Size)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return syncRefuse("RECEIPT_OBJECT_FILE_CHANGED")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, size+1))
	if err != nil || n != size || hex.EncodeToString(h.Sum(nil)) != d.Digest {
		return syncRefuse("RECEIPT_OBJECT_INTEGRITY")
	}
	return nil
}
func (s *localStore) openReceiptObject(d receiptLocalObject) (*os.File, error) {
	path, err := s.receiptObjectFile(d, false)
	if err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, syncRefuse("RECEIPT_OBJECT_UNSAFE_PATH")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*os.File, error) { f.Close(); return nil, e }
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return fail(syncRefuse("RECEIPT_OBJECT_FILE_CHANGED"))
	}
	if err = receiptHashFile(f, d); err != nil {
		return fail(err)
	}
	if err = f.Sync(); err != nil {
		return fail(err)
	}
	if err = receiptSyncDirectory(filepath.Dir(path)); err != nil {
		return fail(err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) {
		return fail(syncRefuse("RECEIPT_OBJECT_FILE_CHANGED"))
	}
	return f, nil
}
func (s *localStore) persistReceiptObject(d receiptLocalObject, source io.Reader) error {
	path, err := s.receiptObjectFile(d, true)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	size, err := receiptObjectSize(d.Size)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(source, size+1))
	if err != nil || n != size || hex.EncodeToString(h.Sum(nil)) != d.Digest {
		return syncRefuse("RECEIPT_OBJECT_INTEGRITY")
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// A crash before the SQL transaction may leave an unreferenced private file,
	// never an acknowledgement or a replacement for an earlier receipt intent.
	return receiptSyncDirectory(filepath.Dir(path))
}
func (s *localStore) stageReceiptObjectTx(tx *sql.Tx, control syncControl, o receiptOperation) error {
	parent, err := receiptParent(tx, o.Object.Parent, control.syncBinding)
	if err != nil {
		return err
	}
	if parent.Message != o.Message || parent.Conversation != o.Conversation || parent.Content != o.Content || parent.Digest != o.Digest || receiptParentQualification(parent) != receiptParentQualification(o) {
		return syncRefuse("OBJECT_RECEIPT_PARENT_CHANGED")
	}
	var terminal int
	if err = tx.QueryRow("SELECT count(*) FROM sync_terminal WHERE id=?", o.Message).Scan(&terminal); err != nil {
		return err
	}
	if terminal != 0 {
		return syncRefuse("RECEIPT_CONTENT_CHANGED")
	}
	raw, _ := json.Marshal(o.Object)
	_, err = tx.Exec("INSERT INTO receipt_objects VALUES(?,?,?,?,?,'ready')", o.ID, s.epoch, o.Message, string(raw), fingerprint(string(raw)))
	return err
}
func (s *localStore) checkReceiptObjectDescriptor(o receiptOperation) error {
	var raw, hash, state string
	if err := s.db.QueryRow("SELECT descriptor,descriptor_hash,state FROM receipt_objects WHERE request_id=? AND epoch=? AND message_id=?", o.ID, o.Binding.RestoreEpoch, o.Message).Scan(&raw, &hash, &state); err != nil {
		return err
	}
	want, _ := json.Marshal(o.Object)
	if state != "ready" || fingerprint(raw) != hash || raw != string(want) {
		return syncRefuse("RECEIPT_OBJECT_DESCRIPTOR_CHANGED")
	}
	return nil
}
func receiptValidateObjectResult(raw json.RawMessage, o receiptOperation) error {
	var v struct {
		ID            string `json:"operationId"`
		Status        string `json:"status"`
		Kind          string `json:"receiptKind"`
		Parent        string `json:"parentReceiptId"`
		Message       string `json:"messageId"`
		Conversation  string `json:"conversationId"`
		Content       string `json:"contentVersion"`
		Epoch         string `json:"restoreEpoch"`
		Device        string `json:"deviceId"`
		Store         string `json:"storeGeneration"`
		Qualification string `json:"qualificationId"`
		Object        string `json:"objectId"`
		Size          string `json:"sizeBytes"`
		ObjectDigest  string `json:"objectDigest"`
		BodyDigest    string `json:"bodyDigest"`
		Qualified     *bool  `json:"storageQualifiedAtCommit"`
		Complete      *bool  `json:"objectComplete"`
		Target        *bool  `json:"targetRangeComplete"`
		Deletion      *bool  `json:"deletionEligible"`
	}
	d := o.Object
	if d == nil || syncDecode(raw, &v) != nil || v.ID != o.ID || v.Status != "recorded" || v.Kind != "object_received" || v.Parent != d.Parent || v.Message != o.Message || v.Conversation != o.Conversation || v.Content != o.Content || v.Epoch != o.Binding.RestoreEpoch || v.Device != o.Binding.DeviceID || v.Store != o.Binding.StoreGeneration || v.Qualification != receiptParentQualification(o) || v.Object != d.Object || v.Size != d.Size || v.ObjectDigest != d.Digest || v.BodyDigest != o.Digest || v.Qualified == nil || !*v.Qualified || v.Complete == nil || !*v.Complete || !receiptFalse(v.Target) || !receiptFalse(v.Deletion) {
		return syncRefuse("OBJECT_RECEIPT_RESULT_INVALID")
	}
	return nil
}
func (c *client) performObjectReceipt(s *localStore, current func() bool, control syncControl, o receiptOperation, explicit, fresh bool) (json.RawMessage, error) {
	if err := c.receiptCapabilitiesFor(true); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err := c.call("POST", "/api/sync/object-receipt-results", json.RawMessage(o.Body), &raw)
	if err == nil {
		if err = receiptValidateObjectResult(raw, o); err != nil {
			return nil, err
		}
		return raw, s.finishReceipt(o, control, raw, "completed", current)
	}
	var api *apiFailure
	if !errors.As(err, &api) || api.Status != 404 || api.Code != "OBJECT_RECEIPT_OPERATION_NOT_FOUND" || (!explicit && !fresh) || o.State != "pending" {
		return nil, err
	}
	if _, err = s.receiptControl(s.db, &control, current, true); err != nil {
		return nil, err
	}
	deadline, e := time.Parse(time.RFC3339Nano, o.Expires)
	if e != nil || !deadline.After(time.Now()) {
		_ = s.finishReceipt(o, control, nil, "frozen", current)
		return nil, syncRefuse("RECEIPT_PROOF_EXPIRED")
	}
	storage, err := c.getReceiptStorage(control)
	if err != nil {
		return nil, err
	}
	if err = receiptObjectQualification(storage); err != nil {
		return nil, err
	}
	if storage.Storage.Qualification != receiptParentQualification(o) {
		return nil, syncRefuse("STORAGE_NOT_QUALIFIED")
	}
	parent, err := receiptParent(s.db, o.Object.Parent, control.syncBinding)
	if err != nil {
		return nil, err
	}
	if parent.Message != o.Message || parent.Conversation != o.Conversation || parent.Content != o.Content || parent.Digest != o.Digest || receiptParentQualification(parent) != receiptParentQualification(o) {
		return nil, syncRefuse("OBJECT_RECEIPT_PARENT_CHANGED")
	}
	if err = s.checkReceiptObjectDescriptor(o); err != nil {
		return nil, err
	}
	var terminal int
	if err = s.db.QueryRow("SELECT count(*) FROM sync_terminal WHERE id=?", o.Message).Scan(&terminal); err != nil {
		return nil, err
	}
	if terminal != 0 {
		return nil, syncRefuse("RECEIPT_CONTENT_CHANGED")
	}
	f, err := s.openReceiptObject(*o.Object)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = receiptDurability(s.db); err != nil {
		return nil, err
	}
	if err = c.receiptCapabilitiesFor(true); err != nil {
		return nil, err
	}
	if _, err = s.receiptControl(s.db, &control, current, true); err != nil {
		return nil, err
	}
	path, err := s.receiptObjectFile(*o.Object, false)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	latest, pathErr := os.Lstat(path)
	if err != nil || pathErr != nil || !os.SameFile(opened, latest) {
		return nil, syncRefuse("RECEIPT_OBJECT_FILE_CHANGED")
	}
	if err = receiptHashFile(f, *o.Object); err != nil {
		return nil, err
	}
	err = c.call(o.Method, o.Route, json.RawMessage(o.Body), &raw)
	if err != nil {
		return nil, err
	}
	if err = receiptValidateObjectResult(raw, o); err != nil {
		return nil, err
	}
	return raw, s.finishReceipt(o, control, raw, "completed", current)
}
func (c *client) receiveReceiptObject(parentID, sourcePath string) (json.RawMessage, error) {
	s, current, err := c.receiptContext()
	if err != nil {
		return nil, err
	}
	control, err := s.receiptControl(s.db, nil, current, true)
	if err != nil {
		return nil, err
	}
	parent, err := receiptParent(s.db, parentID, control.syncBinding)
	if err != nil {
		return nil, err
	}
	var pendingID string
	err = s.db.QueryRow("SELECT id FROM receipt_outbox WHERE epoch=? AND state='pending' AND json_extract(payload,'$.kind')='object_received' AND json_extract(payload,'$.object.parentReceiptId')=? ORDER BY rowid LIMIT 1", s.epoch, parentID).Scan(&pendingID)
	if err == nil {
		return c.performReceipt(pendingID, false, false)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err = c.receiptCapabilitiesFor(true); err != nil {
		return nil, err
	}
	storage, err := c.getReceiptStorage(control)
	if err != nil {
		return nil, err
	}
	if err = receiptObjectQualification(storage); err != nil {
		return nil, err
	}
	if storage.Storage.Qualification != receiptParentQualification(parent) {
		return nil, syncRefuse("STORAGE_NOT_QUALIFIED")
	}
	var challenge receiptObjectChallenge
	if err = c.call("POST", "/api/sync/object-receipt-challenges", map[string]string{"parentReceiptId": parent.ID, "schemaVersion": control.schema()}, &challenge); err != nil {
		return nil, err
	}
	if err = receiptValidateObjectChallenge(challenge, parent, control.Permission, time.Now()); err != nil {
		return nil, err
	}
	request, err := syncNewUUID()
	if err != nil {
		return nil, err
	}
	d := receiptLocalObject{Object: challenge.Object.ID, Size: challenge.Object.Size, Digest: challenge.Object.Digest, ContentType: challenge.Object.ContentType, Path: receiptObjectPath(s.epoch, request), Parent: parent.ID, Request: request, Message: parent.Message, Epoch: s.epoch}
	if sourcePath != "" {
		before, e := os.Lstat(sourcePath)
		if e != nil {
			return nil, e
		}
		if !before.Mode().IsRegular() {
			return nil, syncRefuse("RECEIPT_OBJECT_UNSAFE_SOURCE")
		}
		file, e := os.Open(sourcePath)
		if e != nil {
			return nil, e
		}
		defer file.Close()
		opened, e := file.Stat()
		if e != nil || !os.SameFile(before, opened) {
			return nil, syncRefuse("RECEIPT_OBJECT_FILE_CHANGED")
		}
		err = s.persistReceiptObject(d, file)
	} else {
		response, e := c.authenticatedResponse("GET", "/api/media/"+d.Object, "", nil, 120*time.Second)
		if e != nil {
			return nil, e
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("original object download HTTP %d; no receipt staged", response.StatusCode)
		}
		err = s.persistReceiptObject(d, response.Body)
	}
	if err != nil {
		return nil, err
	}
	if !current() {
		return nil, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	body, _ := json.Marshal(map[string]string{"requestId": request, "storeGeneration": control.StoreGeneration, "revisionEpoch": s.epoch, "qualificationId": challenge.Qualification, "kind": "object_received", "parentReceiptId": parent.ID, "challenge": challenge.Challenge, "objectId": d.Object, "sizeBytes": d.Size, "objectDigest": d.Digest})
	o := receiptOperation{ID: request, Kind: "object_received", Binding: control.syncBinding, Method: "POST", Route: "/api/sync/object-receipts", Body: string(body), Message: parent.Message, Conversation: parent.Conversation, Content: parent.Content, Digest: parent.Digest, Expires: challenge.Expires, State: "pending", Object: &d}
	o.Hash = receiptHash(o)
	if err = s.stageReceipt(control, o, nil, current); err != nil {
		return nil, err
	}
	return c.performReceipt(o.ID, false, true)
}
func (c *client) receiptObjectCommand(cmd, args string, s *localStore) error {
	if cmd == "receipt-file-status" {
		o, err := s.loadReceipt(strings.TrimSpace(args))
		if err != nil {
			return err
		}
		if o.Object == nil {
			return syncRefuse("OBJECT_RECEIPT_REQUIRED")
		}
		_, current, err := c.receiptContext()
		if err != nil {
			return err
		}
		control, err := s.receiptControl(s.db, nil, current, false)
		if err != nil || o.Binding.check(control.syncBinding, false) != nil {
			return syncRefuse("RECEIPT_CONTEXT_CHANGED")
		}
		err = s.checkReceiptObjectDescriptor(o)
		if err == nil {
			var file *os.File
			file, err = s.openReceiptObject(*o.Object)
			if file != nil {
				file.Close()
			}
		}
		return printJSON(map[string]any{"requestId": o.ID, "state": o.State, "objectId": o.Object.Object, "relativePath": o.Object.Path, "sizeBytes": o.Object.Size, "sha256": o.Object.Digest, "localFileVerified": err == nil, "serverResultRecorded": o.State == "completed", "targetRangeComplete": false, "deletionEligible": false, "afterReceiptEnabled": false})
	}
	parent, source := strings.TrimSpace(args), ""
	if cmd == "receive-object-file" {
		var found bool
		parent, source, found = strings.Cut(parent, " ")
		source = strings.TrimSpace(source)
		if !found || source == "" {
			return fmt.Errorf("receive-object-file PARENT_RECEIPT_ID PATH")
		}
	}
	if !syncUUIDPattern.MatchString(parent) {
		return syncRefuse("CONTENT_RECEIPT_REQUIRED")
	}
	raw, err := c.receiveReceiptObject(parent, source)
	if err != nil {
		return err
	}
	return printJSON(raw)
}
