package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const qualifiedReceiptProtocol = "qualified-receipts-v1"

type receiptRuntimeReadiness struct {
	TransportConfigured bool     `json:"transportConfigured"`
	PublishConnected    bool     `json:"publishConnected"`
	ConsumeConnected    bool     `json:"consumeConnected"`
	MessageDurable      bool     `json:"messageDurable"`
	JournalDurable      bool     `json:"journalDurable"`
	JournalIndependent  bool     `json:"journalIndependent"`
	EpochMatches        bool     `json:"epochMatches"`
	Ready               bool     `json:"ready"`
	Blockers            []string `json:"blockers"`
}

func (a *App) receiptRuntime(ctx context.Context, epoch string) receiptRuntimeReadiness {
	v := receiptRuntimeReadiness{TransportConfigured: a.Config.NATSURL != "", PublishConnected: a.syncPublishConnected.Load(), ConsumeConnected: a.syncConsumeConnected.Load(), Blockers: []string{}}
	var current string
	if err := a.DB.QueryRow(ctx, `SELECT restore_epoch::text FROM sync.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&current); err != nil || current != epoch {
		v.Blockers = append(v.Blockers, "sync_epoch_unverified")
	} else {
		var owner receiptRuntimeReadiness
		if err = a.rpc(ctx, "message", "/internal/retention/readiness", map[string]string{"restoreEpoch": epoch}, &owner); err != nil {
			v.Blockers = append(v.Blockers, "deletion_protection_unavailable")
		} else {
			v.MessageDurable = owner.MessageDurable
			v.JournalDurable = owner.JournalDurable
			v.JournalIndependent = owner.JournalIndependent
			v.EpochMatches = owner.EpochMatches
			v.Blockers = append(v.Blockers, owner.Blockers...)
		}
	}
	if !v.TransportConfigured {
		v.Blockers = append(v.Blockers, "jetstream_unconfigured")
	}
	if !v.PublishConnected {
		v.Blockers = append(v.Blockers, "publisher_disconnected")
	}
	if !v.ConsumeConnected {
		v.Blockers = append(v.Blockers, "consumer_disconnected")
	}
	v.Ready = v.TransportConfigured && v.PublishConnected && v.ConsumeConnected && v.MessageDurable && v.JournalDurable && v.JournalIndependent && v.EpochMatches
	return v
}

func (a *App) receiptCapabilityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/sync/receipt-context", a.syncReceiptContext)
	h("GET /api/sync/receipt-profiles", a.syncReceiptProfiles)
	h("GET /api/sync/receipt-targets/{id}", a.syncReceiptTarget)
	h("POST /api/sync/storage-results", a.syncStorageResults)
	h("POST /internal/receipts/lifecycle-capability", internal(a.receiptLifecycleCapability))
}

func (a *App) syncReceiptTarget(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	u, _, store, err := a.receiptContextStore(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if !syncValidationID(id) {
		return fail(400, "INVALID_MESSAGE_ID", "An original canonical message is required")
	}
	qualification := store.ID
	if original := r.URL.Query().Get("qualificationId"); original != "" {
		if !syncValidationID(original) {
			return fail(400, "INVALID_RECEIPT_QUALIFICATION", "An exact original qualification is required")
		}
		var declared bool
		// Older successful declaration rows predate the explicit qualified bit.
		// Their original approved tuple and commit-only table remain the evidence.
		if err = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sync.storage_declarations d WHERE d.scope_id=$1 AND d.user_id=$2 AND d.device_id=$3 AND d.restore_epoch=$4 AND d.result->>'qualificationId'=$5 AND d.result->>'storeGeneration'=$6 AND d.result->>'restoreEpoch'=$4::bigint::text AND (NOT(d.result?'qualified') OR d.result->'qualified'='true'::jsonb) AND d.result->>'storageContract' IN('native_sqlite_full_v1','native_sqlite_full_file_fsync_v1') AND d.result->>'profileId'~'^[a-z0-9_.-]{8,128}$' AND d.result->>'buildDigest'~'^[0-9a-f]{64}$' AND d.result->>'verificationDigest'~'^[0-9a-f]{64}$')`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, original, store.Store).Scan(&declared); err != nil {
			return err
		}
		if !declared {
			return fail(404, "RECEIPT_QUALIFICATION_NOT_FOUND", "No original declaration belongs to this current actor and physical store")
		}
		qualification = original
	}
	var raw []byte
	var state string
	var complete, objectComplete *time.Time
	err = a.DB.QueryRow(r.Context(), `SELECT manifest,state,receipt_complete_at,object_complete_at FROM sync.delivery_manifests WHERE scope_id=$1 AND message_id=$2 AND restore_epoch=$3 ORDER BY generation DESC LIMIT 1`, a.Config.DeploymentID, id, u.RestoreEpoch).Scan(&raw, &state, &complete, &objectComplete)
	if err == pgx.ErrNoRows {
		return fail(404, "RECEIPT_TARGET_NOT_FOUND", "No frozen target range exists; no target was created")
	}
	if err != nil {
		return err
	}
	var manifest receiptDeliveryManifest
	if json.Unmarshal(raw, &manifest) != nil || !validReceiptManifest(manifest) {
		return fail(503, "DELIVERY_MANIFEST_CORRUPT", "Original frozen targets cannot be verified")
	}
	member, err := a.member(r.Context(), u.ID, manifest.Conversation)
	if err != nil {
		return err
	}
	generation := stringValue(member, "membershipGeneration")
	audience := false
	for _, person := range manifest.Audience {
		if person.User == u.ID && person.Membership == generation {
			audience = true
		}
	}
	if !audience {
		return fail(403, "RECEIPT_AUDIENCE_CHANGED", "A new membership cannot inherit the old receipt range")
	}
	var target, objectCapable, revoked bool
	err = a.DB.QueryRow(r.Context(), `SELECT true,object_capable,revocation_event_id IS NOT NULL FROM sync.delivery_targets WHERE scope_id=$1 AND message_id=$2 AND generation=$3 AND user_id=$4 AND device_id=$5 AND store_generation=$6 AND qualification_id::text=$7`, a.Config.DeploymentID, id, manifest.Generation, u.ID, u.DeviceID, store.Store, qualification).Scan(&target, &objectCapable, &revoked)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	var contentRecorded, objectRecorded bool
	err = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sync.content_receipts c WHERE c.scope_id=$1 AND c.message_id=$2 AND c.restore_epoch=$3 AND c.content_version=$4 AND c.user_id=$5 AND c.device_id=$6 AND c.store_generation=$7 AND c.qualification_id::text=$8 AND c.result->>'deliveryGeneration'=$9 AND c.result->>'manifestDigest'=$10),EXISTS(SELECT 1 FROM sync.object_receipts o JOIN sync.content_receipts c ON c.scope_id=o.scope_id AND c.source_context_id=o.source_context_id AND c.user_id=o.user_id AND c.device_id=o.device_id AND c.restore_epoch=o.restore_epoch AND c.request_id=o.parent_request_id WHERE o.scope_id=$1 AND o.message_id=$2 AND o.restore_epoch=$3 AND o.content_version=$4 AND o.user_id=$5 AND o.device_id=$6 AND o.store_generation=$7 AND o.qualification_id::text=$8 AND c.result->>'deliveryGeneration'=$9 AND c.result->>'manifestDigest'=$10)`, a.Config.DeploymentID, id, u.RestoreEpoch, manifest.Content, u.ID, u.DeviceID, store.Store, qualification, manifest.Generation, manifest.Digest).Scan(&contentRecorded, &objectRecorded)
	if err != nil {
		return err
	}
	var checkpoint receiptDeliveryCheckpoint
	if err = a.rpc(r.Context(), "message", "/internal/retention/checkpoint", manifest, &checkpoint); err != nil {
		return err
	}
	current := checkpoint.Current && (state == "waiting" || state == "complete") && manifest.Expires.After(time.Now())
	if checkpoint.Sender != u.ID {
		complete = nil
		objectComplete = nil
	}
	qualifiedNow := store.Qualified && qualification == store.ID
	respond(w, 200, map[string]any{"protocol": qualifiedReceiptProtocol, "messageId": id, "contentVersion": manifest.Content, "restoreEpoch": manifest.Epoch, "qualificationId": qualification, "deliveryGeneration": manifest.Generation, "manifestDigest": manifest.Digest, "receiptScope": manifest.Scope, "originalTarget": target, "contentTarget": target && !revoked && qualifiedNow && current, "objectTarget": target && !revoked && objectCapable && qualifiedNow && store.ObjectQualified && current, "contentRecorded": contentRecorded, "objectRecorded": objectRecorded, "contentCompleteAt": complete, "objectCompleteAt": objectComplete, "current": current, "messageCurrent": checkpoint.Current, "deletionEligible": false})
	return nil
}

func (a *App) receiptContextStore(r *http.Request) (User, syncRegistration, storageQualification, error) {
	u, err := a.syncUser(r)
	if err != nil {
		return u, syncRegistration{}, storageQualification{}, err
	}
	reg, err := a.syncRegistered(r.Context(), u, r.URL.Query().Get("schemaVersion"))
	if err != nil {
		return u, reg, storageQualification{}, err
	}
	store := r.Header.Get("X-JobsIM-Store-Generation")
	if !syncValidationID(store) || store != reg.Store {
		return u, reg, storageQualification{}, fail(409, "RECEIPT_STORE_CHANGED", "Receipt negotiation requires the exact current physical store")
	}
	q, err := a.getStorageQualification(r.Context(), u, reg.Store)
	return u, reg, q, err
}

func (a *App) syncReceiptContext(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	u, reg, q, err := a.receiptContextStore(r)
	if err != nil {
		return err
	}
	ready := a.receiptRuntime(r.Context(), u.RestoreEpoch)
	qualification := "transport_only"
	if q.Qualified {
		qualification = "qualified_content"
	}
	if q.ObjectQualified {
		qualification = "qualified_content_and_objects"
	}
	respond(w, 200, map[string]any{"protocol": qualifiedReceiptProtocol, "scopeId": a.Config.DeploymentID, "sourceContextId": "personal", "principalId": u.ID, "deviceId": u.DeviceID, "storeGeneration": reg.Store, "restoreEpoch": u.RestoreEpoch, "schemaVersion": reg.Schema, "qualification": qualification, "storage": q, "qualifiedAck": q.Qualified, "contentReceiptEnabled": q.Qualified, "objectReceiptEnabled": q.ObjectQualified, "afterReceiptEnabled": q.Qualified && ready.Ready, "afterObjectReceiptEnabled": q.ObjectQualified && ready.Ready, "readiness": ready, "defaultRetentionMode": "cloud_history", "deletionEligible": false, "targetRangeComplete": false})
	return nil
}

func (a *App) syncReceiptProfiles(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	_, _, _, err := a.receiptContextStore(r)
	if err != nil {
		return err
	}
	build := r.URL.Query().Get("buildDigest")
	if !syncValidationDigest(build) {
		return fail(400, "INVALID_RECEIPT_BUILD", "The locally measured exact build digest is required")
	}
	profiles := []ReceiptProfile{}
	for _, profile := range a.Config.ReceiptProfiles {
		if profile.BuildDigest == build {
			profiles = append(profiles, profile)
		}
	}
	respond(w, 200, map[string]any{"protocol": qualifiedReceiptProtocol, "profiles": profiles, "qualifiedAck": false, "declarationRequired": true, "localDurabilityVerificationRequired": true})
	return nil
}

func (a *App) receiptLifecycleCapability(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "message" {
		return fail(403, "RECEIPT_OWNER_REQUIRED", "Only the policy owner may inspect its current actor store")
	}
	var q struct {
		User   string `json:"userId"`
		Device string `json:"deviceId"`
		Store  string `json:"storeGeneration"`
		Epoch  string `json:"restoreEpoch"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.User) || !syncValidationID(q.Device) || !syncValidationID(q.Store) || !syncValidationCounter(q.Epoch, false) {
		return fail(400, "INVALID_RECEIPT_STORE", "Exact current actor, device and physical store required")
	}
	u := User{ID: q.User, DeviceID: q.Device, RestoreEpoch: q.Epoch}
	if err := a.receiptPhysicalStore(r.Context(), u, q.Store); err != nil {
		return err
	}
	store, err := a.getStorageQualification(r.Context(), u, q.Store)
	if err != nil {
		return err
	}
	ready := a.receiptRuntime(r.Context(), q.Epoch)
	respond(w, 200, map[string]any{"afterReceiptEnabled": store.Qualified && ready.Ready, "afterObjectReceiptEnabled": store.ObjectQualified && ready.Ready})
	return nil
}

