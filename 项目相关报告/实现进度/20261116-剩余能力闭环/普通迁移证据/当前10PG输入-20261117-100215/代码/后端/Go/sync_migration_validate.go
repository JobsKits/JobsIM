package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) migrationReadSource(ctx context.Context, u User, store, id string) (migrationSource, error) {
	var value migrationSource
	err := a.rpc(ctx, "message", "/internal/migrations/source", migrationSourceRequest{Message: id, User: u.ID, Device: u.DeviceID, Store: store, Epoch: u.RestoreEpoch}, &value)
	return value, err
}

func (a *App) migrationReceipt(ctx context.Context, u User, store string, m Message, request, digest, membership string) error {
	var permission, generation string
	var valid bool
	err := a.DB.QueryRow(ctx, `SELECT c.permission_generation,COALESCE(c.result->>'deliveryGeneration',''),EXISTS(SELECT 1 FROM sync.delivery_manifests d CROSS JOIN LATERAL jsonb_array_elements(d.required_users) member WHERE d.scope_id=c.scope_id AND d.message_id=c.message_id AND d.restore_epoch=c.restore_epoch AND d.content_version=c.content_version AND d.generation::text=c.result->>'deliveryGeneration' AND d.manifest_digest=c.result->>'manifestDigest' AND member->>'userId'=c.user_id::text AND member->>'membershipGeneration'=$11) FROM sync.content_receipts c WHERE c.scope_id=$1 AND c.source_context_id='personal' AND c.user_id=$2 AND c.device_id=$3 AND c.restore_epoch=$4 AND c.store_generation=$5 AND c.request_id=$6 AND c.message_id=$7 AND c.conversation_id=$8 AND c.content_version=$9 AND c.body_digest=$10 AND c.result->>'storageQualifiedAtCommit'='true'`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, store, request, m.ID, m.ConversationID, m.ContentVersion, digest, membership).Scan(&permission, &generation, &valid)
	if err == pgx.ErrNoRows {
		return fail(403, "MIGRATION_LOCAL_PROVENANCE_REQUIRED", "The original qualified physical-store receipt is required")
	}
	if err != nil {
		return err
	}
	if generation == "" {
		view, viewError := a.syncCurrentView(ctx, u)
		if viewError != nil {
			return viewError
		}
		valid = permission == view.Permission
	}
	if !valid {
		return fail(403, "MIGRATION_LOCAL_PROVENANCE_REQUIRED", "The original committed body digest and membership generation are required")
	}
	return nil
}

func migrationSameMetadata(original, current Message) bool {
	// These facts survive cloud body erasure; mutable reactions/version do not.
	return original.ID == current.ID && original.ConversationID == current.ConversationID && original.SenderID == current.SenderID && original.Type == current.Type && original.ReplyToID == current.ReplyToID && original.AttachmentID == current.AttachmentID && original.CreatedAt.Equal(current.CreatedAt) && original.ContentVersion == current.ContentVersion && original.CreationEpoch == current.CreationEpoch && original.CreationSequence == current.CreationSequence && original.Forwarded == current.Forwarded
}

func (a *App) migrationPrepareMessages(ctx context.Context, u User, q migrationCreateRequest) ([]migrationHistoryMessage, []migrationMessageReference, error) {
	if len(q.Messages) == 0 || len(q.Messages) > migrationMaxMessages || len(q.Objects) > migrationMaxObjects {
		return nil, nil, fail(400, "MIGRATION_LIMIT_EXCEEDED", "Select one to 256 ordinary history rows and at most 64 original objects")
	}
	rows := make([]migrationHistoryMessage, 0, len(q.Messages))
	refs := make([]migrationMessageReference, 0, len(q.Messages))
	seen := map[string]bool{}
	for _, input := range q.Messages {
		m, err := migrationDecodeEnvelope(input.Envelope)
		if err != nil {
			return nil, nil, err
		}
		if !validMigrationMessage(m, input.ObjectMode) || seen[m.ID] {
			return nil, nil, fail(400, "INVALID_MIGRATION_HISTORY", "Unique complete original ordinary history rows are required")
		}
		seen[m.ID] = true
		source, err := a.migrationReadSource(ctx, u, q.Store, m.ID)
		if err != nil {
			return nil, nil, err
		}
		if source.Classification != "available" && source.Classification != "cloud_only" {
			return nil, nil, fail(409, "MIGRATION_SOURCE_UNKNOWN", "An explicit current lifecycle classification is required")
		}
		if source.Classification == "available" {
			if migrationPrimaryDigest(m, input.ObjectMode) != migrationPrimaryDigest(source.Message, input.ObjectMode) {
				return nil, nil, fail(409, "MIGRATION_SOURCE_CHANGED", "The selected immutable source content changed")
			}
		} else {
			if m.Forwarded || !migrationSameMetadata(m, source.Message) {
				return nil, nil, fail(409, "MIGRATION_LOCAL_PROVENANCE_REQUIRED", "A local-only body must retain the exact original non-reference metadata")
			}
			raw, _ := base64.RawURLEncoding.Strict().DecodeString(input.Envelope)
			if err = a.migrationReceipt(ctx, u, q.Store, m, input.Receipt, syncDigest(raw), source.Membership); err != nil {
				return nil, nil, err
			}
		}
		raw, _ := base64.RawURLEncoding.Strict().DecodeString(input.Envelope)
		refs = append(refs, migrationMessageReference{Message: m.ID, Conversation: m.ConversationID, Content: m.ContentVersion, Membership: source.Membership, Classification: source.Classification, PrimaryDigest: migrationPrimaryDigest(m, input.ObjectMode), Object: m.AttachmentID, ObjectMode: input.ObjectMode, Receipt: input.Receipt, BodyDigest: syncDigest(raw), Deadline: source.Deadline})
		rows = append(rows, migrationNormalize(m, input.ObjectMode))
	}
	return rows, refs, nil
}

