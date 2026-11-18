// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const cliMigrationFormat = "jobsim-personal-migration-v1"
const cliMigrationLimit = 128 << 20

type cliMigrationMessage struct {
	ID               string `json:"id"`
	Conversation     string `json:"conversationId"`
	Sender           string `json:"senderId"`
	Type             string `json:"type"`
	Text             string `json:"text"`
	Reply            string `json:"replyToId,omitempty"`
	Object           string `json:"attachmentId,omitempty"`
	Created          string `json:"createdAt"`
	Content          string `json:"contentVersion"`
	CreationEpoch    string `json:"creationEpoch"`
	CreationSequence string `json:"creationSequence"`
	Forwarded        bool   `json:"forwarded,omitempty"`
	Mode             string `json:"objectMode"`
}
type cliMigrationReference struct {
	ID             string `json:"messageId"`
	Conversation   string `json:"conversationId"`
	Content        string `json:"contentVersion"`
	Membership     string `json:"membershipGeneration"`
	Classification string `json:"classification"`
	Digest         string `json:"primaryDigest"`
	Object         string `json:"objectId,omitempty"`
	Mode           string `json:"objectMode"`
	Deadline       string `json:"deadline,omitempty"`
}
type cliMigrationObjectReference struct {
	ID             string `json:"objectId"`
	Message        string `json:"messageId"`
	Size           string `json:"sizeBytes"`
	Digest         string `json:"sha256"`
	ContentType    string `json:"contentType"`
	Classification string `json:"classification"`
	Deadline       string `json:"deadline,omitempty"`
}
type cliMigrationManifest struct {
	Format           string                        `json:"format"`
	ID               string                        `json:"id"`
	Scope            string                        `json:"scopeId"`
	User             string                        `json:"userId"`
	Context          string                        `json:"sourceContextId"`
	SourceDevice     string                        `json:"sourceDeviceId"`
	SourceStore      string                        `json:"sourceStoreGeneration"`
	TargetDevice     string                        `json:"targetDeviceId"`
	TargetStore      string                        `json:"targetStoreGeneration"`
	Epoch            string                        `json:"restoreEpoch"`
	Security         string                        `json:"securityVersion"`
	SourceGeneration string                        `json:"sourceGeneration"`
	Messages         []cliMigrationReference       `json:"messages"`
	Objects          []cliMigrationObjectReference `json:"objects"`
	Expires          string                        `json:"expiresAt"`
}
type cliMigrationView struct {
	ID              string               `json:"id"`
	Request         string               `json:"requestId,omitempty"`
	Status          string               `json:"status"`
	Manifest        cliMigrationManifest `json:"manifest"`
	Envelope        string               `json:"manifestEnvelope"`
	Digest          string               `json:"manifestDigest"`
	CapsuleDigest   string               `json:"capsuleSHA256"`
	Size            string               `json:"sizeBytes"`
	Expires         string               `json:"expiresAt"`
	Preserved       *bool                `json:"sourcePreserved"`
	Qualified       *bool                `json:"qualifiedAck"`
	TargetQualified *bool                `json:"targetQualified"`
}
type cliMigrationObject struct {
	ID      string `json:"objectId"`
	Message string `json:"messageId"`
	Bytes   []byte `json:"bytes"`
}

