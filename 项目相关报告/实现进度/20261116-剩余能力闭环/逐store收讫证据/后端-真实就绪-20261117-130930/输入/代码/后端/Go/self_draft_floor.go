package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type draftFloor struct {
	ScopeID         string `json:"scopeId"`
	SourceContextID string `json:"sourceContextId"`
	UserID          string `json:"userId"`
	ConversationID  string `json:"conversationId"`
	ObjectID        string `json:"objectId"`
	Epoch           string `json:"epoch"`
	Version         string `json:"version"`
	ContentDigest   string `json:"contentDigest"`
	Action          string `json:"action"`
	ReplyState      string `json:"replyState"`
	Operation       string `json:"operation"`
}

type draftReplyReference struct {
	SourceContextID      string `json:"sourceContextId"`
	SourceMessageID      string `json:"sourceMessageId"`
	SourceConversationID string `json:"sourceConversationId"`
	SourceRevisionEpoch  string `json:"sourceRevisionEpoch"`
	SourceVersion        string `json:"sourceVersion"`
	MembershipGeneration string `json:"membershipGeneration"`
	SecurityVersion      string `json:"securityVersion"`
	RestoreEpoch         string `json:"restoreEpoch"`
	HolderID             string `json:"holderId"`
}

type draftPending struct {
	Floor           draftFloor
	RequestDigest   string
	OwnedCiphertext []byte
	CipherEpoch     string
	CipherVersion   string
	Reply           *draftReplyReference
	State           string
}

type draftFloorApplyResult struct {
	Floor          draftFloor
	Advanced       bool
	ViewPresent    bool
	ViewChanged    bool
	PendingRetired int64
}

func migrateSelfDraftFloor(ctx context.Context, tx pgx.Tx, scope string) error {
	if !syncValidationID(scope) {
		return fail(400, "INVALID_DRAFT_SCOPE", "Canonical personal draft scope required")
	}
	work, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer work.Rollback(ctx)
	var deployment string
	if err = work.QueryRow(ctx, "SELECT deployment_id FROM sync.instance WHERE singleton FOR SHARE").Scan(&deployment); err != nil {
		return err
	}
	if deployment != scope {
		return fail(409, "DRAFT_SCOPE_CHANGED", "Draft metadata belongs to another deployment")
	}
	_, err = work.Exec(ctx, `ALTER TABLE sync.conversation_views
ADD COLUMN IF NOT EXISTS draft_revision_epoch bigint NOT NULL DEFAULT 1 CHECK(draft_revision_epoch>0),
ADD COLUMN IF NOT EXISTS draft_version bigint NOT NULL DEFAULT 0 CHECK(draft_version>=0),
ADD COLUMN IF NOT EXISTS draft_operation_id text,
ADD COLUMN IF NOT EXISTS draft_content_state text NOT NULL DEFAULT 'empty' CHECK(draft_content_state IN('empty','owned','unavailable')),
ADD COLUMN IF NOT EXISTS draft_owned_ciphertext bytea,
ADD COLUMN IF NOT EXISTS draft_owned_digest bytea CHECK(draft_owned_digest IS NULL OR octet_length(draft_owned_digest)=32),
ADD COLUMN IF NOT EXISTS draft_cipher_epoch bigint CHECK(draft_cipher_epoch>0),
ADD COLUMN IF NOT EXISTS draft_cipher_version bigint CHECK(draft_cipher_version>0),
ADD COLUMN IF NOT EXISTS draft_reply_reference jsonb CHECK(draft_reply_reference IS NULL OR(jsonb_typeof(draft_reply_reference)='object' AND(draft_reply_reference-ARRAY['sourceContextId','sourceMessageId','sourceConversationId','sourceRevisionEpoch','sourceVersion','membershipGeneration','securityVersion','restoreEpoch','holderId']::text[])='{}'::jsonb)),
ADD COLUMN IF NOT EXISTS draft_reply_state text NOT NULL DEFAULT 'none' CHECK(draft_reply_state IN('none','available','unavailable'));
CREATE TABLE IF NOT EXISTS sync.draft_floors(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,conversation_id uuid NOT NULL,object_id uuid NOT NULL UNIQUE,
epoch bigint NOT NULL CHECK(epoch>0),version bigint NOT NULL CHECK(version>0),content_digest bytea NOT NULL CHECK(octet_length(content_digest)=32),
action text NOT NULL CHECK(action IN('set','clear','reply_unavailable')),reply_state text NOT NULL CHECK(reply_state IN('none','available','unavailable')),
operation_id text NOT NULL CHECK(octet_length(operation_id) BETWEEN 8 AND 200),updated_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,source_context_id,user_id,conversation_id));
CREATE TABLE IF NOT EXISTS sync.draft_pending(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,conversation_id uuid NOT NULL,object_id uuid NOT NULL,
operation_id text NOT NULL CHECK(octet_length(operation_id) BETWEEN 8 AND 200),draft_epoch bigint NOT NULL CHECK(draft_epoch>0),draft_version bigint NOT NULL CHECK(draft_version>0),
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),content_digest bytea NOT NULL CHECK(octet_length(content_digest)=32),
action text NOT NULL CHECK(action IN('set','clear','reply_unavailable')),reply_state text NOT NULL CHECK(reply_state IN('none','available','unavailable')),
owned_ciphertext bytea,cipher_epoch bigint CHECK(cipher_epoch>0),cipher_version bigint CHECK(cipher_version>0),
reply_reference jsonb CHECK(reply_reference IS NULL OR(jsonb_typeof(reply_reference)='object' AND(reply_reference-ARRAY['sourceContextId','sourceMessageId','sourceConversationId','sourceRevisionEpoch','sourceVersion','membershipGeneration','securityVersion','restoreEpoch','holderId']::text[])='{}'::jsonb)),
state text NOT NULL DEFAULT 'prepared' CHECK(state IN('prepared','applied','retired')),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
CHECK((owned_ciphertext IS NULL AND cipher_epoch IS NULL AND cipher_version IS NULL) OR(owned_ciphertext IS NOT NULL AND cipher_epoch IS NOT NULL AND cipher_version IS NOT NULL)),
PRIMARY KEY(scope_id,source_context_id,user_id,conversation_id,operation_id));`)
	if err != nil {
		return err
	}
	return work.Commit(ctx)
}

