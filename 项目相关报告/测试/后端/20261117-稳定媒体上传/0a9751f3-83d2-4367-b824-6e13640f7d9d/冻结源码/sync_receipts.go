package jobsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func migrateSyncReceipts(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS sync.storage_qualifications(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),
user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL CHECK(restore_epoch>0),store_generation uuid NOT NULL,
qualification_id uuid NOT NULL UNIQUE,profile_id text NOT NULL,build_digest text NOT NULL,verification_digest text NOT NULL,
declared_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,source_context_id,user_id,device_id,restore_epoch),
FOREIGN KEY(scope_id,source_context_id,user_id,device_id,restore_epoch) REFERENCES sync.device_stores(scope_id,source_context_id,user_id,device_id,restore_epoch));
ALTER TABLE sync.storage_qualifications ADD COLUMN IF NOT EXISTS storage_contract text NOT NULL DEFAULT 'native_sqlite_full_v1';
CREATE TABLE IF NOT EXISTS sync.storage_declarations(
scope_id uuid NOT NULL,user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL,request_id text NOT NULL,
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),result jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,user_id,device_id,restore_epoch,request_id));
CREATE TABLE IF NOT EXISTS sync.content_receipts(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,device_id uuid NOT NULL,
restore_epoch bigint NOT NULL CHECK(restore_epoch>0),store_generation uuid NOT NULL,qualification_id uuid NOT NULL,
request_id text NOT NULL CHECK(octet_length(request_id) BETWEEN 8 AND 128),request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
message_id uuid NOT NULL,conversation_id uuid NOT NULL,content_version bigint NOT NULL CHECK(content_version>0),body_digest text NOT NULL,
permission_generation text NOT NULL,result jsonb NOT NULL,received_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,source_context_id,user_id,device_id,restore_epoch,request_id));
CREATE INDEX IF NOT EXISTS sync_receipts_content ON sync.content_receipts(scope_id,message_id,restore_epoch,content_version);`)
	if err != nil {
		return err
	}
	if err = migrateSyncObjectReceipts(ctx, tx); err != nil {
		return err
	}
	return migrateReceiptDelivery(ctx, tx)
}

type storageDeclaration struct {
	RequestID          string `json:"requestId"`
	StoreGeneration    string `json:"storeGeneration"`
	RevisionEpoch      string `json:"revisionEpoch"`
	ProfileID          string `json:"profileId"`
	BuildDigest        string `json:"buildDigest"`
	VerificationDigest string `json:"verificationDigest"`
}

type storageQualification struct {
	ID              string `json:"qualificationId"`
	Store           string `json:"storeGeneration"`
	Epoch           string `json:"restoreEpoch"`
	Profile         string `json:"profileId"`
	Build           string `json:"buildDigest"`
	Verification    string `json:"verificationDigest"`
	Qualified       bool   `json:"qualified"`
	Contract        string `json:"storageContract"`
	ObjectQualified bool   `json:"objectQualified"`
}

func validReceiptNonce(value string) bool {
	return len(value) >= 8 && len(value) <= 128 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\x00")
}

func decodeReceiptRequest(r *http.Request, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(400, "INVALID_RECEIPT_REQUEST", "One complete bounded original receipt request required")
	}
	return nil
}

func (a *App) syncReceiptRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("PUT /api/sync/storage", a.declareSyncStorage)
	h("GET /api/sync/storage", a.getSyncStorage)
	h("POST /api/sync/receipt-challenges", a.syncReceiptChallenge)
	h("POST /api/sync/receipts", a.submitSyncReceipt)
	h("POST /api/sync/receipt-results", a.syncReceiptResults)
	a.syncObjectReceiptRoutes(h)
}

func (a *App) getStorageQualification(ctx context.Context, u User, store string) (storageQualification, error) {
	var value storageQualification
	err := a.DB.QueryRow(ctx, `SELECT q.qualification_id::text,q.store_generation::text,q.restore_epoch::text,q.profile_id,q.build_digest,q.verification_digest,q.storage_contract
