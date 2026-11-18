package jobsim

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const migrationFormat = "jobsim-personal-migration-v1"
const migrationMagic = "JIMMIGR1"
const migrationMaxMessages = 256
const migrationMaxObjects = 64
const migrationMaxBytes = 128 << 20

var migrationCreationSlot = make(chan struct{}, 1)

type migrationPrincipal struct {
	User             string     `json:"userId"`
	Device           string     `json:"deviceId"`
	Epoch            string     `json:"restoreEpoch"`
	Security         string     `json:"securityVersion"`
	SourceGeneration string     `json:"sourceGeneration"`
	Active           bool       `json:"active"`
	AccessExpires    *time.Time `json:"accessExpiresAt,omitempty"`
}
type migrationReauthentication struct {
	User     string `json:"userId"`
	Device   string `json:"deviceId"`
	Epoch    string `json:"restoreEpoch"`
	Password string `json:"password"`
	OTP      string `json:"otp,omitempty"`
}
type migrationPairingRequest struct {
	Request  string `json:"requestId"`
	Store    string `json:"storeGeneration"`
	Epoch    string `json:"restoreEpoch"`
	Password string `json:"password"`
	OTP      string `json:"otp,omitempty"`
}
type migrationPairingProof struct {
	ID               string `json:"pairingId"`
	Scope            string `json:"scopeId"`
	User             string `json:"userId"`
	Device           string `json:"targetDeviceId"`
	Store            string `json:"targetStoreGeneration"`
	Epoch            string `json:"restoreEpoch"`
	Security         string `json:"securityVersion"`
	SourceGeneration string `json:"sourceGeneration"`
	Expires          int64  `json:"expires"`
}
type migrationInputMessage struct {
	Envelope   string `json:"bodyEnvelope"`
	Receipt    string `json:"sourceReceiptId,omitempty"`
	ObjectMode string `json:"objectMode"`
}
type migrationInputObject struct {
	Object  string `json:"objectId"`
	Message string `json:"messageId"`
	Receipt string `json:"sourceObjectReceiptId,omitempty"`
	Bytes   []byte `json:"bytes"`
}
type migrationCreateRequest struct {
	Request  string                  `json:"requestId"`
	Context  string                  `json:"sourceContextId"`
	Store    string                  `json:"sourceStoreGeneration"`
	Epoch    string                  `json:"restoreEpoch"`
	Pairing  string                  `json:"targetPairingProof"`
	Password string                  `json:"password"`
	OTP      string                  `json:"otp,omitempty"`
	Messages []migrationInputMessage `json:"messages"`
	Objects  []migrationInputObject  `json:"objects"`
}

// Only ordinary history fields enter a package. Session/refresh credentials,
// pending sends, source receipts and server admission proofs never do.
type migrationHistoryMessage struct {
	ID               string    `json:"id"`
	Conversation     string    `json:"conversationId"`
	Sender           string    `json:"senderId"`
	Type             string    `json:"type"`
	Text             string    `json:"text"`
	Reply            string    `json:"replyToId,omitempty"`
	Object           string    `json:"attachmentId,omitempty"`
	Created          time.Time `json:"createdAt"`
	Content          string    `json:"contentVersion"`
	CreationEpoch    string    `json:"creationEpoch"`
	CreationSequence string    `json:"creationSequence"`
	Forwarded        bool      `json:"forwarded,omitempty"`
	ObjectMode       string    `json:"objectMode"`
}
type migrationMessageReference struct {
	Message        string     `json:"messageId"`
	Conversation   string     `json:"conversationId"`
	Content        string     `json:"contentVersion"`
	Membership     string     `json:"membershipGeneration"`
	Classification string     `json:"classification"`
	PrimaryDigest  string     `json:"primaryDigest"`
	Object         string     `json:"objectId,omitempty"`
	ObjectMode     string     `json:"objectMode"`
	Receipt        string     `json:"sourceReceiptId,omitempty"`
	BodyDigest     string     `json:"sourceBodyDigest,omitempty"`
	Deadline       *time.Time `json:"deadline,omitempty"`
}
type migrationObjectReference struct {
	Object         string     `json:"objectId"`
	Message        string     `json:"messageId"`
	Size           string     `json:"sizeBytes"`
	Digest         string     `json:"sha256"`
	ContentType    string     `json:"contentType"`
	Classification string     `json:"classification"`
	Receipt        string     `json:"sourceObjectReceiptId,omitempty"`
	Deadline       *time.Time `json:"deadline,omitempty"`
}
type migrationManifest struct {
	Format           string                      `json:"format"`
	ID               string                      `json:"id"`
	Scope            string                      `json:"scopeId"`
	User             string                      `json:"userId"`
	Context          string                      `json:"sourceContextId"`
	SourceDevice     string                      `json:"sourceDeviceId"`
	SourceStore      string                      `json:"sourceStoreGeneration"`
	TargetDevice     string                      `json:"targetDeviceId"`
	TargetStore      string                      `json:"targetStoreGeneration"`
	Epoch            string                      `json:"restoreEpoch"`
	Security         string                      `json:"securityVersion"`
	SourceGeneration string                      `json:"sourceGeneration"`
	Messages         []migrationMessageReference `json:"messages"`
	Objects          []migrationObjectReference  `json:"objects"`
	Expires          time.Time                   `json:"expiresAt"`
}
type migrationPackageObject struct {
	Object  string `json:"objectId"`
	Message string `json:"messageId"`
	Bytes   []byte `json:"bytes"`
}
type migrationPackage struct {
	Format         string                    `json:"format"`
	ManifestDigest string                    `json:"manifestDigest"`
	Messages       []migrationHistoryMessage `json:"messages"`
	Objects        []migrationPackageObject  `json:"objects"`
}
type migrationRecord struct {
	ID, User, Request, SourceDevice, SourceStore, TargetDevice, TargetStore, Epoch, Security, SourceGeneration, State, ManifestDigest, CapsuleDigest string
	Digest, Key                                                                                                                                      []byte
	Manifest                                                                                                                                         migrationManifest
	Bytes                                                                                                                                            int64
	Expires                                                                                                                                          time.Time
}
type migrationSource struct {
	Message        Message    `json:"message"`
	Membership     string     `json:"membershipGeneration"`
	Classification string     `json:"classification"`
	Deadline       *time.Time `json:"deadline,omitempty"`
}

