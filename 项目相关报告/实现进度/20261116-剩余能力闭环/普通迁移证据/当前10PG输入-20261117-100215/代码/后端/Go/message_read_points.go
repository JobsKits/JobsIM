package jobsim

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type readPoint struct {
	Epoch    string `json:"epoch"`
	Sequence string `json:"sequence"`
}

type readMessagePoint struct {
	MessageID        string `json:"messageId"`
	CreationEpoch    string `json:"creationEpoch"`
	CreationSequence string `json:"creationSequence"`
	SenderID         string `json:"senderId"`
	Deleted          bool   `json:"deleted"`
	OrderKnown       bool   `json:"orderKnown"`
}

type readPointRequest struct {
	UserID         string    `json:"userId"`
	ConversationID string    `json:"conversationId"`
	MessageIDs     []string  `json:"messageIds"`
	After          readPoint `json:"after"`
	Through        readPoint `json:"through"`
	Limit          int       `json:"limit"`
}

type readPointBoundaryResponse struct {
	RestoreEpoch         string             `json:"restoreEpoch"`
	MembershipGeneration string             `json:"membershipGeneration"`
	HistoryFloor         string             `json:"historyFloor"`
	Points               []readMessagePoint `json:"points"`
	NextEligible         []readMessagePoint `json:"nextEligible"`
	HasMore              bool               `json:"hasMore"`
	HasUnorderedEligible bool               `json:"hasUnorderedEligible"`
}

type readPointMembership struct {
	Generation   string
	HistoryFloor string
	JoinedAt     *time.Time
}

type readPointMessageQuery interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func invalidReadPoint() error {
	return fail(400, "INVALID_READ_POINT", "Valid message identifiers and exact observed creation points required")
}

func parseReadPoint(point readPoint) (int64, int64, error) {
	epoch, epochError := strconv.ParseInt(point.Epoch, 10, 64)
	sequence, sequenceError := strconv.ParseInt(point.Sequence, 10, 64)
	if epochError != nil || sequenceError != nil || str(epoch) != point.Epoch || str(sequence) != point.Sequence || epoch < 0 || sequence < 0 || (epoch == 0) != (sequence == 0) {
		return 0, 0, invalidReadPoint()
	}
	return epoch, sequence, nil
}

func normalizeReadPointRequest(request readPointRequest) (readPointRequest, error) {
	validID := func(value string) bool { return validObjectID(value) && strings.ToLower(value) == value }
	if !validID(request.UserID) || !validID(request.ConversationID) || len(request.MessageIDs) > 200 || request.Limit < 0 || request.Limit > 200 {
		return request, invalidReadPoint()
	}
	seen := make(map[string]bool, len(request.MessageIDs))
	for _, id := range request.MessageIDs {
		if !validID(id) || seen[id] {
			return request, invalidReadPoint()
		}
		seen[id] = true
	}
	afterEpoch, afterSequence, err := parseReadPoint(request.After)
	if err != nil {
		return request, err
	}
	throughEpoch, throughSequence, err := parseReadPoint(request.Through)
	if err != nil || throughEpoch != 0 && (afterEpoch > throughEpoch || afterEpoch == throughEpoch && afterSequence > throughSequence) {
		return request, invalidReadPoint()
	}
	if request.Limit == 0 {
		request.Limit = 200
	}
	return request, nil
}

func readPointMembershipFromMetadata(metadata map[string]any) (readPointMembership, error) {
	var membership readPointMembership
	generation, generationOK := metadata["membershipGeneration"].(string)
	number, err := strconv.ParseInt(generation, 10, 64)
	if !generationOK || err != nil || number < 1 || str(number) != generation {
		return membership, invalidReadPoint()
	}
	membership.Generation = generation
	switch metadata["kind"] {
	case "direct", "self":
		return membership, nil
	case "group":
		raw, ok := metadata["joinedAt"].(string)
		joined, err := time.Parse(time.RFC3339Nano, raw)
		if !ok || err != nil || joined.IsZero() {
			return membership, invalidReadPoint()
		}
		joined = joined.UTC()
		membership.JoinedAt = &joined
		membership.HistoryFloor = joined.Format(time.RFC3339Nano)
		return membership, nil
	default:
		return membership, invalidReadPoint()
	}
}

// The shared internal middleware authenticates the service key, and the outer
// securityHeaders wrapper supplies the canonical StateDir admission proof.
func (a *App) readPointMessageRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/read-boundaries", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "sync" {
			return fail(403, "SYNC_SERVICE_REQUIRED", "Personal read boundaries are private to synchronization")
		}
		var request readPointRequest
		decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			return invalidReadPoint()
		}
		request, err := normalizeReadPointRequest(request)
		if err != nil {
			return err
		}
		metadata, err := a.member(r.Context(), request.UserID, request.ConversationID)
		if err != nil {
			var unavailable *apiError
			if errors.As(err, &unavailable) && unavailable.Status == 403 {
				return fail(403, "READ_POINT_NOT_VISIBLE", "Current message visibility required")
			}
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
		response, err := a.readPointMetadata(r.Context(), tx, request, membership)
		if err != nil {
			return err
		}
		respond(w, 200, response)
		return nil
	}))
}

func scanReadMessagePoint(row pgx.Row, currentEpoch int64) (readMessagePoint, error) {
	point := readMessagePoint{CreationEpoch: "unknown", CreationSequence: "unknown"}
	var epoch, sequence *int64
	err := row.Scan(&point.MessageID, &point.SenderID, &epoch, &sequence, &point.Deleted)
	if err != nil {
		return point, err
	}
	if epoch != nil && (*epoch < 1 || *epoch > currentEpoch) || sequence != nil && *sequence < 1 {
		return point, invalidReadPoint()
	}
	if epoch != nil && sequence != nil {
		point.CreationEpoch, point.CreationSequence = str(*epoch), str(*sequence)
		point.OrderKnown = true
	}
	return point, nil
}

