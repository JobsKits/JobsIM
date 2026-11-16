// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"
)

type syncFailure struct{ Code string }

func (e *syncFailure) Error() string { return e.Code }
func syncRefuse(code string) error   { return &syncFailure{Code: code} }

var syncUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var syncCounterPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,18})$`)
var syncDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func syncNewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6], b[8] = b[6]&15|64, b[8]&63|128
	v := hex.EncodeToString(b[:])
	return v[:8] + "-" + v[8:12] + "-" + v[12:16] + "-" + v[16:20] + "-" + v[20:], nil
}
func syncCounter(v string) (uint64, error) {
	if !syncCounterPattern.MatchString(v) {
		return 0, syncRefuse("SYNC_INVALID_COUNTER")
	}
	n, err := strconv.ParseUint(v, 10, 63)
	if err != nil || n > math.MaxInt64 {
		return 0, syncRefuse("SYNC_INVALID_COUNTER")
	}
	return n, nil
}
func syncDecode(raw []byte, value any) error {
	if !utf8.Valid(raw) {
		return syncRefuse("SYNC_INVALID_JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(value); err != nil {
		return syncRefuse("SYNC_INVALID_JSON")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return syncRefuse("SYNC_INVALID_JSON")
	}
	return nil
}
func syncEqual(a, b []byte) bool {
	var left, right any
	return syncDecode(a, &left) == nil && syncDecode(b, &right) == nil && reflect.DeepEqual(left, right)
}
func syncArrayField(raw []byte, key string) bool {
	var fields map[string]json.RawMessage
	if syncDecode(raw, &fields) != nil {
		return false
	}
	return bytes.HasPrefix(bytes.TrimSpace(fields[key]), []byte("["))
}
func syncEnvelope(encoded, digest string) ([]byte, error) {
	if len(encoded) == 0 || len(encoded) > 16<<20 || !syncDigestPattern.MatchString(digest) {
		return nil, syncRefuse("SYNC_INVALID_ENVELOPE")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return nil, syncRefuse("SYNC_INVALID_ENVELOPE")
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != digest {
		return nil, syncRefuse("SYNC_DIGEST_MISMATCH")
	}
	var value any
	if err = syncDecode(raw, &value); err != nil {
		return nil, err
	}
	return raw, nil
}

type syncBinding struct {
	ScopeID         string `json:"scopeId"`
	PrincipalID     string `json:"principalId"`
	SourceContextID string `json:"sourceContextId"`
	DeviceID        string `json:"deviceId"`
	StoreGeneration string `json:"storeGeneration"`
	RestoreEpoch    string `json:"restoreEpoch"`
	StreamID        string `json:"streamId"`
}

func (b syncBinding) check(expected syncBinding, withStream bool) error {
	for _, v := range []string{b.ScopeID, b.PrincipalID, b.DeviceID, b.StoreGeneration} {
		if !syncUUIDPattern.MatchString(v) {
			return syncRefuse("SYNC_INVALID_BINDING")
		}
	}
	if epoch, err := syncCounter(b.RestoreEpoch); err != nil || epoch == 0 || b.SourceContextID != "personal" {
		return syncRefuse("SYNC_INVALID_BINDING")
	}
	if withStream && !syncUUIDPattern.MatchString(b.StreamID) {
		return syncRefuse("SYNC_INVALID_BINDING")
	}
	if !withStream {
		b.StreamID, expected.StreamID = "", ""
	}
	if b != expected {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	return nil
}

type syncRef struct {
	Owner          string `json:"owner"`
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Revision       string `json:"revision"`
	RevisionEpoch  string `json:"revisionEpoch"`
	SubjectUserID  string `json:"subjectUserId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	TargetUserID   string `json:"targetUserId,omitempty"`
	State          string `json:"state"`
}

