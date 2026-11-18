package jobsim

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type retentionPolicy struct {
	Mode            string `json:"mode"`
	Delay           *int64 `json:"delaySeconds"`
	Scope           string `json:"receiptScope"`
	MaxAge          int64  `json:"undeliveredMaxAgeSeconds"`
	History         *int64 `json:"historyRetentionSeconds"`
	AttachmentMode  string `json:"attachmentMode"`
	AttachmentAge   int64  `json:"attachmentRetentionSeconds"`
	AttachmentDelay *int64 `json:"attachmentDelaySeconds"`
}

func defaultRetentionPolicy() retentionPolicy {
	return retentionPolicy{Mode: "cloud_history", Scope: "all_target_devices", MaxAge: 2592000, AttachmentMode: "fixed_retention", AttachmentAge: 2592000}
}
func validateRetentionPolicy(p retentionPolicy) error {
	if p.Mode != "cloud_history" && p.Mode != "after_receipt" || p.Scope != "all_target_devices" && p.Scope != "per_user_any_device" || p.MaxAge < 3600 || p.MaxAge > 31536000 || p.AttachmentAge < 3600 || p.AttachmentAge > 31536000 || p.AttachmentMode != "fixed_retention" && p.AttachmentMode != "after_object_receipt" {
		return fail(400, "INVALID_RETENTION_POLICY", "Explicit supported mode, audience rule and bounded independent deadlines required")
	}
	if p.Mode == "after_receipt" && (p.Delay == nil || *p.Delay < 0 || *p.Delay > 604800) || p.Mode == "cloud_history" && p.Delay != nil || p.History != nil && (*p.History < 3600 || *p.History > 31536000) {
		return fail(400, "INVALID_RETENTION_POLICY", "Receipt delay must be explicit (including zero); history deadlines must be bounded")
	}
	if p.AttachmentMode == "after_object_receipt" && (p.AttachmentDelay == nil || *p.AttachmentDelay < 0 || *p.AttachmentDelay > 604800) || p.AttachmentMode == "fixed_retention" && p.AttachmentDelay != nil {
		return fail(400, "INVALID_RETENTION_POLICY", "Object receipt delay must be explicit and independent from body delay")
	}
	return nil
}

