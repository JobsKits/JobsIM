package jobsim

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func selfReadTestPoint(id, sender, epoch, sequence string) readMessagePoint {
	return readMessagePoint{MessageID: id, SenderID: sender, CreationEpoch: epoch, CreationSequence: sequence, OrderKnown: true}
}

func selfReadTestError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var rejection *apiError
	if !errors.As(err, &rejection) || rejection.Status != status || rejection.Code != code {
		t.Fatal("personal read operation did not produce its required rejection; details redacted")
	}
}

func TestSelfReadPrefixRetainsHolesAndExactCreationOrder(t *testing.T) {
	peer, ten, eleven := newID(), newID(), newID()
	eligible := []readMessagePoint{selfReadTestPoint(ten, peer, "1", "10"), selfReadTestPoint(eleven, peer, "1", "11")}
	zero := readPoint{"0", "0"}
	observed := map[string]bool{eleven: true}
	point, advanced, err := mergeSelfReadPrefix(zero, eligible, observed, false)
	if err != nil || point != zero || advanced != 0 {
		t.Fatal("out-of-order eleven crossed its unobserved eligible predecessor ten")
	}
	observed[ten] = true
	point, advanced, err = mergeSelfReadPrefix(zero, eligible, observed, false)
	if err != nil || point != (readPoint{"1", "11"}) || advanced != 2 {
		t.Fatal("observed predecessor did not release the stored later observation")
	}
	point, _, err = mergeSelfReadPrefix(zero, eligible, observed, true)
	if err != nil || point != zero {
		t.Fatal("unordered legacy candidate was fabricated as a proven continuous tuple")
	}
	large := selfReadTestPoint(newID(), peer, "2", "9007199254740993")
	point, _, err = mergeSelfReadPrefix(readPoint{"1", "9223372036854775807"}, []readMessagePoint{large}, map[string]bool{large.MessageID: true}, false)
	if err != nil || point != (readPoint{"2", "9007199254740993"}) {
		t.Fatal("creation epochs or sequence precision were compared as mutable/textual order")
	}
	_, _, err = mergeSelfReadPrefix(zero, []readMessagePoint{eligible[1], eligible[0]}, observed, false)
	if err == nil {
		t.Fatal("unordered owner metadata was accepted as a consecutive page")
	}
}

func TestSelfReadStrictOriginalBodyAndNoSourceOverride(t *testing.T) {
	for _, body := range []string{
		`{"requestId":"12345678","revisionEpoch":"1","membershipGeneration":"7"}`,
		`{"requestId":"12345678","revisionEpoch":1,"membershipGeneration":"7","observedMessageIds":[]}`,
		`{"requestId":"12345678","revisionEpoch":"1","membershipGeneration":"7","observedMessageIds":[],"sourceContextId":"foreign"}`,
		`{"requestId":"12345678","revisionEpoch":"1","membershipGeneration":"7","observedMessageIds":[],"manualUnread":true}`,
		`{"requestId":"12345678","revisionEpoch":"1","membershipGeneration":"7","observedMessageIds":[]} {"replay":true}`,
	} {
		request := httptest.NewRequest("PUT", "/api/conversations/unused/read", strings.NewReader(body))
		_, err := decodeSelfReadPatch(request)
		selfReadTestError(t, err, 400, "INVALID_READ_REQUEST")
	}
	app := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x75}, 32)}}
	u, conversation := User{ID: newID()}, newID()
	first, second := newID(), newID()
	patch := selfReadPatch{RequestID: newID(), RevisionEpoch: "1", MembershipGeneration: "7", ObservedMessageIDs: []string{first, second}}
	original := app.selfReadDigest(u, conversation, patch)
	patch.ObservedMessageIDs = []string{second, first}
	if bytes.Equal(original, app.selfReadDigest(u, conversation, patch)) {
		t.Fatal("original observation order was lost from the frozen request digest")
	}
}