func (object *cliMigrationObject) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 || !bytes.HasPrefix(bytes.TrimSpace(fields["bytes"]), []byte{'"'}) {
		return syncRefuse("MIGRATION_ORIGINAL_INVALID")
	}
	var value struct {
		ID      string `json:"objectId"`
		Message string `json:"messageId"`
		Bytes   string `json:"bytes"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil {
		return syncRefuse("MIGRATION_ORIGINAL_INVALID")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(value.Bytes)
	if err != nil || base64.StdEncoding.EncodeToString(data) != value.Bytes || len(data) > 64<<20 {
		return syncRefuse("MIGRATION_ORIGINAL_INVALID")
	}
	object.ID, object.Message, object.Bytes = value.ID, value.Message, data
	return nil
}

type cliMigrationPacket struct {
	Format   string               `json:"format"`
	Digest   string               `json:"manifestDigest"`
	Messages []json.RawMessage    `json:"messages"`
	Objects  []cliMigrationObject `json:"objects"`
}
type cliMigrationKey struct {
	ID        string `json:"id"`
	Digest    string `json:"manifestDigest"`
	Algorithm string `json:"algorithm"`
	Key       string `json:"key"`
	AAD       string `json:"aad"`
	Expires   string `json:"expiresAt"`
	Qualified *bool  `json:"targetQualified"`
}

// JSON object fragments retain the server's exact signed/hash input. In addition
// to unknown fields, duplicate keys are rejected before decoding any capsule.
func cliMigrationDecode(raw []byte, out any) error {
	if !utf8.Valid(raw) || len(raw) > cliMigrationLimit {
		return syncRefuse("MIGRATION_INVALID_JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	tokens := 0
	var walk func(int) error
	walk = func(depth int) error {
		tokens++
		if depth > 24 || tokens > 65536 {
			return syncRefuse("MIGRATION_JSON_DEPTH")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := t.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, e := d.Token()
					name, valid := key.(string)
					if e != nil || !valid || seen[name] {
						return syncRefuse("MIGRATION_DUPLICATE_KEY")
					}
					seen[name] = true
					if err = walk(depth + 1); err != nil {
						return err
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim('}') {
					return syncRefuse("MIGRATION_INVALID_JSON")
				}
			case '[':
				for d.More() {
					if err = walk(depth + 1); err != nil {
						return err
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim(']') {
					return syncRefuse("MIGRATION_INVALID_JSON")
				}
			default:
				return syncRefuse("MIGRATION_INVALID_JSON")
			}
		}
		return nil
	}
	if walk(0) != nil {
		return syncRefuse("MIGRATION_INVALID_JSON")
	}
	if _, err := d.Token(); err != io.EOF {
		return syncRefuse("MIGRATION_INVALID_JSON")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return syncRefuse("MIGRATION_INVALID_JSON")
	}
	return nil
}
func cliMigrationPositive(v string) bool { n, e := syncCounter(v); return e == nil && n > 0 }
func cliMigrationDeadline(value string, now time.Time) bool {
	t, e := time.Parse(time.RFC3339Nano, value)
	return e == nil && t.After(now)
}
func cliMigrationCheckView(v cliMigrationView, b dataExportBinding, target bool, now time.Time) error {
	m := v.Manifest
	if v.Preserved == nil || !*v.Preserved || !receiptFalse(v.Qualified) || !receiptFalse(v.TargetQualified) || !oneOf(v.Status, "ready", "installed_reported") || !syncUUIDPattern.MatchString(v.ID) || m.ID != v.ID || m.Format != cliMigrationFormat || m.Context != "personal" || m.Scope != b.Identity.DeploymentID || m.User != b.Identity.UserID || m.Epoch != b.Epoch || v.Expires != m.Expires || !cliMigrationDeadline(m.Expires, now) || !cliMigrationPositive(m.Security) || !cliMigrationPositive(m.SourceGeneration) || !syncDigestPattern.MatchString(v.CapsuleDigest) {
		return syncRefuse("MIGRATION_CONTEXT_CHANGED")
	}
	for _, id := range []string{m.SourceDevice, m.SourceStore, m.TargetDevice, m.TargetStore} {
		if !syncUUIDPattern.MatchString(id) {
			return syncRefuse("MIGRATION_INVALID_MANIFEST")
		}
	}
	if m.SourceDevice == m.TargetDevice || target && (m.TargetDevice != b.Identity.DeviceID || m.TargetStore != b.Physical) || !target && (m.SourceDevice != b.Identity.DeviceID || m.SourceStore != b.Physical) {
		return syncRefuse("MIGRATION_TARGET_CHANGED")
	}
	raw, e := syncEnvelope(v.Envelope, v.Digest)
	var decoded cliMigrationManifest
	if e != nil || cliMigrationDecode(raw, &decoded) != nil {
		return syncRefuse("MIGRATION_MANIFEST_DIGEST")
	}
	a, _ := json.Marshal(m)
	z, _ := json.Marshal(decoded)
	if !bytes.Equal(a, z) {
		return syncRefuse("MIGRATION_MANIFEST_CHANGED")
	}
	size, e := strconv.ParseInt(v.Size, 10, 64)
	if e != nil || size < 36 || size > cliMigrationLimit+4096 || len(m.Messages) < 1 || len(m.Messages) > 256 || len(m.Objects) > 64 || m.Objects == nil {
		return syncRefuse("MIGRATION_PACKAGE_BOUND")
	}
	ids := map[string]bool{}
	full := map[string]string{}
	for _, r := range m.Messages {
		if ids[r.ID] || !syncUUIDPattern.MatchString(r.ID) || !syncUUIDPattern.MatchString(r.Conversation) || !cliMigrationPositive(r.Content) || !cliMigrationPositive(r.Membership) || !oneOf(r.Classification, "available", "cloud_only") || !oneOf(r.Mode, "none", "metadata_only", "full") || !syncDigestPattern.MatchString(r.Digest) || r.Deadline != "" && !cliMigrationDeadline(r.Deadline, now) || r.Object != "" && !syncUUIDPattern.MatchString(r.Object) {
			return syncRefuse("MIGRATION_INVALID_MANIFEST")
		}
		ids[r.ID] = true
		if r.Mode == "full" {
			if r.Object == "" {
				return syncRefuse("MIGRATION_ORIGINAL_REQUIRED")
			}
			full[r.ID] = r.Object
		}
	}
	objects := map[string]bool{}
	for _, r := range m.Objects {
		n, e := strconv.ParseInt(r.Size, 10, 64)
		if !syncUUIDPattern.MatchString(r.ID) || objects[r.ID] || full[r.Message] != r.ID || e != nil || n < 0 || n > 64<<20 || !syncDigestPattern.MatchString(r.Digest) || !oneOf(r.Classification, "available", "cloud_only") || r.Deadline != "" && !cliMigrationDeadline(r.Deadline, now) {
			return syncRefuse("MIGRATION_INVALID_ORIGINAL")
		}
		objects[r.ID] = true
		delete(full, r.Message)
	}
	if len(full) != 0 {
		return syncRefuse("MIGRATION_ORIGINAL_REQUIRED")
	}
	return nil
}
func cliMigrationOpen(v cliMigrationView, key cliMigrationKey, capsule []byte, b dataExportBinding, now time.Time) (cliMigrationPacket, error) {
	var packet cliMigrationPacket
	if err := cliMigrationCheckView(v, b, true, now); err != nil {
		return packet, err
	}
	if len(capsule) < 36 || len(capsule) > cliMigrationLimit+4096 || string(capsule[:8]) != "JIMMIGR1" || strconv.Itoa(len(capsule)) != v.Size || fingerprint(string(capsule)) != v.CapsuleDigest || key.ID != v.ID || key.Digest != v.Digest || key.Algorithm != "AES-256-GCM" || !receiptFalse(key.Qualified) || !cliMigrationDeadline(key.Expires, now) {
		return packet, syncRefuse("MIGRATION_CAPSULE_INVALID")
	}
	secret, e := base64.RawURLEncoding.Strict().DecodeString(key.Key)
	if e != nil || len(secret) != 32 {
		return packet, syncRefuse("MIGRATION_KEY_INVALID")
	}
	defer erase(secret)
	aad, e := base64.RawURLEncoding.Strict().DecodeString(key.AAD)
	m := v.Manifest
	expected := strings.Join([]string{"JobsIM migration v1", m.Scope, m.ID, m.User, m.SourceDevice, m.SourceStore, m.TargetDevice, m.TargetStore, m.Epoch, m.Security, m.SourceGeneration, v.Digest}, "\x00")
	if e != nil || string(aad) != expected {
		return packet, syncRefuse("MIGRATION_AAD_CHANGED")
	}
	block, e := aes.NewCipher(secret)
	if e != nil {
		return packet, e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return packet, e
	}
	plain, e := gcm.Open(nil, capsule[8:20], capsule[20:], aad)
	if e != nil {
		return packet, syncRefuse("MIGRATION_AUTHENTICATION_FAILED")
	}
	defer erase(plain)
	if cliMigrationDecode(plain, &packet) != nil || packet.Format != cliMigrationFormat || packet.Digest != v.Digest || len(packet.Messages) != len(m.Messages) || len(packet.Objects) != len(m.Objects) || packet.Objects == nil {
		return packet, syncRefuse("MIGRATION_PACKAGE_INVALID")
	}
	refs := map[string]cliMigrationReference{}
	for _, r := range m.Messages {
		refs[r.ID] = r
	}
	for _, raw := range packet.Messages {
		var message cliMigrationMessage
		if cliMigrationDecode(raw, &message) != nil {
			return packet, syncRefuse("MIGRATION_MESSAGE_INVALID")
		}
		r, ok := refs[message.ID]
		if !ok || fingerprint(string(raw)) != r.Digest || message.Conversation != r.Conversation || message.Content != r.Content || message.Object != r.Object || message.Mode != r.Mode || !syncUUIDPattern.MatchString(message.Sender) || message.Reply != "" && !syncUUIDPattern.MatchString(message.Reply) || !cliMigrationPositive(message.CreationEpoch) || !cliMigrationPositive(message.CreationSequence) || len(message.Text) > 16384 {
			return packet, syncRefuse("MIGRATION_MESSAGE_CHANGED")
		}
		if _, e = time.Parse(time.RFC3339Nano, message.Created); e != nil {
			return packet, syncRefuse("MIGRATION_MESSAGE_INVALID")
		}
		if message.Type == "text" {
			if message.Object != "" || message.Mode != "none" {
				return packet, syncRefuse("MIGRATION_MESSAGE_INVALID")
			}
		} else if !oneOf(message.Type, "image", "audio", "video", "file") || message.Forwarded && (message.Mode != "metadata_only" || message.Object != "") || !message.Forwarded && (message.Object == "" || !oneOf(message.Mode, "metadata_only", "full")) {
			return packet, syncRefuse("MIGRATION_MESSAGE_INVALID")
		}
		delete(refs, message.ID)
	}
	objects := map[string]cliMigrationObjectReference{}
	for _, r := range m.Objects {
		objects[r.ID] = r
	}
	for _, o := range packet.Objects {
		r, ok := objects[o.ID]
		if !ok || r.Message != o.Message || r.Size != strconv.Itoa(len(o.Bytes)) || fingerprint(string(o.Bytes)) != r.Digest {
			return packet, syncRefuse("MIGRATION_ORIGINAL_CHANGED")
		}
		delete(objects, o.ID)
	}
	if len(refs) != 0 || len(objects) != 0 {
		return packet, syncRefuse("MIGRATION_PACKAGE_INCOMPLETE")
	}
	return packet, nil
}
