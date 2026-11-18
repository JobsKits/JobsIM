// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type cliMigrationFixture struct {
	f       *cliSyncFixture
	b       dataExportBinding
	control syncControl
	intent  cliMigrationIntent
	view    cliMigrationView
	key     cliMigrationKey
	capsule []byte
	packet  cliMigrationPacket
}

func newCLIMigrationFixture(t *testing.T, full, forwarded bool) *cliMigrationFixture {
	return newCLIMigrationFixtureWithOriginal(t, full, forwarded, []byte("全部原件\x00\xffbytes"))
}
func newCLIMigrationFixtureWithOriginal(t *testing.T, full, forwarded bool, original []byte) *cliMigrationFixture {
	t.Helper()
	f := newCLISyncFixture(t)
	if err := f.client.authoritativeSync(); err != nil {
		t.Fatal(err)
	}
	control, ok, err := syncLoadControl(f.store.db, "1")
	if err != nil || !ok {
		t.Fatal(err)
	}
	b := dataExportBinding{Identity: f.identity, Physical: f.physical, Epoch: "1", Family: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}
	_, err = f.store.db.Exec("UPDATE conversations SET payload=json_set(payload,'$.membershipGeneration','1','$.state','active') WHERE epoch='1' AND id=?", f.conversation)
	if err != nil {
		t.Fatal(err)
	}
	message := cliMigrationMessage{ID: f.messageID, Conversation: f.conversation, Sender: f.identity.UserID, Type: "text", Text: "精确原字节 <&> བོད་", Created: "2026-11-17T00:00:00.123456789Z", Content: "9007199254740993", CreationEpoch: "1", CreationSequence: "42", Mode: "none", Forwarded: forwarded}
	objects := []cliMigrationObject{}
	refs := []cliMigrationObjectReference{}
	if full {
		message.Type = "file"
		message.Mode = "full"
		message.Object = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		data := append([]byte{}, original...)
		objects = append(objects, cliMigrationObject{message.Object, message.ID, data})
		refs = append(refs, cliMigrationObjectReference{ID: message.Object, Message: message.ID, Size: "20", Digest: fingerprint(string(data)), ContentType: "application/octet-stream", Classification: "available"})
		refs[0].Size = fmtInt(len(data))
	}
	raw := bytes.ReplaceAll(cliMigrationJSONBytes(message), []byte(`\u003c`), []byte("<"))
	raw = bytes.ReplaceAll(raw, []byte(`\u003e`), []byte(">"))
	raw = bytes.ReplaceAll(raw, []byte(`\u0026`), []byte("&"))
	m := cliMigrationManifest{Format: cliMigrationFormat, ID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Scope: f.identity.DeploymentID, User: f.identity.UserID, Context: "personal", SourceDevice: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", SourceStore: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", TargetDevice: f.identity.DeviceID, TargetStore: f.physical, Epoch: "1", Security: "1", SourceGeneration: "2", Messages: []cliMigrationReference{{ID: message.ID, Conversation: message.Conversation, Content: message.Content, Membership: "1", Classification: "available", Digest: fingerprint(string(raw)), Object: message.Object, Mode: message.Mode}}, Objects: refs, Expires: time.Now().Add(3 * time.Minute).UTC().Format(time.RFC3339Nano)}
	manifest := cliMigrationJSONBytes(m)
	yes, no := true, false
	v := cliMigrationView{ID: m.ID, Request: "ffffffff-ffff-4fff-8fff-ffffffffffff", Status: "ready", Manifest: m, Envelope: base64.RawURLEncoding.EncodeToString(manifest), Digest: fingerprint(string(manifest)), Expires: m.Expires, Preserved: &yes, Qualified: &no, TargetQualified: &no}
	packet := cliMigrationPacket{Format: cliMigrationFormat, Digest: v.Digest, Messages: []json.RawMessage{raw}, Objects: objects}
	plain := append([]byte(`{"format":"`+cliMigrationFormat+`","manifestDigest":"`+v.Digest+`","messages":[`), raw...)
	plain = append(plain, []byte(`],"objects":`)...)
	plain = append(plain, cliMigrationJSONBytes(objects)...)
	plain = append(plain, '}')
	secret := bytes.Repeat([]byte{17}, 32)
	aad := strings.Join([]string{"JobsIM migration v1", m.Scope, m.ID, m.User, m.SourceDevice, m.SourceStore, m.TargetDevice, m.TargetStore, m.Epoch, m.Security, m.SourceGeneration, v.Digest}, "\x00")
	block, err := aes.NewCipher(secret)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{11}, 12)
	capsule := append([]byte("JIMMIGR1"), nonce...)
	capsule = gcm.Seal(capsule, nonce, plain, []byte(aad))
	v.CapsuleDigest = fingerprint(string(capsule))
	v.Size = fmtInt(len(capsule))
	key := cliMigrationKey{ID: v.ID, Digest: v.Digest, Algorithm: "AES-256-GCM", Key: base64.RawURLEncoding.EncodeToString(secret), AAD: base64.RawURLEncoding.EncodeToString([]byte(aad)), Expires: v.Expires, Qualified: &no}
	intent := cliMigrationIntent{ID: v.Request, Kind: "target", Binding: b, Created: time.Now().UTC().Format(time.RFC3339Nano), Migration: v.ID, Digest: v.Digest, State: "prepared"}
	if _, err = f.store.cliMigrationChange(b, func() bool { return true }, intent.ID, &intent, nil); err != nil {
		t.Fatal(err)
	}
	intent, err = f.store.cliMigrationChange(b, func() bool { return true }, intent.ID, nil, func(row *cliMigrationIntent) error { row.State = "unknown"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	return &cliMigrationFixture{f, b, control, intent, v, key, capsule, packet}
}
func fmtInt(n int) string { return strconv.Itoa(n) }
func TestMigrationExactRawJSONAndCompleteOriginal(t *testing.T) {
	m := newCLIMigrationFixture(t, true, false)
	packet, err := cliMigrationOpen(m.view, m.key, m.capsule, m.b, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packet.Messages[0], m.packet.Messages[0]) || !bytes.Equal(packet.Objects[0].Bytes, m.packet.Objects[0].Bytes) {
		t.Fatal("original bytes changed")
	}
	if err = m.f.store.cliMigrationCommit(m.b, m.control, func() bool { return true }, m.intent, m.view, packet, time.Now().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	after, ok, err := syncLoadControl(m.f.store.db, "1")
	if err != nil || !ok || syncCheckControl(after, m.control) != nil {
		t.Fatal("migration changed sync cursor/guard")
	}
	var receipts, outbox int
	_ = m.f.store.db.QueryRow("SELECT count(*) FROM receipt_outbox").Scan(&receipts)
	_ = m.f.store.db.QueryRow("SELECT count(*) FROM outbox").Scan(&outbox)
	if receipts != 0 || outbox != 0 {
		t.Fatal("migration manufactured qualified receipts or sends")
	}
}
func TestMigrationRejectsCrossTargetAADAndTamperedCapsule(t *testing.T) {
	m := newCLIMigrationFixture(t, false, false)
	wrong := m.b
	wrong.Physical = "99999999-9999-4999-8999-999999999999"
	if _, err := cliMigrationOpen(m.view, m.key, m.capsule, wrong, time.Now()); err == nil {
		t.Fatal("cross target accepted")
	}
	key := m.key
	key.AAD = base64.RawURLEncoding.EncodeToString([]byte("wrong"))
	if _, err := cliMigrationOpen(m.view, key, m.capsule, m.b, time.Now()); err == nil {
		t.Fatal("wrong AAD accepted")
	}
	capsule := append([]byte{}, m.capsule...)
	capsule[len(capsule)-1] ^= 1
	v := m.view
	v.CapsuleDigest = fingerprint(string(capsule))
	if _, err := cliMigrationOpen(v, m.key, capsule, m.b, time.Now()); err == nil {
		t.Fatal("bad GCM tag accepted")
	}
}
func TestMigrationRejectsUnknownCredentialsDuplicateKeysAndWrongOriginalDigest(t *testing.T) {
	for _, raw := range []string{`{"format":"x","password":"secret"}`, `{"format":"x","format":"y"}`, `{"messages":[{"id":"a","id":"b"}]}`} {
		var p cliMigrationPacket
		if cliMigrationDecode([]byte(raw), &p) == nil {
			t.Fatalf("unsafe JSON accepted: %s", raw)
		}
	}
	m := newCLIMigrationFixture(t, true, false)
	m.view.Manifest.Objects[0].Digest = strings.Repeat("0", 64)
	if _, err := cliMigrationOpen(m.view, m.key, m.capsule, m.b, time.Now()); err == nil {
		t.Fatal("manifest/object tamper accepted")
	}
}
func TestMigrationGlobalPrunesArchiveOriginalAndEncryptedStageAndBlocksLateInstall(t *testing.T) {
	m := newCLIMigrationFixture(t, true, false)
	if err := m.f.store.cliMigrationCommit(m.b, m.control, func() bool { return true }, m.intent, m.view, m.packet, time.Now().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err := m.f.store.db.Exec("INSERT INTO migration_stages VALUES(?,?,?,?)", "1", m.view.ID, m.view.Digest, m.capsule)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.f.store.db.Exec("INSERT INTO migration_stage_messages VALUES(?,?,?)", "1", m.view.ID, m.f.messageID); err != nil {
		t.Fatal(err)
	}
	var stagedMessage string
	if err = m.f.store.db.QueryRow("SELECT message_id FROM migration_stage_messages WHERE epoch=? AND id=?", "1", m.view.ID).Scan(&stagedMessage); err != nil || stagedMessage != m.f.messageID {
		t.Fatal("global race fixture did not stage its original message identity", err)
	}
	tx, err := m.f.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = syncRecordGlobalTx(tx, "1", m.f.messageID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"migration_messages", "migration_objects", "migration_stages", "migration_stage_messages"} {
		var n int
		if err = m.f.store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s retained content: %d %v", table, n, err)
		}
	}
	// A different original request cannot republish the same globally deleted ID.
	_, _ = m.f.store.db.Exec("DELETE FROM migration_archives")
	late := m.intent
	late.ID, _ = syncNewUUID()
	late.State = "prepared"
	if _, err = m.f.store.cliMigrationChange(m.b, func() bool { return true }, late.ID, &late, nil); err != nil {
		t.Fatal(err)
	}
	if err = m.f.store.cliMigrationCommit(m.b, m.control, func() bool { return true }, late, m.view, m.packet, time.Now().Add(5*time.Second)); err == nil {
		t.Fatal("late global import accepted")
	}
}
func TestMigrationAtomicRollbackOnGuardExpiryMembershipAndNewerLocalVersion(t *testing.T) {
	for _, kind := range []string{"guard", "expiry", "membership", "newer"} {
		t.Run(kind, func(t *testing.T) {
			m := newCLIMigrationFixture(t, false, false)
			before := time.Now().Add(5 * time.Second)
			control := m.control
			switch kind {
			case "guard":
				control.Guard = "99999999-9999-4999-8999-999999999999"
			case "expiry":
				before = time.Now().Add(-time.Second)
			case "membership":
				_, _ = m.f.store.db.Exec("UPDATE conversations SET payload=json_set(payload,'$.membershipGeneration','2')")
			case "newer":
				_, _ = m.f.store.db.Exec("UPDATE messages SET payload=json_set(payload,'$.contentVersion','9007199254740994')")
			}
			if err := m.f.store.cliMigrationCommit(m.b, control, func() bool { return true }, m.intent, m.view, m.packet, before); err == nil {
				t.Fatalf("%s accepted", kind)
			}
			var n int
			_ = m.f.store.db.QueryRow("SELECT count(*) FROM migration_messages").Scan(&n)
			if n != 0 {
				t.Fatal("partial archive published")
			}
		})
	}
}
func TestMigrationIntentRejectsFamilyReplacementAndOriginalRebase(t *testing.T) {
	m := newCLIMigrationFixture(t, false, false)
	wrong := m.b
	wrong.Family = "99999999-9999-4999-8999-999999999999"
	if _, err := m.f.store.cliMigrationChange(wrong, func() bool { return true }, m.intent.ID, nil, nil); err == nil {
		t.Fatal("successor family borrowed original")
	}
	if _, err := m.f.store.cliMigrationChange(m.b, func() bool { return true }, m.intent.ID, nil, func(o *cliMigrationIntent) error { o.Digest = strings.Repeat("1", 64); return nil }); err == nil {
		t.Fatal("manifest silently rebound")
	}
	raw := string(cliMigrationJSONBytes(m.intent))
	for _, secret := range []string{"password", "pairingProof", "installToken", "bodyEnvelope", "refreshToken"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("intent persisted %s", secret)
		}
	}
}
func TestMigrationForwardReobservationErasesIndependentCopiedText(t *testing.T) {
	m := newCLIMigrationFixture(t, false, true)
	if err := m.f.store.cliMigrationCommit(m.b, m.control, func() bool { return true }, m.intent, m.view, m.packet, time.Now().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	tx, err := m.f.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = cliMigrationRedactForwardTx(tx, "1", m.f.messageID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = m.f.store.db.QueryRow("SELECT payload FROM migration_messages").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	_ = json.Unmarshal([]byte(raw), &row)
	if row["text"] != "" || row["forwardUnavailable"] != true {
		t.Fatal("reference body retained")
	}
}

func TestMigrationOwnerUnavailableReadOnlyHistoryRemainsArchivable(t *testing.T) {
	m := newCLIMigrationFixture(t, false, false)
	if _, err := m.f.store.db.Exec("UPDATE conversations SET payload=json_set(payload,'$.state','owner_unavailable','$.readOnly',json('true'))"); err != nil {
		t.Fatal(err)
	}
	if err := m.f.store.cliMigrationCommit(m.b, m.control, func() bool { return true }, m.intent, m.view, m.packet, time.Now().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := m.f.store.db.QueryRow("SELECT count(*) FROM migration_messages").Scan(&count); err != nil || count != 1 {
		t.Fatal("read-only history failed publication", err)
	}
}

func TestMigrationZeroByteOriginalRoundTripsAndSavesCompleteFile(t *testing.T) {
	m := newCLIMigrationFixtureWithOriginal(t, true, false, []byte{})
	packet, err := cliMigrationOpen(m.view, m.key, m.capsule, m.b, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Objects) != 1 || len(packet.Objects[0].Bytes) != 0 {
		t.Fatal("zero byte original was dropped")
	}
	if err = m.f.store.cliMigrationCommit(m.b, m.control, func() bool { return true }, m.intent, m.view, packet, time.Now().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.f.root, "empty-original")
	if err = m.f.client.cliMigrationSaveObject(m.view.ID, packet.Objects[0].ID, path, m.b, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != 0 {
		t.Fatal("complete zero-byte file missing", err)
	}
}

func TestMigrationObjectPublicationSerializesConcurrentGlobalWriter(t *testing.T) {
	m := newCLIMigrationFixture(t, true, false)
	if err := m.f.store.cliMigrationCommit(m.b, m.control, func() bool { return true }, m.intent, m.view, m.packet, time.Now().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	other, err := openStore(m.f.store.path, m.f.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer other.close()
	if _, err = other.db.Exec("PRAGMA busy_timeout=20"); err != nil {
		t.Fatal(err)
	}
	attempted := false
	current := func() bool {
		if !attempted {
			attempted = true
			tx, beginErr := other.db.Begin()
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			globalErr := syncRecordGlobalTx(tx, "1", m.f.messageID)
			if globalErr == nil {
				globalErr = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
			if globalErr == nil {
				t.Error("global committed through an active original publication")
			}
		}
		return true
	}
	path := filepath.Join(m.f.root, "serialized-original")
	if err = m.f.client.cliMigrationSaveObject(m.view.ID, m.packet.Objects[0].ID, path, m.b, current); err != nil {
		t.Fatal(err)
	}
	if !attempted {
		t.Fatal("concurrent writer was not exercised")
	}
	tx, err := other.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = syncRecordGlobalTx(tx, "1", m.f.messageID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("already delivered user file was removed", err)
	}
}
