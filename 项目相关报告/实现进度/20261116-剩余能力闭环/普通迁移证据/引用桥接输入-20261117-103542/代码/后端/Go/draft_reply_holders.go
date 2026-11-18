package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func migrateDraftReplyHolders(ctx context.Context, tx pgx.Tx, scope string) error {
	if !syncValidationID(scope) {
		return fail(400, "INVALID_SCOPE", "Canonical draft-reply scope required")
	}
	var deployment string
	if err := tx.QueryRow(ctx, `SELECT deployment_id FROM message.instance WHERE singleton FOR SHARE`).Scan(&deployment); err != nil {
		return err
	}
	if deployment != scope {
		return fail(409, "DRAFT_SCOPE_CHANGED", "Draft-reply storage belongs to another instance")
	}
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS message.draft_reply_holders(
id uuid PRIMARY KEY,scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),
user_id uuid NOT NULL,conversation_id uuid NOT NULL,draft_id uuid NOT NULL,operation_id uuid NOT NULL,
source_message_id uuid NOT NULL REFERENCES message.messages(id),source_content_version bigint NOT NULL CHECK(source_content_version>0),
membership_generation bigint NOT NULL CHECK(membership_generation>0),security_version bigint NOT NULL CHECK(security_version>0),restore_epoch bigint NOT NULL CHECK(restore_epoch>0),
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),state text NOT NULL CHECK(state IN('prepared','active','revoking','cleared')),
revocation_operation_id text,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
UNIQUE(scope_id,source_context_id,user_id,operation_id),CHECK((state='revoking')=(revocation_operation_id IS NOT NULL)));
CREATE INDEX IF NOT EXISTS draft_reply_holder_source ON message.draft_reply_holders(source_message_id,state);
CREATE INDEX IF NOT EXISTS draft_reply_holder_principal ON message.draft_reply_holders(user_id,conversation_id,membership_generation,state);`)
	return err
}

type draftReplyPrepare struct {
	HolderID       string `json:"holderId"`
	UserID         string `json:"userId"`
	ConversationID string `json:"conversationId"`
	DraftID        string `json:"draftId"`
	OperationID    string `json:"operationId"`
	SourceID       string `json:"sourceMessageId"`
	SourceVersion  string `json:"sourceVersion"`
	Admission      string `json:"replyAdmission"`
}

type draftReplyHolder struct {
	HolderID             string `json:"holderId"`
	UserID               string `json:"userId"`
	ConversationID       string `json:"conversationId"`
	DraftID              string `json:"draftId"`
	OperationID          string `json:"operationId"`
	SourceID             string `json:"sourceMessageId"`
	SourceVersion        string `json:"sourceVersion"`
	MembershipGeneration string `json:"membershipGeneration"`
	SecurityVersion      string `json:"securityVersion"`
	RestoreEpoch         string `json:"restoreEpoch"`
	State                string `json:"state"`
}

const draftReplyHolderColumns = `id::text,user_id::text,conversation_id::text,draft_id::text,operation_id::text,source_message_id::text,source_content_version::text,membership_generation::text,security_version::text,restore_epoch::text,state`

func scanDraftReplyHolder(row pgx.Row) (draftReplyHolder, error) {
	var holder draftReplyHolder
	err := row.Scan(&holder.HolderID, &holder.UserID, &holder.ConversationID, &holder.DraftID, &holder.OperationID, &holder.SourceID, &holder.SourceVersion, &holder.MembershipGeneration, &holder.SecurityVersion, &holder.RestoreEpoch, &holder.State)
	return holder, err
}

func (a *App) validateDraftReplyPrepare(q draftReplyPrepare) error {
	for _, id := range []string{q.HolderID, q.UserID, q.ConversationID, q.DraftID, q.OperationID, q.SourceID} {
		if !syncValidationID(id) {
			return fail(400, "INVALID_DRAFT_HOLDER", "Canonical original draft/reference identities required")
		}
	}
	if !syncValidationCounter(q.SourceVersion, false) || len(q.Admission) > 256 || q.Admission == "" || q.DraftID != stableID(a.Config.Key, "personal-view:"+a.Config.DeploymentID+":"+q.UserID+":"+q.ConversationID) || q.HolderID != stableID(a.Config.Key, "draft-reply-holder:"+a.Config.DeploymentID+":personal:"+q.UserID+":"+q.OperationID+":"+q.SourceID) {
		return fail(400, "INVALID_DRAFT_HOLDER", "Original source version, proof and scoped draft holder required")
	}
	return nil
}

func (a *App) draftReplyHolderRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/drafts/inspect-reply", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "sync" {
			return fail(403, "SYNC_SERVICE_REQUIRED", "Draft reference projection belongs to the synchronization owner")
		}
		var q struct {
			HolderID string `json:"holderId"`
			UserID   string `json:"userId"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		if !syncValidationID(q.HolderID) || !syncValidationID(q.UserID) {
			return fail(400, "INVALID_DRAFT_HOLDER", "Original scoped holder identity required")
		}
		holder, err := scanDraftReplyHolder(a.DB.QueryRow(r.Context(), `SELECT `+draftReplyHolderColumns+` FROM message.draft_reply_holders WHERE scope_id=$1 AND id=$2 AND user_id=$3`, a.Config.DeploymentID, q.HolderID, q.UserID))
		if err == pgx.ErrNoRows {
			return fail(404, "DRAFT_REPLY_HOLDER_NOT_FOUND", "No original draft reply holder exists")
		}
		if err != nil {
			return err
		}
		if holder.State != "active" {
			return fail(409, "DRAFT_REPLY_ADMISSION_CHANGED", "Original draft reply is not active")
		}
		source, err := a.loadMessage(r.Context(), holder.SourceID, holder.UserID)
		if err != nil {
			return err
		}
		if source.forwardUncertain {
			return fail(503, "DRAFT_REPLY_QUALIFICATION_UNAVAILABLE", "Current source qualification is not known")
		}
		if source.Deleted || source.ContentUnavailable || source.ForwardUnavailable || source.sourcePending || source.ContentVersion != holder.SourceVersion || source.forwardMembership != holder.MembershipGeneration || source.forwardSecurity != holder.SecurityVersion || source.forwardEpoch != holder.RestoreEpoch {
			return fail(409, "DRAFT_REPLY_ADMISSION_CHANGED", "Original source qualification no longer applies")
		}
		respond(w, 200, holder)
		return nil
	}))
	h("POST /internal/drafts/prepare-reply", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "sync" {
			return fail(403, "SYNC_SERVICE_REQUIRED", "Draft reference preparation belongs to the synchronization owner")
		}
		var q draftReplyPrepare
		if err := decode(r, &q); err != nil {
			return err
		}
		holder, err := a.prepareDraftReplyHolder(r.Context(), q)
		if err != nil {
			return err
		}
		respond(w, 200, holder)
		return nil
	}))
	for _, action := range []string{"promote", "release"} {
		action := action
		h("POST /internal/drafts/"+action+"-reply", internal(func(w http.ResponseWriter, r *http.Request) error {
			if r.Header.Get("X-JobsIM-Caller") != "sync" {
				return fail(403, "SYNC_SERVICE_REQUIRED", "Draft references are private to the synchronization owner")
			}
			var q draftReplyHolder
			if err := decode(r, &q); err != nil {
				return err
			}
			holder, err := a.finishDraftReplyHolder(r.Context(), q, action == "release")
			if err != nil {
				return err
			}
			respond(w, 200, holder)
			return nil
		}))
	}
}

