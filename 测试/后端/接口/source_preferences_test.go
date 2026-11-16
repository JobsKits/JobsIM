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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The future parent runner supplies a new realm marker and an explicit creation
// connection. These tests touch only the database they create, never HTTP/auth.
func newSourcePreferenceTestRealm(t *testing.T) (context.Context, *App, *pgx.ConnConfig) {
	t.Helper()
	if os.Getenv("JOBS_IM_SOURCE_PREFERENCES_TEST") != "1" {
		t.Skip("explicit future isolated source-preferences test window required")
	}
	realm, err := url.Parse(os.Getenv("JOBS_IM_SOURCE_PREFERENCES_REALM_URL"))
	if err != nil || realm.Scheme != "https" || realm.User != nil || realm.Port() == "" || realm.Port() == "44443" || net.ParseIP(realm.Hostname()) == nil || !net.ParseIP(realm.Hostname()).IsLoopback() {
		t.Fatal("explicit future loopback TLS realm distinct from the frozen 44443 baseline required")
	}
	dsn := os.Getenv("JOBS_IM_SOURCE_PREFERENCES_ADMIN_URL")
	configuration, err := pgx.ParseConfig(dsn)
	if dsn == "" || err != nil || configuration.Database != "postgres" {
		t.Fatal("explicit PostgreSQL maintenance database connection required; details redacted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	guardS2PGConnection(t, configuration, "")
	admin, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("explicit database creation connection unavailable; details redacted")
	}
	name := "jobsim_source_preferences_" + strings.ReplaceAll(newID(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		admin.Close(ctx)
		t.Fatal("owned source-preferences database could not be created")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted); err != nil {
			t.Error("owned preference database cleanup failed; no shared connection was terminated")
		}
		admin.Close(cleanup)
	})
	child := configuration.Copy()
	child.Database = name
	child.RuntimeParams["application_name"] = "jobsim-source-preference-test"
	child.RuntimeParams["statement_timeout"] = "15000"
	child.RuntimeParams["lock_timeout"] = "10000"
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("owned preference pool configuration invalid")
	}
	poolConfig.ConnConfig = child.Copy()
	poolConfig.MaxConns = 4
	guardS2PGConnection(t, poolConfig.ConnConfig, "jobsim_source_preferences_")
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("owned preference pool unavailable")
	}
	t.Cleanup(pool.Close)
	scope := newID()
	if _, err = pool.Exec(ctx, migrations["directory"]+"CREATE TABLE directory.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,restore_epoch bigint NOT NULL CHECK(restore_epoch>0));"); err != nil {
		t.Fatal("owned directory preference migration failed; details redacted")
	}
	if _, err = pool.Exec(ctx, "INSERT INTO directory.instance VALUES(true,$1,1)", scope); err != nil {
		t.Fatal("owned directory instance fixture failed")
	}
	return ctx, &App{Config: Config{DeploymentID: scope, Key: bytes.Repeat([]byte{0x71}, 32)}, Role: "directory", DB: pool}, child
}

func sourcePreferenceTestError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var failure *apiError
	if !errors.As(err, &failure) || failure.Status != status || failure.Code != code {
		t.Fatal("preference request did not produce its required rejection; details redacted")
	}
}

func sourcePreferenceTestCount(t *testing.T, ctx context.Context, a *App, table string) int64 {
	t.Helper()
	if table != "source_preferences" && table != "source_preference_operations" {
		t.Fatal("test count requires an owned preference table")
	}
	var count int64
	if a.DB.QueryRow(ctx, "SELECT count(*) FROM directory."+table).Scan(&count) != nil {
		t.Fatal("owned preference count unavailable")
	}
	return count
}