func migrationNormalize(m Message, mode string) migrationHistoryMessage {
	return migrationHistoryMessage{ID: m.ID, Conversation: m.ConversationID, Sender: m.SenderID, Type: m.Type, Text: m.Text, Reply: m.ReplyToID, Object: m.AttachmentID, Created: m.CreatedAt, Content: m.ContentVersion, CreationEpoch: m.CreationEpoch, CreationSequence: m.CreationSequence, Forwarded: m.Forwarded, ObjectMode: mode}
}
func migrationPrimaryDigest(m Message, mode string) string {
	return syncJSONDigest(migrationNormalize(m, mode))
}
func validMigrationMessage(m Message, mode string) bool {
	if !syncValidationID(m.ID) || !syncValidationID(m.ConversationID) || !syncValidationID(m.SenderID) || !syncValidationCounter(m.ContentVersion, false) || m.CreatedAt.IsZero() || m.Deleted || m.ContentUnavailable || m.ForwardUnavailable || !utf8.ValidString(m.Text) || len(m.Text) > 16384 || !syncValidationCounter(m.CreationEpoch, false) || !syncValidationCounter(m.CreationSequence, false) {
		return false
	}
	switch m.Type {
	case "text":
		return m.AttachmentID == "" && mode == "none"
	case "image", "audio", "video", "file":
		if m.Forwarded {
			return mode == "metadata_only" && m.AttachmentID == ""
		}
		return syncValidationID(m.AttachmentID) && (mode == "metadata_only" || mode == "full")
	default:
		return false
	}
}
func decodeMigrationRequest(r *http.Request, value any, limit int64) error {
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, limit))
	if err != nil || !utf8.Valid(raw) {
		return fail(400, "INVALID_MIGRATION_REQUEST", "One bounded UTF-8 migration request required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(400, "INVALID_MIGRATION_REQUEST", "One complete bounded personal migration request required")
	}
	return nil
}
func migrationDecodeEnvelope(value string) (Message, error) {
	var m Message
	if len(value) > 128<<10 {
		return m, fail(400, "MIGRATION_MESSAGE_TOO_LARGE", "Original message envelope exceeds the migration bound")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || !utf8.Valid(raw) {
		return m, fail(400, "INVALID_MIGRATION_ENVELOPE", "Original Base64URL message envelope required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil || decoder.Decode(new(any)) != io.EOF {
		return m, fail(400, "INVALID_MIGRATION_ENVELOPE", "Credentials, queues and unknown package fields are not accepted")
	}
	return m, nil
}
func (a *App) migrationInputDigest(q migrationCreateRequest) []byte {
	type object struct {
		ID, Message, Receipt, Digest string
		Bytes                        int
	}
	objects := []object{}
	for _, o := range q.Objects {
		objects = append(objects, object{o.Object, o.Message, o.Receipt, syncDigest(o.Bytes), len(o.Bytes)})
	}
	raw, _ := json.Marshal(struct {
		Request, Context, Store, Epoch, Pairing string
		Messages                                []migrationInputMessage
		Objects                                 []object
	}{q.Request, q.Context, q.Store, q.Epoch, q.Pairing, q.Messages, objects})
	return keyedDigest(a.Config.Key, "migration-intent-v1:"+a.Config.DeploymentID, raw)
}
func (a *App) migrationAAD(m migrationManifest, digest string) []byte {
	return []byte(strings.Join([]string{"JobsIM migration v1", m.Scope, m.ID, m.User, m.SourceDevice, m.SourceStore, m.TargetDevice, m.TargetStore, m.Epoch, m.Security, m.SourceGeneration, digest}, "\x00"))
}
func (a *App) migrationKeyAAD(m migrationManifest) string {
	return "migration-key-v1:" + base64.RawURLEncoding.EncodeToString(a.migrationAAD(m, ""))
}
func migrationSeal(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte(migrationMagic), nonce...)
	return gcm.Seal(out, nonce, plaintext, aad), nil
}
func (a *App) migrationPath(id string) (string, error) {
	if !syncValidationID(id) {
		return "", fail(400, "INVALID_MIGRATION_ID", "Canonical server migration identity required")
	}
	root := filepath.Join(a.Config.StateDir, "Migrations")
	if info, err := os.Lstat(root); err == nil && !info.IsDir() {
		return "", fail(503, "MIGRATION_STORAGE_UNAVAILABLE", "Private regular migration storage required")
	}
	return filepath.Join(root, id+".capsule"), nil
}
func (a *App) migrationStoreCurrent(ctx context.Context, u User, store string) error {
	if !syncValidationID(store) {
		return fail(400, "INVALID_MIGRATION_STORE", "Exact physical store required")
	}
	var current bool
	err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.device_stores d JOIN sync.instance i ON i.singleton AND i.restore_epoch=d.restore_epoch WHERE d.scope_id=$1 AND d.source_context_id='personal' AND d.user_id=$2 AND d.device_id=$3 AND d.restore_epoch=$4 AND d.store_generation=$5 AND i.deployment_id=$1::uuid::text)`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, store).Scan(&current)
	if err != nil {
		return err
	}
	if !current {
		return fail(409, "MIGRATION_STORE_CHANGED", "An old physical store cannot transfer its qualification or queue")
	}
	return nil
}
func (a *App) migrationPrincipal(ctx context.Context, u User, password, otp string) (migrationPrincipal, error) {
	var result migrationPrincipal
	path := "/internal/migrations/principal"
	if password != "" {
		path = "/internal/migrations/reauthenticate"
	}
	err := a.rpc(ctx, "identity", path, migrationReauthentication{User: u.ID, Device: u.DeviceID, Epoch: u.RestoreEpoch, Password: password, OTP: otp}, &result)
	if err != nil {
		return result, err
	}
	if !result.Active || result.AccessExpires == nil || !result.AccessExpires.After(time.Now()) || result.User != u.ID || result.Device != u.DeviceID || result.Epoch != u.RestoreEpoch || !syncValidationCounter(result.Security, false) || !syncValidationCounter(result.SourceGeneration, false) {
		return result, fail(403, "MIGRATION_PRINCIPAL_CHANGED", "Current personal device authorization required")
	}
	return result, nil
}
func migrationDeadline(now time.Time, refs []migrationMessageReference, objects []migrationObjectReference) time.Time {
	expires := now.Add(10 * time.Minute)
	for _, r := range refs {
		if r.Deadline != nil && r.Deadline.Before(expires) {
			expires = *r.Deadline
		}
	}
	for _, r := range objects {
		if r.Deadline != nil && r.Deadline.Before(expires) {
			expires = *r.Deadline
		}
	}
	return expires
}
func migrationPrivateTLSProof(a *App, r *http.Request) string {
	return base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "migration-tls-v1", []byte(a.admissionFenceIdentity()+"\x00"+r.Method+"\x00"+r.URL.Path+"\x00"+r.Header.Get("X-JobsIM-Migration-TLS-Time"))))
}
func (a *App) requireMigrationTLS(r *http.Request) error {
	value := r.Header.Get("X-JobsIM-Migration-TLS-Time")
	stamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil || stamp < time.Now().Add(-10*time.Second).Unix() || stamp > time.Now().Add(time.Second).Unix() || (!a.hasGatewayProjectionFence(r, false) && !a.hasGatewayProjectionFence(r, true)) || !hmac.Equal([]byte(r.Header.Get("X-JobsIM-Migration-TLS-Proof")), []byte(migrationPrivateTLSProof(a, r))) {
		return fail(403, "MIGRATION_AUTHENTICATED_TLS_REQUIRED", "Migration and separate key handoff require the authenticated HTTPS gateway")
	}
	return nil
}
func migrationProof(a *App, kind string, value any) string {
	raw, _ := json.Marshal(value)
	return kind + "." + base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "migration-proof:"+kind, raw))
}
func migrationDecodeProof(a *App, kind, token string, value any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != kind {
		return fail(400, "INVALID_MIGRATION_PROOF", "Original typed pairing proof required")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(raw) > 8192 {
		return fail(400, "INVALID_MIGRATION_PROOF", "Bounded pairing proof required")
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, keyedDigest(a.Config.Key, "migration-proof:"+kind, raw)) {
		return fail(403, "MIGRATION_PROOF_REJECTED", "Pairing identity or signature differs")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(400, "INVALID_MIGRATION_PROOF", "Proof fields or trailing values differ")
	}
	return nil
}