func init() {
	migrations["message"] += `CREATE TABLE IF NOT EXISTS message.retention_policies(scope_id uuid NOT NULL,conversation_id uuid NOT NULL,policy_version bigint NOT NULL,id uuid NOT NULL,policy jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,conversation_id,policy_version),UNIQUE(scope_id,id));
CREATE TABLE IF NOT EXISTS message.retention_policy_operations(scope_id uuid NOT NULL,conversation_id uuid NOT NULL,user_id uuid NOT NULL,request_id text NOT NULL,digest text NOT NULL,result jsonb NOT NULL,PRIMARY KEY(scope_id,conversation_id,user_id,request_id));
CREATE TABLE IF NOT EXISTS message.retention_current(scope_id uuid NOT NULL,conversation_id uuid NOT NULL,policy_version bigint NOT NULL,PRIMARY KEY(scope_id,conversation_id));
CREATE TABLE IF NOT EXISTS message.retention_preparations(scope_id uuid NOT NULL,message_id uuid PRIMARY KEY,conversation_id uuid NOT NULL,restore_epoch bigint NOT NULL,policy_version bigint NOT NULL,policy jsonb NOT NULL,manifest jsonb NOT NULL,ready boolean NOT NULL DEFAULT false);
CREATE TABLE IF NOT EXISTS message.retention_assignments(scope_id uuid NOT NULL,message_id uuid PRIMARY KEY REFERENCES message.messages(id),conversation_id uuid NOT NULL,policy_version bigint NOT NULL,policy jsonb NOT NULL,restore_epoch bigint NOT NULL,content_version bigint NOT NULL,generation bigint NOT NULL,manifest jsonb NOT NULL,manifest_digest text NOT NULL,undelivered_deadline timestamptz NOT NULL,receipt_complete_at timestamptz,object_complete_at timestamptz,state text NOT NULL DEFAULT 'waiting',updated_at timestamptz NOT NULL DEFAULT now());
ALTER TABLE message.retention_assignments ADD COLUMN IF NOT EXISTS object_complete_at timestamptz;
CREATE TABLE IF NOT EXISTS message.lifecycle_jobs(scope_id uuid NOT NULL,id uuid PRIMARY KEY,message_id uuid NOT NULL REFERENCES message.messages(id),generation bigint NOT NULL,policy_version bigint NOT NULL,restore_epoch bigint NOT NULL,reason text NOT NULL,delete_scope text NOT NULL CHECK(delete_scope IN('cloud_only','global')),due_at timestamptz NOT NULL,state text NOT NULL DEFAULT 'protection_pending',lease_token uuid,lease_until timestamptz,attempt int NOT NULL DEFAULT 0,before_event uuid NOT NULL,after_event uuid NOT NULL,last_error text,UNIQUE(scope_id,message_id,generation,reason));
CREATE TABLE IF NOT EXISTS message.lifecycle_events(scope_id uuid NOT NULL,event_id uuid PRIMARY KEY,job_id uuid NOT NULL,phase text NOT NULL,payload jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS message.retention_floors(scope_id uuid NOT NULL,message_id uuid NOT NULL,delete_scope text NOT NULL CHECK(delete_scope IN('cloud_only','global')),generation bigint NOT NULL CHECK(generation>0),due_at timestamptz NOT NULL,PRIMARY KEY(scope_id,message_id));
CREATE TABLE IF NOT EXISTS message.lifecycle_clock(singleton boolean PRIMARY KEY CHECK(singleton),last_database_time timestamptz NOT NULL,alarmed boolean NOT NULL DEFAULT false);
CREATE TABLE IF NOT EXISTS message.retention_activation(singleton boolean PRIMARY KEY CHECK(singleton),activated_at timestamptz NOT NULL DEFAULT clock_timestamp());
INSERT INTO message.retention_activation(singleton) VALUES(true) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS message.retention_legacy_objects(scope_id uuid NOT NULL,message_id uuid NOT NULL,object_id uuid NOT NULL,PRIMARY KEY(scope_id,message_id,object_id));

CREATE INDEX IF NOT EXISTS retention_job_due ON message.lifecycle_jobs(state,due_at);
ALTER TABLE message.messages ADD COLUMN IF NOT EXISTS delete_scope text NOT NULL DEFAULT '' CHECK(delete_scope IN('','cloud_only','global'));
ALTER TABLE message.messages ADD COLUMN IF NOT EXISTS deletion_generation bigint NOT NULL DEFAULT 0;`
}

func (a *App) retentionRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	if a.Role != "message" {
		return
	}
	h("GET /api/conversations/{id}/retention", a.retentionPolicyRead)
	h("PUT /api/conversations/{id}/retention", a.retentionPolicyWrite)
	h("POST /internal/retention/checkpoint", internal(a.retentionCheckpoint))
	h("POST /internal/retention/readiness", internal(a.retentionReadiness))
	h("POST /internal/retention/receipt-source", internal(a.retentionReceiptSource))
	h("POST /internal/retention/complete", internal(a.retentionComplete))
	h("POST /internal/retention/attachment-access", internal(a.retentionAttachmentAccess))
	h("POST /internal/retention/media-plan", internal(a.retentionMediaPlan))
}

func (a *App) currentRetentionPolicy(ctx context.Context, conversation string) (retentionPolicy, string, error) {
	p := defaultRetentionPolicy()
	var raw []byte
	var version string
	err := a.DB.QueryRow(ctx, `SELECT p.policy,p.policy_version::text FROM message.retention_current c JOIN message.retention_policies p USING(scope_id,conversation_id,policy_version) WHERE c.scope_id=$1 AND c.conversation_id=$2`, a.Config.DeploymentID, conversation).Scan(&raw, &version)
	if err == pgx.ErrNoRows {
		return p, "1", nil
	}
	if err != nil {
		return p, "", err
	}
	if json.Unmarshal(raw, &p) != nil {
		return p, "", fail(503, "RETENTION_POLICY_CORRUPT", "Original policy is unavailable")
	}
	return p, version, validateRetentionPolicy(p)
}

func (a *App) retentionPolicyRead(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	if _, err = a.member(r.Context(), u.ID, r.PathValue("id")); err != nil {
		return err
	}
	p, version, err := a.currentRetentionPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	respond(w, 200, map[string]any{"policy": p, "version": version, "appliesTo": "new_messages_only", "requiresApprovedNativeStorage": true})
	return nil
}

