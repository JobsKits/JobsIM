package jobsim

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Frozen delivery membership belongs to the original accepted content. A later
// device registration or rejoin never repairs an old manifest's missing target.
type receiptAudienceUser struct {
	User       string `json:"userId"`
	Membership string `json:"membershipGeneration"`
}

type receiptDeliveryTarget struct {
	User          string `json:"userId"`
	Device        string `json:"deviceId"`
	Store         string `json:"storeGeneration"`
	Qualification string `json:"qualificationId"`
	ObjectCapable bool   `json:"objectCapable"`
}

type receiptDeliveryManifest struct {
	Message      string                  `json:"messageId"`
	Conversation string                  `json:"conversationId"`
	Epoch        string                  `json:"restoreEpoch"`
	Content      string                  `json:"contentVersion"`
	Generation   string                  `json:"generation"`
	Policy       string                  `json:"policyVersion"`
	Scope        string                  `json:"receiptScope"`
	Audience     []receiptAudienceUser   `json:"requiredUsers"`
	Targets      []receiptDeliveryTarget `json:"targets"`
	Digest       string                  `json:"manifestDigest"`
	Expires      time.Time               `json:"expiresAt"`
	FrozenAt     time.Time               `json:"frozenAt"`
	CompleteAt   *time.Time              `json:"receiptCompleteAt,omitempty"`
}

const receiptDeliverySQL = `CREATE TABLE IF NOT EXISTS sync.delivery_manifests(
scope_id uuid NOT NULL,message_id uuid NOT NULL,generation bigint NOT NULL CHECK(generation>0),
conversation_id uuid NOT NULL,restore_epoch bigint NOT NULL CHECK(restore_epoch>0),content_version bigint NOT NULL CHECK(content_version>0),
policy_version bigint NOT NULL CHECK(policy_version>0),receipt_scope text NOT NULL CHECK(receipt_scope IN('all_target_devices','per_user_any_device')),
required_users jsonb NOT NULL,manifest jsonb NOT NULL,manifest_digest text NOT NULL,
state text NOT NULL DEFAULT 'waiting' CHECK(state IN('waiting','complete','superseded','expired')),
receipt_complete_at timestamptz,object_complete_at timestamptz,expires_at timestamptz NOT NULL,last_attempt_at timestamptz,
PRIMARY KEY(scope_id,message_id,generation));
CREATE TABLE IF NOT EXISTS sync.delivery_targets(
scope_id uuid NOT NULL,message_id uuid NOT NULL,generation bigint NOT NULL,user_id uuid NOT NULL,device_id uuid NOT NULL,
store_generation uuid NOT NULL,qualification_id uuid NOT NULL,object_capable boolean NOT NULL,
content_received_at timestamptz,object_received_at timestamptz,revocation_event_id text,
PRIMARY KEY(scope_id,message_id,generation,user_id,device_id),
FOREIGN KEY(scope_id,message_id,generation) REFERENCES sync.delivery_manifests(scope_id,message_id,generation));
CREATE TABLE IF NOT EXISTS sync.delivery_completions(
scope_id uuid NOT NULL,message_id uuid NOT NULL,generation bigint NOT NULL,kind text NOT NULL,
manifest_digest text NOT NULL,completed_at timestamptz NOT NULL,published boolean NOT NULL DEFAULT false,
PRIMARY KEY(scope_id,message_id,generation,kind));
ALTER TABLE sync.delivery_manifests ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz;`

func init() {
	migrations["sync"] += receiptDeliverySQL
}

func migrateReceiptDelivery(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, receiptDeliverySQL)
	return err
}

func receiptManifestDigest(value receiptDeliveryManifest) string {
	value.Digest, value.CompleteAt = "", nil
	return syncJSONDigest(value)
}