func (r syncRef) key() string { return r.Owner + "|" + r.Kind + "|" + r.ID }
func (r syncRef) validate() error {
	allowed := map[string][]string{"auth": {"account", "devices"}, "directory": {"profile", "contacts", "relationship", "contact_request"}, "conversation": {"conversation", "announcement"}, "message": {"message"}}
	valid := false
	for _, kind := range allowed[r.Owner] {
		valid = valid || kind == r.Kind
	}
	if !valid || !syncUUIDPattern.MatchString(r.ID) || (r.State != "active" && r.State != "removed") {
		return syncRefuse("SYNC_INVALID_REFERENCE")
	}
	if _, err := syncCounter(r.Revision); err != nil {
		return err
	}
	if n, err := syncCounter(r.RevisionEpoch); err != nil || n == 0 {
		return syncRefuse("SYNC_INVALID_REFERENCE")
	}
	for _, v := range []string{r.SubjectUserID, r.ConversationID, r.TargetUserID} {
		if v != "" && !syncUUIDPattern.MatchString(v) {
			return syncRefuse("SYNC_INVALID_REFERENCE")
		}
	}
	return nil
}
func syncReadRef(raw json.RawMessage) (syncRef, error) {
	var r syncRef
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, syncRefuse("SYNC_INVALID_REFERENCE")
	}
	return r, r.validate()
}

type syncProjection struct {
	Ref                  syncRef         `json:"ref"`
	State                string          `json:"state"`
	CurrentRevision      string          `json:"currentRevision,omitempty"`
	CurrentRevisionEpoch string          `json:"currentRevisionEpoch,omitempty"`
	RevisionMatch        *bool           `json:"revisionMatch"`
	BodyDigest           string          `json:"bodyDigest,omitempty"`
	BodyEnvelope         string          `json:"bodyEnvelope,omitempty"`
	Object               json.RawMessage `json:"object,omitempty"`
}

func syncReadProjection(raw json.RawMessage, ref syncRef, epoch string) (syncProjection, error) {
	var p syncProjection
	if err := syncDecode(raw, &p); err != nil {
		return p, err
	}
	var fields map[string]json.RawMessage
	if err := syncDecode(raw, &fields); err != nil {
		return p, err
	}
	r, err := syncReadRef(fields["ref"])
	if err != nil || r != ref || r.RevisionEpoch != epoch {
		return p, syncRefuse("SYNC_REFERENCE_MISMATCH")
	}
	if p.RevisionMatch == nil {
		return p, syncRefuse("SYNC_INVALID_PROJECTION")
	}
	switch p.State {
	case "available", "global", "unavailable", "unknown", "redacted":
	default:
		return p, syncRefuse("SYNC_INVALID_PROJECTION")
	}
	if p.CurrentRevision != "" || p.CurrentRevisionEpoch != "" {
		if _, err = syncCounter(p.CurrentRevision); err != nil {
			return p, err
		}
		if p.CurrentRevisionEpoch != epoch {
			return p, syncRefuse("RESTORE_EPOCH_CHANGED")
		}
	} else if p.State == "available" || p.State == "global" || p.State == "unavailable" {
		return p, syncRefuse("SYNC_INVALID_PROJECTION")
	}
	if *p.RevisionMatch != (p.CurrentRevision != "" && p.CurrentRevision == r.Revision && p.CurrentRevisionEpoch == r.RevisionEpoch) {
		return p, syncRefuse("SYNC_REVISION_MISMATCH")
	}
	if len(p.Object) != 0 && string(p.Object) != "null" {
		original, e := syncEnvelope(p.BodyEnvelope, p.BodyDigest)
		if e != nil {
			return p, e
		}
		if !syncEqual(original, p.Object) || p.State == "unknown" || p.State == "redacted" {
			return p, syncRefuse("SYNC_DIGEST_MISMATCH")
		}
		if r.Kind == "message" {
			m, x := syncValidateProjectionMessage(original, p.State == "global")
			if x != nil {
				return p, x
			}
			if m.ID != r.ID || r.ConversationID != "" && m.ConversationID != "" && m.ConversationID != r.ConversationID {
				return p, syncRefuse("SYNC_OBJECT_ID_MISMATCH")
			}
		}
	} else if p.State == "available" || p.BodyEnvelope != "" || p.BodyDigest != "" {
		return p, syncRefuse("SYNC_INVALID_PROJECTION")
	}
	return p, nil
}