// Future isolated execution creates only a new tiny sync-owned metadata DB.
// The owner boundary is an explicit protocol stub, not live message/auth proof.
func newSelfReadTestRealm(t *testing.T) (context.Context, *App, User, string, readPointMembership) {
	t.Helper()
	if os.Getenv("JOBS_IM_SELF_READ_TEST") != "1" {
		t.Skip("explicit future isolated personal-read test window required")
	}
	address, err := url.Parse(os.Getenv("JOBS_IM_SELF_READ_REALM_URL"))
	if err != nil || address.Scheme != "https" || address.User != nil || address.Port() == "" || address.Port() == "44443" || net.ParseIP(address.Hostname()) == nil || !net.ParseIP(address.Hostname()).IsLoopback() {
		t.Fatal("future loopback TLS realm distinct from frozen 44443 required")
	}
	dsn := os.Getenv("JOBS_IM_SELF_READ_ADMIN_URL")
	configuration, err := pgx.ParseConfig(dsn)
	if dsn == "" || err != nil || configuration.Database != "postgres" {
		t.Fatal("explicit PostgreSQL maintenance connection required; details redacted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	guardS2PGConnection(t, configuration, "")
	admin, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("owned personal-read creation connection unavailable")
	}
	name := "jobsim_self_read_" + strings.ReplaceAll(newID(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		admin.Close(ctx)
		t.Fatal("owned personal-read metadata database could not be created")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted); err != nil {
			t.Error("owned personal-read cleanup failed; no shared connection was terminated")
		}
		admin.Close(cleanup)
	})
	child := configuration.Copy()
	child.Database = name
	child.RuntimeParams["application_name"] = "jobsim-self-read-metadata-test"
	child.RuntimeParams["statement_timeout"] = "15000"
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("owned personal-read pool configuration unavailable")
	}
	poolConfig.ConnConfig, poolConfig.MaxConns = child, 4
	guardS2PGConnection(t, poolConfig.ConnConfig, "jobsim_self_read_")
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("owned personal-read pool unavailable")
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, "CREATE SCHEMA sync; CREATE TABLE sync.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,restore_epoch bigint NOT NULL)"); err != nil {
		t.Fatal("owned personal-read instance schema failed")
	}
	scope := newID()
	if _, err = pool.Exec(ctx, "INSERT INTO sync.instance VALUES(true,$1,1)", scope); err != nil {
		t.Fatal("owned personal-read instance fixture failed")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("owned personal-read migration transaction unavailable")
	}
	if migrateSelfRead(ctx, tx, scope) != nil || tx.Commit(ctx) != nil {
		tx.Rollback(ctx)
		t.Fatal("owned personal-read migration failed")
	}
	app := &App{Config: Config{DeploymentID: scope, Key: bytes.Repeat([]byte{0x76}, 32)}, Role: "sync", DB: pool}
	return ctx, app, User{ID: newID(), RestoreEpoch: "1"}, newID(), readPointMembership{Generation: "7"}
}

func selfReadTestBoundary(u User, membership readPointMembership, messages []readMessagePoint, unordered bool) selfReadBoundaryLookup {
	return func(_ context.Context, request readPointRequest) (readPointBoundaryResponse, error) {
		response := readPointBoundaryResponse{RestoreEpoch: u.RestoreEpoch, MembershipGeneration: membership.Generation, HistoryFloor: membership.HistoryFloor, Points: []readMessagePoint{}, NextEligible: []readMessagePoint{}, HasUnorderedEligible: unordered}
		byID := map[string]readMessagePoint{}
		for _, point := range messages {
			byID[point.MessageID] = point
		}
		for _, id := range request.MessageIDs {
			point, exists := byID[id]
			if !exists {
				return response, fail(403, "READ_POINT_NOT_VISIBLE", "Synthetic boundary visibility rejected")
			}
			response.Points = append(response.Points, point)
		}
		if request.Through == (readPoint{"0", "0"}) {
			return response, nil
		}
		for _, point := range messages {
			if !point.OrderKnown || point.Deleted || point.SenderID == u.ID {
				continue
			}
			position := readPoint{point.CreationEpoch, point.CreationSequence}
			after, _ := compareSelfReadPoints(request.After, position)
			through, _ := compareSelfReadPoints(position, request.Through)
			if after < 0 && through <= 0 {
				response.NextEligible = append(response.NextEligible, point)
			}
		}
		if len(response.NextEligible) > request.Limit {
			response.NextEligible = response.NextEligible[:request.Limit]
			response.HasMore = true
		}
		return response, nil
	}
}