func validDraftIdentity(identity draftFloor) bool {
	return identity.SourceContextID == "personal" && syncValidationID(identity.ScopeID) && syncValidationID(identity.UserID) && syncValidationID(identity.ConversationID) && syncValidationID(identity.ObjectID)
}

func draftDigestBytes(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 || strings.ToLower(value) != value {
		return nil, fail(400, "INVALID_DRAFT_DIGEST", "Canonical 32-byte owned-content HMAC required")
	}
	return decoded, nil
}

func validateDraftFloor(floor draftFloor) error {
	if !validDraftIdentity(floor) || !syncValidationCounter(floor.Epoch, false) || !syncValidationCounter(floor.Version, false) || len(floor.Operation) < 8 || len(floor.Operation) > 200 || strings.ContainsAny(floor.Operation, "\r\n\x00") {
		return fail(400, "INVALID_DRAFT_FLOOR", "Original personal draft identity and exact epoch/version required")
	}
	if _, err := draftDigestBytes(floor.ContentDigest); err != nil {
		return err
	}
	if floor.ReplyState != "none" && floor.ReplyState != "available" && floor.ReplyState != "unavailable" {
		return fail(400, "INVALID_DRAFT_FLOOR", "Typed reply state required")
	}
	switch floor.Action {
	case "set":
	case "clear":
		if floor.ReplyState != "none" {
			return fail(400, "INVALID_DRAFT_FLOOR", "Cleared draft cannot retain a reply grant")
		}
	case "reply_unavailable":
		if floor.ReplyState != "unavailable" {
			return fail(400, "INVALID_DRAFT_FLOOR", "Reply invalidation must retain a denial")
		}
	default:
		return fail(400, "INVALID_DRAFT_FLOOR", "Unsupported draft floor action")
	}
	return nil
}

