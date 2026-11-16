package jobsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func migrateSyncObjectReceipts(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS sync.object_receipts(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),
user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL CHECK(restore_epoch>0),
store_generation uuid NOT NULL,qualification_id uuid NOT NULL,request_id text NOT NULL,
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),parent_request_id text NOT NULL,
message_id uuid NOT NULL,conversation_id uuid NOT NULL,content_version bigint NOT NULL CHECK(content_version>0),
object_id uuid NOT NULL,size_bytes bigint NOT NULL CHECK(size_bytes>=0 AND size_bytes<=67108864),
object_digest text NOT NULL,body_digest text NOT NULL,permission_generation text NOT NULL,
result jsonb NOT NULL,received_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,source_context_id,user_id,device_id,restore_epoch,request_id),
FOREIGN KEY(scope_id,source_context_id,user_id,device_id,restore_epoch,parent_request_id)
REFERENCES sync.content_receipts(scope_id,source_context_id,user_id,device_id,restore_epoch,request_id));
CREATE INDEX IF NOT EXISTS sync_object_receipts_content ON sync.object_receipts(scope_id,message_id,restore_epoch,content_version,object_id);`)
	return err
}

type receiptObject struct {
	ID          string `json:"objectId"`
	Size        string `json:"sizeBytes"`
	Digest      string `json:"sha256"`
	ContentType string `json:"contentType"`
}

type objectReceiptProof struct {
	receiptProof
	Parent string `json:"parentReceiptId"`
	Object string `json:"objectId"`
	Size   string `json:"sizeBytes"`
	Digest string `json:"objectDigest"`
}

type objectReceiptRequest struct {
	RequestID     string `json:"requestId"`
	Store         string `json:"storeGeneration"`
	Epoch         string `json:"revisionEpoch"`
	Qualification string `json:"qualificationId"`
	Kind          string `json:"kind"`
	Parent        string `json:"parentReceiptId"`
	Challenge     string `json:"challenge"`
	Object        string `json:"objectId"`
	Size          string `json:"sizeBytes"`
	Digest        string `json:"objectDigest"`
}

type contentReceiptParent struct {
	Store, Qualification, Message, Conversation, Content, BodyDigest, Permission string
}

func validReceiptObjectSize(raw string) bool {
	n, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && n >= 0 && n <= 64<<20 && str(n) == raw
}

func (a *App) syncObjectReceiptRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("POST /api/sync/object-receipt-challenges", a.syncObjectReceiptChallenge)
	h("POST /api/sync/object-receipts", a.submitSyncObjectReceipt)
	h("POST /api/sync/object-receipt-results", a.syncObjectReceiptResults)
}

func (a *App) encodeObjectReceiptProof(proof objectReceiptProof) string {
	raw, _ := json.Marshal(proof)
	return "ro1." + base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "object-receipt-v1", raw))
}

func (a *App) decodeObjectReceiptProof(token string, now time.Time) (objectReceiptProof, error) {
	var proof objectReceiptProof
	parts := strings.Split(token, ".")
	if len(token) > 8192 || len(parts) != 3 || parts[0] != "ro1" {
		return proof, fail(400, "INVALID_OBJECT_RECEIPT_PROOF", "An original complete-object challenge is required")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return proof, fail(400, "INVALID_OBJECT_RECEIPT_PROOF", "Invalid object challenge envelope")
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, keyedDigest(a.Config.Key, "object-receipt-v1", raw)) {
		return proof, fail(400, "INVALID_OBJECT_RECEIPT_PROOF", "Object challenge signature or document differs")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&proof) != nil || decoder.Decode(new(any)) != io.EOF {
		return proof, fail(400, "INVALID_OBJECT_RECEIPT_PROOF", "Exactly one complete original object challenge is required")
	}
	// Reuse the exact identity and deadline rules without accepting an rc1 token
	// as an object proof. The domain-separated ro1 signature is checked first.
	if _, err = a.decodeReceiptProof(a.encodeReceiptProof(proof.receiptProof), now); err != nil {
		return proof, err
	}
	if !validReceiptNonce(proof.Parent) || !syncValidationID(proof.Object) || !validReceiptObjectSize(proof.Size) || !syncValidationDigest(proof.Digest) {
		return proof, fail(400, "INVALID_OBJECT_RECEIPT_PROOF", "Complete object identity, original byte count and digest are required")
	}
	return proof, nil
}

func (a *App) objectReceiptParent(ctx context.Context, u User, request string) (contentReceiptParent, error) {
	var parent contentReceiptParent
	err := a.DB.QueryRow(ctx, `SELECT store_generation::text,qualification_id::text,message_id::text,conversation_id::text,