func (a *App) migrationObjectReference(ctx context.Context, u User, store string, ref migrationMessageReference, id string) (migrationObjectReference, error) {
	var value migrationObjectReference
	err := a.rpc(ctx, "media", "/internal/migrations/object", struct {
		migrationSourceRequest
		Object  string `json:"objectId"`
		Content string `json:"contentVersion"`
	}{migrationSourceRequest{Message: ref.Message, User: u.ID, Device: u.DeviceID, Store: store, Epoch: u.RestoreEpoch}, id, ref.Content}, &value)
	if err == nil && (value.Object != id || value.Message != ref.Message || !validReceiptObjectSize(value.Size) || !syncValidationDigest(value.Digest) || (value.Classification != "available" && value.Classification != "cloud_only")) {
		err = fail(503, "MIGRATION_OBJECT_UNKNOWN", "Complete original owner descriptor required")
	}
	return value, err
}

func (a *App) migrationObjectReceipt(ctx context.Context, u User, store string, parent migrationMessageReference, obj migrationObjectReference, request string) error {
	if err := a.migrationReceipt(ctx, u, store, Message{ID: parent.Message, ConversationID: parent.Conversation, ContentVersion: parent.Content}, parent.Receipt, parent.BodyDigest, parent.Membership); err != nil {
		return err
	}
	var valid bool
	err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.object_receipts r JOIN sync.content_receipts p ON p.scope_id=r.scope_id AND p.source_context_id=r.source_context_id AND p.user_id=r.user_id AND p.device_id=r.device_id AND p.restore_epoch=r.restore_epoch AND p.request_id=r.parent_request_id WHERE r.scope_id=$1 AND r.source_context_id='personal' AND r.user_id=$2 AND r.device_id=$3 AND r.restore_epoch=$4 AND r.store_generation=$5 AND r.request_id=$6 AND r.message_id=$7 AND r.content_version=$8 AND r.object_id=$9 AND r.size_bytes=$10 AND r.object_digest=$11 AND p.store_generation=r.store_generation AND p.qualification_id=r.qualification_id AND p.body_digest=$12 AND r.parent_request_id=$13 AND r.result->>'objectComplete'='true' AND EXISTS(SELECT 1 FROM sync.storage_declarations d WHERE d.scope_id=r.scope_id AND d.user_id=r.user_id AND d.device_id=r.device_id AND d.restore_epoch=r.restore_epoch AND d.result->>'qualificationId'=r.qualification_id::text AND d.result->>'storeGeneration'=r.store_generation::text AND d.result->>'storageContract'=$14))`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, store, request, parent.Message, parent.Content, obj.Object, obj.Size, obj.Digest, parent.BodyDigest, parent.Receipt, receiptFileContract).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return fail(403, "MIGRATION_OBJECT_PROVENANCE_REQUIRED", "The original whole-file receipt and parent body digest are required")
	}
	return nil
}

func (a *App) migrationPrepareObjects(ctx context.Context, u User, q migrationCreateRequest, refs []migrationMessageReference) ([]migrationPackageObject, []migrationObjectReference, error) {
	wanted := map[string]migrationMessageReference{}
	for _, ref := range refs {
		if ref.ObjectMode == "full" {
			if _, exists := wanted[ref.Object]; exists {
				return nil, nil, fail(400, "MIGRATION_OBJECT_DUPLICATE", "Each selected object must have one original message binding")
			}
			wanted[ref.Object] = ref
		}
	}
	if len(wanted) != len(q.Objects) {
		return nil, nil, fail(400, "MIGRATION_OBJECT_MANIFEST_CHANGED", "Full objects must exactly match the selected complete-file manifest")
	}
	objects := []migrationPackageObject{}
	descriptors := []migrationObjectReference{}
	for _, input := range q.Objects {
		parent, ok := wanted[input.Object]
		if !ok || parent.Message != input.Message {
			return nil, nil, fail(400, "MIGRATION_OBJECT_MANIFEST_CHANGED", "Unknown or duplicate original object")
		}
		delete(wanted, input.Object)
		obj, err := a.migrationObjectReference(ctx, u, q.Store, parent, input.Object)
		if err != nil {
			return nil, nil, err
		}
		if str(int64(len(input.Bytes))) != obj.Size || syncDigest(input.Bytes) != obj.Digest {
			return nil, nil, fail(400, "MIGRATION_OBJECT_INTEGRITY", "Complete original object bytes and digest differ")
		}
		if obj.Classification == "cloud_only" {
			if err = a.migrationObjectReceipt(ctx, u, q.Store, parent, obj, input.Receipt); err != nil {
				return nil, nil, err
			}
		}
		obj.Receipt = input.Receipt
		descriptors = append(descriptors, obj)
		objects = append(objects, migrationPackageObject{input.Object, input.Message, input.Bytes})
	}
	return objects, descriptors, nil
}

func (a *App) migrationValidateRecord(ctx context.Context, rec migrationRecord) error {
	if rec.State != "ready" {
		return fail(409, "MIGRATION_NOT_DELIVERABLE", "This original migration is not ready for delivery")
	}
	if !time.Now().Before(rec.Expires) {
		return fail(410, "MIGRATION_EXPIRED", "The original transfer deadline has passed")
	}
	u := User{ID: rec.User, DeviceID: rec.SourceDevice, RestoreEpoch: rec.Epoch}
	target := User{ID: rec.User, DeviceID: rec.TargetDevice, RestoreEpoch: rec.Epoch}
	for _, tuple := range []struct {
		user  User
		store string
	}{{u, rec.SourceStore}, {target, rec.TargetStore}} {
		if err := a.migrationStoreCurrent(ctx, tuple.user, tuple.store); err != nil {
			return err
		}
		p, err := a.migrationPrincipal(ctx, tuple.user, "", "")
		if err != nil {
			return err
		}
		if p.Security != rec.Security || p.SourceGeneration != rec.SourceGeneration {
			return fail(409, "MIGRATION_PRINCIPAL_CHANGED", "An old family or source generation cannot authorize a package")
		}
	}
	for _, ref := range rec.Manifest.Messages {
		source, err := a.migrationReadSource(ctx, u, rec.SourceStore, ref.Message)
		if err != nil {
			return err
		}
		if source.Membership != ref.Membership || source.Message.ContentVersion != ref.Content || source.Classification != ref.Classification {
			return fail(409, "MIGRATION_SOURCE_CHANGED", "The original membership, content or lifecycle classification changed")
		}
		if source.Classification == "available" {
			if migrationPrimaryDigest(source.Message, ref.ObjectMode) != ref.PrimaryDigest {
				return fail(409, "MIGRATION_SOURCE_CHANGED", "Original content changed before atomic installation")
			}
		} else {
			if err = a.migrationReceipt(ctx, u, rec.SourceStore, source.Message, ref.Receipt, ref.BodyDigest, ref.Membership); err != nil {
				return err
			}
		}
		if source.Deadline != nil && (!source.Deadline.After(time.Now()) || source.Deadline.Before(rec.Expires)) {
			return fail(409, "MIGRATION_SOURCE_DEADLINE_CHANGED", "A new source deadline retires the old transfer")
		}
	}
	for _, obj := range rec.Manifest.Objects {
		var parent migrationMessageReference
		for _, ref := range rec.Manifest.Messages {
			if ref.Message == obj.Message {
				parent = ref
				break
			}
		}
		current, err := a.migrationObjectReference(ctx, u, rec.SourceStore, parent, obj.Object)
		if err != nil {
			return err
		}
		if current.Size != obj.Size || current.Digest != obj.Digest || current.ContentType != obj.ContentType || current.Classification != obj.Classification {
			return fail(409, "MIGRATION_OBJECT_CHANGED", "The selected original object changed")
		}
		if current.Classification == "cloud_only" {
			if err = a.migrationObjectReceipt(ctx, u, rec.SourceStore, parent, current, obj.Receipt); err != nil {
				return err
			}
		}
		if current.Deadline != nil && (!current.Deadline.After(time.Now()) || current.Deadline.Before(rec.Expires)) {
			return fail(409, "MIGRATION_OBJECT_DEADLINE_CHANGED", "A new object deadline retires the old package")
		}
	}
	return nil
}

func migrationManifestBytes(m migrationManifest) ([]byte, string, error) {
	raw, err := json.Marshal(migrationPublicManifest(m))
	if err != nil {
		return nil, "", err
	}
	return raw, syncDigest(raw), nil
}
func migrationDigestMatches(a, b []byte) bool {
	return len(a) == 32 && len(b) == 32 && hmac.Equal(a, b)
}

func migrationPublicManifest(m migrationManifest) migrationManifest {
	m.Messages = append([]migrationMessageReference{}, m.Messages...)
	for i := range m.Messages {
		m.Messages[i].Receipt, m.Messages[i].BodyDigest = "", ""
	}
	m.Objects = append([]migrationObjectReference{}, m.Objects...)
	for i := range m.Objects {
		m.Objects[i].Receipt = ""
	}
	return m
}