func compareDraftFloorTuple(first, second draftFloor) (int, error) {
	for _, floor := range []draftFloor{first, second} {
		if !syncValidationCounter(floor.Epoch, false) || !syncValidationCounter(floor.Version, false) {
			return 0, fail(400, "INVALID_DRAFT_FLOOR", "Exact positive draft tuples required")
		}
	}
	firstEpoch, _ := strconv.ParseInt(first.Epoch, 10, 64)
	secondEpoch, _ := strconv.ParseInt(second.Epoch, 10, 64)
	firstVersion, _ := strconv.ParseInt(first.Version, 10, 64)
	secondVersion, _ := strconv.ParseInt(second.Version, 10, 64)
	if firstEpoch < secondEpoch || firstEpoch == secondEpoch && firstVersion < secondVersion {
		return -1, nil
	}
	if firstEpoch == secondEpoch && firstVersion == secondVersion {
		return 0, nil
	}
	return 1, nil
}

func draftFloorRank(action string) int {
	if action == "clear" {
		return 2
	}
	if action == "reply_unavailable" {
		return 1
	}
	return 0
}

// Equal-tuple denial wins over an old positive set, but an unrelated original
// operation at the same strength cannot rewrite the recorded nonce identity.
func chooseDraftFloor(stored, incoming draftFloor) (draftFloor, bool, error) {
	if stored.ScopeID != incoming.ScopeID || stored.SourceContextID != incoming.SourceContextID || stored.UserID != incoming.UserID || stored.ConversationID != incoming.ConversationID || stored.ObjectID != incoming.ObjectID {
		return stored, false, fail(409, "DRAFT_FLOOR_CONFLICT", "Original draft identity differs")
	}
	comparison, err := compareDraftFloorTuple(stored, incoming)
	if err != nil {
		return stored, false, err
	}
	if comparison < 0 || comparison == 0 && draftFloorRank(incoming.Action) > draftFloorRank(stored.Action) {
		return incoming, true, nil
	}
	if comparison > 0 || draftFloorRank(incoming.Action) < draftFloorRank(stored.Action) {
		return stored, false, nil
	}
	if stored != incoming {
		return stored, false, fail(409, "DRAFT_FLOOR_CONFLICT", "Original draft floor metadata differs")
	}
	return stored, false, nil
}

func draftOwnedAAD(identity draftFloor, cipherEpoch, cipherVersion string) (string, error) {
	if !validDraftIdentity(identity) || !syncValidationCounter(cipherEpoch, false) || !syncValidationCounter(cipherVersion, false) {
		return "", fail(400, "INVALID_DRAFT_CIPHER_BINDING", "Original cipher epoch/version and private identity required")
	}
	return strings.Join([]string{"self-draft-owned-v1", identity.ScopeID, identity.SourceContextID, identity.UserID, identity.ConversationID, identity.ObjectID, cipherEpoch, cipherVersion}, "\x00"), nil
}

func (a *App) draftOwnedDigest(identity draftFloor, ownedText string) (string, error) {
	if !validDraftIdentity(identity) {
		return "", fail(400, "INVALID_DRAFT_SCOPE", "Private owned-text identity required")
	}
	identityKey := strings.Join([]string{identity.ScopeID, identity.SourceContextID, identity.UserID, identity.ConversationID, identity.ObjectID}, ":")
	return hex.EncodeToString(keyedDigest(a.Config.Key, "self-draft-owned-digest-v1:"+identityKey, []byte(ownedText))), nil
}

func (a *App) sealDraftOwnedText(identity draftFloor, cipherEpoch, cipherVersion, ownedText string) ([]byte, string, error) {
	aad, err := draftOwnedAAD(identity, cipherEpoch, cipherVersion)
	if err != nil {
		return nil, "", err
	}
	digest, err := a.draftOwnedDigest(identity, ownedText)
	if err != nil {
		return nil, "", err
	}
	sealed, err := a.seal(ownedText, aad)
	return sealed, digest, err
}

