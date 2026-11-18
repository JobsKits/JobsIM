package jobsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type selfDraftReply struct {
	MessageID      string `json:"messageId"`
	SourceVersion  string `json:"sourceVersion"`
	ReplyAdmission string `json:"replyAdmission,omitempty"`
}

type selfDraftPatch struct {
	RequestID       string          `json:"requestId"`
	RevisionEpoch   string          `json:"revisionEpoch"`
	ExpectedVersion string          `json:"expectedVersion"`
	DraftAdmission  string          `json:"draftAdmission"`
	OwnedText       string          `json:"ownedText,omitempty"`
	Reply           *selfDraftReply `json:"reply"`
}

type selfDraftResult struct {
	RevisionEpoch string `json:"revisionEpoch"`
	Version       string `json:"version"`
	ContentState  string `json:"contentState"`
	ReplyState    string `json:"replyState"`
}

type selfDraftOperation struct {
	OperationID string          `json:"operationId"`
	Status      string          `json:"status"`
	Current     bool            `json:"current"`
	Result      selfDraftResult `json:"result"`
}

type selfDraftView struct {
	ID                 string          `json:"id"`
	ScopeID            string          `json:"scopeId"`
	SourceContextID    string          `json:"sourceContextId"`
	UserID             string          `json:"userId"`
	ConversationID     string          `json:"conversationId"`
	RevisionEpoch      string          `json:"revisionEpoch"`
	Version            string          `json:"version"`
	ContentState       string          `json:"contentState"`
	OwnedText          *string         `json:"ownedText"`
	ReplyState         string          `json:"replyState"`
	Reply              *selfDraftReply `json:"reply"`
	DraftAdmission     string          `json:"draftAdmission,omitempty"`
	Writable           bool            `json:"writable"`
	PendingOperationID string          `json:"pendingOperationId,omitempty"`
	Recovery           *struct {
		OperationID string `json:"operationId"`
		OwnedText   string `json:"ownedText"`
	} `json:"recovery,omitempty"`
}

type selfDraftStoredOperation struct {
	Identity draftFloor
	Digest   []byte
	Intent   []byte
	Method   string
	Status   string
	Holder   *draftReplyHolder
	Result   selfDraftResult
}

