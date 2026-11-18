package jobsim

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type readPointTestRealm struct {
	app        *App
	db         *pgx.Conn
	user       string
	peer       string
	conv       string
	otherConv  string
	membership readPointMembership
}

// Only a future explicit test window may create this metadata-only database.
// No message body column, real account, live HTTP route or shared row is used.
func newReadPointTestRealm(t *testing.T) (context.Context, readPointTestRealm) {
	t.Helper()
	var realm readPointTestRealm
	if os.Getenv("JOBS_IM_READ_POINT_TEST") != "1" {
		t.Skip("explicit future isolated read-point test window required")
	}
	address, err := url.Parse(os.Getenv("JOBS_IM_READ_POINT_REALM_URL"))
	if err != nil || address.Scheme != "https" || address.User != nil || address.Port() == "" || address.Port() == "44443" || net.ParseIP(address.Hostname()) == nil || !net.ParseIP(address.Hostname()).IsLoopback() {
		t.Fatal("explicit future loopback TLS realm distinct from frozen 44443 required")
	}
	dsn := os.Getenv("JOBS_IM_READ_POINT_ADMIN_URL")
	configuration, err := pgx.ParseConfig(dsn)
	if dsn == "" || err != nil || configuration.Database != "postgres" {
		t.Fatal("explicit PostgreSQL maintenance URL required; details redacted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	guardS2PGConnection(t, configuration, "")
	admin, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("explicit read-point database creation connection unavailable")
	}
	name := "jobsim_read_point_" + strings.ReplaceAll(newID(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		admin.Close(ctx)
		t.Fatal("owned read-point metadata database could not be created")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted); err != nil {
			t.Error("owned read-point cleanup failed; no shared connection was terminated")
		}
		admin.Close(cleanup)
	})
	child := configuration.Copy()
	child.Database = name
	child.RuntimeParams["application_name"] = "jobsim-read-point-metadata-test"
	child.RuntimeParams["statement_timeout"] = "10000"
	guardS2PGConnection(t, child, "jobsim_read_point_")
	realm.db, err = pgx.ConnectConfig(ctx, child)
	if err != nil {
		t.Fatal("owned read-point metadata database unavailable")
	}
	t.Cleanup(func() { realm.db.Close(context.Background()) })
	_, err = realm.db.Exec(ctx, `CREATE SCHEMA message;
CREATE TABLE message.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,restore_epoch bigint NOT NULL);
CREATE TABLE message.messages(id uuid PRIMARY KEY,conversation_id uuid NOT NULL,sender_id uuid NOT NULL,created_at timestamptz NOT NULL,deleted boolean NOT NULL,sequence bigint NOT NULL,sequence_epoch bigint,creation_sequence bigint);`)
	if err != nil {
		t.Fatal("owned metadata-only schema unavailable")
	}
	scope := newID()
	if _, err = realm.db.Exec(ctx, "INSERT INTO message.instance VALUES(true,$1,2)", scope); err != nil {
		t.Fatal("owned message epoch fixture failed")
	}
	realm.app = &App{Config: Config{DeploymentID: scope}, Role: "message"}
	realm.user, realm.peer, realm.conv, realm.otherConv = newID(), newID(), newID(), newID()
	joined := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	realm.membership = readPointMembership{Generation: "7", HistoryFloor: joined.Format(time.RFC3339Nano), JoinedAt: &joined}
	return ctx, realm
}

func readPointTestInsert(t *testing.T, ctx context.Context, realm readPointTestRealm, conversation, sender string, epoch, sequence any, created time.Time, deleted bool, editedSequence int64) string {
	t.Helper()
	id := newID()
	if _, err := realm.db.Exec(ctx, "INSERT INTO message.messages VALUES($1,$2,$3,$4,$5,$6,$7,$8)", id, conversation, sender, created, deleted, editedSequence, epoch, sequence); err != nil {
		t.Fatal("owned read-point metadata fixture failed")
	}
	return id
}

func readPointTestQuery(ctx context.Context, realm readPointTestRealm, request readPointRequest) (readPointBoundaryResponse, error) {
	tx, err := realm.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return readPointBoundaryResponse{}, err
	}
	defer tx.Rollback(ctx)
	return realm.app.readPointMetadata(ctx, tx, request, realm.membership)
}

