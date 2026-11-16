package jobsim

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func (a *App) selfReadLiveSummary(ctx context.Context, u User, conversation string, membership readPointMembership) (selfReadView, error) {
	var view selfReadView
	for attempt := 0; attempt < 3; attempt++ {
		result, err := a.selfReadSummaryAttempt(ctx, u, conversation, membership)
		var changed *apiError
		if !errors.As(err, &changed) || changed.Code != "READ_SUMMARY_CHANGED" {
			return result, err
		}
		view = result
	}
	return view, fail(503, "READ_SUMMARY_BUSY", "Current source changed during unread reconciliation; retain the previous count as unconfirmed")
}

// A read-only local snapshot fixes observed IDs. Source counts and every ID
// intersection must retain one committed message watermark; no body is copied.
func (a *App) selfReadSummaryAttempt(ctx context.Context, u User, conversation string, membership readPointMembership) (selfReadView, error) {
	var view selfReadView
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return view, err
	}
	defer tx.Rollback(ctx)
	state, err := a.loadSelfReadState(ctx, tx, u, conversation)
	if err != nil {
		return view, err
	}
	view = a.selfReadView(u, conversation, membership, state)
	request := readSummaryRequest{UserID: u.ID, ConversationID: conversation, After: state.Point, ObservedIDs: []string{}}
	var first readSummaryResponse
	if err = a.rpc(ctx, "message", "/internal/read-summary", request, &first); err != nil {
		return view, err
	}
	validSource := func(response readSummaryResponse) bool {
		return response.RestoreEpoch == u.RestoreEpoch && response.MembershipGeneration == membership.Generation && response.HistoryFloor == membership.HistoryFloor && response.Watermark == first.Watermark && response.EligibleTotal == first.EligibleTotal && response.UnorderedTotal == first.UnorderedTotal
	}
	if !validSource(first) || !syncValidationCounter(first.Watermark, true) || !syncValidationCounter(first.EligibleTotal, true) || !syncValidationCounter(first.UnorderedTotal, true) {
		return view, fail(503, "READ_SUMMARY_INTEGRITY_FAILED", "Current unread source metadata does not agree")
	}
	request.ExpectedWatermark = first.Watermark
	var observed int64
	var afterID any
	for examined := 0; ; {
		rows, queryError := tx.Query(ctx, `SELECT message_id::text FROM sync.read_observations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 AND membership_generation=$4 AND ($5::uuid IS NULL OR message_id>$5::uuid) AND (NOT order_known OR (creation_epoch,creation_sequence)>($6::bigint,$7::bigint)) ORDER BY message_id LIMIT 200`, a.Config.DeploymentID, u.ID, conversation, membership.Generation, afterID, state.Point.Epoch, state.Point.Sequence)
		if queryError != nil {
			return view, queryError
		}
		batch := []string{}
		allowed := map[string]bool{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return view, err
			}
			batch = append(batch, id)
			allowed[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return view, err
		}
		if len(batch) == 0 {
			break
		}
		examined += len(batch)
		if examined > 20000 {
			return view, fail(503, "READ_CAPACITY_EXCEEDED", "Sparse observation reconciliation exceeds the basic resource bound; no approximate count was published")
		}
		request.ObservedIDs = batch
		var current readSummaryResponse
		if err = a.rpc(ctx, "message", "/internal/read-summary", request, &current); err != nil {
			return view, err
		}
		if !validSource(current) {
			return view, fail(409, "READ_SUMMARY_CHANGED", "Source metadata changed between observation batches")
		}
		seen := map[string]bool{}
		for _, id := range current.ObservedEligibleIDs {
			if !allowed[id] || seen[id] {
				return view, fail(503, "READ_SUMMARY_INTEGRITY_FAILED", "Source returned duplicate or unrequested observation identities")
			}
			seen[id] = true
			observed++
		}
		afterID = batch[len(batch)-1]
		if len(batch) < 200 {
			break
		}
	}
	request.ObservedIDs = []string{}
	var final readSummaryResponse
	if err = a.rpc(ctx, "message", "/internal/read-summary", request, &final); err != nil {
		return view, err
	}
	if !validSource(final) {
		return view, fail(409, "READ_SUMMARY_CHANGED", "Source changed before unread reconciliation finished")
	}
	total, _ := strconv.ParseInt(first.EligibleTotal, 10, 64)
	if observed > total {
		return view, fail(503, "READ_SUMMARY_INTEGRITY_FAILED", "Observed messages cannot exceed the current eligible set")
	}
	count := str(total - observed)
	view.UnreadCount, view.CountSourceWatermark, view.CountStatus = &count, first.Watermark, "exact_current_source"
	return view, nil
}