func migrateSelfDraftOperations(ctx context.Context, tx pgx.Tx, scope string) error {
	if !syncValidationID(scope) {
		return fail(400, "INVALID_DRAFT_SCOPE", "Canonical private draft scope required")
	}
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS sync.draft_operations(
id uuid PRIMARY KEY,scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,conversation_id uuid NOT NULL,
revision_epoch bigint NOT NULL CHECK(revision_epoch>0),draft_version bigint NOT NULL CHECK(draft_version>0),request_id text NOT NULL CHECK(octet_length(request_id) BETWEEN 8 AND 128),
method text NOT NULL CHECK(method IN('PUT','DELETE')),request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),intent_ciphertext bytea,
planned_floor jsonb NOT NULL,reply_holder jsonb,state text NOT NULL CHECK(state IN('prepared','applied','retired')),result jsonb NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
UNIQUE(scope_id,source_context_id,user_id,revision_epoch,request_id));
CREATE INDEX IF NOT EXISTS draft_operation_pending ON sync.draft_operations(state,created_at);
CREATE TABLE IF NOT EXISTS sync.draft_holder_releases(id uuid PRIMARY KEY,holder jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now());`)
	return err
}

func (a *App) selfDraftIdentity(u User, conversation string) draftFloor {
	return draftFloor{ScopeID: a.Config.DeploymentID, SourceContextID: "personal", UserID: u.ID, ConversationID: conversation, ObjectID: stableID(a.Config.Key, "personal-view:"+a.Config.DeploymentID+":"+u.ID+":"+conversation), Epoch: u.RestoreEpoch}
}

func (a *App) selfDraftAdmission(ctx context.Context, u User, conversation string, allowRetained bool) (string, bool, error) {
	principal, err := a.readForwardPrincipal(ctx, u.ID)
	if err != nil {
		return "", false, err
	}
	if principal.RestoreEpoch != u.RestoreEpoch {
		return "", false, fail(409, "RESTORE_EPOCH_CHANGED", "Draft principal and owner epochs differ")
	}
	member, err := a.member(ctx, u.ID, conversation)
	writable := err == nil
	qualification := "retained"
	if err != nil {
		var rejected *apiError
		if !allowRetained || !errors.As(err, &rejected) || rejected.Status != 403 {
			return "", false, err
		}
	} else {
		qualification = stringValue(member, "membershipGeneration") + ":" + stringValue(member, "roleGeneration") + ":" + stringValue(member, "sendAdmission")
	}
	material := strings.Join([]string{a.Config.DeploymentID, u.ID, conversation, u.RestoreEpoch, principal.SecurityVersion, qualification}, "\x00")
	proof := "da1." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "self-draft-admission-v1", []byte(material)))
	return proof, writable, nil
}

func decodeSelfDraftPatch(r *http.Request, method string) (selfDraftPatch, error) {
	var patch selfDraftPatch
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<10))
	var raw json.RawMessage
	if decoder.Decode(&raw) != nil || decoder.Decode(new(any)) != io.EOF {
		return patch, fail(400, "INVALID_DRAFT_REQUEST", "One bounded original draft body required")
	}
	return decodeSelfDraftBytes(method, raw)
}

func decodeSelfDraftBytes(method string, raw []byte) (selfDraftPatch, error) {
	var patch selfDraftPatch
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&patch) != nil || decoder.Decode(new(any)) != io.EOF {
		return patch, fail(400, "INVALID_DRAFT_REQUEST", "Only the complete typed original draft body is accepted")
	}
	if method == "PUT" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || len(fields["ownedText"]) == 0 || bytes.Equal(bytes.TrimSpace(fields["ownedText"]), []byte("null")) {
			return patch, fail(400, "INVALID_DRAFT_REQUEST", "PUT requires an explicit owned text string, including an explicit empty string")
		}
	}
	return patch, validateSelfDraftPatch(method, patch)
}

func validateSelfDraftPatch(method string, patch selfDraftPatch) error {
	if method != "PUT" && method != "DELETE" || len(patch.RequestID) < 8 || len(patch.RequestID) > 128 || strings.ContainsAny(patch.RequestID, "\r\n\x00") || !syncValidationCounter(patch.RevisionEpoch, false) || !syncValidationCounter(patch.ExpectedVersion, true) || len(patch.DraftAdmission) > 256 || patch.DraftAdmission == "" || !utf8.ValidString(patch.OwnedText) || len(patch.OwnedText) > 16384 {
		return fail(400, "INVALID_DRAFT_REQUEST", "Stable nonce, exact CAS tuple, original admission and at most 16KiB text required")
	}
	if method == "DELETE" && (patch.OwnedText != "" || patch.Reply != nil) {
		return fail(400, "INVALID_DRAFT_REQUEST", "Clear requests cannot carry a replacement body or reply")
	}
	if patch.Reply != nil && (!syncValidationID(patch.Reply.MessageID) || !syncValidationCounter(patch.Reply.SourceVersion, false) || len(patch.Reply.ReplyAdmission) > 256 || patch.Reply.ReplyAdmission == "") {
		return fail(400, "INVALID_DRAFT_REPLY", "Original same-conversation source version and reply proof required")
	}
	return nil
}

func (a *App) selfDraftDigest(u User, conversation, method string, patch selfDraftPatch) []byte {
	raw, _ := json.Marshal(struct {
		Method string         `json:"method"`
		Body   selfDraftPatch `json:"body"`
	}{method, patch})
	return keyedDigest(a.Config.Key, "self-draft-request-v1:"+a.Config.DeploymentID+":"+u.ID+":"+conversation, raw)
}

func (a *App) selfDraftIntentAAD(identity draftFloor) string {
	return strings.Join([]string{"self-draft-intent-v1", identity.ScopeID, identity.UserID, identity.ConversationID, identity.Epoch, identity.Operation}, ":")
}

func scanSelfDraftOperation(row pgx.Row) (selfDraftStoredOperation, error) {
	var operation selfDraftStoredOperation
	var floor, holder, result []byte
	err := row.Scan(&floor, &operation.Digest, &operation.Intent, &operation.Method, &operation.Status, &holder, &result)
	if err != nil {
		return operation, err
	}
	if json.Unmarshal(floor, &operation.Identity) != nil || json.Unmarshal(result, &operation.Result) != nil {
		return operation, fail(503, "DRAFT_OPERATION_CORRUPT", "Original draft metadata cannot be verified")
	}
	if len(holder) != 0 && string(holder) != "null" {
		operation.Holder = &draftReplyHolder{}
		if json.Unmarshal(holder, operation.Holder) != nil {
			return operation, fail(503, "DRAFT_OPERATION_CORRUPT", "Original draft holder metadata cannot be verified")
		}
	}
	return operation, nil
}

const selfDraftOperationColumns = `planned_floor,request_digest,intent_ciphertext,method,state,reply_holder,result`

func (a *App) selfDraftFind(ctx context.Context, db sourcePreferenceQuery, u User, conversation, method string, patch selfDraftPatch) (selfDraftStoredOperation, bool, error) {
	operation, err := scanSelfDraftOperation(db.QueryRow(ctx, `SELECT `+selfDraftOperationColumns+` FROM sync.draft_operations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND revision_epoch=$3 AND request_id=$4`, a.Config.DeploymentID, u.ID, patch.RevisionEpoch, patch.RequestID))
	if err == pgx.ErrNoRows {
		return operation, false, nil
	}
	if err != nil {
		return operation, false, err
	}
	if operation.Identity.ConversationID != conversation || !hmac.Equal(operation.Digest, a.selfDraftDigest(u, conversation, method, patch)) {
		return operation, true, fail(409, "IDEMPOTENCY_CONFLICT", "Original draft method, body or conversation differs")
	}
	return operation, true, nil
}

func (a *App) selfDraftOperationView(ctx context.Context, u User, operation selfDraftStoredOperation) (selfDraftOperation, error) {
	result := selfDraftOperation{OperationID: operation.Identity.Operation, Status: operation.Status, Result: operation.Result}
	var epoch, version, current, contentState, replyState string
	err := a.DB.QueryRow(ctx, `SELECT draft_revision_epoch::text,draft_version::text,COALESCE(draft_operation_id,''),draft_content_state,draft_reply_state FROM sync.conversation_views WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3`, a.Config.DeploymentID, u.ID, operation.Identity.ConversationID).Scan(&epoch, &version, &current, &contentState, &replyState)
	if err != nil && err != pgx.ErrNoRows {
		return result, err
	}
	result.Current = operation.Status == "applied" && epoch == u.RestoreEpoch && epoch == operation.Identity.Epoch && version == operation.Identity.Version && current == operation.Identity.Operation && contentState == operation.Result.ContentState && replyState == operation.Result.ReplyState
	return result, nil
}

func (a *App) selfDraftRead(ctx context.Context, u User, conversation string, includeRecovery bool) (selfDraftView, error) {
	identity := a.selfDraftIdentity(u, conversation)
	view := selfDraftView{ID: identity.ObjectID, ScopeID: identity.ScopeID, SourceContextID: "personal", UserID: u.ID, ConversationID: conversation, RevisionEpoch: u.RestoreEpoch, Version: "0", ContentState: "empty", ReplyState: "none"}
	if !validDraftIdentity(identity) {
		return view, fail(400, "INVALID_DRAFT_CONTEXT", "Canonical current private draft context required")
	}
	var stored draftStoredContent
	var reply []byte
	err := a.DB.QueryRow(ctx, `SELECT draft_revision_epoch::text,draft_version::text,COALESCE(draft_operation_id,''),draft_content_state,draft_owned_digest,draft_owned_ciphertext,draft_cipher_epoch,draft_cipher_version,draft_reply_state,draft_reply_reference FROM sync.conversation_views WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3`, identity.ScopeID, u.ID, conversation).Scan(&stored.Epoch, &stored.Version, &stored.Operation, &stored.ContentState, &stored.Digest, &stored.Ciphertext, &stored.CipherEpoch, &stored.CipherVersion, &stored.ReplyState, &reply)
	found := err == nil
	if err != nil && err != pgx.ErrNoRows {
		return view, err
	}
	proof, writable, err := a.selfDraftAdmission(ctx, u, conversation, found)
	if err != nil {
		return view, err
	}
	view.DraftAdmission, view.Writable = proof, writable
	if found {
		view.ContentState, view.ReplyState = stored.ContentState, stored.ReplyState
		if stored.Epoch == u.RestoreEpoch {
			view.Version = stored.Version
		}
		if stored.ContentState == "owned" && stored.CipherEpoch != nil && stored.CipherVersion != nil {
			text, err := a.openDraftOwnedText(identity, str(*stored.CipherEpoch), str(*stored.CipherVersion), stored.Ciphertext, hex.EncodeToString(stored.Digest))
			if err != nil {
				view.ContentState = "unavailable"
			} else {
				view.OwnedText = &text
			}
		} else if stored.ContentState == "owned" {
			view.ContentState = "unavailable"
		}
		if view.ReplyState == "available" && stored.Epoch == u.RestoreEpoch {
			var reference draftReplyReference
			if json.Unmarshal(reply, &reference) != nil {
				return view, fail(503, "DRAFT_REPLY_CORRUPT", "Current reply reference metadata is invalid")
			}
			var holder draftReplyHolder
			err := a.rpc(ctx, "message", "/internal/drafts/inspect-reply", map[string]string{"holderId": reference.HolderID, "userId": u.ID}, &holder)
			if err == nil && holder.State == "active" && holder.SourceVersion == reference.SourceVersion && holder.MembershipGeneration == reference.MembershipGeneration && holder.SecurityVersion == reference.SecurityVersion && holder.RestoreEpoch == u.RestoreEpoch && writable {
				view.Reply = &selfDraftReply{MessageID: holder.SourceID, SourceVersion: holder.SourceVersion}
			} else {
				// Only the reference preview is unavailable; this does not withhold
				// or erase the owner's independent text during a source outage.
				view.ReplyState = "unavailable"
			}
		} else if view.ReplyState == "available" {
			view.ReplyState = "unavailable"
		}
	}
	if !includeRecovery {
		view.DraftAdmission = ""
		return view, nil
	}
	var pending string
	err = a.DB.QueryRow(ctx, `SELECT id::text FROM sync.draft_operations WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 AND revision_epoch=$4 AND state='prepared' ORDER BY draft_version DESC LIMIT 1`, identity.ScopeID, u.ID, conversation, u.RestoreEpoch).Scan(&pending)
	if err != nil && err != pgx.ErrNoRows {
		return view, err
	}
	view.PendingOperationID = pending
	var intent []byte
	var floorBytes []byte
	err = a.DB.QueryRow(ctx, `SELECT planned_floor,intent_ciphertext FROM sync.draft_operations WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 AND state='retired' AND intent_ciphertext IS NOT NULL ORDER BY revision_epoch DESC,draft_version DESC LIMIT 1`, identity.ScopeID, u.ID, conversation).Scan(&floorBytes, &intent)
	if err == nil {
		var old draftFloor
		if json.Unmarshal(floorBytes, &old) != nil {
			return view, fail(503, "DRAFT_OPERATION_CORRUPT", "Retired owned-text metadata unavailable")
		}
		var superseded bool
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.draft_floors WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 AND (action='clear' OR ((epoch,version)>($4::bigint,$5::bigint) AND NOT(action='reply_unavailable' AND operation_id=$6))))`, identity.ScopeID, u.ID, conversation, old.Epoch, old.Version, old.Operation).Scan(&superseded); err != nil {
			return view, err
		}
		if !superseded {
			raw, err := a.unseal(intent, a.selfDraftIntentAAD(old))
			var patch selfDraftPatch
			if err == nil && json.Unmarshal([]byte(raw), &patch) == nil {
				view.Recovery = &struct {
					OperationID string `json:"operationId"`
					OwnedText   string `json:"ownedText"`
				}{old.Operation, patch.OwnedText}
			}
		}
	} else if err != pgx.ErrNoRows {
		return view, err
	}
	return view, nil
}