func (a *App) openDraftOwnedText(identity draftFloor, cipherEpoch, cipherVersion string, ciphertext []byte, digest string) (string, error) {
	aad, err := draftOwnedAAD(identity, cipherEpoch, cipherVersion)
	if err != nil {
		return "", err
	}
	expected, err := draftDigestBytes(digest)
	if err != nil {
		return "", err
	}
	text, err := a.unseal(ciphertext, aad)
	if err != nil {
		return "", fail(409, "DRAFT_CONTENT_UNAVAILABLE", "Original owned-content cipher cannot be verified")
	}
	actual, err := a.draftOwnedDigest(identity, text)
	if err != nil {
		return "", err
	}
	decoded, _ := hex.DecodeString(actual)
	if !hmac.Equal(expected, decoded) {
		return "", fail(409, "DRAFT_CONTENT_UNAVAILABLE", "Original owned-content digest differs")
	}
	return text, nil
}

// Every floor/pending helper takes this context lock before a view row lock.
// Callers must commit before journal/message RPCs and never reverse that order.
func lockDraftContextTx(ctx context.Context, tx pgx.Tx, identity draftFloor) error {
	if !validDraftIdentity(identity) {
		return fail(400, "INVALID_DRAFT_SCOPE", "Private draft identity required")
	}
	var deployment string
	if err := tx.QueryRow(ctx, "SELECT deployment_id FROM sync.instance WHERE singleton FOR SHARE").Scan(&deployment); err != nil {
		return err
	}
	if deployment != identity.ScopeID {
		return fail(409, "DRAFT_SCOPE_CHANGED", "Draft owner deployment differs")
	}
	key := strings.Join([]string{"self-draft", identity.ScopeID, identity.SourceContextID, identity.UserID, identity.ConversationID}, ":")
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", key)
	return err
}

func loadDraftFloorTx(ctx context.Context, tx pgx.Tx, identity draftFloor) (draftFloor, bool, error) {
	floor := identity
	var digest []byte
	err := tx.QueryRow(ctx, `SELECT object_id::text,epoch::text,version::text,content_digest,action,reply_state,operation_id FROM sync.draft_floors WHERE scope_id=$1 AND source_context_id=$2 AND user_id=$3 AND conversation_id=$4 FOR UPDATE`, identity.ScopeID, identity.SourceContextID, identity.UserID, identity.ConversationID).Scan(&floor.ObjectID, &floor.Epoch, &floor.Version, &digest, &floor.Action, &floor.ReplyState, &floor.Operation)
	if err == pgx.ErrNoRows {
		return floor, false, nil
	}
	floor.ContentDigest = hex.EncodeToString(digest)
	return floor, err == nil, err
}

type draftStoredContent struct {
	Epoch, Version             string
	Operation                  string
	ContentState               string
	Digest                     []byte
	Ciphertext                 []byte
	CipherEpoch, CipherVersion *int64
	ReplyState                 string
}

type draftFloorPlan struct {
	Apply        bool
	Epoch        string
	Version      string
	KeepCipher   bool
	KeepReply    bool
	ContentState string
	ReplyState   string
}