content_version::text,body_digest,permission_generation FROM sync.content_receipts
WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND request_id=$5`,
		a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, request).Scan(&parent.Store, &parent.Qualification, &parent.Message, &parent.Conversation, &parent.Content, &parent.BodyDigest, &parent.Permission)
	if err == pgx.ErrNoRows {
		return parent, fail(409, "CONTENT_RECEIPT_REQUIRED", "This same physical store must first commit the original message descriptor receipt")
	}
	return parent, err
}

func (a *App) currentReceiptObject(ctx context.Context, u User, parent contentReceiptParent) (receiptObject, error) {
	var object receiptObject
	_, message, err := a.receiptMessage(ctx, u, parent.Message)
	if err != nil {
		return object, err
	}
	if message.ContentVersion != parent.Content || message.ConversationID != parent.Conversation {
		return object, fail(409, "RECEIPT_CONTENT_CHANGED", "An earlier content receipt cannot confirm an edited attachment")
	}
	if !syncValidationID(message.AttachmentID) {
		return object, fail(409, "RECEIPT_OBJECT_UNAVAILABLE", "A currently authorized original attachment is required")
	}
	if err = a.rpc(ctx, "media", "/internal/receipts/object", map[string]string{"userId": u.ID, "messageId": message.ID, "objectId": message.AttachmentID}, &object); err != nil {
		return object, err
	}
	if object.ID != message.AttachmentID || !validReceiptObjectSize(object.Size) || !syncValidationDigest(object.Digest) || object.ContentType == "" {
		return object, fail(503, "RECEIPT_OBJECT_MISMATCH", "Media owner did not return a complete original object descriptor")
	}
	return object, nil
}

func (a *App) syncObjectReceiptChallenge(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	var q struct {
		Parent string `json:"parentReceiptId"`
		Schema string `json:"schemaVersion"`
	}
	if err = decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validReceiptNonce(q.Parent) {
		return fail(400, "INVALID_OBJECT_RECEIPT", "Original committed parent receipt identity required")
	}
	reg, err := a.syncRegistered(r.Context(), u, q.Schema)
	if err != nil {
		return err
	}
	parent, err := a.objectReceiptParent(r.Context(), u, q.Parent)
	if err != nil {
		return err
	}
	storage, err := a.getStorageQualification(r.Context(), u, reg.Store)
	if err != nil {
		return err
	}
	if !storage.ObjectQualified || storage.ID != parent.Qualification || reg.Store != parent.Store {
		return fail(409, "STORAGE_NOT_QUALIFIED", "The original descriptor must belong to this current approved physical store")
	}
	view, err := a.syncCurrentView(r.Context(), u, reg.Schema)
	if err != nil {
		return err
	}
	if view.Permission != parent.Permission {
		return fail(409, "RECEIPT_PERMISSION_CHANGED", "Original descriptor authorization is no longer current")
	}
	object, err := a.currentReceiptObject(r.Context(), u, parent)
	if err != nil {
		return err
	}
	proof := objectReceiptProof{receiptProof: receiptProof{Scope: a.Config.DeploymentID, User: u.ID, Device: u.DeviceID, Store: parent.Store, Epoch: u.RestoreEpoch, Qualification: parent.Qualification, Message: parent.Message, Conversation: parent.Conversation, Content: parent.Content, BodyDigest: parent.BodyDigest, Permission: parent.Permission, Expires: time.Now().Add(time.Hour).Unix()}, Parent: q.Parent, Object: object.ID, Size: object.Size, Digest: object.Digest}
	respond(w, 200, map[string]any{"challenge": a.encodeObjectReceiptProof(proof), "parentReceiptId": q.Parent, "messageId": parent.Message, "conversationId": parent.Conversation, "contentVersion": parent.Content, "restoreEpoch": u.RestoreEpoch, "storeGeneration": parent.Store, "qualificationId": parent.Qualification, "bodyDigest": parent.BodyDigest, "object": object, "expiresAt": time.Unix(proof.Expires, 0).UTC(), "requiresDurableOriginalObject": true, "objectComplete": false, "targetRangeComplete": false, "deletionEligible": false})
	return nil
}

func (a *App) objectReceiptRequest(r *http.Request, u User) (objectReceiptRequest, error) {
	var q objectReceiptRequest
	if err := decodeReceiptRequest(r, &q); err != nil {
		return q, err
	}
	if !validReceiptNonce(q.RequestID) || !validReceiptNonce(q.Parent) || !syncValidationID(q.Store) || !syncValidationID(q.Qualification) || !syncValidationCounter(q.Epoch, false) || q.Kind != "object_received" || !syncValidationID(q.Object) || !validReceiptObjectSize(q.Size) || !syncValidationDigest(q.Digest) || len(q.Challenge) == 0 || len(q.Challenge) > 8192 {
		return q, fail(400, "INVALID_OBJECT_RECEIPT", "Exact original object receipt and byte-level identity required")
	}
	if q.Epoch != u.RestoreEpoch {
		return q, fail(409, "RESTORE_EPOCH_CHANGED", "Old object receipts do not acknowledge restored objects")
	}
	var current bool
	err := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sync.device_stores WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND store_generation=$5)`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.Store).Scan(&current)
	if err != nil {
		return q, err
	}
	if !current {
		return q, fail(409, "STORE_GENERATION_CHANGED", "Object receipt belongs to another physical local store")
	}
	return q, nil
}

