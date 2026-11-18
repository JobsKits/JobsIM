package jobsim

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestUnreadSummaryCannotSelectArbitrarySourceOrInexactPrefix(t *testing.T) {
	request := readSummaryRequest{UserID: newID(), ConversationID: newID(), After: readPoint{"2", "9007199254740993"}, ExpectedWatermark: "9223372036854775807"}
	if normalizeReadSummaryRequest(request) != nil {
		t.Fatal("exact creation tuple was truncated or rejected")
	}
	for _, bad := range []string{"01", "-1", "9223372036854775808"} {
		changed := request
		changed.ExpectedWatermark = bad
		if normalizeReadSummaryRequest(changed) == nil {
			t.Fatal("inexact source watermark accepted")
		}
	}
	request.ObservedIDs = []string{newID()}
	request.ObservedIDs = append(request.ObservedIDs, request.ObservedIDs[0])
	if normalizeReadSummaryRequest(request) == nil {
		t.Fatal("duplicate observed IDs could subtract unread twice")
	}
}

func readSummaryTestQuery(ctx context.Context, realm readPointTestRealm, request readSummaryRequest) (readSummaryResponse, error) {
	tx, err := realm.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return readSummaryResponse{}, err
	}
	defer tx.Rollback(ctx)
	return realm.app.readSummaryMetadata(ctx, tx, request, realm.membership)
}

// The existing gated helper creates a new metadata-only database. This proves
// SQL counting semantics later; it is not live owner RPC/auth or a receipt test.
func TestUnreadSummaryCountsCurrentEligibleIDsAndPreservesHoles(t *testing.T) {
	ctx, realm := newReadPointTestRealm(t)
	if _, err := realm.db.Exec(ctx, `CREATE TABLE message.sync_clock(singleton boolean PRIMARY KEY CHECK(singleton),revision bigint NOT NULL);INSERT INTO message.sync_clock VALUES(true,5)`); err != nil {
		t.Fatal("owned message metadata clock could not be created")
	}
	joined := *realm.membership.JoinedAt
	ten := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(2), int64(10), joined.Add(time.Second), false, 900)
	eleven := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(2), int64(11), joined.Add(2*time.Second), false, 902)
	own := readPointTestInsert(t, ctx, realm, realm.conv, realm.user, int64(2), int64(12), joined.Add(3*time.Second), false, 903)
	deleted := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(2), int64(13), joined.Add(4*time.Second), true, 904)
	prejoin := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(2), int64(14), joined.Add(-time.Second), false, 905)
	foreign := readPointTestInsert(t, ctx, realm, realm.otherConv, realm.peer, int64(2), int64(15), joined.Add(5*time.Second), false, 906)
	legacy := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, nil, nil, joined.Add(6*time.Second), false, 907)
	request := readSummaryRequest{UserID: realm.user, ConversationID: realm.conv, After: readPoint{"2", "10"}, ExpectedWatermark: "5", ObservedIDs: []string{ten, eleven, own, deleted, prejoin, foreign, legacy}}
	response, err := readSummaryTestQuery(ctx, realm, request)
	if err != nil || response.EligibleTotal != "2" || response.UnorderedTotal != "1" || len(response.ObservedEligibleIDs) != 2 || response.MembershipGeneration != "7" || response.RestoreEpoch != "2" {
		t.Fatal("current unread set used mutable edit sequence, own/deleted/prejoin/foreign rows or fabricated legacy order")
	}
	seen := map[string]bool{}
	for _, id := range response.ObservedEligibleIDs {
		seen[id] = true
	}
	if !seen[eleven] || !seen[legacy] {
		t.Fatal("eligible observed ID intersection was not authoritative")
	}
	request.After, request.ObservedIDs = readPoint{"0", "0"}, []string{eleven}
	response, err = readSummaryTestQuery(ctx, realm, request)
	if err != nil || response.EligibleTotal != "3" || len(response.ObservedEligibleIDs) != 1 {
		t.Fatal("out-of-order eleven hid unobserved ten")
	}
	if _, err = realm.db.Exec(ctx, `UPDATE message.sync_clock SET revision=6`); err != nil {
		t.Fatal("owned watermark update failed")
	}
	_, err = readSummaryTestQuery(ctx, realm, request)
	readPointTestError(t, err, 409, "READ_SUMMARY_CHANGED")
	request.ExpectedWatermark = "6"
	_ = readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(0), int64(1), joined.Add(7*time.Second), false, 908)
	_, err = readSummaryTestQuery(ctx, realm, request)
	readPointTestError(t, err, 503, "READ_METADATA_INTEGRITY_FAILED")
}