func planDraftFloor(floor draftFloor, stored draftStoredContent) (draftFloorPlan, error) {
	plan := draftFloorPlan{Apply: true, Epoch: floor.Epoch, Version: floor.Version, ContentState: "unavailable", ReplyState: "none"}
	if err := validateDraftFloor(floor); err != nil {
		return plan, err
	}
	if !syncValidationCounter(stored.Epoch, false) || !syncValidationCounter(stored.Version, true) {
		return plan, fail(409, "DRAFT_CONTENT_UNAVAILABLE", "Stored draft logical tuple is invalid")
	}
	storedEpoch, _ := strconv.ParseInt(stored.Epoch, 10, 64)
	storedVersion, _ := strconv.ParseInt(stored.Version, 10, 64)
	floorEpoch, _ := strconv.ParseInt(floor.Epoch, 10, 64)
	floorVersion, _ := strconv.ParseInt(floor.Version, 10, 64)
	logical := 0
	if storedEpoch < floorEpoch || storedEpoch == floorEpoch && storedVersion < floorVersion {
		logical = -1
	} else if storedEpoch > floorEpoch || storedVersion > floorVersion {
		logical = 1
	}
	if logical > 0 && stored.Version != "0" {
		plan.Apply = false
		return plan, nil
	}
	if logical > 0 {
		plan.Epoch, plan.Version = stored.Epoch, stored.Version
		if stored.CipherEpoch != nil && stored.CipherVersion != nil {
			comparison, err := compareDraftFloorTuple(draftFloor{Epoch: str(*stored.CipherEpoch), Version: str(*stored.CipherVersion)}, floor)
			if err != nil {
				return plan, err
			}
			if comparison > 0 {
				plan.Apply = false
				return plan, nil
			}
		}
	}
	if floor.Action == "clear" {
		plan.ContentState = "empty"
		return plan, nil
	}
	expected, err := draftDigestBytes(floor.ContentDigest)
	if err != nil {
		return plan, err
	}
	plan.KeepCipher = stored.ContentState == "owned" && len(stored.Ciphertext) != 0 && stored.CipherEpoch != nil && stored.CipherVersion != nil && hmac.Equal(expected, stored.Digest)
	if plan.KeepCipher {
		plan.ContentState = "owned"
	}
	if floor.ReplyState != "none" {
		plan.ReplyState = "unavailable"
	}
	if floor.Action == "set" && logical == 0 && stored.Operation == floor.Operation && plan.KeepCipher {
		plan.KeepReply, plan.ReplyState = true, stored.ReplyState
	}
	return plan, nil
}