func TestSourcePreferencesReadUnknownAndRejectedRequestsDoNotWrite(t *testing.T) {
	ctx, a, _ := newSourcePreferenceTestRealm(t)
	u := User{ID: newID(), RestoreEpoch: "1"}
	for range 2 {
		view, err := a.sourcePreferencesRead(ctx, u)
		if err != nil || view.ScopeID != a.Config.DeploymentID || view.SourceContextID != "personal" || view.UserID != u.ID || view.RevisionEpoch != "1" || view.Version != "0" || view.FontPreset != "standard" || view.Explicit {
			t.Fatal("missing preference did not produce the nonmaterialized current-epoch default")
		}
	}
	unknown := newID()
	_, err := a.sourcePreferencesOriginal(ctx, u, unknown, "1")
	sourcePreferenceTestError(t, err, 404, "PREFERENCE_OPERATION_NOT_FOUND")
	patch := sourcePreferencePatch{FontPreset: "large", RevisionEpoch: "1", ExpectedVersion: "7", RequestID: unknown}
	_, err = a.sourcePreferencesApply(ctx, u, patch)
	sourcePreferenceTestError(t, err, 409, "PREFERENCE_VERSION_CONFLICT")
	patch.FontPreset = "unsupported"
	_, err = a.sourcePreferencesApply(ctx, u, patch)
	sourcePreferenceTestError(t, err, 400, "INVALID_FONT_PRESET")
	if sourcePreferenceTestCount(t, ctx, a, "source_preferences") != 0 || sourcePreferenceTestCount(t, ctx, a, "source_preference_operations") != 0 {
		t.Fatal("read, unknown nonce or rejected CAS persisted a stub or operation")
	}
	patch.FontPreset, patch.ExpectedVersion = "large", "0"
	applied, err := a.sourcePreferencesApply(ctx, u, patch)
	if err != nil || applied.OperationID != unknown || applied.Result.Version != "1" {
		t.Fatal("prior unknown lookup or rejected request reserved the nonce against its first real write")
	}
	original, err := a.sourcePreferencesOriginal(ctx, u, unknown, "1")
	if err != nil || !original.Current || original.Result != applied.Result {
		t.Fatal("the formerly unknown nonce did not resolve to its single real applied fact")
	}
}

func TestSourcePreferencesConcurrentCASAndOriginalFact(t *testing.T) {
	ctx, a, _ := newSourcePreferenceTestRealm(t)
	u := User{ID: newID(), RestoreEpoch: "1"}
	type outcome struct {
		patch sourcePreferencePatch
		op    sourcePreferenceOperation
		err   error
	}
	start, done := make(chan struct{}), make(chan outcome, 2)
	for _, font := range []string{"small", "large"} {
		patch := sourcePreferencePatch{FontPreset: font, RevisionEpoch: "1", ExpectedVersion: "0", RequestID: newID()}
		go func() {
			<-start
			op, err := a.sourcePreferencesApply(ctx, u, patch)
			done <- outcome{patch, op, err}
		}()
	}
	close(start)
	var winner outcome
	successes := 0
	for range 2 {
		result := <-done
		if result.err == nil {
			winner = result
			successes++
		} else {
			sourcePreferenceTestError(t, result.err, 409, "PREFERENCE_VERSION_CONFLICT")
		}
	}
	if successes != 1 || winner.op.Result.Version != "1" || !winner.op.Current || sourcePreferenceTestCount(t, ctx, a, "source_preference_operations") != 1 {
		t.Fatal("two same-version writers did not produce exactly one applied original fact")
	}
	conflict := winner.patch
	conflict.FontPreset = "standard"
	_, err := a.sourcePreferencesApply(ctx, u, conflict)
	sourcePreferenceTestError(t, err, 409, "IDEMPOTENCY_CONFLICT")
	conflict = winner.patch
	conflict.ExpectedVersion = "1"
	_, err = a.sourcePreferencesApply(ctx, u, conflict)
	sourcePreferenceTestError(t, err, 409, "IDEMPOTENCY_CONFLICT")
	later, err := a.sourcePreferencesApply(ctx, u, sourcePreferencePatch{FontPreset: "standard", RevisionEpoch: "1", ExpectedVersion: "1", RequestID: newID()})
	if err != nil || later.Result.Version != "2" {
		t.Fatal("later explicit preference did not advance once")
	}
	for _, replay := range []bool{false, true} {
		var original sourcePreferenceOperation
		if replay {
			original, err = a.sourcePreferencesApply(ctx, u, winner.patch)
		} else {
			original, err = a.sourcePreferencesOriginal(ctx, u, winner.patch.RequestID, "1")
		}
		if err != nil || original.Current || original.Result != winner.op.Result {
			t.Fatal("old nonce was overwritten by the later font or treated as the current preference")
		}
	}
	other := User{ID: newID(), RestoreEpoch: "1"}
	_, err = a.sourcePreferencesOriginal(ctx, other, winner.patch.RequestID, "1")
	sourcePreferenceTestError(t, err, 404, "PREFERENCE_OPERATION_NOT_FOUND")
	view, err := a.sourcePreferencesRead(ctx, u)
	if err != nil || view.Version != "2" || view.FontPreset != "standard" || !view.Explicit {
		t.Fatal("original-fact lookup changed the later preference")
	}
}