func TestSelfReadPersistentHolesOriginalFactsAndLegacy(t *testing.T) {
	ctx, app, user, conversation, membership := newSelfReadTestRealm(t)
	peer, ten, eleven := newID(), newID(), newID()
	points := []readMessagePoint{selfReadTestPoint(ten, peer, "1", "10"), selfReadTestPoint(eleven, peer, "1", "11")}
	boundary := selfReadTestBoundary(user, membership, points, false)
	patch := selfReadPatch{RequestID: newID(), RevisionEpoch: "1", MembershipGeneration: "7", ObservedMessageIDs: []string{eleven}}
	_, err := app.selfReadOriginal(ctx, user, conversation, patch, membership)
	selfReadTestError(t, err, 404, "READ_OPERATION_NOT_FOUND")
	state, err := app.loadSelfReadState(ctx, app.DB, user, conversation)
	if err != nil || state.Exists {
		t.Fatal("GET or original-fact lookup materialized a personal cursor")
	}
	first, err := app.applySelfRead(ctx, user, conversation, patch, membership, boundary)
	if err != nil || first.Result.ReadPoint != (readPoint{"0", "0"}) || first.Result.ContinuityStatus != "observation_gap" || first.Result.CountStatus != "not_computed" {
		t.Fatal("persistent out-of-order observation crossed an unobserved gap")
	}
	secondPatch := selfReadPatch{RequestID: newID(), RevisionEpoch: "1", MembershipGeneration: "7", ObservedMessageIDs: []string{ten}}
	second, err := app.applySelfRead(ctx, user, conversation, secondPatch, membership, boundary)
	if err != nil || second.Result.ReadPoint != (readPoint{"1", "11"}) || second.Result.Version != "2" {
		t.Fatal("persistent later hole did not merge after its predecessor arrived")
	}
	var sparse int
	if app.DB.QueryRow(ctx, `SELECT count(*) FROM sync.read_observations WHERE user_id=$1 AND conversation_id=$2`, user.ID, conversation).Scan(&sparse) != nil || sparse != 0 {
		t.Fatal("proven prefix kept redundant individual rows instead of only sparse holes")
	}
	for _, originalLookup := range []bool{false, true} {
		var original selfReadOperation
		if originalLookup {
			original, err = app.selfReadOriginal(ctx, user, conversation, patch, membership)
		} else {
			original, err = app.applySelfRead(ctx, user, conversation, patch, membership, func(context.Context, readPointRequest) (readPointBoundaryResponse, error) {
				t.Error("committed original retry invoked message boundary RPC")
				return readPointBoundaryResponse{}, errors.New("unexpected owner call")
			})
		}
		if err != nil || original.Current || original.Result.ReadPoint != first.Result.ReadPoint || original.Result.Version != first.Result.Version {
			t.Fatal("old request was rerun or overwritten with the later read cursor")
		}
	}
	changed := patch
	changed.ObservedMessageIDs = []string{ten}
	_, err = app.applySelfRead(ctx, user, conversation, changed, membership, boundary)
	selfReadTestError(t, err, 409, "IDEMPOTENCY_CONFLICT")
	legacy := readMessagePoint{MessageID: newID(), SenderID: peer, CreationEpoch: "unknown", CreationSequence: "unknown"}
	legacyPatch := selfReadPatch{RequestID: newID(), RevisionEpoch: "1", MembershipGeneration: "7", ObservedMessageIDs: []string{legacy.MessageID}}
	legacyResult, err := app.applySelfRead(ctx, user, conversation, legacyPatch, membership, selfReadTestBoundary(user, membership, append(points, legacy), true))
	if err != nil || legacyResult.Result.ReadPoint != second.Result.ReadPoint || !legacyResult.Result.HasUnorderedEligible {
		t.Fatal("legacy observation fabricated or retreated a continuous creation point")
	}
	var individual bool
	if app.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sync.read_observations WHERE user_id=$1 AND message_id=$2 AND NOT order_known AND creation_epoch IS NULL AND creation_sequence IS NULL)", user.ID, legacy.MessageID).Scan(&individual) != nil || !individual {
		t.Fatal("legacy ID observation was dropped instead of recorded without a fake tuple")
	}
}