func (a *App) selfDraftPrepare(ctx context.Context, u User, conversation, method string, patch selfDraftPatch) (selfDraftStoredOperation, error) {
	var result selfDraftStoredOperation
	if err := validateSelfDraftPatch(method, patch); err != nil {
		return result, err
	}
	if original, found, err := a.selfDraftFind(ctx, a.DB, u, conversation, method, patch); found || err != nil {
		return original, err
	}
	if patch.RevisionEpoch != u.RestoreEpoch {
		return result, fail(409, "RESTORE_EPOCH_CHANGED", "An old draft intent cannot be first created")
	}
	proof, writable, err := a.selfDraftAdmission(ctx, u, conversation, method == "DELETE")
	if err != nil {
		return result, err
	}
	if !hmac.Equal([]byte(proof), []byte(patch.DraftAdmission)) || method == "PUT" && !writable {
		return result, fail(409, "DRAFT_ADMISSION_CHANGED", "Original draft admission no longer applies")
	}
	identity := a.selfDraftIdentity(u, conversation)
	identity.Operation = stableID(a.Config.Key, "self-draft-operation:"+identity.ScopeID+":"+u.ID+":"+patch.RevisionEpoch+":"+patch.RequestID)
	digest := a.selfDraftDigest(u, conversation, method, patch)
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if err = lockDraftContextTx(ctx, tx, identity); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "draft-original:"+identity.ScopeID+":"+u.ID+":"+patch.RequestID); err != nil {
		return result, err
	}
	if original, found, err := a.selfDraftFind(ctx, tx, u, conversation, method, patch); found || err != nil {
		return original, err
	}
	var prepared bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.draft_operations WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 AND revision_epoch=$4 AND state='prepared')`, identity.ScopeID, u.ID, conversation, u.RestoreEpoch).Scan(&prepared); err != nil {
		return result, err
	}
	if prepared {
		return result, fail(409, "DRAFT_OPERATION_PENDING", "Reconcile the original pending draft before replacing it")
	}
	var epoch string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM sync.instance WHERE singleton AND deployment_id=$1`, identity.ScopeID).Scan(&epoch); err != nil {
		return result, err
	}
	if epoch != u.RestoreEpoch {
		return result, fail(409, "RESTORE_EPOCH_CHANGED", "Draft owner epoch changed")
	}
	err = tx.QueryRow(ctx, `SELECT CASE WHEN draft_revision_epoch=$4::bigint THEN draft_version ELSE 0 END FROM sync.conversation_views WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 FOR UPDATE`, identity.ScopeID, u.ID, conversation, epoch).Scan(&version)
	if err != nil && err != pgx.ErrNoRows {
		return result, err
	}
	if str(version) != patch.ExpectedVersion || version == math.MaxInt64 {
		return result, fail(409, "DRAFT_VERSION_CONFLICT", "Reload the current independent draft CAS tuple")
	}
	identity.Version, identity.Action, identity.ReplyState = str(version+1), "set", "none"
	if method == "DELETE" {
		identity.Action = "clear"
	} else if patch.Reply != nil {
		identity.ReplyState = "available"
	}
	identity.ContentDigest, err = a.draftOwnedDigest(identity, patch.OwnedText)
	if err != nil {
		return result, err
	}
	raw, _ := json.Marshal(patch)
	ciphertext, err := a.seal(string(raw), a.selfDraftIntentAAD(identity))
	if err != nil {
		return result, err
	}
	content := "owned"
	if method == "DELETE" {
		content = "empty"
	}
	metadata := selfDraftResult{epoch, identity.Version, content, identity.ReplyState}
	floorRaw, _ := json.Marshal(identity)
	resultRaw, _ := json.Marshal(metadata)
	if _, err = tx.Exec(ctx, `INSERT INTO sync.conversation_views(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch) VALUES($1,$2,'personal',$3,$4,$5) ON CONFLICT DO NOTHING`, identity.ObjectID, identity.ScopeID, u.ID, conversation, epoch); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sync.draft_operations(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch,draft_version,request_id,method,request_digest,intent_ciphertext,planned_floor,state,result) VALUES($1,$2,'personal',$3,$4,$5,$6,$7,$8,$9,$10,$11,'prepared',$12)`, identity.Operation, identity.ScopeID, u.ID, conversation, epoch, identity.Version, patch.RequestID, method, digest, ciphertext, floorRaw, resultRaw); err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return selfDraftStoredOperation{Identity: identity, Digest: digest, Intent: ciphertext, Method: method, Status: "prepared", Result: metadata}, nil
}

func (a *App) selfDraftResume(ctx context.Context, operation selfDraftStoredOperation) (selfDraftStoredOperation, error) {
	latest, err := scanSelfDraftOperation(a.DB.QueryRow(ctx, `SELECT `+selfDraftOperationColumns+` FROM sync.draft_operations WHERE id=$1`, operation.Identity.Operation))
	if err != nil {
		return operation, err
	}
	operation = latest
	if operation.Status != "prepared" {
		return operation, nil
	}
	identity := operation.Identity
	raw, err := a.unseal(operation.Intent, a.selfDraftIntentAAD(identity))
	var patch selfDraftPatch
	if err != nil || json.Unmarshal([]byte(raw), &patch) != nil || validateSelfDraftPatch(operation.Method, patch) != nil {
		return operation, fail(503, "DRAFT_OPERATION_CORRUPT", "Original pending draft intent unavailable")
	}
	u := User{ID: identity.UserID, RestoreEpoch: identity.Epoch}
	proof, writable, err := a.selfDraftAdmission(ctx, u, identity.ConversationID, operation.Method == "DELETE")
	if err != nil || !hmac.Equal([]byte(proof), []byte(patch.DraftAdmission)) || operation.Method == "PUT" && !writable {
		if err != nil && forwardUncertain(err) {
			return operation, err
		}
		return a.retireSelfDraft(ctx, operation)
	}
	var holder *draftReplyHolder
	if patch.Reply != nil {
		q := draftReplyPrepare{HolderID: stableID(a.Config.Key, "draft-reply-holder:"+identity.ScopeID+":personal:"+u.ID+":"+identity.Operation+":"+patch.Reply.MessageID), UserID: u.ID, ConversationID: identity.ConversationID, DraftID: identity.ObjectID, OperationID: identity.Operation, SourceID: patch.Reply.MessageID, SourceVersion: patch.Reply.SourceVersion, Admission: patch.Reply.ReplyAdmission}
		holder = &draftReplyHolder{}
		if err = a.rpc(ctx, "message", "/internal/drafts/prepare-reply", q, holder); err != nil {
			if forwardUncertain(err) {
				return operation, err
			}
			return a.retireSelfDraft(ctx, operation)
		}
		if holder.State != "prepared" && holder.State != "active" {
			return a.retireSelfDraft(ctx, operation)
		}
	}
	proposed := draftPending{Floor: identity, RequestDigest: hex.EncodeToString(operation.Digest)}
	if operation.Method == "PUT" {
		proposed.CipherEpoch, proposed.CipherVersion = identity.Epoch, identity.Version
		proposed.OwnedCiphertext, _, err = a.sealDraftOwnedText(identity, identity.Epoch, identity.Version, patch.OwnedText)
		if err != nil {
			return operation, err
		}
	}
	if holder != nil {
		proposed.Reply = &draftReplyReference{"personal", holder.SourceID, holder.ConversationID, holder.RestoreEpoch, holder.SourceVersion, holder.MembershipGeneration, holder.SecurityVersion, holder.RestoreEpoch, holder.HolderID}
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return operation, err
	}
	defer tx.Rollback(ctx)
	if err = lockDraftContextTx(ctx, tx, identity); err != nil {
		return operation, err
	}
	latest, err = scanSelfDraftOperation(tx.QueryRow(ctx, `SELECT `+selfDraftOperationColumns+` FROM sync.draft_operations WHERE id=$1 FOR UPDATE`, identity.Operation))
	if err != nil || latest.Status != "prepared" {
		return latest, err
	}
	if _, err = stageDraftPendingTx(ctx, tx, proposed); err != nil {
		return operation, err
	}
	var holderRaw any
	if holder != nil {
		holderRaw, _ = json.Marshal(holder)
	}
	if _, err = tx.Exec(ctx, `UPDATE sync.draft_operations SET reply_holder=$2,updated_at=now() WHERE id=$1 AND state='prepared'`, identity.Operation, holderRaw); err != nil {
		return operation, err
	}
	if err = tx.Commit(ctx); err != nil {
		return operation, err
	}
	if err = a.protectDraftFloor(ctx, identity); err != nil {
		return operation, err
	}
	if holder != nil {
		var promoted draftReplyHolder
		if err = a.rpc(ctx, "message", "/internal/drafts/promote-reply", holder, &promoted); err != nil {
			if forwardUncertain(err) {
				return operation, err
			}
			return a.retireSelfDraft(ctx, operation)
		}
	}
	tx, err = a.DB.Begin(ctx)
	if err != nil {
		return operation, err
	}
	defer tx.Rollback(ctx)
	if err = lockDraftContextTx(ctx, tx, identity); err != nil {
		return operation, err
	}
	latest, err = scanSelfDraftOperation(tx.QueryRow(ctx, `SELECT `+selfDraftOperationColumns+` FROM sync.draft_operations WHERE id=$1 FOR UPDATE`, identity.Operation))
	if err != nil || latest.Status != "prepared" {
		return latest, err
	}
	if err = a.queuePreviousDraftHolder(ctx, tx, identity, holder); err != nil {
		return operation, err
	}
	if _, err = applyDraftFloorTx(ctx, tx, identity); err != nil {
		return operation, err
	}
	allowed, err := draftPendingAllowedTx(ctx, tx, identity)
	if err != nil {
		return operation, err
	}
	if !allowed {
		return operation, fail(409, "DRAFT_OPERATION_RETIRED", "Original draft publication was retired")
	}
	content := "owned"
	var ciphertext, digest, cipherEpoch, cipherVersion any
	if operation.Method == "PUT" {
		ciphertext = proposed.OwnedCiphertext
		digest, _ = draftDigestBytes(identity.ContentDigest)
		cipherEpoch, cipherVersion = identity.Epoch, identity.Version
	} else {
		content = "empty"
	}
	var replyRaw any
	if proposed.Reply != nil {
		replyRaw, _ = json.Marshal(proposed.Reply)
	}
	_, err = tx.Exec(ctx, `UPDATE sync.conversation_views SET draft_revision_epoch=$5,draft_version=$6,draft_operation_id=$7,draft_content_state=$8,draft_owned_ciphertext=$9,draft_owned_digest=$10,draft_cipher_epoch=$11,draft_cipher_version=$12,draft_reply_reference=$13,draft_reply_state=$14,updated_at=now() WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 AND id=$4`, identity.ScopeID, identity.UserID, identity.ConversationID, identity.ObjectID, identity.Epoch, identity.Version, identity.Operation, content, ciphertext, digest, cipherEpoch, cipherVersion, replyRaw, identity.ReplyState)
	if err != nil {
		return operation, err
	}
	if err = finishDraftPendingTx(ctx, tx, identity); err != nil {
		return operation, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sync.draft_operations SET state='applied',intent_ciphertext=NULL,updated_at=now() WHERE id=$1 AND state='prepared'`, identity.Operation); err != nil {
		return operation, err
	}
	// Replacement/clear erases older unpublished owned payloads, never a newer intent.
	if _, err = tx.Exec(ctx, `UPDATE sync.draft_operations SET intent_ciphertext=NULL WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 AND (revision_epoch,draft_version)<($4::bigint,$5::bigint)`, identity.ScopeID, u.ID, identity.ConversationID, identity.Epoch, identity.Version); err != nil {
		return operation, err
	}
	if err = tx.Commit(ctx); err != nil {
		return operation, err
	}
	operation.Status, operation.Holder = "applied", holder
	return operation, nil
}