func validReceiptManifest(value receiptDeliveryManifest) bool {
	if !syncValidationID(value.Message) || !syncValidationID(value.Conversation) || !syncValidationCounter(value.Epoch, false) || !syncValidationCounter(value.Content, false) || !syncValidationCounter(value.Generation, false) || !syncValidationCounter(value.Policy, false) || value.FrozenAt.IsZero() || !value.Expires.After(value.FrozenAt) || (value.Scope != "all_target_devices" && value.Scope != "per_user_any_device") || len(value.Audience) == 0 || len(value.Audience) > 10000 || len(value.Targets) > 100000 {
		return false
	}
	users, targets := map[string]bool{}, map[string]bool{}
	for _, user := range value.Audience {
		if !syncValidationID(user.User) || !syncValidationCounter(user.Membership, false) || users[user.User] {
			return false
		}
		users[user.User] = true
	}
	for _, target := range value.Targets {
		key := target.User + ":" + target.Device
		if !users[target.User] || !syncValidationID(target.Device) || !syncValidationID(target.Store) || !syncValidationID(target.Qualification) || targets[key] {
			return false
		}
		targets[key] = true
	}
	return value.Digest == receiptManifestDigest(value)
}

// An empty device set never removes the corresponding required human user.
func receiptRangeComplete(users []receiptAudienceUser, targets []receiptDeliveryTarget, scope string, received, revoked map[string]bool, objects bool) bool {
	if len(users) == 0 || (scope != "all_target_devices" && scope != "per_user_any_device") {
		return false
	}
	for _, user := range users {
		active, complete := 0, 0
		for _, target := range targets {
			if target.User != user.User || revoked[target.User+":"+target.Device] {
				continue
			}
			active++
			if received[target.User+":"+target.Device] && (!objects || target.ObjectCapable) {
				complete++
			}
		}
		if active == 0 || complete == 0 || scope == "all_target_devices" && complete != active {
			return false
		}
	}
	return true
}

func (a *App) receiptDeliveryRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	switch a.Role {
	case "conversation":
		h("POST /internal/receipts/audience", internal(a.receiptAudience))
	case "identity":
		h("POST /internal/receipts/devices", internal(a.receiptDevices))
	case "sync":
		h("POST /internal/receipts/freeze", internal(a.freezeReceiptDelivery))
		h("POST /internal/receipts/retained-access", internal(a.receiptRetainedAccess))
	}
}