// This is a denial application, not a positive body/holder commit. It never
// creates a missing conversation view or reconstructs a reply grant from a floor.
func applyDraftFloorTx(ctx context.Context, tx pgx.Tx, incoming draftFloor) (draftFloorApplyResult, error) {
	result := draftFloorApplyResult{Floor: incoming}
	if err := validateDraftFloor(incoming); err != nil {
		return result, err
	}
	if err := lockDraftContextTx(ctx, tx, incoming); err != nil {
		return result, err
	}
	stored, found, err := loadDraftFloorTx(ctx, tx, incoming)
	if err != nil {
		return result, err
	}
	winner, advanced := incoming, true
	if found {
		winner, advanced, err = chooseDraftFloor(stored, incoming)
		if err != nil {
			return result, err
		}
	}
	digest, _ := draftDigestBytes(winner.ContentDigest)
	if advanced {
		_, err = tx.Exec(ctx, `INSERT INTO sync.draft_floors(scope_id,source_context_id,user_id,conversation_id,object_id,epoch,version,content_digest,action,reply_state,operation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(scope_id,source_context_id,user_id,conversation_id) DO UPDATE SET object_id=EXCLUDED.object_id,epoch=EXCLUDED.epoch,version=EXCLUDED.version,content_digest=EXCLUDED.content_digest,action=EXCLUDED.action,reply_state=EXCLUDED.reply_state,operation_id=EXCLUDED.operation_id,updated_at=now()`, winner.ScopeID, winner.SourceContextID, winner.UserID, winner.ConversationID, winner.ObjectID, winner.Epoch, winner.Version, digest, winner.Action, winner.ReplyState, winner.Operation)
		if err != nil {
			return result, err
		}
	}
	result.Floor, result.Advanced = winner, advanced
	var content draftStoredContent
	var object string
	err = tx.QueryRow(ctx, `SELECT id::text,draft_revision_epoch::text,draft_version::text,COALESCE(draft_operation_id,''),draft_content_state,draft_owned_digest,draft_owned_ciphertext,draft_cipher_epoch,draft_cipher_version,draft_reply_state FROM sync.conversation_views WHERE scope_id=$1 AND source_context_id=$2 AND user_id=$3 AND conversation_id=$4 FOR UPDATE`, winner.ScopeID, winner.SourceContextID, winner.UserID, winner.ConversationID).Scan(&object, &content.Epoch, &content.Version, &content.Operation, &content.ContentState, &content.Digest, &content.Ciphertext, &content.CipherEpoch, &content.CipherVersion, &content.ReplyState)
	if err != nil && err != pgx.ErrNoRows {
		return result, err
	}
	if err == nil {
		result.ViewPresent = true
		if object != winner.ObjectID {
			return result, fail(409, "DRAFT_FLOOR_CONFLICT", "Draft object differs from its private view identity")
		}
		plan, err := planDraftFloor(winner, content)
		if err != nil {
			return result, err
		}
		if plan.Apply {
			_, err = tx.Exec(ctx, `UPDATE sync.conversation_views SET draft_revision_epoch=$5,draft_version=$6,draft_operation_id=$7,draft_content_state=$8,draft_owned_digest=$9,
draft_owned_ciphertext=CASE WHEN $10 THEN draft_owned_ciphertext ELSE NULL END,draft_cipher_epoch=CASE WHEN $10 THEN draft_cipher_epoch ELSE NULL END,draft_cipher_version=CASE WHEN $10 THEN draft_cipher_version ELSE NULL END,
draft_reply_reference=CASE WHEN $11 THEN draft_reply_reference ELSE NULL END,draft_reply_state=$12,updated_at=now()
WHERE scope_id=$1 AND source_context_id=$2 AND user_id=$3 AND conversation_id=$4`, winner.ScopeID, winner.SourceContextID, winner.UserID, winner.ConversationID, plan.Epoch, plan.Version, winner.Operation, plan.ContentState, digest, plan.KeepCipher, plan.KeepReply, plan.ReplyState)
			if err != nil {
				return result, err
			}
			result.ViewChanged = true
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE sync.draft_pending SET state='retired',
owned_ciphertext=CASE WHEN $11 THEN owned_ciphertext ELSE NULL END,cipher_epoch=CASE WHEN $11 THEN cipher_epoch ELSE NULL END,cipher_version=CASE WHEN $11 THEN cipher_version ELSE NULL END,
reply_reference=NULL,updated_at=now()
WHERE scope_id=$1 AND source_context_id=$2 AND user_id=$3 AND conversation_id=$4 AND(draft_epoch,draft_version)<=($5::bigint,$6::bigint)
AND NOT(draft_epoch=$5 AND draft_version=$6 AND operation_id=$7 AND content_digest=$8 AND action=$9 AND reply_state=$10 AND state='prepared')`, winner.ScopeID, winner.SourceContextID, winner.UserID, winner.ConversationID, winner.Epoch, winner.Version, winner.Operation, digest, winner.Action, winner.ReplyState, winner.Action == "reply_unavailable")
	if err != nil {
		return result, err
	}
	result.PendingRetired = tag.RowsAffected()
	return result, nil
}

func validateDraftReply(reference *draftReplyReference) error {
	if reference == nil {
		return nil
	}
	if reference.SourceContextID != "personal" || !syncValidationID(reference.SourceMessageID) || !syncValidationID(reference.SourceConversationID) || !syncValidationID(reference.HolderID) {
		return fail(400, "INVALID_DRAFT_REPLY", "Typed original message/holder identities required")
	}
	for _, value := range []string{reference.SourceRevisionEpoch, reference.SourceVersion, reference.MembershipGeneration, reference.SecurityVersion, reference.RestoreEpoch} {
		if !syncValidationCounter(value, false) {
			return fail(400, "INVALID_DRAFT_REPLY", "Exact original message qualification strings required")
		}
	}
	return nil
}

func loadDraftPendingTx(ctx context.Context, tx pgx.Tx, identity draftFloor) (draftPending, bool, error) {
	pending := draftPending{Floor: identity}
	var contentDigest, requestDigest, reply []byte
	var cipherEpoch, cipherVersion *int64
	err := tx.QueryRow(ctx, `SELECT object_id::text,draft_epoch::text,draft_version::text,request_digest,content_digest,action,reply_state,owned_ciphertext,cipher_epoch,cipher_version,reply_reference,state
FROM sync.draft_pending WHERE scope_id=$1 AND source_context_id=$2 AND user_id=$3 AND conversation_id=$4 AND operation_id=$5 FOR UPDATE`, identity.ScopeID, identity.SourceContextID, identity.UserID, identity.ConversationID, identity.Operation).Scan(&pending.Floor.ObjectID, &pending.Floor.Epoch, &pending.Floor.Version, &requestDigest, &contentDigest, &pending.Floor.Action, &pending.Floor.ReplyState, &pending.OwnedCiphertext, &cipherEpoch, &cipherVersion, &reply, &pending.State)
	if err == pgx.ErrNoRows {
		return pending, false, nil
	}
	if err != nil {
		return pending, false, err
	}
	pending.RequestDigest, pending.Floor.ContentDigest = hex.EncodeToString(requestDigest), hex.EncodeToString(contentDigest)
	if cipherEpoch != nil && cipherVersion != nil {
		pending.CipherEpoch, pending.CipherVersion = str(*cipherEpoch), str(*cipherVersion)
	}
	if len(reply) != 0 && string(reply) != "null" {
		pending.Reply = &draftReplyReference{}
		if err = json.Unmarshal(reply, pending.Reply); err != nil {
			return pending, false, err
		}
	}
	return pending, true, nil
}

// A missing floor is unresolved authority, not evidence that an old stage may
// publish. Unknown stages keep their ciphertext until the original fact resolves.
func draftPendingAllowedTx(ctx context.Context, tx pgx.Tx, identity draftFloor) (bool, error) {
	if err := validateDraftFloor(identity); err != nil {
		return false, err
	}
	if err := lockDraftContextTx(ctx, tx, identity); err != nil {
		return false, err
	}
	var currentEpoch string
	if err := tx.QueryRow(ctx, "SELECT restore_epoch::text FROM sync.instance WHERE singleton").Scan(&currentEpoch); err != nil {
		return false, err
	}
	if currentEpoch != identity.Epoch {
		return false, nil
	}
	pending, found, err := loadDraftPendingTx(ctx, tx, identity)
	if err != nil || !found || pending.State != "prepared" {
		return false, err
	}
	floor, known, err := loadDraftFloorTx(ctx, tx, identity)
	if err != nil || !known {
		return false, err
	}
	return floor == pending.Floor && floor == identity, nil
}

func stageDraftPendingTx(ctx context.Context, tx pgx.Tx, proposed draftPending) (draftPending, error) {
	if len(proposed.OwnedCiphertext) == 0 {
		proposed.OwnedCiphertext = nil
	}
	if err := validateDraftFloor(proposed.Floor); err != nil {
		return proposed, err
	}
	requestDigest, err := draftDigestBytes(proposed.RequestDigest)
	if err != nil {
		return proposed, err
	}
	if err = validateDraftReply(proposed.Reply); err != nil {
		return proposed, err
	}
	if proposed.Floor.Action == "clear" && (len(proposed.OwnedCiphertext) != 0 || proposed.Reply != nil) || proposed.Floor.ReplyState == "available" && proposed.Reply == nil {
		return proposed, fail(400, "INVALID_DRAFT_STAGE", "Typed clear/reply state does not match the original stage")
	}
	if len(proposed.OwnedCiphertext) > 0 {
		if _, err = draftOwnedAAD(proposed.Floor, proposed.CipherEpoch, proposed.CipherVersion); err != nil {
			return proposed, err
		}
	} else if proposed.CipherEpoch != "" || proposed.CipherVersion != "" {
		return proposed, fail(400, "INVALID_DRAFT_STAGE", "Absent ciphertext cannot retain a cipher binding")
	}
	if err = lockDraftContextTx(ctx, tx, proposed.Floor); err != nil {
		return proposed, err
	}
	original, found, err := loadDraftPendingTx(ctx, tx, proposed.Floor)
	if err != nil {
		return proposed, err
	}
	if found {
		oldDigest, _ := draftDigestBytes(original.RequestDigest)
		oldReply, _ := json.Marshal(original.Reply)
		newReply, _ := json.Marshal(proposed.Reply)
		if original.Floor != proposed.Floor || !hmac.Equal(oldDigest, requestDigest) || original.State == "prepared" && string(oldReply) != string(newReply) {
			return original, fail(409, "DRAFT_OPERATION_CONFLICT", "Original draft stage metadata differs")
		}
		return original, nil
	}
	var currentEpoch string
	if err = tx.QueryRow(ctx, "SELECT restore_epoch::text FROM sync.instance WHERE singleton").Scan(&currentEpoch); err != nil {
		return proposed, err
	}
	if currentEpoch != proposed.Floor.Epoch {
		return proposed, fail(409, "RESTORE_EPOCH_CHANGED", "An old draft stage cannot be newly created after restore")
	}
	floor, known, err := loadDraftFloorTx(ctx, tx, proposed.Floor)
	if err != nil {
		return proposed, err
	}
	proposed.State = "prepared"
	if known {
		winner, _, err := chooseDraftFloor(floor, proposed.Floor)
		if err != nil {
			return proposed, err
		}
		if winner != proposed.Floor {
			proposed.State, proposed.Reply = "retired", nil
			if winner.Action != "reply_unavailable" {
				proposed.OwnedCiphertext = nil
				proposed.CipherEpoch, proposed.CipherVersion = "", ""
			}
		}
	}
	var cipherEpoch, cipherVersion any
	if len(proposed.OwnedCiphertext) > 0 {
		cipherEpoch, cipherVersion = proposed.CipherEpoch, proposed.CipherVersion
	}
	var reply any
	if proposed.Reply != nil {
		reply, _ = json.Marshal(proposed.Reply)
	}
	contentDigest, _ := draftDigestBytes(proposed.Floor.ContentDigest)
	_, err = tx.Exec(ctx, `INSERT INTO sync.draft_pending(scope_id,source_context_id,user_id,conversation_id,object_id,operation_id,draft_epoch,draft_version,request_digest,content_digest,action,reply_state,owned_ciphertext,cipher_epoch,cipher_version,reply_reference,state)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, proposed.Floor.ScopeID, proposed.Floor.SourceContextID, proposed.Floor.UserID, proposed.Floor.ConversationID, proposed.Floor.ObjectID, proposed.Floor.Operation, proposed.Floor.Epoch, proposed.Floor.Version, requestDigest, contentDigest, proposed.Floor.Action, proposed.Floor.ReplyState, proposed.OwnedCiphertext, cipherEpoch, cipherVersion, reply, proposed.State)
	return proposed, err
}

// Call only in the same local transaction as the parent's verified positive
// body/holder commit. No helper here publishes a body or grants a source ACL.
func finishDraftPendingTx(ctx context.Context, tx pgx.Tx, identity draftFloor) error {
	allowed, err := draftPendingAllowedTx(ctx, tx, identity)
	if err != nil {
		return err
	}
	if !allowed {
		return fail(409, "DRAFT_FLOOR_UNCONFIRMED", "Original draft stage is unresolved or retired")
	}
	_, err = tx.Exec(ctx, `UPDATE sync.draft_pending SET state='applied',owned_ciphertext=NULL,cipher_epoch=NULL,cipher_version=NULL,reply_reference=NULL,updated_at=now() WHERE scope_id=$1 AND source_context_id=$2 AND user_id=$3 AND conversation_id=$4 AND operation_id=$5 AND state='prepared'`, identity.ScopeID, identity.SourceContextID, identity.UserID, identity.ConversationID, identity.Operation)
	return err
}

func retireDraftPendingTx(ctx context.Context, tx pgx.Tx, identity draftFloor) error {
	if err := lockDraftContextTx(ctx, tx, identity); err != nil {
		return err
	}
	// Retirement removes publication authority, not the user's independent text.
	// Explicit clear/replacement floors own erasure; metadata-only results never
	// return these retained bytes or make them publishable again.
	_, err := tx.Exec(ctx, `UPDATE sync.draft_pending SET state='retired',reply_reference=NULL,updated_at=now() WHERE scope_id=$1 AND source_context_id=$2 AND user_id=$3 AND conversation_id=$4 AND operation_id=$5`, identity.ScopeID, identity.SourceContextID, identity.UserID, identity.ConversationID, identity.Operation)
	return err
}