// The outer caller retains the host SH fence. No PG transaction is held while
// identity, conversation or forward-chain qualification is consulted.
func (a *App) prepareDraftReplyHolder(ctx context.Context, q draftReplyPrepare) (draftReplyHolder, error) {
	var holder draftReplyHolder
	if err := a.validateDraftReplyPrepare(q); err != nil {
		return holder, err
	}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "draft-reply-prepare-v1:"+a.Config.DeploymentID, raw)
	lookup := func(tx sourcePreferenceQuery) (draftReplyHolder, bool, error) {
		var old []byte
		err := tx.QueryRow(ctx, `SELECT request_digest FROM message.draft_reply_holders WHERE scope_id=$1 AND source_context_id='personal' AND id=$2`, a.Config.DeploymentID, q.HolderID).Scan(&old)
		if err == pgx.ErrNoRows {
			return holder, false, nil
		}
		if err != nil {
			return holder, false, err
		}
		if !hmac.Equal(old, digest) {
			return holder, true, fail(409, "IDEMPOTENCY_CONFLICT", "Original draft reference preparation differs")
		}
		original, err := scanDraftReplyHolder(tx.QueryRow(ctx, `SELECT `+draftReplyHolderColumns+` FROM message.draft_reply_holders WHERE scope_id=$1 AND source_context_id='personal' AND id=$2`, a.Config.DeploymentID, q.HolderID))
		return original, true, err
	}
	if original, found, err := lookup(a.DB); found || err != nil {
		return original, err
	}
	source, err := a.prepareDraftReply(ctx, q.UserID, q.ConversationID, q.SourceID, q.SourceVersion, q.Admission)
	if err != nil {
		return holder, err
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return holder, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "draft-reply-operation:"+a.Config.DeploymentID+":"+q.UserID+":"+q.OperationID); err != nil {
		return holder, err
	}
	if original, found, err := lookup(tx); found || err != nil {
		return original, err
	}
	var available bool
	if err = tx.QueryRow(ctx, `SELECT NOT deleted AND NOT content_unavailable AND export_pending_operation_id IS NULL AND content_version=$2 FROM message.messages WHERE id=$1 FOR UPDATE`, q.SourceID, q.SourceVersion).Scan(&available); err != nil {
		return holder, err
	}
	if !available {
		return holder, fail(409, "DRAFT_REPLY_SOURCE_UNAVAILABLE", "Reply source changed before the holder committed")
	}
	var another bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message.draft_reply_holders WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND operation_id=$3)`, a.Config.DeploymentID, q.UserID, q.OperationID).Scan(&another); err != nil {
		return holder, err
	}
	if another {
		return holder, fail(409, "IDEMPOTENCY_CONFLICT", "This draft operation already prepared another source reference")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO message.draft_reply_holders(id,scope_id,source_context_id,user_id,conversation_id,draft_id,operation_id,source_message_id,source_content_version,membership_generation,security_version,restore_epoch,request_digest,state) VALUES($1,$2,'personal',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'prepared')`, q.HolderID, a.Config.DeploymentID, q.UserID, q.ConversationID, q.DraftID, q.OperationID, q.SourceID, q.SourceVersion, source.forwardMembership, source.forwardSecurity, source.forwardEpoch, digest); err != nil {
		return holder, err
	}
	if err = tx.Commit(ctx); err != nil {
		return holder, err
	}
	return draftReplyHolder{HolderID: q.HolderID, UserID: q.UserID, ConversationID: q.ConversationID, DraftID: q.DraftID, OperationID: q.OperationID, SourceID: q.SourceID, SourceVersion: q.SourceVersion, MembershipGeneration: source.forwardMembership, SecurityVersion: source.forwardSecurity, RestoreEpoch: source.forwardEpoch, State: "prepared"}, nil
}