func readPointTestError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var failure *apiError
	if !errors.As(err, &failure) || failure.Status != status || failure.Code != code {
		t.Fatal("read point did not produce its required rejection; details redacted")
	}
}

func TestReadPointsPrivateCallerAndStrictMetadata(t *testing.T) {
	app := &App{}
	var route func(http.ResponseWriter, *http.Request) error
	app.readPointMessageRoutes(func(pattern string, handler func(http.ResponseWriter, *http.Request) error) {
		if pattern != "POST /internal/read-boundaries" {
			t.Fatal("read boundary route escaped its private owner path")
		}
		route = handler
	}, func(handler func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
		return handler
	})
	request := httptest.NewRequest("POST", "/internal/read-boundaries", strings.NewReader("{}"))
	request.Header.Set("X-JobsIM-Caller", "message")
	readPointTestError(t, route(httptest.NewRecorder(), request), 403, "SYNC_SERVICE_REQUIRED")
	valid := readPointRequest{UserID: newID(), ConversationID: newID(), After: readPoint{"0", "0"}, Through: readPoint{"2", "10"}}
	normalized, err := normalizeReadPointRequest(valid)
	if err != nil || normalized.Limit != 200 {
		t.Fatal("default eligible page size was not bounded at 200")
	}
	for _, point := range []readPoint{{"0", "1"}, {"1", "0"}, {"01", "1"}, {"1", "-1"}, {"unknown", "unknown"}, {"9223372036854775808", "1"}} {
		copy := valid
		copy.After = point
		_, err := normalizeReadPointRequest(copy)
		readPointTestError(t, err, 400, "INVALID_READ_POINT")
	}
	duplicate := valid
	id := newID()
	duplicate.MessageIDs = []string{id, id}
	_, err = normalizeReadPointRequest(duplicate)
	readPointTestError(t, err, 400, "INVALID_READ_POINT")
	for _, point := range []readPoint{{"1", "9007199254740993"}, {"2", "9223372036854775807"}} {
		epoch, sequence, err := parseReadPoint(point)
		if err != nil || str(epoch) != point.Epoch || str(sequence) != point.Sequence {
			t.Fatal("exact creation-point decimal precision was lost")
		}
	}
	member, err := readPointMembershipFromMetadata(map[string]any{"kind": "group", "membershipGeneration": "7", "joinedAt": "2026-11-17T08:00:00+08:00"})
	if err != nil || member.HistoryFloor != "2026-11-17T00:00:00Z" {
		t.Fatal("history floor was not derived from the server member timestamp")
	}
	_, err = readPointMembershipFromMetadata(map[string]any{"kind": "group", "membershipGeneration": "7"})
	readPointTestError(t, err, 400, "INVALID_READ_POINT")
}

func TestReadPointsInputOrderUnknownAndUniformVisibility(t *testing.T) {
	ctx, realm := newReadPointTestRealm(t)
	floor := *realm.membership.JoinedAt
	known := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), int64(10), floor, false, 99999)
	legacy := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, nil, nil, floor.Add(time.Second), false, 20)
	partial := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), nil, floor.Add(time.Second), false, 21)
	deleted := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), int64(12), floor.Add(time.Second), true, 100)
	own := readPointTestInsert(t, ctx, realm, realm.conv, realm.user, int64(2), int64(1), floor.Add(time.Second), false, 101)
	prejoin := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), int64(9), floor.Add(-time.Second), false, 9)
	foreign := readPointTestInsert(t, ctx, realm, realm.otherConv, realm.peer, int64(2), int64(2), floor.Add(time.Second), false, 102)
	request := readPointRequest{UserID: realm.user, ConversationID: realm.conv, MessageIDs: []string{partial, deleted, legacy, known, own}, After: readPoint{"0", "0"}, Through: readPoint{"0", "0"}}
	result, err := readPointTestQuery(ctx, realm, request)
	if err != nil || result.RestoreEpoch != "2" || result.MembershipGeneration != "7" || result.HistoryFloor != realm.membership.HistoryFloor || len(result.Points) != len(request.MessageIDs) || len(result.NextEligible) != 0 || result.HasMore || !result.HasUnorderedEligible {
		t.Fatal("ID-only metadata lookup did not retain its current owner/membership boundary")
	}
	for index, id := range request.MessageIDs {
		if result.Points[index].MessageID != id {
			t.Fatal("point responses were reordered away from the input IDs")
		}
	}
	if result.Points[0].OrderKnown || result.Points[0].CreationEpoch != "unknown" || result.Points[2].CreationSequence != "unknown" || !result.Points[1].Deleted || result.Points[3].CreationSequence != "10" {
		t.Fatal("legacy NULL or edited mutable sequence was fabricated as a creation point")
	}
	encoded, _ := json.Marshal(result)
	for _, forbidden := range []string{"text", "attachment", "body", "token", "readReceipt"} {
		if strings.Contains(string(encoded), `"`+forbidden+`"`) {
			t.Fatal("read-boundary metadata contained a body or unrelated credential field")
		}
	}
	for _, hidden := range []string{prejoin, foreign, newID()} {
		request.MessageIDs = []string{known, hidden}
		_, err = readPointTestQuery(ctx, realm, request)
		readPointTestError(t, err, 403, "READ_POINT_NOT_VISIBLE")
	}
	realm.membership = readPointMembership{Generation: "8"}
	request.MessageIDs = []string{prejoin}
	result, err = readPointTestQuery(ctx, realm, request)
	if err != nil || result.HistoryFloor != "" || len(result.Points) != 1 {
		t.Fatal("non-group history inherited an unrelated group join floor")
	}
}