func (a *App) queuePreviousDraftHolder(ctx context.Context, tx pgx.Tx, identity draftFloor, next *draftReplyHolder) error {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT draft_reply_reference FROM sync.conversation_views WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3`, identity.ScopeID, identity.UserID, identity.ConversationID).Scan(&raw)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return err
	}
	var old draftReplyReference
	if json.Unmarshal(raw, &old) != nil || next != nil && old.HolderID == next.HolderID {
		return nil
	}
	var holder draftReplyHolder
	var holderBytes []byte
	err = tx.QueryRow(ctx, `SELECT reply_holder FROM sync.draft_operations WHERE scope_id=$1 AND user_id=$2 AND reply_holder->>'holderId'=$3 LIMIT 1`, identity.ScopeID, identity.UserID, old.HolderID).Scan(&holderBytes)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if json.Unmarshal(holderBytes, &holder) != nil {
		return fail(503, "DRAFT_REPLY_CORRUPT", "Old holder release metadata unavailable")
	}
	_, err = tx.Exec(ctx, `INSERT INTO sync.draft_holder_releases(id,holder) VALUES($1,$2) ON CONFLICT DO NOTHING`, holder.HolderID, holderBytes)
	return err
}

func (a *App) retireSelfDraft(ctx context.Context, operation selfDraftStoredOperation) (selfDraftStoredOperation, error) {
	floor := operation.Identity
	floor.Action, floor.ReplyState = "reply_unavailable", "unavailable"
	if err := a.protectDraftFloor(ctx, floor); err != nil {
		return operation, err
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return operation, err
	}
	defer tx.Rollback(ctx)
	if _, err = applyDraftFloorTx(ctx, tx, floor); err != nil {
		return operation, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sync.draft_operations SET state='retired',updated_at=now() WHERE id=$1 AND state='prepared'`, floor.Operation); err != nil {
		return operation, err
	}
	if err = tx.Commit(ctx); err != nil {
		return operation, err
	}
	operation.Status = "retired"
	return operation, nil
}