FROM sync.storage_qualifications q JOIN sync.device_stores d USING(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation) WHERE q.scope_id=$1 AND q.source_context_id='personal' AND q.user_id=$2 AND q.device_id=$3 AND q.restore_epoch=$4 AND q.store_generation=$5`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, store).Scan(&value.ID, &value.Store, &value.Epoch, &value.Profile, &value.Build, &value.Verification, &value.Contract)
	if err == pgx.ErrNoRows {
		return storageQualification{Store: store, Epoch: u.RestoreEpoch}, nil
	}
	if err != nil {
		return value, err
	}
	profile, approved := a.receiptProfile(value.Profile, value.Build, value.Verification)
	value.Qualified = approved && profile.StorageContract == value.Contract
	value.ObjectQualified = value.Qualified && value.Contract == receiptFileContract
	return value, nil
}

func (a *App) getSyncStorage(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	reg, err := a.syncRegistered(r.Context(), u, r.URL.Query().Get("schemaVersion"))
	if err != nil {
		return err
	}
	value, err := a.getStorageQualification(r.Context(), u, reg.Store)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]any{"storage": value, "afterReceiptEnabled": a.receiptLifecycleReady(r.Context(), value), "deletionEligible": false})
	return nil
}

func (a *App) declareSyncStorage(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	var q storageDeclaration
	if err = decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validReceiptNonce(q.RequestID) || !syncValidationID(q.StoreGeneration) || !syncValidationCounter(q.RevisionEpoch, false) {
		return fail(400, "INVALID_STORAGE_DECLARATION", "Original nonce, physical store and restore epoch required")
	}
	if q.RevisionEpoch != u.RestoreEpoch {
		return fail(409, "RESTORE_EPOCH_CHANGED", "A restored instance never inherits old storage qualification")
	}
	profile, allowed := a.receiptProfile(q.ProfileID, q.BuildDigest, q.VerificationDigest)
	if !allowed {
		return fail(409, "STORAGE_PROFILE_UNAPPROVED", "This exact native build and verification report are not approved by the deployment")
	}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "storage-declaration:"+u.ID+":"+u.DeviceID, raw)
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var store string
	err = tx.QueryRow(r.Context(), `SELECT store_generation::text FROM sync.device_stores WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 FOR UPDATE`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch).Scan(&store)
	if err == pgx.ErrNoRows {
		return fail(409, "SYNC_DEVICE_REQUIRED", "Register this physical local store first")
	}
	if err != nil {
		return err
	}
	if store != q.StoreGeneration {
		return fail(409, "STORE_GENERATION_CHANGED", "Storage declarations cannot replace the physical local store")
	}
	var oldDigest, result []byte
	err = tx.QueryRow(r.Context(), `SELECT request_digest,result FROM sync.storage_declarations WHERE scope_id=$1 AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND request_id=$5`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.RequestID).Scan(&oldDigest, &result)
	if err == nil {
		if !hmac.Equal(oldDigest, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "The original storage declaration differs")
		}
		if err = tx.Commit(r.Context()); err != nil {
			return err
		}
		return a.respondStorageDeclaration(w, r, u, store, result)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	var qualification string
	err = tx.QueryRow(r.Context(), `INSERT INTO sync.storage_qualifications(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation,qualification_id,profile_id,build_digest,verification_digest,storage_contract)