func TestReadPointsNumericCreationScanAndPagination(t *testing.T) {
	ctx, realm := newReadPointTestRealm(t)
	floor := *realm.membership.JoinedAt
	first := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), int64(9), floor, false, 99999)
	second := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), int64(10), floor, false, 10)
	third := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(2), int64(1), floor, false, 1000)
	readPointTestInsert(t, ctx, realm, realm.conv, realm.user, int64(2), int64(2), floor, false, 1001)
	readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(2), int64(3), floor, true, 1002)
	readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), int64(8), floor.Add(-time.Second), false, 8)
	readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, nil, nil, floor, false, 1003)
	request := readPointRequest{UserID: realm.user, ConversationID: realm.conv, After: readPoint{"0", "0"}, Through: readPoint{"2", "3"}, Limit: 2}
	result, err := readPointTestQuery(ctx, realm, request)
	if err != nil || !result.HasMore || !result.HasUnorderedEligible || len(result.NextEligible) != 2 || result.NextEligible[0].MessageID != first || result.NextEligible[1].MessageID != second {
		t.Fatal("numeric creation order or limit-plus-one eligible pagination was incorrect")
	}
	request.After = readPoint{"1", "10"}
	result, err = readPointTestQuery(ctx, realm, request)
	if err != nil || result.HasMore || len(result.NextEligible) != 1 || result.NextEligible[0].MessageID != third || len(result.Points) != 0 {
		t.Fatal("creation tuple continuation included an outgoing/deleted/prejoin message or skipped an eligible tuple")
	}
	request.Through = readPoint{"2", "999"}
	_, err = readPointTestQuery(ctx, realm, request)
	readPointTestError(t, err, 400, "INVALID_READ_POINT")
}

func TestReadPointsMalformedMetadataCannotAdvance(t *testing.T) {
	ctx, realm := newReadPointTestRealm(t)
	floor := *realm.membership.JoinedAt
	known := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(2), int64(10), floor, false, 200)
	malformed := readPointTestInsert(t, ctx, realm, realm.conv, realm.peer, int64(1), int64(0), floor, false, 201)
	request := readPointRequest{UserID: realm.user, ConversationID: realm.conv, MessageIDs: []string{malformed}, After: readPoint{"0", "0"}, Through: readPoint{"0", "0"}}
	_, err := readPointTestQuery(ctx, realm, request)
	readPointTestError(t, err, 400, "INVALID_READ_POINT")
	request.MessageIDs = []string{malformed, newID()}
	_, err = readPointTestQuery(ctx, realm, request)
	readPointTestError(t, err, 403, "READ_POINT_NOT_VISIBLE")
	request.MessageIDs = []string{known}
	request.Through = readPoint{"2", "10"}
	_, err = readPointTestQuery(ctx, realm, request)
	readPointTestError(t, err, 400, "INVALID_READ_POINT")
}