func (a *App) retentionPolicyWrite(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	meta, err := a.member(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if meta["kind"] == "group" && meta["ownerId"] != u.ID {
		return fail(403, "RETENTION_OWNER_REQUIRED", "Only the current group owner can publish retention")
	}
	var q struct {
		Request  string          `json:"requestId"`
		Expected string          `json:"expectedVersion"`
		Policy   retentionPolicy `json:"policy"`
	}
	if err = decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validReceiptNonce(q.Request) || !syncValidationCounter(q.Expected, true) {
		return fail(400, "INVALID_RETENTION_POLICY", "Original policy operation and expected version required")
	}
	if err = validateRetentionPolicy(q.Policy); err != nil {
		return err
	}
	requestDigest := syncJSONDigest(q)
	var recordedDigest string
	var recorded []byte
	lookup := a.DB.QueryRow(r.Context(), `SELECT digest,result FROM message.retention_policy_operations WHERE scope_id=$1 AND conversation_id=$2 AND user_id=$3 AND request_id=$4`, a.Config.DeploymentID, r.PathValue("id"), u.ID, q.Request).Scan(&recordedDigest, &recorded)
	if lookup == nil {
		if recordedDigest != requestDigest {
			return fail(409, "OPERATION_CONFLICT", "Original policy request cannot be replaced")
		}
		var result any
		if json.Unmarshal(recorded, &result) != nil {
			return fail(503, "RETENTION_POLICY_CORRUPT", "Original result unavailable")
		}
		respond(w, 200, result)
		return nil
	}
	if lookup != pgx.ErrNoRows {
		return lookup
	}
	if q.Policy.Mode == "after_receipt" || q.Policy.AttachmentMode == "after_object_receipt" {
		store := r.Header.Get("X-JobsIM-Store-Generation")
		if !syncValidationID(store) {
			return fail(409, "RETENTION_QUALIFICATION_REQUIRED", "The original current physical store must negotiate qualified receipts")
		}
		var capability struct {
			Body   bool `json:"afterReceiptEnabled"`
			Object bool `json:"afterObjectReceiptEnabled"`
		}
		if err = a.rpc(r.Context(), "sync", "/internal/receipts/lifecycle-capability", map[string]string{"userId": u.ID, "deviceId": u.DeviceID, "storeGeneration": store, "restoreEpoch": u.RestoreEpoch}, &capability); err != nil {
			return err
		}
		if q.Policy.Mode == "after_receipt" && !capability.Body || q.Policy.AttachmentMode == "after_object_receipt" && !capability.Object {
			return fail(409, "RETENTION_QUALIFICATION_REQUIRED", "The current store and live deletion protection do not qualify for this policy")
		}
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, a.Config.DeploymentID+":retention:"+r.PathValue("id")); err != nil {
		return err
	}
	var priorDigest string
	var prior []byte
	priorErr := tx.QueryRow(r.Context(), `SELECT digest,result FROM message.retention_policy_operations WHERE scope_id=$1 AND conversation_id=$2 AND user_id=$3 AND request_id=$4`, a.Config.DeploymentID, r.PathValue("id"), u.ID, q.Request).Scan(&priorDigest, &prior)
	if priorErr == nil {
		if priorDigest != requestDigest {
			return fail(409, "OPERATION_CONFLICT", "Original policy request cannot be replaced")
		}
		var result any
		if json.Unmarshal(prior, &result) != nil {
			return fail(503, "RETENTION_POLICY_CORRUPT", "Original result unavailable")
		}
		respond(w, 200, result)
		return nil
	}
	if priorErr != pgx.ErrNoRows {
		return priorErr
	}
	var version int64
	err = tx.QueryRow(r.Context(), `SELECT policy_version FROM message.retention_current WHERE scope_id=$1 AND conversation_id=$2 FOR UPDATE`, a.Config.DeploymentID, r.PathValue("id")).Scan(&version)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	// The implicit default is version 1 and is never retroactively replaced.
	if version == 0 {
		version = 1
	}
	if q.Expected != str(version) {
		return fail(409, "RETENTION_VERSION_CONFLICT", "The published policy changed")
	}
	raw, _ := json.Marshal(q.Policy)
	version++
	if _, err = tx.Exec(r.Context(), `INSERT INTO message.retention_policies VALUES($1,$2,$3,$4,$5,now())`, a.Config.DeploymentID, r.PathValue("id"), version, newID(), raw); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO message.retention_current VALUES($1,$2,$3) ON CONFLICT(scope_id,conversation_id) DO UPDATE SET policy_version=EXCLUDED.policy_version`, a.Config.DeploymentID, r.PathValue("id"), version); err != nil {
		return err
	}
	response := map[string]any{"policy": q.Policy, "version": str(version), "appliesTo": "new_messages_only"}
	resultBytes, _ := json.Marshal(response)
	if _, err = tx.Exec(r.Context(), `INSERT INTO message.retention_policy_operations VALUES($1,$2,$3,$4,$5,$6)`, a.Config.DeploymentID, r.PathValue("id"), u.ID, q.Request, requestDigest, resultBytes); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"policy": q.Policy, "version": str(version), "appliesTo": "new_messages_only"})
	return nil
}

type retentionPrepared struct {
	Policy        retentionPolicy
	PolicyVersion string
	Manifest      receiptDeliveryManifest
}

func (a *App) prepareMessageRetention(ctx context.Context, u User, id, conversation string) (retentionPrepared, error) {
	var result retentionPrepared
	var err error
	if err = a.retentionSendFloor(ctx, id); err != nil {
		return result, err
	}
	result.Policy, result.PolicyVersion, err = a.currentRetentionPolicy(ctx, conversation)
	if err != nil {
		return result, err
	}
	var audience struct {
		Users []receiptAudienceUser `json:"users"`
	}
	if err = a.rpc(ctx, "conversation", "/internal/receipts/audience", map[string]string{"conversationId": conversation, "userId": u.ID}, &audience); err != nil {
		return result, err
	}
	var now time.Time
	if err = a.DB.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return result, err
	}
	lifetime := result.Policy.MaxAge
	if result.Policy.AttachmentAge > lifetime {
		lifetime = result.Policy.AttachmentAge
	}
	manifest := receiptDeliveryManifest{Message: id, Conversation: conversation, Epoch: u.RestoreEpoch, Content: "1", Generation: "1", Policy: result.PolicyVersion, Scope: result.Policy.Scope, Audience: audience.Users, Targets: []receiptDeliveryTarget{}, FrozenAt: now, Expires: now.Add(time.Duration(lifetime) * time.Second)}
	policyBytes, _ := json.Marshal(result.Policy)
	manifestBytes, _ := json.Marshal(manifest)
	if _, err = a.DB.Exec(ctx, `INSERT INTO message.retention_preparations(scope_id,message_id,conversation_id,restore_epoch,policy_version,policy,manifest) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, id, conversation, u.RestoreEpoch, result.PolicyVersion, policyBytes, manifestBytes); err != nil {
		return result, err
	}
	var ready bool
	var originalPolicy, originalManifest []byte
	if err = a.DB.QueryRow(ctx, `SELECT policy,policy_version::text,manifest,ready FROM message.retention_preparations WHERE scope_id=$1 AND message_id=$2`, a.Config.DeploymentID, id).Scan(&originalPolicy, &result.PolicyVersion, &originalManifest, &ready); err != nil {
		return result, err
	}
	if json.Unmarshal(originalPolicy, &result.Policy) != nil || json.Unmarshal(originalManifest, &manifest) != nil {
		return result, fail(503, "RETENTION_PREPARATION_CORRUPT", "Original policy preparation cannot be recovered")
	}
	if manifest.Conversation != conversation || manifest.Epoch != u.RestoreEpoch || syncJSONDigest(manifest.Audience) != syncJSONDigest(audience.Users) {
		return result, fail(409, "SEND_ADMISSION_CHANGED", "The original frozen delivery audience changed before acceptance")
	}
	if ready {
		result.Manifest = manifest
		return result, nil
	}
	if a.Config.NATSURL != "" {
		if err = a.rpc(ctx, "sync", "/internal/receipts/freeze", manifest, &result.Manifest); err != nil {
			return result, err
		}
	} else {
		manifest.Digest = receiptManifestDigest(manifest)
		result.Manifest = manifest
	}
	if !validReceiptManifest(result.Manifest) {
		return result, fail(503, "DELIVERY_MANIFEST_INVALID", "The owner did not freeze a complete original audience")
	}
	manifestBytes, _ = json.Marshal(result.Manifest)
	if _, err = a.DB.Exec(ctx, `UPDATE message.retention_preparations SET manifest=$3,ready=true WHERE scope_id=$1 AND message_id=$2 AND NOT ready`, a.Config.DeploymentID, id, manifestBytes); err != nil {
		return result, err
	}
	return result, nil
}