VALUES($1,'personal',$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(scope_id,source_context_id,user_id,device_id,restore_epoch) DO UPDATE SET
qualification_id=CASE WHEN sync.storage_qualifications.profile_id=EXCLUDED.profile_id AND sync.storage_qualifications.build_digest=EXCLUDED.build_digest AND sync.storage_qualifications.verification_digest=EXCLUDED.verification_digest AND sync.storage_qualifications.storage_contract=EXCLUDED.storage_contract THEN sync.storage_qualifications.qualification_id ELSE EXCLUDED.qualification_id END,
profile_id=EXCLUDED.profile_id,build_digest=EXCLUDED.build_digest,verification_digest=EXCLUDED.verification_digest,storage_contract=EXCLUDED.storage_contract,declared_at=now()
RETURNING qualification_id::text`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, store, newID(), q.ProfileID, q.BuildDigest, q.VerificationDigest, profile.StorageContract).Scan(&qualification)
	if err != nil {
		return err
	}
	result, _ = json.Marshal(map[string]any{"qualificationId": qualification, "storeGeneration": store, "restoreEpoch": u.RestoreEpoch, "profileId": q.ProfileID, "buildDigest": q.BuildDigest, "verificationDigest": q.VerificationDigest, "storageContract": profile.StorageContract})
	if _, err = tx.Exec(r.Context(), `INSERT INTO sync.storage_declarations(scope_id,user_id,device_id,restore_epoch,request_id,request_digest,result) VALUES($1,$2,$3,$4,$5,$6,$7)`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.RequestID, digest, result); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return a.respondStorageDeclaration(w, r, u, store, result)
}

func (a *App) respondStorageDeclaration(w http.ResponseWriter, r *http.Request, u User, store string, result []byte) error {
	current, err := a.getStorageQualification(r.Context(), u, store)
	if err != nil {
		return err
	}
	var original map[string]any
	if err = json.Unmarshal(result, &original); err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]any{"result": original, "current": current.Qualified && original["qualificationId"] == current.ID, "storage": current, "deletionEligible": false, "afterReceiptEnabled": a.receiptLifecycleReady(r.Context(), current)})
	return nil
}

type receiptProof struct {
	Scope              string `json:"scope"`
	User               string `json:"user"`
	Device             string `json:"device"`
	Store              string `json:"store"`
	Epoch              string `json:"epoch"`
	Qualification      string `json:"qualification"`
	Message            string `json:"message"`
	Conversation       string `json:"conversation"`
	Content            string `json:"contentVersion"`
	BodyDigest         string `json:"bodyDigest"`
	Permission         string `json:"permission"`
	Expires            int64  `json:"expires"`
	DeliveryGeneration string `json:"deliveryGeneration,omitempty"`
	ManifestDigest     string `json:"manifestDigest,omitempty"`
}

func (a *App) encodeReceiptProof(proof receiptProof) string {
	raw, _ := json.Marshal(proof)
	return "rc1." + base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "content-receipt-v1", raw))
}

func (a *App) decodeReceiptProof(token string, now time.Time) (receiptProof, error) {
	var proof receiptProof
	parts := strings.Split(token, ".")
	if len(token) > 4096 || len(parts) != 3 || parts[0] != "rc1" {
		return proof, fail(400, "INVALID_RECEIPT_PROOF", "Original content receipt challenge required")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return proof, fail(400, "INVALID_RECEIPT_PROOF", "Invalid challenge envelope")
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, keyedDigest(a.Config.Key, "content-receipt-v1", raw)) {
		return proof, fail(400, "INVALID_RECEIPT_PROOF", "Challenge signature mismatch")
	}
	if err = json.Unmarshal(raw, &proof); err != nil || !syncValidationID(proof.Scope) || proof.Scope != a.Config.DeploymentID || !syncValidationID(proof.User) || !syncValidationID(proof.Device) || !syncValidationID(proof.Store) || !syncValidationID(proof.Qualification) || !syncValidationID(proof.Message) || !syncValidationID(proof.Conversation) || !syncValidationCounter(proof.Epoch, false) || !syncValidationCounter(proof.Content, false) || !syncValidationDigest(proof.BodyDigest) || !syncValidationDigest(proof.Permission) {
		return proof, fail(400, "INVALID_RECEIPT_PROOF", "Challenge identity or content tuple invalid")
	}
	if proof.Expires <= now.Unix() || proof.Expires > now.Add(time.Hour+time.Minute).Unix() {
		return proof, fail(409, "RECEIPT_PROOF_EXPIRED", "Retain the old receipt intent; do not replace its proof or identity")
	}
	if (proof.DeliveryGeneration == "") != (proof.ManifestDigest == "") || proof.DeliveryGeneration != "" && (!syncValidationCounter(proof.DeliveryGeneration, false) || !syncValidationDigest(proof.ManifestDigest)) {
		return proof, fail(400, "INVALID_RECEIPT_PROOF", "Delivery generation and frozen manifest must be paired")
	}
	return proof, nil
}

func (a *App) receiptMessage(ctx context.Context, u User, id string) (syncProjection, Message, error) {
	projection, err := a.syncProject(ctx, u, syncRef{Owner: "message", Kind: "message", ID: id, Revision: "0", RevisionEpoch: u.RestoreEpoch, State: "active"})
	if err != nil {
		return projection, Message{}, err
	}
	if projection.State != "available" {
		return projection, Message{}, fail(409, "RECEIPT_CONTENT_UNAVAILABLE", "Only complete currently authorized message content can be acknowledged")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(projection.BodyEnvelope)
	var message Message
	if err != nil || syncDigest(raw) != projection.BodyDigest || json.Unmarshal(raw, &message) != nil || message.ID != id || !syncValidationID(message.ConversationID) || !syncValidationCounter(message.ContentVersion, false) || message.Deleted || message.ContentUnavailable || message.Forwarded || message.ForwardUnavailable {
		return projection, Message{}, fail(409, "RECEIPT_CONTENT_UNSUPPORTED", "A complete ordinary message is required; reference forwards need their own source-holder receipt contract")
	}
	object, err := json.Marshal(projection.Object)
	decodedObject, objectErr := receiptJSONValue(object)
	decodedEnvelope, envelopeErr := receiptJSONValue(raw)
	if err != nil || objectErr != nil || envelopeErr != nil || !reflect.DeepEqual(decodedObject, decodedEnvelope) {
		return projection, Message{}, fail(503, "RECEIPT_PROJECTION_MISMATCH", "Owner object and original body envelope differ")
	}
	return projection, message, nil
}

func receiptJSONValue(raw []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	err := decoder.Decode(&value)
	return value, err
}

func (a *App) syncReceiptChallenge(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	var q struct {
		Message string `json:"messageId"`
		Schema  string `json:"schemaVersion"`
	}
	if err = decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.Message) {
		return fail(400, "INVALID_MESSAGE_ID", "Canonical message identity required")
	}
	reg, err := a.syncRegistered(r.Context(), u, q.Schema)
	if err != nil {
		return err
	}
	storage, err := a.getStorageQualification(r.Context(), u, reg.Store)
	if err != nil {
		return err
	}
	if !storage.Qualified {
		return fail(409, "STORAGE_NOT_QUALIFIED", "Transport-only storage cannot issue durable content receipts")
	}
	projection, message, err := a.receiptMessage(r.Context(), u, q.Message)
	if err != nil {
		return err
	}
	view, err := a.syncCurrentView(r.Context(), u, reg.Schema)
	if err != nil {
		return err
	}
	proof := receiptProof{Scope: a.Config.DeploymentID, User: u.ID, Device: u.DeviceID, Store: reg.Store, Epoch: u.RestoreEpoch, Qualification: storage.ID, Message: message.ID, Conversation: message.ConversationID, Content: message.ContentVersion, BodyDigest: projection.BodyDigest, Permission: view.Permission, Expires: time.Now().Add(time.Hour).Unix()}
	proof.DeliveryGeneration, proof.ManifestDigest, err = a.receiptDeliveryTuple(r.Context(), message.ID, u.RestoreEpoch, message.ContentVersion)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]any{"challenge": a.encodeReceiptProof(proof), "messageId": message.ID, "conversationId": message.ConversationID, "contentVersion": message.ContentVersion, "restoreEpoch": u.RestoreEpoch, "storeGeneration": reg.Store, "qualificationId": storage.ID, "bodyEnvelope": projection.BodyEnvelope, "bodyDigest": projection.BodyDigest, "expiresAt": time.Unix(proof.Expires, 0).UTC(), "requiresAtomicLocalCommit": true, "objectComplete": false, "deletionEligible": false})
	return nil
}

type contentReceiptRequest struct {
	RequestID     string `json:"requestId"`
	Store         string `json:"storeGeneration"`
	Epoch         string `json:"revisionEpoch"`
	Qualification string `json:"qualificationId"`
	Kind          string `json:"kind"`
	Challenge     string `json:"challenge"`
	BodyDigest    string `json:"bodyDigest"`
}

func (a *App) readContentReceipt(r *http.Request, u User, q contentReceiptRequest) (map[string]any, bool, error) {
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "content-receipt-request:"+u.ID+":"+u.DeviceID, raw)
	var prior, result []byte
	err := a.DB.QueryRow(r.Context(), `SELECT request_digest,result FROM sync.content_receipts WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND request_id=$5`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.RequestID).Scan(&prior, &result)
	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !hmac.Equal(digest, prior) {
		return nil, false, fail(409, "IDEMPOTENCY_CONFLICT", "Original receipt body and nonce must be retained")
	}
	var original map[string]any
	if err = json.Unmarshal(result, &original); err != nil {
		return nil, false, err
	}
	return original, true, nil
}

func (a *App) receiptRequest(r *http.Request, u User) (contentReceiptRequest, error) {
	var q contentReceiptRequest
	if err := decodeReceiptRequest(r, &q); err != nil {
		return q, err
	}
	if !validReceiptNonce(q.RequestID) || !syncValidationID(q.Store) || !syncValidationID(q.Qualification) || !syncValidationCounter(q.Epoch, false) || !syncValidationDigest(q.BodyDigest) || q.Kind != "message_content" || len(q.Challenge) == 0 || len(q.Challenge) > 4096 {
		return q, fail(400, "INVALID_CONTENT_RECEIPT", "Exact original message-content receipt required; cumulative and object receipts are separate contracts")
	}
	if q.Epoch != u.RestoreEpoch {
		return q, fail(409, "RESTORE_EPOCH_CHANGED", "Old content receipts cannot acknowledge restored content")
	}
	var matches bool
	if err := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sync.device_stores WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND store_generation=$5)`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.Store).Scan(&matches); err != nil {
		return q, err
	}
	if !matches {
		return q, fail(409, "STORE_GENERATION_CHANGED", "Receipt belongs to another physical local store")
	}
	return q, nil
}