func (a *App) receiptAudience(w http.ResponseWriter, r *http.Request) error {
	if caller := r.Header.Get("X-JobsIM-Caller"); caller != "message" && caller != "sync" {
		return fail(403, "RECEIPT_OWNER_REQUIRED", "Only the message owner may freeze delivery membership")
	}
	var q struct {
		Conversation string `json:"conversationId"`
		User         string `json:"userId"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.Conversation) {
		return fail(400, "INVALID_RECEIPT_AUDIENCE", "Canonical original conversation required")
	}
	if q.User != "" {
		if _, err := a.member(r.Context(), q.User, q.Conversation); err != nil {
			return err
		}
	}
	rows, err := a.DB.Query(r.Context(), `SELECT user_id::text,membership_generation::text FROM conversation.members WHERE conversation_id=$1 AND state='active' AND export_pending_operation_id IS NULL ORDER BY user_id`, q.Conversation)
	if err != nil {
		return err
	}
	defer rows.Close()
	users := []receiptAudienceUser{}
	for rows.Next() {
		var user receiptAudienceUser
		if err = rows.Scan(&user.User, &user.Membership); err != nil {
			return err
		}
		users = append(users, user)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"users": users})
	return nil
}

type receiptDeviceState struct {
	User       string `json:"userId"`
	Device     string `json:"deviceId"`
	Active     bool   `json:"active"`
	Revocation string `json:"revocationEventId,omitempty"`
}

// This response is derived from durable auth rows, never a client-provided
// removal. Missing rows do not produce a revocation proof.
func (a *App) receiptDevices(w http.ResponseWriter, r *http.Request) error {
	if caller := r.Header.Get("X-JobsIM-Caller"); caller != "sync" && caller != "message" {
		return fail(403, "RECEIPT_OWNER_REQUIRED", "Private delivery qualification protocol required")
	}
	var q struct {
		Users []string `json:"users"`
		Epoch string   `json:"restoreEpoch"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if len(q.Users) == 0 || len(q.Users) > 10000 || !syncValidationCounter(q.Epoch, false) {
		return fail(400, "INVALID_RECEIPT_TARGETS", "A bounded original audience and epoch are required")
	}
	for _, user := range q.Users {
		if !syncValidationID(user) {
			return fail(400, "INVALID_RECEIPT_TARGETS", "Canonical audience required")
		}
	}
	rows, err := a.DB.Query(r.Context(), `SELECT s.user_id::text,s.device_id::text,
(s.revoked_at IS NULL AND s.expires_at>now() AND s.generation=u.credential_generation AND s.security_version=u.security_version AND s.restore_epoch=$2 AND i.restore_epoch=$2 AND s.source_generation=i.personal_source_generation AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL),
COALESCE(s.revoked_at::text,'')||':'||u.security_version::text||':'||u.credential_generation::text||':'||u.state||':'||u.deletion_state
FROM auth.sessions s JOIN auth.users u ON u.id=s.user_id CROSS JOIN auth.instance i WHERE i.singleton AND s.user_id=ANY($1::uuid[]) AND s.family_id IS NOT NULL AND s.source_context_id='personal'`, q.Users, q.Epoch)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []receiptDeviceState{}
	for rows.Next() {
		var value receiptDeviceState
		var cause string
		if err = rows.Scan(&value.User, &value.Device, &value.Active, &cause); err != nil {
			return err
		}
		if !value.Active {
			value.Revocation = syncDigest([]byte(value.User + ":" + value.Device + ":" + cause))
		}
		items = append(items, value)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"items": items})
	return nil
}