func (a *App) persistMessageRetention(ctx context.Context, tx pgx.Tx, id string, prepared retentionPrepared) error {
	p, _ := json.Marshal(prepared.Policy)
	m, _ := json.Marshal(prepared.Manifest)
	bodyDeadline := prepared.Manifest.FrozenAt.Add(time.Duration(prepared.Policy.MaxAge) * time.Second)
	_, err := tx.Exec(ctx, `INSERT INTO message.retention_assignments(scope_id,message_id,conversation_id,policy_version,policy,restore_epoch,content_version,generation,manifest,manifest_digest,undelivered_deadline) VALUES($1,$2,$3,$4,$5,$6,1,1,$7,$8,$9)`, a.Config.DeploymentID, id, prepared.Manifest.Conversation, prepared.PolicyVersion, p, prepared.Manifest.Epoch, m, prepared.Manifest.Digest, bodyDeadline)
	if err != nil {
		return err
	}
	if prepared.Policy.Mode == "after_receipt" {
		if err = a.insertRetentionJob(ctx, tx, id, "1", prepared.PolicyVersion, prepared.Manifest.Epoch, "undelivered_expired", "cloud_only", bodyDeadline); err != nil {
			return err
		}
	}
	if prepared.Policy.History != nil {
		var created time.Time
		if err = tx.QueryRow(ctx, `SELECT created_at FROM message.messages WHERE id=$1`, id).Scan(&created); err != nil {
			return err
		}
		if err = a.insertRetentionJob(ctx, tx, id, "1", prepared.PolicyVersion, prepared.Manifest.Epoch, "history_expired", "cloud_only", created.Add(time.Duration(*prepared.Policy.History)*time.Second)); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) insertRetentionJob(ctx context.Context, tx pgx.Tx, id, generation, policy, epoch, reason, scope string, due time.Time) error {
	key := stableID(a.Config.Key, "retention-job:"+id+":"+generation+":"+reason)
	_, err := tx.Exec(ctx, `INSERT INTO message.lifecycle_jobs(scope_id,id,message_id,generation,policy_version,restore_epoch,reason,delete_scope,due_at,before_event,after_event) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(scope_id,message_id,generation,reason) DO NOTHING`, a.Config.DeploymentID, key, id, generation, policy, epoch, reason, scope, due.UTC(), stableID(a.Config.Key, key+":before"), stableID(a.Config.Key, key+":after"))
	return err
}

func (a *App) retentionCheckpoint(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "sync" {
		return fail(403, "RETENTION_OWNER_REQUIRED", "The durable delivery owner is required")
	}
	var q receiptDeliveryManifest
	if err := decodeDeliveryManifest(r, &q); err != nil {
		return err
	}
	var content, generation, epoch, currentEpoch, digest, attachment, sender string
	var deleted bool
	err := a.DB.QueryRow(r.Context(), `SELECT a.content_version::text,a.generation::text,a.restore_epoch::text,a.manifest_digest,COALESCE(m.attachment_id::text,''),m.deleted OR EXISTS(SELECT 1 FROM message.account_erasure_floors f WHERE f.user_id=m.sender_id) OR EXISTS(SELECT 1 FROM message.lifecycle_jobs j WHERE j.scope_id=a.scope_id AND j.message_id=m.id AND j.delete_scope='global' AND j.due_at<=clock_timestamp()),i.restore_epoch::text,m.sender_id::text FROM message.retention_assignments a JOIN message.messages m ON m.id=a.message_id CROSS JOIN message.instance i WHERE i.singleton AND a.scope_id=$1 AND a.message_id=$2`, a.Config.DeploymentID, q.Message).Scan(&content, &generation, &epoch, &digest, &attachment, &deleted, &currentEpoch, &sender)
	if err == pgx.ErrNoRows {
		respond(w, 200, map[string]any{"current": false, "state": "preparing"})
		return nil
	}
	if err != nil {
		return err
	}
	current := !deleted && epoch == currentEpoch && content == q.Content && generation == q.Generation && epoch == q.Epoch && digest == q.Digest
	state := "current"
	if !current {
		state = "superseded"
		qn, _ := strconv.ParseInt(q.Generation, 10, 64)
		currentN, _ := strconv.ParseInt(generation, 10, 64)
		if !deleted && q.Epoch == epoch && qn > currentN {
			state = "preparing"
		}
	}
	respond(w, 200, map[string]any{"current": current, "state": state, "attachmentId": attachment, "senderId": sender})
	return nil
}

func (a *App) retentionComplete(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "sync" {
		return fail(403, "RETENTION_OWNER_REQUIRED", "Only the frozen range owner can announce completion")
	}
	var q receiptCompletion
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.Message) || !syncValidationCounter(q.Generation, false) || !syncValidationDigest(q.Digest) || q.At.IsZero() || q.At.After(time.Now().Add(time.Second)) || q.Kind != "content" && q.Kind != "object" {
		return fail(400, "INVALID_RECEIPT_COMPLETION", "Original range completion identity and first server time required")
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var raw, manifestRaw []byte
	var policy, epoch, digest, generation string
	var deadline time.Time
	err = tx.QueryRow(r.Context(), `SELECT a.policy,a.policy_version::text,a.restore_epoch::text,a.manifest_digest,a.generation::text,a.undelivered_deadline,a.manifest FROM message.retention_assignments a CROSS JOIN message.instance i WHERE i.singleton AND a.scope_id=$1 AND a.message_id=$2 AND a.restore_epoch=i.restore_epoch FOR UPDATE OF a`, a.Config.DeploymentID, q.Message).Scan(&raw, &policy, &epoch, &digest, &generation, &deadline, &manifestRaw)
	if err != nil {
		return err
	}
	if digest != q.Digest || generation != q.Generation {
		return fail(409, "RECEIPT_GENERATION_CHANGED", "A late completion cannot finish an edited generation")
	}
	var p retentionPolicy
	if json.Unmarshal(raw, &p) != nil {
		return fail(503, "RETENTION_POLICY_CORRUPT", "Original assigned policy unavailable")
	}
	var originalManifest receiptDeliveryManifest
	if json.Unmarshal(manifestRaw, &originalManifest) != nil || !validReceiptManifest(originalManifest) || q.At.Before(originalManifest.FrozenAt) {
		return fail(409, "INVALID_RECEIPT_COMPLETION", "Completion cannot precede the original frozen send")
	}
	if q.Kind == "object" {
		var manifest receiptDeliveryManifest
		if json.Unmarshal(manifestRaw, &manifest) != nil || manifest.FrozenAt.IsZero() {
			return fail(503, "DELIVERY_MANIFEST_CORRUPT", "Original object deadline unavailable")
		}
		deadline = manifest.FrozenAt.Add(time.Duration(p.AttachmentAge) * time.Second)
	}
	if !q.At.Before(deadline) {
		respond(w, 200, map[string]any{"recorded": false, "reason": "expired_undelivered", "generation": q.Generation})
		return nil
	}
	if q.Kind == "content" {
		if _, err = tx.Exec(r.Context(), `UPDATE message.retention_assignments SET receipt_complete_at=COALESCE(receipt_complete_at,$3),state=CASE WHEN state IN('cloud_deleted','global_deleted') THEN state WHEN policy->>'mode'='after_receipt' THEN 'protection_pending' ELSE state END WHERE scope_id=$1 AND message_id=$2`, a.Config.DeploymentID, q.Message, q.At); err != nil {
			return err
		}
		var first time.Time
		if err = tx.QueryRow(r.Context(), `SELECT receipt_complete_at FROM message.retention_assignments WHERE scope_id=$1 AND message_id=$2`, a.Config.DeploymentID, q.Message).Scan(&first); err != nil {
			return err
		}
		if p.Mode == "after_receipt" {
			if err = a.insertRetentionJob(r.Context(), tx, q.Message, generation, policy, epoch, "receipt_complete", "cloud_only", first.Add(time.Duration(*p.Delay)*time.Second)); err != nil {
				return err
			}
			due := first.Add(time.Duration(*p.Delay) * time.Second)
			if _, err = tx.Exec(r.Context(), `UPDATE message.export_holders SET expires_at=LEAST(expires_at,$2) WHERE message_id=$1 AND state IN('prepared','active')`, q.Message, due); err != nil {
				return err
			}

		}
	}
	if q.Kind == "object" {
		if _, err = tx.Exec(r.Context(), `UPDATE message.retention_assignments SET object_complete_at=COALESCE(object_complete_at,$3) WHERE scope_id=$1 AND message_id=$2`, a.Config.DeploymentID, q.Message, q.At); err != nil {
			return err
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	if err = a.protectRetentionDeadlines(r.Context(), q.Message); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"recorded": true, "generation": q.Generation})
	return nil
}

type retentionMediaPlan struct {
	Message          string     `json:"messageId"`
	Object           string     `json:"objectId"`
	Content          string     `json:"contentVersion"`
	Generation       string     `json:"receiptGeneration"`
	Policy           string     `json:"policyVersion"`
	Epoch            string     `json:"restoreEpoch"`
	Digest           string     `json:"manifestDigest"`
	AttachmentMode   string     `json:"attachmentMode"`
	ObjectDeadline   time.Time  `json:"objectDeadline"`
	ObjectCompleteAt *time.Time `json:"objectCompleteAt"`
	ObjectDelay      *int64     `json:"attachmentDelaySeconds"`
	DeleteScope      string     `json:"deleteScope"`
	Legacy           bool       `json:"legacy"`
	ErasureOnly      bool       `json:"erasureOnly"`
	SourceJob        string     `json:"sourceJobId"`
	SourceEvent      string     `json:"sourceEventId"`
	CleanupEligible  bool       `json:"cleanupEligible"`
}

func (a *App) retentionMediaPlan(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "media" {
		return fail(403, "RETENTION_OWNER_REQUIRED", "Only the bound media owner may read its cleanup plan")
	}
	var q struct {
		Message string `json:"messageId"`
		Object  string `json:"objectId"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.Message) || !syncValidationID(q.Object) {
		return fail(400, "INVALID_RETENTION_OBJECT", "Original source binding required")
	}
	var plan retentionMediaPlan
	var raw, manifestRaw []byte
	var deleted bool
	err := a.DB.QueryRow(r.Context(), `SELECT a.message_id::text,m.attachment_id::text,a.content_version::text,a.generation::text,a.policy_version::text,a.restore_epoch::text,a.manifest_digest,a.policy,a.manifest,a.object_complete_at,m.delete_scope,m.deleted FROM message.retention_assignments a JOIN message.messages m ON m.id=a.message_id WHERE a.scope_id=$1 AND a.message_id=$2 AND m.attachment_id=$3`, a.Config.DeploymentID, q.Message, q.Object).Scan(&plan.Message, &plan.Object, &plan.Content, &plan.Generation, &plan.Policy, &plan.Epoch, &plan.Digest, &raw, &manifestRaw, &plan.ObjectCompleteAt, &plan.DeleteScope, &deleted)
	if err == pgx.ErrNoRows {
		return a.retentionLegacyMediaPlan(w, r, q.Message, q.Object)
	}
	if err != nil {
		return fail(409, "RETENTION_PLAN_PENDING", "The original accepted source plan is not yet available")
	}
	var p retentionPolicy
	var manifest receiptDeliveryManifest
	if json.Unmarshal(raw, &p) != nil || json.Unmarshal(manifestRaw, &manifest) != nil || !validReceiptManifest(manifest) {
		return fail(503, "RETENTION_PLAN_CORRUPT", "The frozen object plan cannot be verified")
	}
	plan.CleanupEligible = true
	plan.AttachmentMode, plan.ObjectDelay = p.AttachmentMode, p.AttachmentDelay
	plan.ObjectDeadline = manifest.FrozenAt.Add(time.Duration(p.AttachmentAge) * time.Second)
	if deleted {
		plan.DeleteScope = "global"
	}
	respond(w, 200, plan)
	return nil
}

// This is the common authorization hook for media, exports and future account
// erasure: current membership is mandatory even for an already received target.
func (a *App) retentionAttachmentAccess(w http.ResponseWriter, r *http.Request) error {
	caller := r.Header.Get("X-JobsIM-Caller")
	if caller != "media" && caller != "sync" {
		return fail(403, "RETENTION_OWNER_REQUIRED", "Private media/receipt owner protocol required")
	}
	var q retainedAccessRequest
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.Object) {
		return fail(400, "INVALID_RETAINED_ACCESS", "Original object identity required")
	}
	m, err := a.authorizedRetainedAttachment(r.Context(), q)
	if err != nil {
		return err
	}
	respond(w, 200, map[string]any{"allowed": true, "contentVersion": m.ContentVersion, "attachmentId": m.AttachmentID, "cloudOnly": m.ContentUnavailable})
	return nil
}