func sameDraftReplyHolder(first, second draftReplyHolder) bool {
	first.State, second.State = "", ""
	return first == second
}

func (a *App) finishDraftReplyHolder(ctx context.Context, q draftReplyHolder, release bool) (draftReplyHolder, error) {
	var holder draftReplyHolder
	if !syncValidationID(q.HolderID) || !syncValidationID(q.UserID) {
		return holder, fail(400, "INVALID_DRAFT_HOLDER", "Original scoped holder identity required")
	}
	holder, err := scanDraftReplyHolder(a.DB.QueryRow(ctx, `SELECT `+draftReplyHolderColumns+` FROM message.draft_reply_holders WHERE scope_id=$1 AND source_context_id='personal' AND id=$2 AND user_id=$3`, a.Config.DeploymentID, q.HolderID, q.UserID))
	if err == pgx.ErrNoRows {
		return holder, fail(404, "DRAFT_REPLY_HOLDER_NOT_FOUND", "No original draft reference preparation exists")
	}
	if err != nil {
		return holder, err
	}
	if !sameDraftReplyHolder(q, holder) {
		return holder, fail(409, "IDEMPOTENCY_CONFLICT", "Original prepared draft qualification differs")
	}
	if !release {
		if holder.State == "revoking" || holder.State == "cleared" {
			return holder, fail(409, "DRAFT_REPLY_ADMISSION_CHANGED", "Prepared reply reference has been retired")
		}
		source, loadError := a.loadMessage(ctx, holder.SourceID, holder.UserID)
		if loadError != nil {
			return holder, loadError
		}
		if source.forwardUncertain {
			return holder, fail(503, "DRAFT_REPLY_QUALIFICATION_UNAVAILABLE", "Current reference qualification cannot be proved")
		}
		if source.Deleted || source.ContentUnavailable || source.ForwardUnavailable || source.sourcePending || source.ContentVersion != holder.SourceVersion || source.forwardMembership != holder.MembershipGeneration || source.forwardSecurity != holder.SecurityVersion || source.forwardEpoch != holder.RestoreEpoch {
			return holder, fail(409, "DRAFT_REPLY_ADMISSION_CHANGED", "Original reply qualification no longer applies")
		}
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return holder, err
	}
	defer tx.Rollback(ctx)
	locked, err := scanDraftReplyHolder(tx.QueryRow(ctx, `SELECT `+draftReplyHolderColumns+` FROM message.draft_reply_holders WHERE scope_id=$1 AND source_context_id='personal' AND id=$2 AND user_id=$3 FOR UPDATE`, a.Config.DeploymentID, q.HolderID, q.UserID))
	if err == pgx.ErrNoRows {
		return holder, fail(404, "DRAFT_REPLY_HOLDER_NOT_FOUND", "Original draft reference preparation is unavailable")
	}
	if err != nil {
		return holder, err
	}
	if !sameDraftReplyHolder(q, locked) {
		return holder, fail(409, "IDEMPOTENCY_CONFLICT", "Prepared reference changed before completion")
	}
	state := "active"
	if release {
		if locked.State == "revoking" {
			return holder, fail(409, "DRAFT_REPLY_REVOCATION_PENDING", "Original reply revocation must retain its callback responsibility")
		}
		state = "cleared"
	} else if locked.State == "revoking" || locked.State == "cleared" {
		return holder, fail(409, "DRAFT_REPLY_ADMISSION_CHANGED", "Prepared reference retired before publication")
	}
	if _, err = tx.Exec(ctx, `UPDATE message.draft_reply_holders SET state=$2,revocation_operation_id=NULL,updated_at=now() WHERE id=$1`, q.HolderID, state); err != nil {
		return holder, err
	}
	if err = tx.Commit(ctx); err != nil {
		return holder, err
	}
	locked.State = state
	return locked, nil
}