func TestSelfReadConcurrentObservationUnionUsesCASRetry(t *testing.T) {
	ctx, app, user, conversation, membership := newSelfReadTestRealm(t)
	peer, ten, eleven := newID(), newID(), newID()
	points := []readMessagePoint{selfReadTestPoint(ten, peer, "1", "10"), selfReadTestPoint(eleven, peer, "1", "11")}
	owner := selfReadTestBoundary(user, membership, points, false)
	ready, release := make(chan struct{}, 2), make(chan struct{})
	var scans atomic.Int32
	boundary := func(ctx context.Context, request readPointRequest) (readPointBoundaryResponse, error) {
		if request.Through != (readPoint{"0", "0"}) && scans.Add(1) <= 2 {
			ready <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return readPointBoundaryResponse{}, ctx.Err()
			}
		}
		return owner(ctx, request)
	}
	done := make(chan error, 2)
	for _, id := range []string{ten, eleven} {
		patch := selfReadPatch{RequestID: newID(), RevisionEpoch: "1", MembershipGeneration: "7", ObservedMessageIDs: []string{id}}
		go func() {
			_, err := app.applySelfRead(ctx, user, conversation, patch, membership, boundary)
			done <- err
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("both independent read requests did not collect the same unlocked base")
		}
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal("concurrent observed-ID union did not resolve through a fresh owner collect:", err)
		}
	}
	state, err := app.loadSelfReadState(ctx, app.DB, user, conversation)
	if err != nil || state.Point != (readPoint{"1", "11"}) || state.Version != 2 {
		t.Fatal("concurrent read writers lost an observation or jumped over the unresolved predecessor")
	}
}

func TestSelfReadEpochZeroRebaseAndQualification(t *testing.T) {
	ctx, app, user, conversation, membership := newSelfReadTestRealm(t)
	point := selfReadTestPoint(newID(), newID(), "1", "10")
	patch := selfReadPatch{RequestID: newID(), RevisionEpoch: "1", MembershipGeneration: "7", ObservedMessageIDs: []string{point.MessageID}}
	first, err := app.applySelfRead(ctx, user, conversation, patch, membership, selfReadTestBoundary(user, membership, []readMessagePoint{point}, false))
	if err != nil {
		t.Fatal("owned epoch-one read fixture failed")
	}
	if _, err = app.DB.Exec(ctx, "UPDATE sync.read_cursors SET row_version=20; UPDATE sync.instance SET restore_epoch=2 WHERE singleton"); err != nil {
		t.Fatal("owned personal-read restore fixture failed")
	}
	current := user
	current.RestoreEpoch = "2"
	state, err := app.loadSelfReadState(ctx, app.DB, current, conversation)
	view := app.selfReadView(current, conversation, membership, state)
	if err != nil || view.Version != "0" || view.RevisionEpoch != "2" || view.ReadPoint != first.Result.ReadPoint {
		t.Fatal("old epoch version twenty blocked current version zero or changed real creation provenance")
	}
	original, err := app.selfReadOriginal(ctx, current, conversation, patch, membership)
	if err != nil || original.Current || original.Result.RevisionEpoch != "1" {
		t.Fatal("old applied read fact was lost or relabeled into the restored epoch")
	}
	missing := patch
	missing.RequestID = newID()
	_, err = app.applySelfRead(ctx, current, conversation, missing, membership, nil)
	selfReadTestError(t, err, 409, "RESTORE_EPOCH_CHANGED")
	missing.RevisionEpoch, missing.MembershipGeneration = "2", "6"
	_, err = app.applySelfRead(ctx, current, conversation, missing, membership, nil)
	selfReadTestError(t, err, 409, "QUALIFICATION_CHANGED")
	missing.MembershipGeneration = "7"
	second, err := app.applySelfRead(ctx, current, conversation, missing, membership, selfReadTestBoundary(current, membership, []readMessagePoint{point}, false))
	if err != nil || second.Result.Version != "1" || second.Result.RevisionEpoch != "2" || second.Result.ReadPoint != first.Result.ReadPoint {
		t.Fatal("new epoch did not write version one while retaining the actual old creation read tuple")
	}
	var count int
	if app.DB.QueryRow(ctx, "SELECT count(*) FROM sync.read_operations WHERE user_id=$1", user.ID).Scan(&count) != nil || count != 2 {
		t.Fatal("old epoch or changed qualification created an operation instead of rejecting")
	}
}