func (a *App) selfDraftRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/conversations/{id}/draft", func(w http.ResponseWriter, r *http.Request) error {
		u, err := a.syncUser(r)
		if err != nil {
			return err
		}
		view, err := a.selfDraftRead(r.Context(), u, r.PathValue("id"), true)
		if err == nil {
			respond(w, 200, view)
		}
		return err
	})
	for _, method := range []string{"PUT", "DELETE"} {
		method := method
		h(method+" /api/conversations/{id}/draft", func(w http.ResponseWriter, r *http.Request) error {
			u, err := a.syncUser(r)
			if err != nil {
				return err
			}
			patch, err := decodeSelfDraftPatch(r, method)
			if err != nil {
				return err
			}
			operation, err := a.selfDraftPrepare(r.Context(), u, r.PathValue("id"), method, patch)
			if err == nil {
				operation, err = a.selfDraftResume(r.Context(), operation)
			}
			if err != nil {
				return err
			}
			result, err := a.selfDraftOperationView(r.Context(), u, operation)
			if err == nil {
				respond(w, 200, result)
			}
			return err
		})
	}
	h("POST /api/conversations/{id}/draft-results", func(w http.ResponseWriter, r *http.Request) error {
		u, err := a.syncUser(r)
		if err != nil {
			return err
		}
		var q struct {
			Method string          `json:"method"`
			Body   json.RawMessage `json:"body"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&q) != nil || decoder.Decode(new(any)) != io.EOF {
			return fail(400, "INVALID_DRAFT_REQUEST", "Complete original draft method and body required")
		}
		patch, err := decodeSelfDraftBytes(q.Method, q.Body)
		if err != nil {
			return err
		}
		operation, found, err := a.selfDraftFind(r.Context(), a.DB, u, r.PathValue("id"), q.Method, patch)
		if err != nil {
			return err
		}
		if !found {
			return fail(404, "DRAFT_OPERATION_NOT_FOUND", "No original draft operation exists")
		}
		result, err := a.selfDraftOperationView(r.Context(), u, operation)
		if err == nil {
			respond(w, 200, result)
		}
		return err
	})
	h("POST /internal/drafts/revoke-reply", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "message" {
			return fail(403, "MESSAGE_SERVICE_REQUIRED", "Only the original message holder owner may retire a reply")
		}
		var q draftReplyRevocationRequest
		if err := decode(r, &q); err != nil {
			return err
		}
		if err := a.invalidateSelfDraftReply(r.Context(), q); err != nil {
			return err
		}
		respond(w, 200, map[string]string{"state": "cleared"})
		return nil
	}))
}

func (a *App) StartSelfDraftWorkers(ctx context.Context) {
	if a.Role != "sync" {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
				fence, err := a.admissionFence(attempt, true)
				if err == nil {
					markers, markerError := a.admissionMarkers()
					if markerError == nil && len(markers) == 0 {
						_ = a.resumeSelfDraftOperations(attempt)
						_ = a.drainPersonalExportInvalidations(attempt)
					}
					closeExportFence(fence)
				}
				cancel()
			}
		}
	}()
}

func (a *App) resumeSelfDraftOperations(ctx context.Context) error {
	rows, err := a.DB.Query(ctx, `SELECT `+selfDraftOperationColumns+` FROM sync.draft_operations WHERE state='prepared' ORDER BY created_at LIMIT 20`)
	if err != nil {
		return err
	}
	operations := []selfDraftStoredOperation{}
	for rows.Next() {
		operation, err := scanSelfDraftOperation(rows)
		if err != nil {
			rows.Close()
			return err
		}
		operations = append(operations, operation)
	}
	rows.Close()
	for _, operation := range operations {
		if _, err = a.selfDraftResume(ctx, operation); err != nil && ctx.Err() != nil {
			return err
		}
	}
	rows, err = a.DB.Query(ctx, `SELECT id::text,holder FROM sync.draft_holder_releases ORDER BY created_at LIMIT 100`)
	if err != nil {
		return err
	}
	type release struct {
		id     string
		holder draftReplyHolder
	}
	releases := []release{}
	for rows.Next() {
		var item release
		var raw []byte
		if rows.Scan(&item.id, &raw) != nil || json.Unmarshal(raw, &item.holder) != nil {
			rows.Close()
			return fail(503, "DRAFT_REPLY_CORRUPT", "Durable holder release metadata unavailable")
		}
		releases = append(releases, item)
	}
	rows.Close()
	for _, item := range releases {
		if err = a.rpc(ctx, "message", "/internal/drafts/release-reply", item.holder, nil); err == nil {
			_, err = a.DB.Exec(ctx, `DELETE FROM sync.draft_holder_releases WHERE id=$1`, item.id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Used by sync's current-ACL hydration; no quote body or reader proof is stored.
func (a *App) syncPersonalDraftProjection(ctx context.Context, user string, ref syncRef) (any, error) {
	var epoch string
	if err := a.DB.QueryRow(ctx, `SELECT restore_epoch::text FROM sync.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch); err != nil {
		return nil, err
	}
	if ref.ID != a.selfDraftIdentity(User{ID: user, RestoreEpoch: epoch}, ref.ConversationID).ObjectID {
		return nil, fail(403, "FORBIDDEN", "Private draft identity differs from this principal")
	}
	return a.selfDraftRead(ctx, User{ID: user, RestoreEpoch: epoch}, ref.ConversationID, false)
}

func draftVersionAfter(raw string) (string, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 || n == math.MaxInt64 {
		return "", fail(409, "DRAFT_VERSION_CONFLICT", "Draft version cannot advance")
	}
	return str(n + 1), nil
}