func (a *App) syncReceiptResults(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	q, err := a.receiptRequest(r, u)
	if err != nil {
		return err
	}
	result, exists, err := a.readContentReceipt(r, u, q)
	if err != nil {
		return err
	}
	if !exists {
		return fail(404, "RECEIPT_OPERATION_NOT_FOUND", "No matching committed receipt; no acknowledgement was created")
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, result)
	return nil
}

func (a *App) submitSyncReceipt(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	q, err := a.receiptRequest(r, u)
	if err != nil {
		return err
	}
	if original, exists, err := a.readContentReceipt(r, u, q); err != nil {
		return err
	} else if exists {
		respond(w, 200, original)
		return nil
	}
	proof, err := a.decodeReceiptProof(q.Challenge, time.Now())
	if err != nil {
		return err
	}
	if proof.User != u.ID || proof.Device != u.DeviceID || proof.Epoch != u.RestoreEpoch || proof.Store != q.Store || proof.Qualification != q.Qualification || proof.BodyDigest != q.BodyDigest {
		return fail(409, "RECEIPT_CONTEXT_CHANGED", "Proof belongs to another device, store, content or restore epoch")
	}
	storage, err := a.getStorageQualification(r.Context(), u, q.Store)
	if err != nil {
		return err
	}
	if !storage.Qualified || storage.ID != proof.Qualification {
		return fail(409, "STORAGE_NOT_QUALIFIED", "Original approved storage qualification is no longer current")
	}
	_, message, err := a.receiptMessage(r.Context(), u, proof.Message)
	if err != nil {
		return err
	}
	if message.ContentVersion != proof.Content || message.ConversationID != proof.Conversation {
		return fail(409, "RECEIPT_CONTENT_CHANGED", "An old receipt never acknowledges an edited message")
	}
	generation, manifestDigest, err := a.receiptDeliveryTuple(r.Context(), message.ID, proof.Epoch, proof.Content)
	if err != nil {
		return err
	}
	if generation != proof.DeliveryGeneration || manifestDigest != proof.ManifestDigest {
		return fail(409, "RECEIPT_GENERATION_CHANGED", "An earlier manifest cannot acknowledge the current delivery generation")
	}
	view, err := a.syncCurrentView(r.Context(), u)
	if err != nil {
		return err
	}
	if view.Permission != proof.Permission {
		return fail(409, "RECEIPT_PERMISSION_CHANGED", "Current authorization no longer matches the original content delivery")
	}
	result := map[string]any{"operationId": q.RequestID, "status": "recorded", "messageId": proof.Message, "conversationId": proof.Conversation, "contentVersion": proof.Content, "restoreEpoch": proof.Epoch, "deviceId": proof.Device, "storeGeneration": proof.Store, "qualificationId": proof.Qualification, "bodyDigest": proof.BodyDigest, "receiptKind": "message_content", "storageQualifiedAtCommit": true, "deletionEligible": false, "objectComplete": false, "targetRangeComplete": false}
	result["deliveryGeneration"], result["manifestDigest"] = proof.DeliveryGeneration, proof.ManifestDigest
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "content-receipt-request:"+u.ID+":"+u.DeviceID, raw)
	encodedResult, _ := json.Marshal(result)
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var currentStore, qualification, epoch string
	if err = tx.QueryRow(r.Context(), `SELECT d.store_generation::text,q.qualification_id::text,i.restore_epoch::text FROM sync.device_stores d
JOIN sync.storage_qualifications q USING(scope_id,source_context_id,user_id,device_id,restore_epoch) JOIN sync.instance i ON i.singleton
WHERE d.scope_id=$1 AND d.source_context_id='personal' AND d.user_id=$2 AND d.device_id=$3 AND d.restore_epoch=$4 FOR SHARE OF d,q,i`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch).Scan(&currentStore, &qualification, &epoch); err != nil {
		return err
	}
	if currentStore != proof.Store || qualification != proof.Qualification || epoch != proof.Epoch {
		return fail(409, "RECEIPT_CONTEXT_CHANGED", "Storage or restore context changed before receipt commit")
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO sync.content_receipts(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation,qualification_id,request_id,request_digest,message_id,conversation_id,content_version,body_digest,permission_generation,result)
VALUES($1,'personal',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, u.ID, u.DeviceID, proof.Epoch, proof.Store, proof.Qualification, q.RequestID, digest, proof.Message, proof.Conversation, proof.Content, proof.BodyDigest, proof.Permission, encodedResult)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	// A racing same-nonce request is checked against its committed original body.
	stored, _, err := a.readContentReceipt(r, u, q)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, stored)
	return nil
}
