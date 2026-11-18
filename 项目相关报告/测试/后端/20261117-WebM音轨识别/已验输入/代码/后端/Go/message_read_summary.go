package jobsim

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
)

type readSummaryRequest struct {
	UserID            string    `json:"userId"`
	ConversationID    string    `json:"conversationId"`
	After             readPoint `json:"after"`
	ObservedIDs       []string  `json:"observedMessageIds"`
	ExpectedWatermark string    `json:"expectedWatermark,omitempty"`
}

type readSummaryResponse struct {
	RestoreEpoch         string   `json:"restoreEpoch"`
	MembershipGeneration string   `json:"membershipGeneration"`
	HistoryFloor         string   `json:"historyFloor"`
	Watermark            string   `json:"watermark"`
	EligibleTotal        string   `json:"eligibleTotal"`
	UnorderedTotal       string   `json:"unorderedTotal"`
	ObservedEligibleIDs  []string `json:"observedEligibleIds"`
}

func normalizeReadSummaryRequest(request readSummaryRequest) error {
	_, err := normalizeReadPointRequest(readPointRequest{UserID: request.UserID, ConversationID: request.ConversationID, MessageIDs: request.ObservedIDs, After: request.After, Through: readPoint{"0", "0"}})
	if err != nil {
		return err
	}
	if request.ExpectedWatermark != "" && !syncValidationCounter(request.ExpectedWatermark, true) {
		return invalidReadPoint()
	}
	return nil
}

func (a *App) readSummaryMessageRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/read-summary", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "sync" {
			return fail(403, "SYNC_SERVICE_REQUIRED", "Personal unread metadata is private")
		}
		var request readSummaryRequest
		if err := decode(r, &request); err != nil {
			return err
		}
		if err := normalizeReadSummaryRequest(request); err != nil {
			return err
		}
		metadata, err := a.member(r.Context(), request.UserID, request.ConversationID)
		if err != nil {
			return err
		}
		membership, err := readPointMembershipFromMetadata(metadata)
		if err != nil {
			return err
		}
		tx, err := a.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			return err
		}
		defer tx.Rollback(r.Context())
		response, err := a.readSummaryMetadata(r.Context(), tx, request, membership)
		if err != nil {
			return err
		}
		respond(w, 200, response)
		return nil
	}))
}

func (a *App) readSummaryMetadata(ctx context.Context, tx pgx.Tx, request readSummaryRequest, membership readPointMembership) (readSummaryResponse, error) {
	response := readSummaryResponse{MembershipGeneration: membership.Generation, HistoryFloor: membership.HistoryFloor, ObservedEligibleIDs: []string{}}
	if err := normalizeReadSummaryRequest(request); err != nil {
		return response, err
	}
	if err := tx.QueryRow(ctx, `SELECT i.restore_epoch::text,c.revision::text FROM message.instance i CROSS JOIN message.sync_clock c WHERE i.singleton AND c.singleton AND i.deployment_id=$1`, a.Config.DeploymentID).Scan(&response.RestoreEpoch, &response.Watermark); err != nil {
		return response, err
	}
	if request.ExpectedWatermark != "" && request.ExpectedWatermark != response.Watermark {
		return response, fail(409, "READ_SUMMARY_CHANGED", "Message metadata changed during unread reconciliation")
	}
	afterEpoch, afterSequence, _ := parseReadPoint(request.After)
	currentEpoch, _ := sourcePreferenceCounter(response.RestoreEpoch, false)
	if afterEpoch > currentEpoch {
		return response, fail(409, "RESTORE_EPOCH_CHANGED", "Read prefix belongs to a later creation epoch")
	}
	var malformed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message.messages WHERE conversation_id=$1 AND sender_id<>$2 AND NOT deleted AND ($3::timestamptz IS NULL OR created_at>=$3) AND (sequence_epoch<1 OR sequence_epoch>$4 OR creation_sequence<1))`, request.ConversationID, request.UserID, membership.JoinedAt, currentEpoch).Scan(&malformed); err != nil {
		return response, err
	}
	if malformed {
		return response, fail(503, "READ_METADATA_INTEGRITY_FAILED", "Creation metadata cannot certify an unread count")
	}
	const eligible = `conversation_id=$1 AND sender_id<>$2 AND NOT deleted AND ($3::timestamptz IS NULL OR created_at>=$3)
AND (sequence_epoch IS NULL OR creation_sequence IS NULL OR (sequence_epoch,creation_sequence)>($4::bigint,$5::bigint))`
	if err := tx.QueryRow(ctx, `SELECT count(*)::text,count(*) FILTER(WHERE sequence_epoch IS NULL OR creation_sequence IS NULL)::text
FROM message.messages WHERE `+eligible, request.ConversationID, request.UserID, membership.JoinedAt, afterEpoch, afterSequence).Scan(&response.EligibleTotal, &response.UnorderedTotal); err != nil {
		return response, err
	}
	if len(request.ObservedIDs) == 0 {
		return response, nil
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM message.messages WHERE `+eligible+` AND id=ANY($6::uuid[]) ORDER BY id`, request.ConversationID, request.UserID, membership.JoinedAt, afterEpoch, afterSequence, request.ObservedIDs)
	if err != nil {
		return response, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return response, err
		}
		response.ObservedEligibleIDs = append(response.ObservedEligibleIDs, id)
	}
	return response, rows.Err()
}