// This helper queries only message-owned metadata. No mutable edit sequence,
// text, attachment, acknowledgement or read-state column participates.
func (a *App) readPointMetadata(ctx context.Context, db readPointMessageQuery, request readPointRequest, membership readPointMembership) (readPointBoundaryResponse, error) {
	response := readPointBoundaryResponse{MembershipGeneration: membership.Generation, HistoryFloor: membership.HistoryFloor, Points: []readMessagePoint{}, NextEligible: []readMessagePoint{}}
	request, err := normalizeReadPointRequest(request)
	if err != nil {
		return response, err
	}
	var currentEpoch int64
	if err = db.QueryRow(ctx, "SELECT restore_epoch FROM message.instance WHERE singleton AND deployment_id=$1", a.Config.DeploymentID).Scan(&currentEpoch); err != nil {
		return response, err
	}
	if currentEpoch < 1 {
		return response, invalidReadPoint()
	}
	response.RestoreEpoch = str(currentEpoch)
	afterEpoch, afterSequence, _ := parseReadPoint(request.After)
	throughEpoch, throughSequence, _ := parseReadPoint(request.Through)
	if afterEpoch > currentEpoch || throughEpoch > currentEpoch {
		return response, invalidReadPoint()
	}
	if len(request.MessageIDs) != 0 {
		var visible int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM message.messages WHERE id=ANY($1::uuid[]) AND conversation_id=$2 AND ($3::timestamptz IS NULL OR created_at>=$3)`, request.MessageIDs, request.ConversationID, membership.JoinedAt).Scan(&visible); err != nil {
			return response, err
		}
		if visible != len(request.MessageIDs) {
			return response, fail(403, "READ_POINT_NOT_VISIBLE", "Every observed message must be currently visible in this conversation")
		}
		rows, err := db.Query(ctx, `SELECT m.id::text,m.sender_id::text,m.sequence_epoch,m.creation_sequence,m.deleted
FROM unnest($1::uuid[]) WITH ORDINALITY requested(id,position) JOIN message.messages m ON m.id=requested.id
WHERE m.conversation_id=$2 AND ($3::timestamptz IS NULL OR m.created_at>=$3)
ORDER BY requested.position`, request.MessageIDs, request.ConversationID, membership.JoinedAt)
		if err != nil {
			return response, err
		}
		for rows.Next() {
			point, scanError := scanReadMessagePoint(rows, currentEpoch)
			if scanError != nil {
				rows.Close()
				return response, scanError
			}
			response.Points = append(response.Points, point)
			if !point.OrderKnown && !point.Deleted && point.SenderID != request.UserID {
				response.HasUnorderedEligible = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return response, err
		}
		if len(response.Points) != len(request.MessageIDs) {
			return response, fail(403, "READ_POINT_NOT_VISIBLE", "Every observed message must be currently visible in this conversation")
		}
	}
	if throughEpoch == 0 {
		return response, nil
	}
	var genuine bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message.messages WHERE conversation_id=$1 AND ($2::timestamptz IS NULL OR created_at>=$2) AND sequence_epoch=$3 AND creation_sequence=$4)`, request.ConversationID, membership.JoinedAt, throughEpoch, throughSequence).Scan(&genuine); err != nil {
		return response, err
	}
	if !genuine {
		return response, invalidReadPoint()
	}
	var malformed bool
	if err = db.QueryRow(ctx, `SELECT
EXISTS(SELECT 1 FROM message.messages WHERE conversation_id=$1 AND sender_id<>$2 AND NOT deleted AND ($3::timestamptz IS NULL OR created_at>=$3) AND (sequence_epoch IS NULL OR creation_sequence IS NULL)),
EXISTS(SELECT 1 FROM message.messages WHERE conversation_id=$1 AND sender_id<>$2 AND NOT deleted AND ($3::timestamptz IS NULL OR created_at>=$3) AND (sequence_epoch<1 OR sequence_epoch>$4 OR creation_sequence<1))`, request.ConversationID, request.UserID, membership.JoinedAt, currentEpoch).Scan(&response.HasUnorderedEligible, &malformed); err != nil {
		return response, err
	}
	if malformed {
		return response, invalidReadPoint()
	}
	rows, err := db.Query(ctx, `SELECT m.id::text,m.sender_id::text,m.sequence_epoch,m.creation_sequence,m.deleted FROM message.messages m
WHERE m.conversation_id=$1 AND m.sender_id<>$2 AND NOT m.deleted AND ($3::timestamptz IS NULL OR m.created_at>=$3)
AND m.sequence_epoch IS NOT NULL AND m.creation_sequence IS NOT NULL
AND (m.sequence_epoch,m.creation_sequence)>($4::bigint,$5::bigint) AND (m.sequence_epoch,m.creation_sequence)<=($6::bigint,$7::bigint)
ORDER BY m.sequence_epoch,m.creation_sequence,m.id LIMIT $8`, request.ConversationID, request.UserID, membership.JoinedAt, afterEpoch, afterSequence, throughEpoch, throughSequence, request.Limit+1)
	if err != nil {
		return response, err
	}
	defer rows.Close()
	for rows.Next() {
		point, scanError := scanReadMessagePoint(rows, currentEpoch)
		if scanError != nil {
			return response, scanError
		}
		response.NextEligible = append(response.NextEligible, point)
	}
	if err = rows.Err(); err != nil {
		return response, err
	}
	response.HasMore = len(response.NextEligible) > request.Limit
	if response.HasMore {
		response.NextEligible = response.NextEligible[:request.Limit]
	}
	return response, nil
}