type syncPageDescriptor struct {
	Page   *int   `json:"page"`
	Count  *int   `json:"count"`
	Digest string `json:"digest"`
}
type syncManifest struct {
	syncBinding
	SnapshotID              string               `json:"snapshotId"`
	SchemaVersion           string               `json:"schemaVersion"`
	Boundary                string               `json:"boundary"`
	BoundaryToken           string               `json:"boundaryToken"`
	OwnerWatermarks         map[string]string    `json:"ownerWatermarks"`
	PermissionGeneration    string               `json:"permissionGeneration"`
	ClassificationWatermark string               `json:"classificationWatermark"`
	Coverage                json.RawMessage      `json:"coverage"`
	PageManifest            []syncPageDescriptor `json:"pageManifest"`
	ExpiresAt               string               `json:"expiresAt"`
	Raw                     json.RawMessage      `json:"-"`
}

func syncReadManifest(raw json.RawMessage, b syncBinding) (syncManifest, error) {
	var m syncManifest
	if err := syncDecode(raw, &m); err != nil {
		return m, err
	}
	m.Raw = bytes.Clone(raw)
	if err := m.syncBinding.check(b, true); err != nil {
		return m, err
	}
	if !syncUUIDPattern.MatchString(m.SnapshotID) || m.SchemaVersion != "1" || len(m.BoundaryToken) == 0 || len(m.BoundaryToken) > 4096 || !syncDigestPattern.MatchString(m.PermissionGeneration) || len(m.PageManifest) < 1 || len(m.PageManifest) > 200000 {
		return m, syncRefuse("SYNC_INVALID_MANIFEST")
	}
	if _, err := syncCounter(m.Boundary); err != nil {
		return m, err
	}
	if _, err := syncCounter(m.ClassificationWatermark); err != nil {
		return m, err
	}
	for _, owner := range []string{"auth", "directory", "conversation", "message"} {
		if _, err := syncCounter(m.OwnerWatermarks[owner]); err != nil {
			return m, err
		}
	}
	var coverage map[string]any
	if syncDecode(m.Coverage, &coverage) != nil || coverage == nil {
		return m, syncRefuse("SYNC_INVALID_MANIFEST")
	}
	count := 0
	for index, page := range m.PageManifest {
		if page.Page == nil || *page.Page != index || page.Count == nil || *page.Count < 0 || *page.Count > 100 || !syncDigestPattern.MatchString(page.Digest) {
			return m, syncRefuse("SYNC_INVALID_MANIFEST")
		}
		count += *page.Count
	}
	if count > 200000 {
		return m, syncRefuse("SYNC_LOCAL_CAPACITY")
	}
	if err := m.alive(); err != nil {
		return m, err
	}
	return m, nil
}
func (m syncManifest) alive() error {
	expires, err := time.Parse(time.RFC3339Nano, m.ExpiresAt)
	if err != nil || !expires.After(time.Now()) {
		return syncRefuse("SYNC_SNAPSHOT_EXPIRED")
	}
	return nil
}

type syncPage struct {
	Index    int              `json:"page"`
	Digest   string           `json:"pageDigest"`
	Envelope string           `json:"referenceEnvelope"`
	Refs     []syncRef        `json:"refs"`
	Items    []syncProjection `json:"-"`
}