func (a *App) syncStorageResults(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	var q storageDeclaration
	if err = decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validReceiptNonce(q.RequestID) || !syncValidationID(q.StoreGeneration) || !syncValidationCounter(q.RevisionEpoch, false) {
		return fail(400, "INVALID_STORAGE_DECLARATION", "Original declaration and restore epoch required")
	}
	if q.RevisionEpoch != u.RestoreEpoch {
		return fail(409, "RESTORE_EPOCH_CHANGED", "An earlier declaration cannot qualify a restored physical store")
	}
	if err = a.receiptPhysicalStore(r.Context(), u, q.StoreGeneration); err != nil {
		return err
	}
	raw, err := a.readStorageDeclaration(r.Context(), u, q)
	if err != nil {
		return err
	}
	return a.respondStorageDeclaration(w, r, u, q.StoreGeneration, raw)
}

func (a *App) readStorageDeclaration(ctx context.Context, u User, q storageDeclaration) ([]byte, error) {
	var original, raw []byte
	err := a.DB.QueryRow(ctx, `SELECT request_digest,result FROM sync.storage_declarations WHERE scope_id=$1 AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 AND request_id=$5`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.RequestID).Scan(&original, &raw)
	if err == pgx.ErrNoRows {
		return nil, fail(404, "STORAGE_DECLARATION_NOT_FOUND", "No original storage declaration exists; none was created")
	}
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(q)
	if !hmac.Equal(original, keyedDigest(a.Config.Key, "storage-declaration:"+u.ID+":"+u.DeviceID, body)) {
		return nil, fail(409, "IDEMPOTENCY_CONFLICT", "The original physical store declaration differs")
	}
	return raw, nil
}

func (a *App) receiptPhysicalStore(ctx context.Context, u User, store string) error {
	if !syncValidationID(store) {
		return fail(400, "INVALID_RECEIPT_STORE", "An exact physical store is required")
	}
	var current bool
	if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.device_stores d JOIN sync.instance i ON i.singleton AND i.restore_epoch=d.restore_epoch WHERE d.scope_id=$1 AND d.source_context_id='personal' AND d.user_id=$2 AND d.device_id=$3 AND d.restore_epoch=$4 AND d.store_generation=$5 AND i.deployment_id=$1::uuid::text)`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, store).Scan(&current); err != nil {
		return err
	}
	if !current {
		return fail(409, "RECEIPT_STORE_CHANGED", "The original current physical store and restore epoch are required")
	}
	return nil
}