func TestSourcePreferencesConcurrentSameNonceRechecksAfterLock(t *testing.T) {
	ctx, a, config := newSourcePreferenceTestRealm(t)
	u := User{ID: newID(), RestoreEpoch: "1"}
	if _, err := a.DB.Exec(ctx, "INSERT INTO directory.source_preferences(scope_id,source_context_id,user_id,revision_epoch) VALUES($1,'personal',$2,1)", a.Config.DeploymentID, u.ID); err != nil {
		t.Fatal("owned concurrent preference fixture failed")
	}
	lockConfig := config.Copy()
	lockConfig.RuntimeParams["application_name"] = "jobsim-source-preference-test-lock-holder"
	guardS2PGConnection(t, lockConfig, "jobsim_source_preferences_")
	locker, err := pgx.ConnectConfig(ctx, lockConfig)
	if err != nil {
		t.Fatal("owned preference lock-holder connection unavailable")
	}
	defer locker.Close(context.Background())
	hold, err := locker.Begin(ctx)
	if err != nil {
		t.Fatal("owned preference lock transaction unavailable")
	}
	defer hold.Rollback(context.Background())
	if _, err = hold.Exec(ctx, "SELECT user_id FROM directory.source_preferences WHERE user_id=$1 FOR UPDATE", u.ID); err != nil {
		t.Fatal("owned preference row could not be held")
	}
	patch := sourcePreferencePatch{FontPreset: "large", RevisionEpoch: "1", ExpectedVersion: "0", RequestID: newID()}
	type result struct {
		op  sourcePreferenceOperation
		err error
	}
	done := make(chan result, 2)
	for range 2 {
		go func() {
			op, err := a.sourcePreferencesApply(ctx, u, patch)
			done <- result{op, err}
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	blocked := 0
	for time.Now().Before(deadline) {
		if a.DB.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name='jobsim-source-preference-test' AND wait_event_type='Lock'").Scan(&blocked) != nil {
			t.Fatal("owned waiter observation unavailable")
		}
		if blocked == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if blocked != 2 {
		t.Fatal("both original requests did not reach the deliberately held preference lock")
	}
	if hold.Commit(ctx) != nil {
		t.Fatal("owned preference row lock could not be released")
	}
	for range 2 {
		result := <-done
		if result.err != nil || !result.op.Current || result.op.Result.Version != "1" || result.op.Result.FontPreset != "large" {
			t.Fatal("same nonce after an actual lock wait did not recover its single committed fact")
		}
	}
	if sourcePreferenceTestCount(t, ctx, a, "source_preference_operations") != 1 {
		t.Fatal("same concurrent nonce created multiple original operations")
	}
}

func TestSourcePreferencesEpochResetRetainsOriginalFacts(t *testing.T) {
	ctx, a, _ := newSourcePreferenceTestRealm(t)
	u := User{ID: newID(), RestoreEpoch: "1"}
	var last sourcePreferencePatch
	for index := int64(0); index < 20; index++ {
		last = sourcePreferencePatch{FontPreset: "small", RevisionEpoch: "1", ExpectedVersion: str(index), RequestID: newID()}
		if _, err := a.sourcePreferencesApply(ctx, u, last); err != nil {
			t.Fatal("owned epoch-one preference sequence failed")
		}
	}
	if _, err := a.DB.Exec(ctx, "UPDATE directory.instance SET restore_epoch=2 WHERE singleton"); err != nil {
		t.Fatal("owned directory epoch transition failed")
	}
	current := u
	current.RestoreEpoch = "2"
	view, err := a.sourcePreferencesRead(ctx, current)
	if err != nil || view.RevisionEpoch != "2" || view.Version != "0" || view.FontPreset != "small" || !view.Explicit {
		t.Fatal("old epoch version twenty blocked the new-epoch version-zero view")
	}
	old, err := a.sourcePreferencesOriginal(ctx, current, last.RequestID, "1")
	if err != nil || old.Current || old.Result.Version != "20" || old.Result.RevisionEpoch != "1" {
		t.Fatal("old original operation lost its epoch or was relabeled as current")
	}
	retried, err := a.sourcePreferencesApply(ctx, current, last)
	if err != nil || retried.Current || retried.Result != old.Result {
		t.Fatal("old successful request could not be reconciled without a new preference write")
	}
	missing := sourcePreferencePatch{FontPreset: "large", RevisionEpoch: "1", ExpectedVersion: "0", RequestID: newID()}
	_, err = a.sourcePreferencesApply(ctx, current, missing)
	sourcePreferenceTestError(t, err, 409, "RESTORE_EPOCH_CHANGED")
	_, err = a.sourcePreferencesOriginal(ctx, current, missing.RequestID, "1")
	sourcePreferenceTestError(t, err, 404, "PREFERENCE_OPERATION_NOT_FOUND")
	changed, err := a.sourcePreferencesApply(ctx, current, sourcePreferencePatch{FontPreset: "large", RevisionEpoch: "2", ExpectedVersion: "0", RequestID: newID()})
	if err != nil || changed.Result.RevisionEpoch != "2" || changed.Result.Version != "1" || !changed.Current {
		t.Fatal("new-epoch first explicit write did not start at version one")
	}
	_, err = a.sourcePreferencesRead(ctx, u)
	sourcePreferenceTestError(t, err, 409, "RESTORE_EPOCH_CHANGED")
	if sourcePreferenceTestCount(t, ctx, a, "source_preference_operations") != 21 {
		t.Fatal("epoch mismatch or original retry created an extra operation")
	}
}

func TestSourcePreferencesStrictRequestAndExactCounters(t *testing.T) {
	for _, body := range []string{
		`{"fontPreset":"large","revisionEpoch":"1","expectedVersion":"0","requestId":"12345678","sourceContextId":"foreign"}`,
		`{"fontPreset":"large","revisionEpoch":"1","expectedVersion":"0","requestId":"12345678","userId":"foreign"}`,
		`{"fontPreset":"large","revisionEpoch":"1","expectedVersion":"0","requestId":"12345678","readReceipt":true}`,
		`{"fontPreset":"large","revisionEpoch":"1","expectedVersion":"0","requestId":"12345678"} {"scopeId":"foreign"}`,
	} {
		request := httptest.NewRequest("PATCH", "/api/me/preferences", strings.NewReader(body))
		var patch sourcePreferencePatch
		sourcePreferenceTestError(t, decodeSourcePreferencePatch(request, &patch), 400, "INVALID_JSON")
	}
	for _, raw := range []string{"9007199254740993", "9223372036854775807"} {
		value, err := sourcePreferenceCounter(raw, false)
		if err != nil || str(value) != raw {
			t.Fatal("preference counter lost exact decimal precision")
		}
	}
	for _, raw := range []string{"01", "-1", "+1", "1.0", "9223372036854775808"} {
		if _, err := sourcePreferenceCounter(raw, true); err == nil {
			t.Fatal("noncanonical or overflowing preference revision was accepted")
		}
	}
}