func (a *App) readObjectReceipt(r *http.Request, u User, q objectReceiptRequest) (map[string]any, bool, error) {
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "object-receipt-request:"+u.ID+":"+u.DeviceID, raw)
	var prior, result []byte
	err := a.DB.QueryRow(r.Context(), `SELECT request_digest,result FROM sync.object_receipts WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND request_id=$5`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.RequestID).Scan(&prior, &result)
	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !hmac.Equal(prior, digest) {
		return nil, false, fail(409, "IDEMPOTENCY_CONFLICT", "Original object receipt nonce, proof and byte identity must be retained")
	}
	var original map[string]any
	if err = json.Unmarshal(result, &original); err != nil {
		return nil, false, err
	}
	return original, true, nil
}

func (a *App) syncObjectReceiptResults(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	q, err := a.objectReceiptRequest(r, u)
	if err != nil {
		return err
	}
	result, exists, err := a.readObjectReceipt(r, u, q)
	if err != nil {
		return err
	}
	if !exists {
		return fail(404, "OBJECT_RECEIPT_OPERATION_NOT_FOUND", "No matching object receipt exists; none was created")
	}
	respond(w, 200, result)
	return nil
}

func (a *App) submitSyncObjectReceipt(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	q, err := a.objectReceiptRequest(r, u)
	if err != nil {
		return err
	}
	if original, exists, err := a.readObjectReceipt(r, u, q); err != nil {
		return err
	} else if exists {
		respond(w, 200, original)
		return nil
	}
	proof, err := a.decodeObjectReceiptProof(q.Challenge, time.Now())
	if err != nil {
		return err
	}
	if proof.User != u.ID || proof.Device != u.DeviceID || proof.Store != q.Store || proof.Epoch != u.RestoreEpoch || proof.Qualification != q.Qualification || proof.Parent != q.Parent || proof.Object != q.Object || proof.Size != q.Size || proof.Digest != q.Digest {
		return fail(409, "OBJECT_RECEIPT_CONTEXT_CHANGED", "Object proof does not match the complete original local receipt")
	}
	parent, err := a.objectReceiptParent(r.Context(), u, q.Parent)
	if err != nil {
		return err
	}
	if parent != (contentReceiptParent{proof.Store, proof.Qualification, proof.Message, proof.Conversation, proof.Content, proof.BodyDigest, proof.Permission}) {
		return fail(409, "OBJECT_RECEIPT_PARENT_CHANGED", "Original message descriptor receipt differs")
	}
	storage, err := a.getStorageQualification(r.Context(), u, q.Store)
	if err != nil {
		return err
	}
	if !storage.ObjectQualified || storage.ID != proof.Qualification {
		return fail(409, "STORAGE_NOT_QUALIFIED", "Original approved physical store is no longer current")
	}
	view, err := a.syncCurrentView(r.Context(), u)
	if err != nil {
		return err
	}
	if view.Permission != proof.Permission {
		return fail(409, "RECEIPT_PERMISSION_CHANGED", "Object delivery authorization changed")
	}
	object, err := a.currentReceiptObject(r.Context(), u, parent)
	if err != nil {
		return err
	}
	if object.ID != proof.Object || object.Size != proof.Size || object.Digest != proof.Digest {
		return fail(409, "RECEIPT_OBJECT_CHANGED", "The current original object bytes differ from this receipt")
	}
	result := map[string]any{"operationId": q.RequestID, "status": "recorded", "receiptKind": "object_received", "parentReceiptId": proof.Parent, "messageId": proof.Message, "conversationId": proof.Conversation, "contentVersion": proof.Content, "restoreEpoch": proof.Epoch, "deviceId": proof.Device, "storeGeneration": proof.Store, "qualificationId": proof.Qualification, "objectId": proof.Object, "sizeBytes": proof.Size, "objectDigest": proof.Digest, "bodyDigest": proof.BodyDigest, "storageQualifiedAtCommit": true, "objectComplete": true, "targetRangeComplete": false, "deletionEligible": false}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "object-receipt-request:"+u.ID+":"+u.DeviceID, raw)
	encoded, _ := json.Marshal(result)
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var currentStore, qualification, epoch string
	err = tx.QueryRow(r.Context(), `SELECT d.store_generation::text,q.qualification_id::text,i.restore_epoch::text
FROM sync.device_stores d JOIN sync.storage_qualifications q USING(scope_id,source_context_id,user_id,device_id,restore_epoch) JOIN sync.instance i ON i.singleton
WHERE d.scope_id=$1 AND d.source_context_id='personal' AND d.user_id=$2 AND d.device_id=$3 AND d.restore_epoch=$4 FOR SHARE OF d,q,i`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch).Scan(&currentStore, &qualification, &epoch)
	if err != nil {
		return err
	}
	if currentStore != proof.Store || qualification != proof.Qualification || epoch != proof.Epoch {
		return fail(409, "OBJECT_RECEIPT_CONTEXT_CHANGED", "Physical storage or restore context changed before commit")
	}
	var lockedParent contentReceiptParent
	err = tx.QueryRow(r.Context(), `SELECT store_generation::text,qualification_id::text,message_id::text,conversation_id::text,content_version::text,body_digest,permission_generation
FROM sync.content_receipts WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND request_id=$5 FOR SHARE`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, proof.Parent).Scan(&lockedParent.Store, &lockedParent.Qualification, &lockedParent.Message, &lockedParent.Conversation, &lockedParent.Content, &lockedParent.BodyDigest, &lockedParent.Permission)
	if err != nil {
		return err
	}
	if lockedParent != parent {
		return fail(409, "OBJECT_RECEIPT_PARENT_CHANGED", "The locked original descriptor receipt differs")
	}
	if time.Now().Unix() >= proof.Expires {
		return fail(409, "RECEIPT_PROOF_EXPIRED", "Original object challenge expired while awaiting its local commit")
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO sync.object_receipts(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation,qualification_id,request_id,request_digest,parent_request_id,message_id,conversation_id,content_version,object_id,size_bytes,object_digest,body_digest,permission_generation,result)
SELECT $1,'personal',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18 WHERE clock_timestamp()<to_timestamp($19::double precision) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, u.ID, u.DeviceID, proof.Epoch, proof.Store, proof.Qualification, q.RequestID, digest, proof.Parent, proof.Message, proof.Conversation, proof.Content, proof.Object, proof.Size, proof.Digest, proof.BodyDigest, proof.Permission, encoded, proof.Expires)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	stored, exists, err := a.readObjectReceipt(r, u, q)
	if err != nil {
		return err
	}
	if !exists {
		return fail(409, "RECEIPT_PROOF_EXPIRED", "No object receipt committed before the original deadline")
	}
	respond(w, 200, stored)
	return nil
}