func syncReadPage(raw json.RawMessage, m syncManifest, index int) (syncPage, error) {
	p := syncPage{Index: index}
	var v struct {
		SnapshotID  string            `json:"snapshotId"`
		Page        *int              `json:"page"`
		Refs        json.RawMessage   `json:"refs"`
		Envelope    string            `json:"referenceEnvelope"`
		Digest      string            `json:"pageDigest"`
		Items       []json.RawMessage `json:"items"`
		Epoch       string            `json:"restoreEpoch"`
		Permission  string            `json:"permissionGeneration"`
		Installable *bool             `json:"installable"`
		Final       *bool             `json:"requiresFinalValidation"`
		Atomic      *bool             `json:"requiresAtomicInstall"`
		Ack         *bool             `json:"qualifiedAck"`
		DigestScope string            `json:"digestScope"`
	}
	if err := syncDecode(raw, &v); err != nil {
		return p, err
	}
	if index < 0 || index >= len(m.PageManifest) || v.Page == nil || *v.Page != index || v.SnapshotID != m.SnapshotID || v.Epoch != m.RestoreEpoch || v.Permission != m.PermissionGeneration {
		return p, syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	if v.Installable == nil || !*v.Installable || v.Final == nil || !*v.Final || v.Atomic == nil || !*v.Atomic || v.Ack == nil || *v.Ack || v.DigestScope != "immutable_references_only" {
		return p, syncRefuse("SYNC_PAGE_NOT_INSTALLABLE")
	}
	if v.Digest != m.PageManifest[index].Digest {
		return p, syncRefuse("SYNC_DIGEST_MISMATCH")
	}
	original, err := syncEnvelope(v.Envelope, v.Digest)
	if err != nil {
		return p, err
	}
	var refs []json.RawMessage
	if !bytes.HasPrefix(bytes.TrimSpace(original), []byte("[")) || !syncArrayField(raw, "items") || syncDecode(original, &refs) != nil || !syncEqual(original, v.Refs) || len(refs) != *m.PageManifest[index].Count || len(refs) != len(v.Items) {
		return p, syncRefuse("SYNC_INVALID_PAGE")
	}
	p.Digest, p.Envelope = v.Digest, v.Envelope
	seen := map[string]bool{}
	for i, rawRef := range refs {
		r, e := syncReadRef(rawRef)
		if e != nil {
			return p, e
		}
		if seen[r.key()] {
			return p, syncRefuse("SYNC_DUPLICATE_REFERENCE")
		}
		seen[r.key()] = true
		item, e := syncReadProjection(v.Items[i], r, m.RestoreEpoch)
		if e != nil {
			return p, e
		}
		if item.RevisionMatch == nil || !*item.RevisionMatch || item.State == "unknown" || item.State == "redacted" {
			return p, syncRefuse("SYNC_PAGE_NOT_INSTALLABLE")
		}
		p.Refs = append(p.Refs, r)
		p.Items = append(p.Items, item)
	}
	return p, nil
}
func syncValidation(raw json.RawMessage, m syncManifest) (string, error) {
	var v struct {
		CanInstall *bool  `json:"canInstall"`
		Token      string `json:"boundaryToken"`
		Epoch      string `json:"restoreEpoch"`
		Permission string `json:"permissionGeneration"`
		Watermark  string `json:"classificationWatermark"`
		Atomic     *bool  `json:"requiresLocalAtomicInstall"`
		Ack        *bool  `json:"qualifiedAck"`
	}
	if err := syncDecode(raw, &v); err != nil {
		return "", err
	}
	watermark, err := syncCounter(v.Watermark)
	minimum, _ := syncCounter(m.ClassificationWatermark)
	if err != nil || watermark < minimum || v.CanInstall == nil || !*v.CanInstall || v.Atomic == nil || !*v.Atomic || v.Ack == nil || *v.Ack || v.Token != m.BoundaryToken || v.Epoch != m.RestoreEpoch || v.Permission != m.PermissionGeneration {
		return "", syncRefuse("SYNC_SNAPSHOT_INVALIDATED")
	}
	return v.Watermark, m.alive()
}

type syncDelta struct {
	Items                              []syncProjection
	Events                             []json.RawMessage
	Cursor, Position, Head, Permission string
	HasMore                            bool
}

func syncReadChanges(raw json.RawMessage, previous, epoch, permission string) (syncDelta, error) {
	var d syncDelta
	var v struct {
		Items      []json.RawMessage `json:"items"`
		Cursor     string            `json:"cursor"`
		Position   string            `json:"position"`
		Head       string            `json:"head"`
		HasMore    *bool             `json:"hasMore"`
		Epoch      string            `json:"restoreEpoch"`
		Permission string            `json:"permissionGeneration"`
		Snapshot   *bool             `json:"snapshotRequired"`
		Ack        *bool             `json:"qualifiedAck"`
	}
	if err := syncDecode(raw, &v); err != nil {
		return d, err
	}
	if v.Epoch != epoch {
		return d, syncRefuse("RESTORE_EPOCH_CHANGED")
	}
	if v.Permission != permission || !syncDigestPattern.MatchString(v.Permission) || v.Snapshot == nil || *v.Snapshot {
		return d, syncRefuse("SYNC_PERMISSION_CHANGED")
	}
	if v.Ack == nil || *v.Ack || v.HasMore == nil || !syncArrayField(raw, "items") || len(v.Items) > 500 || len(v.Cursor) == 0 || len(v.Cursor) > 4096 {
		return d, syncRefuse("SYNC_INVALID_DELTA")
	}
	next, err := syncCounter(previous)
	if err != nil {
		return d, err
	}
	next++
	seen := map[string]bool{}
	for _, event := range v.Items {
		var e struct {
			Kind        string          `json:"kind"`
			Position    string          `json:"position"`
			Origin      string          `json:"originEventId"`
			Digest      string          `json:"eventDigest"`
			Envelope    string          `json:"eventEnvelope"`
			Ref         json.RawMessage `json:"ref"`
			Projection  json.RawMessage `json:"projection"`
			From        string          `json:"from"`
			To          string          `json:"to"`
			RangeDigest string          `json:"rangeDigest"`
		}
		if err = syncDecode(event, &e); err != nil {
			return d, err
		}
		if e.Kind == "redacted" {
			from, x := syncCounter(e.From)
			to, y := syncCounter(e.To)
			var fields map[string]json.RawMessage
			_ = syncDecode(event, &fields)
			if x != nil || y != nil || from != next || to < from || !syncDigestPattern.MatchString(e.RangeDigest) || len(fields) != 4 {
				return d, syncRefuse("SYNC_CONTINUITY_GAP")
			}
			next = to + 1
			d.Events = append(d.Events, bytes.Clone(event))
			continue
		}
		at, x := syncCounter(e.Position)
		if e.Kind != "reference" || x != nil || at != next || !syncUUIDPattern.MatchString(e.Origin) || seen[e.Origin] {
			return d, syncRefuse("SYNC_CONTINUITY_GAP")
		}
		seen[e.Origin] = true
		ref, x := syncReadRef(e.Ref)
		if x != nil {
			return d, x
		}
		original, x := syncEnvelope(e.Envelope, e.Digest)
		if x != nil {
			return d, x
		}
		fact, _ := json.Marshal(map[string]any{"position": e.Position, "originEventId": e.Origin, "ref": ref})
		if !syncEqual(original, fact) {
			return d, syncRefuse("SYNC_DIGEST_MISMATCH")
		}
		projection, x := syncReadProjection(e.Projection, ref, epoch)
		if x != nil {
			return d, x
		}
		if projection.State == "unknown" || projection.State == "redacted" {
			return d, syncRefuse("SYNC_PROJECTION_UNKNOWN")
		}
		d.Items = append(d.Items, projection)
		metadata, _ := json.Marshal(map[string]any{"kind": "reference", "position": e.Position, "originEventId": e.Origin, "eventDigest": e.Digest, "eventEnvelope": e.Envelope, "ref": ref})
		d.Events = append(d.Events, metadata)
		next++
	}
	at, x := syncCounter(v.Position)
	head, y := syncCounter(v.Head)
	if x != nil || y != nil || at != next-1 || head < at || *v.HasMore != (at < head) {
		return d, syncRefuse("SYNC_CONTINUITY_GAP")
	}
	d.Cursor, d.Position, d.Head, d.Permission, d.HasMore = v.Cursor, v.Position, v.Head, v.Permission, *v.HasMore
	return d, nil
}

// Proofs require the complete minimum Message DTO, not merely an echoed tuple.
func syncValidateMessage(raw json.RawMessage, minimalGlobal bool) (cacheMessage, error) {
	var fields map[string]json.RawMessage
	var m cacheMessage
	if syncDecode(raw, &fields) != nil || fields == nil || syncDecode(raw, &m) != nil || !syncUUIDPattern.MatchString(m.ID) {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	if version, err := syncCounter(m.Version); err != nil || version == 0 {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	for _, key := range []string{"contentVersion"} {
		if value, ok := fields[key]; ok {
			var counter string
			if json.Unmarshal(value, &counter) != nil {
				return m, syncRefuse("SYNC_INVALID_MESSAGE")
			}
			if _, err := syncCounter(counter); err != nil {
				return m, err
			}
		}
	}
	creationEpoch, hasEpoch := fields["creationEpoch"]
	creationSequence, hasSequence := fields["creationSequence"]
	if hasEpoch || hasSequence {
		var e, q string
		if !hasEpoch || !hasSequence || json.Unmarshal(creationEpoch, &e) != nil || json.Unmarshal(creationSequence, &q) != nil {
			return m, syncRefuse("SYNC_INVALID_MESSAGE")
		}
		if e == "unknown" || q == "unknown" {
			if e != "unknown" || q != "unknown" {
				return m, syncRefuse("SYNC_INVALID_MESSAGE")
			}
		} else {
			if _, err := syncCounter(e); err != nil {
				return m, err
			}
			if _, err := syncCounter(q); err != nil {
				return m, err
			}
		}
	}
	if minimalGlobal {
		var deleted bool
		if json.Unmarshal(fields["deleted"], &deleted) != nil || !deleted {
			return m, syncRefuse("SYNC_INVALID_GLOBAL")
		}
		return m, nil
	}
	var contentVersion string
	if json.Unmarshal(fields["contentVersion"], &contentVersion) != nil {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	if n, e := syncCounter(contentVersion); e != nil || n == 0 {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	deleted := bytes.TrimSpace(fields["deleted"])
	if !bytes.Equal(deleted, []byte("true")) && !bytes.Equal(deleted, []byte("false")) {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	if !syncUUIDPattern.MatchString(m.ConversationID) || !syncUUIDPattern.MatchString(m.SenderID) || !utf8.ValidString(m.ClientMsgID) || len(m.ClientMsgID) < 8 || len(m.ClientMsgID) > 128 {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	var sequence, kind, text string
	if !bytes.HasPrefix(bytes.TrimSpace(fields["text"]), []byte("\"")) || json.Unmarshal(fields["sequence"], &sequence) != nil || json.Unmarshal(fields["type"], &kind) != nil || json.Unmarshal(fields["text"], &text) != nil {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	if n, err := syncCounter(sequence); err != nil || n == 0 {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	switch kind {
	case "text", "image", "audio", "video", "file":
	case "forward":
		var forwarded bool
		_ = json.Unmarshal(fields["forwarded"], &forwarded)
		if !forwarded {
			return m, syncRefuse("SYNC_INVALID_MESSAGE")
		}
	default:
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	if _, err := time.Parse(time.RFC3339Nano, m.CreatedAt); err != nil {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	return m, nil
}

func syncValidateProjectionMessage(raw json.RawMessage, minimalGlobal bool) (cacheMessage, error) {
	m, err := syncValidateMessage(raw, minimalGlobal)
	if err != nil || minimalGlobal {
		return m, err
	}
	var state struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Unavailable bool   `json:"forwardUnavailable"`
	}
	if syncDecode(raw, &state) != nil || state.Type == "forward" && (!state.Unavailable || state.Text != "") {
		return m, syncRefuse("SYNC_INVALID_MESSAGE")
	}
	return m, nil
}