func (a *App) freezeReceiptDelivery(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "message" {
		return fail(403, "RECEIPT_OWNER_REQUIRED", "Only the accepted message owner may freeze its targets")
	}
	var manifest receiptDeliveryManifest
	if err := decodeDeliveryManifest(r, &manifest); err != nil {
		return err
	}
	if len(manifest.Targets) != 0 || manifest.Digest != "" || !manifest.Expires.After(time.Now()) {
		return fail(400, "INVALID_DELIVERY_MANIFEST", "Only the original audience may be supplied")
	}
	if manifest.Generation != "1" {
		generation, e := strconv.ParseInt(manifest.Generation, 10, 64)
		if e != nil || generation <= 1 {
			return fail(400, "INVALID_DELIVERY_GENERATION", "Canonical edit generation required")
		}
		var raw []byte
		if e = a.DB.QueryRow(r.Context(), `SELECT manifest FROM sync.delivery_manifests WHERE scope_id=$1 AND message_id=$2 AND generation=$3`, a.Config.DeploymentID, manifest.Message, generation-1).Scan(&raw); e != nil {
			return fail(409, "DELIVERY_PREVIOUS_REQUIRED", "An edit must retain the original target snapshot")
		}
		var previous receiptDeliveryManifest
		if json.Unmarshal(raw, &previous) != nil || !validReceiptManifest(previous) {
			return fail(503, "DELIVERY_MANIFEST_CORRUPT", "Original targets cannot be verified")
		}
		cv, e := strconv.ParseInt(previous.Content, 10, 64)
		if e != nil || cv == 9223372036854775807 || manifest.Content != str(cv+1) || manifest.Conversation != previous.Conversation || manifest.Epoch != previous.Epoch || manifest.Policy != previous.Policy || manifest.Scope != previous.Scope || !manifest.FrozenAt.Equal(previous.FrozenAt) || !manifest.Expires.Equal(previous.Expires) || syncJSONDigest(manifest.Audience) != syncJSONDigest(previous.Audience) {
			return fail(409, "DELIVERY_MANIFEST_CONFLICT", "An edit cannot change the original audience, policy or deadlines")
		}
		manifest.Targets = previous.Targets
	} else {
		users := make([]string, 0, len(manifest.Audience))
		for _, user := range manifest.Audience {
			users = append(users, user.User)
		}
		var devices struct {
			Items []receiptDeviceState `json:"items"`
		}
		if err := a.rpc(r.Context(), "identity", "/internal/receipts/devices", map[string]any{"users": users, "restoreEpoch": manifest.Epoch}, &devices); err != nil {
			return err
		}
		active := map[string]bool{}
		for _, device := range devices.Items {
			if device.Active {
				active[device.User+":"+device.Device] = true
			}
		}
		rows, err := a.DB.Query(r.Context(), `SELECT q.user_id::text,q.device_id::text,q.store_generation::text,q.qualification_id::text,q.profile_id,q.build_digest,q.verification_digest,q.storage_contract FROM sync.storage_qualifications q JOIN sync.device_stores d USING(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation) WHERE q.scope_id=$1 AND q.restore_epoch=$2 AND q.user_id=ANY($3::uuid[]) ORDER BY q.user_id,q.device_id`, a.Config.DeploymentID, manifest.Epoch, users)
		if err != nil {
			return err
		}
		manifest.Targets = []receiptDeliveryTarget{}
		for rows.Next() {
			var target receiptDeliveryTarget
			var profile, build, verification, contract string
			if err = rows.Scan(&target.User, &target.Device, &target.Store, &target.Qualification, &profile, &build, &verification, &contract); err != nil {
				rows.Close()
				return err
			}
			approved, ok := a.receiptProfile(profile, build, verification)
			if ok && approved.StorageContract == contract && active[target.User+":"+target.Device] {
				target.ObjectCapable = contract == receiptFileContract
				manifest.Targets = append(manifest.Targets, target)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	sort.Slice(manifest.Audience, func(i, j int) bool { return manifest.Audience[i].User < manifest.Audience[j].User })
	manifest.Digest = receiptManifestDigest(manifest)
	if !validReceiptManifest(manifest) {
		return fail(400, "INVALID_DELIVERY_MANIFEST", "Exact original content, policy and nonempty human audience required")
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	raw, _ := json.Marshal(manifest)
	audience, _ := json.Marshal(manifest.Audience)
	_, err = tx.Exec(r.Context(), `INSERT INTO sync.delivery_manifests(scope_id,message_id,generation,conversation_id,restore_epoch,content_version,policy_version,receipt_scope,required_users,manifest,manifest_digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, manifest.Message, manifest.Generation, manifest.Conversation, manifest.Epoch, manifest.Content, manifest.Policy, manifest.Scope, audience, raw, manifest.Digest, manifest.Expires)
	if err != nil {
		return err
	}
	var original []byte
	if err = tx.QueryRow(r.Context(), `SELECT manifest FROM sync.delivery_manifests WHERE scope_id=$1 AND message_id=$2 AND generation=$3 FOR UPDATE`, a.Config.DeploymentID, manifest.Message, manifest.Generation).Scan(&original); err != nil {
		return err
	}
	var stored receiptDeliveryManifest
	if json.Unmarshal(original, &stored) != nil || !validReceiptManifest(stored) {
		return fail(503, "DELIVERY_MANIFEST_CORRUPT", "Frozen original manifest cannot be verified")
	}
	// A response-loss retry reuses the first frozen device set. It does not
	// compare newly registered devices or extend the first absolute expiry.
	if stored.Message != manifest.Message || stored.Conversation != manifest.Conversation || stored.Content != manifest.Content || stored.Epoch != manifest.Epoch || stored.Policy != manifest.Policy || stored.Scope != manifest.Scope || !stored.Expires.Equal(manifest.Expires) || !stored.FrozenAt.Equal(manifest.FrozenAt) || syncJSONDigest(stored.Audience) != syncJSONDigest(manifest.Audience) {
		return fail(409, "DELIVERY_MANIFEST_CONFLICT", "Original frozen audience or content differs")
	}
	for _, target := range stored.Targets {
		_, err = tx.Exec(r.Context(), `INSERT INTO sync.delivery_targets(scope_id,message_id,generation,user_id,device_id,store_generation,qualification_id,object_capable) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, stored.Message, stored.Generation, target.User, target.Device, target.Store, target.Qualification, target.ObjectCapable)
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, stored)
	return nil
}

// Media calls through the message owner after its current ACL check. This hook
// survives cloud-only erasure and never authorizes a new device or empty store.
func (a *App) receiptRetainedAccess(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "message" {
		return fail(403, "RECEIPT_OWNER_REQUIRED", "Current message authorization required")
	}
	var q struct {
		Message    string `json:"messageId"`
		User       string `json:"userId"`
		Device     string `json:"deviceId"`
		Store      string `json:"storeGeneration"`
		Epoch      string `json:"restoreEpoch"`
		Content    string `json:"contentVersion"`
		Membership string `json:"membershipGeneration"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !syncValidationID(q.Message) || !syncValidationID(q.User) || !syncValidationID(q.Device) || !syncValidationCounter(q.Epoch, false) || !syncValidationCounter(q.Content, false) {
		return fail(400, "INVALID_RETAINED_ACCESS", "Exact original actor and content tuple required")
	}
	var store, qualification string
	err := a.DB.QueryRow(r.Context(), `SELECT t.store_generation::text,t.qualification_id::text FROM sync.delivery_manifests m JOIN sync.delivery_targets t USING(scope_id,message_id,generation) JOIN sync.device_stores d ON d.scope_id=t.scope_id AND d.source_context_id='personal' AND d.user_id=t.user_id AND d.device_id=t.device_id AND d.restore_epoch=m.restore_epoch AND d.store_generation=t.store_generation WHERE m.scope_id=$1 AND m.message_id=$2 AND m.restore_epoch=$3 AND m.content_version=$4 AND m.state IN('waiting','complete') AND m.expires_at>now() AND t.user_id=$5 AND t.device_id=$6 AND ($7='' OR t.store_generation::text=$7) AND t.content_received_at IS NOT NULL AND t.revocation_event_id IS NULL AND EXISTS(SELECT 1 FROM jsonb_array_elements(m.required_users) u WHERE u->>'userId'=$5::uuid::text AND u->>'membershipGeneration'=$8) ORDER BY m.generation DESC LIMIT 1`, a.Config.DeploymentID, q.Message, q.Epoch, q.Content, q.User, q.Device, q.Store, q.Membership).Scan(&store, &qualification)
	allowed := false
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		current, e := a.getStorageQualification(r.Context(), User{ID: q.User, DeviceID: q.Device, RestoreEpoch: q.Epoch}, store)
		if e != nil {
			return e
		}
		allowed = current.Qualified && current.ID == qualification
	}

	respond(w, 200, map[string]bool{"allowed": allowed})
	return nil
}

func (a *App) receiptDeliveryTuple(ctx context.Context, message, epoch, content string) (string, string, error) {
	var generation, digest string
	err := a.DB.QueryRow(ctx, `SELECT generation::text,manifest_digest FROM sync.delivery_manifests WHERE scope_id=$1 AND message_id=$2 AND restore_epoch=$3 AND content_version=$4 AND state IN('waiting','complete') ORDER BY generation DESC LIMIT 1`, a.Config.DeploymentID, message, epoch, content).Scan(&generation, &digest)
	if err == pgx.ErrNoRows {
		return "", "", nil
	}
	return generation, digest, err
}

func decodeDeliveryManifest(r *http.Request, value *receiptDeliveryManifest) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(400, "INVALID_DELIVERY_MANIFEST", "One bounded complete private manifest required")
	}
	return nil
}
