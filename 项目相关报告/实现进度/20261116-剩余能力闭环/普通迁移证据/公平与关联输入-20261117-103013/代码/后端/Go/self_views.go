package jobsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func migrateSelfViews(ctx context.Context, tx pgx.Tx, scope string) error {
	if !syncValidationID(scope) {
		return fail(400, "INVALID_SCOPE", "Canonical instance scope required")
	}
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS sync.conversation_views(
id uuid NOT NULL UNIQUE,scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),
user_id uuid NOT NULL,conversation_id uuid NOT NULL,pinned_rank integer CHECK(pinned_rank BETWEEN 0 AND 999999),muted_until timestamptz,
archived boolean NOT NULL DEFAULT false,hidden boolean NOT NULL DEFAULT false,
manual_unread boolean NOT NULL DEFAULT false,manual_unread_generation bigint NOT NULL DEFAULT 0 CHECK(manual_unread_generation>=0),manual_unread_marked_at timestamptz,
manual_unread_from_epoch bigint,manual_unread_from_seq bigint,
revision_epoch bigint NOT NULL CHECK(revision_epoch>0),row_version bigint NOT NULL DEFAULT 0 CHECK(row_version>=0),
updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,source_context_id,user_id,conversation_id),
CHECK(manual_unread=(manual_unread_marked_at IS NOT NULL)),
CHECK((manual_unread_from_epoch IS NULL AND manual_unread_from_seq IS NULL) OR (manual_unread AND manual_unread_from_epoch>0 AND manual_unread_from_seq>0)));
CREATE TABLE IF NOT EXISTS sync.conversation_view_operations(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,conversation_id uuid NOT NULL,
revision_epoch bigint NOT NULL CHECK(revision_epoch>0),request_id text NOT NULL CHECK(octet_length(request_id) BETWEEN 8 AND 128),
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),result jsonb NOT NULL CHECK(jsonb_typeof(result)='object'),created_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,source_context_id,user_id,revision_epoch,request_id));`)
	return err
}

type selfView struct {
	ID                     string     `json:"id"`
	ScopeID                string     `json:"scopeId"`
	SourceContextID        string     `json:"sourceContextId"`
	UserID                 string     `json:"userId"`
	ConversationID         string     `json:"conversationId"`
	RevisionEpoch          string     `json:"revisionEpoch"`
	Version                string     `json:"version"`
	PinnedRank             *int       `json:"pinnedRank"`
	MutedUntil             *time.Time `json:"mutedUntil"`
	Archived               bool       `json:"archived"`
	Hidden                 bool       `json:"hidden"`
	ManualUnread           bool       `json:"manualUnread"`
	ManualUnreadGeneration string     `json:"manualUnreadGeneration"`
	ManualUnreadMarkedAt   *time.Time `json:"manualUnreadMarkedAt"`
	ManualUnreadFrom       *readPoint `json:"manualUnreadFrom"`
}

type selfViewChanges struct {
	PinnedRank                     json.RawMessage `json:"pinnedRank,omitempty"`
	MutedUntil                     json.RawMessage `json:"mutedUntil,omitempty"`
	Archived                       *bool           `json:"archived,omitempty"`
	Hidden                         *bool           `json:"hidden,omitempty"`
	ManualUnread                   *bool           `json:"manualUnread,omitempty"`
	ExpectedManualUnreadGeneration string          `json:"expectedManualUnreadGeneration,omitempty"`
	ManualUnreadFromMessageID      string          `json:"manualUnreadFromMessageId,omitempty"`
}

func (changes *selfViewChanges) UnmarshalJSON(raw []byte) error {
	type fields selfViewChanges
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*fields)(changes)); err != nil {
		return err
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(raw, &present); err != nil {
		return err
	}
	for _, field := range []string{"archived", "hidden", "manualUnread"} {
		if value, exists := present[field]; exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fail(400, "INVALID_VIEW_REQUEST", "Boolean private view fields cannot be null")
		}
	}
	return nil
}

type selfViewPatch struct {
	RequestID       string          `json:"requestId"`
	RevisionEpoch   string          `json:"revisionEpoch"`
	ExpectedVersion string          `json:"expectedVersion"`
	Changes         selfViewChanges `json:"changes"`
}

type selfViewOperation struct {
	OperationID string   `json:"operationId"`
	Status      string   `json:"status"`
	Current     bool     `json:"current"`
	Result      selfView `json:"result"`
}

func decodeSelfViewPatch(r *http.Request, patch *selfViewPatch) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(patch) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(400, "INVALID_VIEW_REQUEST", "One bounded private view request with supported fields required")
	}
	return nil
}

func validateSelfViewPatch(patch selfViewPatch) error {
	if len(patch.RequestID) < 8 || len(patch.RequestID) > 128 || !syncValidationCounter(patch.RevisionEpoch, false) || !syncValidationCounter(patch.ExpectedVersion, true) {
		return fail(400, "INVALID_VIEW_REQUEST", "Stable request and exact epoch/version strings required")
	}
	changes := patch.Changes
	if len(changes.PinnedRank) == 0 && len(changes.MutedUntil) == 0 && changes.Archived == nil && changes.Hidden == nil && changes.ManualUnread == nil {
		return fail(400, "INVALID_VIEW_REQUEST", "At least one private view field required")
	}
	if len(changes.PinnedRank) != 0 {
		var value *int
		if json.Unmarshal(changes.PinnedRank, &value) != nil || value != nil && (*value < 0 || *value > 999999) {
			return fail(400, "INVALID_PIN_RANK", "Pin rank must be null or an integer from 0 through 999999")
		}
	}
	if len(changes.MutedUntil) != 0 {
		var value *time.Time
		if json.Unmarshal(changes.MutedUntil, &value) != nil || value != nil && (value.IsZero() || value.Location() != time.UTC) {
			return fail(400, "INVALID_MUTE_TIME", "Mute deadline must be null or an explicit UTC timestamp")
		}
	}
	if changes.ManualUnread != nil {
		if !syncValidationCounter(changes.ExpectedManualUnreadGeneration, true) {
			return fail(400, "MANUAL_UNREAD_GENERATION_REQUIRED", "Capture the current independent manual-unread generation")
		}
	} else if changes.ExpectedManualUnreadGeneration != "" {
		return fail(400, "INVALID_VIEW_REQUEST", "Manual generation is only valid with an explicit marker action")
	}
	if changes.ManualUnreadFromMessageID != "" && (changes.ManualUnread == nil || !*changes.ManualUnread || !syncValidationID(changes.ManualUnreadFromMessageID)) {
		return fail(400, "INVALID_VIEW_REQUEST", "A visible message anchor is only valid when marking unread")
	}
	return nil
}

func (a *App) selfViewRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/conversations/{id}/view", func(w http.ResponseWriter, r *http.Request) error {
		u, err := a.syncUser(r)
		if err != nil {
			return err
		}
		view, found, err := a.selfViewRead(r.Context(), a.DB, u, r.PathValue("id"))
		if err != nil {
			return err
		}
		if !found {
			if _, err = a.member(r.Context(), u.ID, view.ConversationID); err != nil {
				return err
			}
		}
		respond(w, 200, view)
		return nil
	})
	for _, proofOnly := range []bool{false, true} {
		proof := proofOnly
		pattern := "PATCH /api/conversations/{id}/view"
		if proof {
			pattern = "POST /api/conversations/{id}/view-results"
		}
		h(pattern, func(w http.ResponseWriter, r *http.Request) error {
			u, err := a.syncUser(r)
			if err != nil {
				return err
			}
			var patch selfViewPatch
			if err = decodeSelfViewPatch(r, &patch); err != nil {
				return err
			}
			operation, err := a.selfViewApply(r.Context(), u, r.PathValue("id"), patch, proof)
			if err != nil {
				return err
			}
			respond(w, 200, operation)
			return nil
		})
	}
}

func (a *App) selfViewRead(ctx context.Context, db sourcePreferenceQuery, u User, conversation string) (selfView, bool, error) {
	view := selfView{ScopeID: a.Config.DeploymentID, SourceContextID: "personal", UserID: u.ID, ConversationID: conversation, RevisionEpoch: u.RestoreEpoch, Version: "0", ManualUnreadGeneration: "0"}
	if !syncValidationID(conversation) || !syncValidationID(u.ID) || !syncValidationCounter(u.RestoreEpoch, false) {
		return view, false, fail(400, "INVALID_VIEW_CONTEXT", "Current principal and conversation required")
	}
	view.ID = stableID(a.Config.Key, "personal-view:"+view.ScopeID+":"+u.ID+":"+conversation)
	var rowEpoch string
	var fromEpoch, fromSequence *int64
	err := db.QueryRow(ctx, `SELECT revision_epoch::text,row_version::text,pinned_rank,muted_until,archived,hidden,manual_unread,manual_unread_generation::text,manual_unread_marked_at,manual_unread_from_epoch,manual_unread_from_seq FROM sync.conversation_views WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3`, view.ScopeID, u.ID, conversation).Scan(&rowEpoch, &view.Version, &view.PinnedRank, &view.MutedUntil, &view.Archived, &view.Hidden, &view.ManualUnread, &view.ManualUnreadGeneration, &view.ManualUnreadMarkedAt, &fromEpoch, &fromSequence)
	if err == pgx.ErrNoRows {
		return view, false, nil
	}
	if err != nil {
		return view, false, err
	}
	if rowEpoch != u.RestoreEpoch {
		view.Version, view.ManualUnreadGeneration = "0", "0"
	}
	if fromEpoch != nil && fromSequence != nil {
		view.ManualUnreadFrom = &readPoint{Epoch: str(*fromEpoch), Sequence: str(*fromSequence)}
	}
	return view, true, nil
}

func (a *App) selfViewFind(ctx context.Context, tx pgx.Tx, u User, conversation string, patch selfViewPatch, digest []byte) (selfViewOperation, bool, error) {
	operation := selfViewOperation{OperationID: patch.RequestID, Status: "applied"}
	var original, result []byte
	var originalConversation string
	err := tx.QueryRow(ctx, `SELECT conversation_id::text,request_digest,result FROM sync.conversation_view_operations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND revision_epoch=$3 AND request_id=$4`, a.Config.DeploymentID, u.ID, patch.RevisionEpoch, patch.RequestID).Scan(&originalConversation, &original, &result)
	if err == pgx.ErrNoRows {
		return operation, false, nil
	}
	if err != nil {
		return operation, false, err
	}
	if originalConversation != conversation || !hmac.Equal(original, digest) {
		return operation, true, fail(409, "IDEMPOTENCY_CONFLICT", "Original private view request differs")
	}
	if err = json.Unmarshal(result, &operation.Result); err != nil {
		return operation, true, err
	}
	current, _, err := a.selfViewRead(ctx, tx, u, conversation)
	operation.Current = err == nil && operation.Result.RevisionEpoch == current.RevisionEpoch && operation.Result.Version == current.Version
	return operation, true, err
}

func (a *App) selfViewApply(ctx context.Context, u User, conversation string, patch selfViewPatch, proof bool) (selfViewOperation, error) {
	var operation selfViewOperation
	if err := validateSelfViewPatch(patch); err != nil {
		return operation, err
	}
	view, found, err := a.selfViewRead(ctx, a.DB, u, conversation)
	if err != nil {
		return operation, err
	}
	var manualAnchor *readPoint
	raw, err := json.Marshal(patch)
	if err != nil {
		return operation, err
	}
	digest := keyedDigest(a.Config.Key, "personal-view-v1:"+a.Config.DeploymentID+":"+u.ID+":"+conversation, raw)
	options := pgx.TxOptions{}
	if proof {
		options = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	}
	tx, err := a.DB.BeginTx(ctx, options)
	if err != nil {
		return operation, err
	}
	defer tx.Rollback(ctx)
	if !proof {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "personal-view-operation:"+a.Config.DeploymentID+":"+u.ID+":"+patch.RevisionEpoch+":"+patch.RequestID); err != nil {
			return operation, err
		}
	}
	var epoch string
	if err = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM sync.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch); err != nil {
		return operation, err
	}
	if epoch != u.RestoreEpoch {
		return operation, fail(409, "RESTORE_EPOCH_CHANGED", "Authenticated and view-owner epochs do not agree")
	}
	if original, found, lookupError := a.selfViewFind(ctx, tx, u, conversation, patch, digest); found || lookupError != nil {
		return original, lookupError
	}
	if proof {
		return operation, fail(404, "VIEW_OPERATION_NOT_FOUND", "No original private view operation exists")
	}
	if patch.RevisionEpoch != epoch {
		return operation, fail(409, "RESTORE_EPOCH_CHANGED", "An old uncommitted view request cannot mutate the restored owner")
	}
	// Retained private metadata may be managed after leaving. Creating a row
	// still requires current membership and never grants cloud-history access.
	if !found {
		if _, err = a.member(ctx, u.ID, conversation); err != nil {
			return operation, err
		}
	}
	if patch.Changes.ManualUnreadFromMessageID != "" {
		var boundary readPointBoundaryResponse
		request := readPointRequest{UserID: u.ID, ConversationID: conversation, MessageIDs: []string{patch.Changes.ManualUnreadFromMessageID}, After: readPoint{"0", "0"}, Through: readPoint{"0", "0"}}
		if err = a.rpc(ctx, "message", "/internal/read-boundaries", request, &boundary); err != nil {
			return operation, err
		}
		if boundary.RestoreEpoch != u.RestoreEpoch || len(boundary.Points) != 1 {
			return operation, fail(409, "RESTORE_EPOCH_CHANGED", "Current message creation boundary unavailable")
		}
		if boundary.Points[0].OrderKnown {
			manualAnchor = &readPoint{Epoch: boundary.Points[0].CreationEpoch, Sequence: boundary.Points[0].CreationSequence}
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sync.conversation_views(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch) VALUES($1,$2,'personal',$3,$4,$5) ON CONFLICT DO NOTHING`, view.ID, view.ScopeID, u.ID, conversation, epoch); err != nil {
		return operation, err
	}
	if err = tx.QueryRow(ctx, `SELECT id::text FROM sync.conversation_views WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 FOR UPDATE`, view.ScopeID, u.ID, conversation).Scan(&view.ID); err != nil {
		return operation, err
	}
	if original, found, lookupError := a.selfViewFind(ctx, tx, u, conversation, patch, digest); found || lookupError != nil {
		return original, lookupError
	}
	view, _, err = a.selfViewRead(ctx, tx, u, conversation)
	if err != nil {
		return operation, err
	}
	version, _ := sourcePreferenceCounter(view.Version, true)
	if view.Version != patch.ExpectedVersion || version == math.MaxInt64 {
		return operation, fail(409, "VIEW_VERSION_CONFLICT", "Reload the current private view epoch and version")
	}
	changes := patch.Changes
	if len(changes.PinnedRank) != 0 {
		_ = json.Unmarshal(changes.PinnedRank, &view.PinnedRank)
	}
	if len(changes.MutedUntil) != 0 {
		_ = json.Unmarshal(changes.MutedUntil, &view.MutedUntil)
	}
	if changes.Archived != nil {
		view.Archived = *changes.Archived
	}
	if changes.Hidden != nil {
		view.Hidden = *changes.Hidden
	}
	if changes.ManualUnread != nil {
		generation, _ := sourcePreferenceCounter(view.ManualUnreadGeneration, true)
		if changes.ExpectedManualUnreadGeneration != view.ManualUnreadGeneration || generation == math.MaxInt64 {
			return operation, fail(409, "MANUAL_UNREAD_CHANGED", "The current manual unread marker belongs to a different intent")
		}
		view.ManualUnread, view.ManualUnreadGeneration = *changes.ManualUnread, str(generation+1)
		view.ManualUnreadMarkedAt = nil
		view.ManualUnreadFrom = nil
		if view.ManualUnread {
			now := time.Now().UTC()
			view.ManualUnreadMarkedAt = &now
			view.ManualUnreadFrom = manualAnchor
		}
	}
	view.Version, view.RevisionEpoch = str(version+1), epoch
	var fromEpoch, fromSequence *string
	if view.ManualUnreadFrom != nil {
		fromEpoch, fromSequence = &view.ManualUnreadFrom.Epoch, &view.ManualUnreadFrom.Sequence
	}
	if _, err = tx.Exec(ctx, `UPDATE sync.conversation_views SET pinned_rank=$4,muted_until=$5,archived=$6,hidden=$7,manual_unread=$8,manual_unread_generation=$9,manual_unread_marked_at=$10,revision_epoch=$11,row_version=$12,manual_unread_from_epoch=$13,manual_unread_from_seq=$14,updated_at=now() WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3`, view.ScopeID, u.ID, conversation, view.PinnedRank, view.MutedUntil, view.Archived, view.Hidden, view.ManualUnread, view.ManualUnreadGeneration, view.ManualUnreadMarkedAt, epoch, view.Version, fromEpoch, fromSequence); err != nil {
		return operation, err
	}
	result, err := json.Marshal(view)
	if err != nil {
		return operation, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sync.conversation_view_operations(scope_id,source_context_id,user_id,conversation_id,revision_epoch,request_id,request_digest,result) VALUES($1,'personal',$2,$3,$4,$5,$6,$7)`, view.ScopeID, u.ID, conversation, epoch, patch.RequestID, digest, result); err != nil {
		return operation, err
	}
	if err = tx.Commit(ctx); err != nil {
		return operation, err
	}
	return selfViewOperation{OperationID: patch.RequestID, Status: "applied", Current: true, Result: view}, nil
}
