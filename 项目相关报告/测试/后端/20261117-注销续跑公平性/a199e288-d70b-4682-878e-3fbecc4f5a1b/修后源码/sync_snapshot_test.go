package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type syncSnapshotTestFixture struct {
	app      *App
	user     User
	store    string
	snapshot syncSnapshot
	seed     struct {
		FreshSyncRealm   bool   `json:"freshSyncRealm"`
		RunDir           string `json:"runDir"`
		StateDir         string `json:"stateDir"`
		BusinessURL      string `json:"businessURL"`
		JournalURL       string `json:"journalURL"`
		ServerURL        string `json:"serverURL"`
		PGPassFile       string `json:"pgPassFile"`
		NATSURL          string `json:"natsURL"`
		NATSTokenFile    string `json:"natsTokenFile"`
		InternalBasePort int    `json:"internalBasePort"`
	}
}

// The parent grants a ready window explicitly. Merely finding a seed never
// authorizes this suite to connect, migrate, restart, or create an account.
func openSyncSnapshotTestFixture(t *testing.T) (context.Context, syncSnapshotTestFixture) {
	t.Helper()
	var fixture syncSnapshotTestFixture
	if os.Getenv("JOBS_IM_SYNC_SNAPSHOT_READY") != "1" {
		t.Skip("explicit ready window for the fresh sync realm required")
	}
	path := os.Getenv("JOBS_IM_SYNC_SNAPSHOT_TEST_ENV")
	if path == "" {
		t.Fatal("explicit fresh synchronization seed required")
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &fixture.seed) != nil || !fixture.seed.FreshSyncRealm {
		t.Fatal("fresh synchronization seed unavailable or invalid; details redacted")
	}
	server, err := url.Parse(fixture.seed.ServerURL)
	if err != nil || server.Scheme != "https" || server.Port() != "44443" || net.ParseIP(server.Hostname()) == nil || !net.ParseIP(server.Hostname()).IsLoopback() {
		t.Fatal("this suite is restricted to the explicitly authorized fresh TLS sync realm")
	}
	run := filepath.Dir(path)
	if filepath.Clean(fixture.seed.RunDir) != filepath.Clean(run) || fixture.seed.InternalBasePort < 1 {
		t.Fatal("seed does not prove the dedicated synchronization run directory")
	}
	privatePath := filepath.Join(filepath.Dir(path), "Secrets", "first-sync-fixture.json")
	private, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatal("first private synchronization fixture unavailable")
	}
	// Deliberately do not deserialize access or refresh tokens from the fixture.
	var first struct {
		Auth struct {
			DeviceID     string `json:"deviceId"`
			RestoreEpoch string `json:"restoreEpoch"`
			User         struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"auth"`
		Store    string       `json:"store"`
		Snapshot syncSnapshot `json:"snapshot"`
	}
	err = json.Unmarshal(private, &first)
	for i := range private {
		private[i] = 0
	}
	if err != nil || !validObjectID(first.Auth.User.ID) || !validObjectID(first.Auth.DeviceID) || !syncValidationCounter(first.Auth.RestoreEpoch, false) || !validObjectID(first.Store) || !validObjectID(first.Snapshot.ID) {
		t.Fatal("private first fixture identity incomplete; credential material is not printed")
	}
	fixture.user = User{ID: first.Auth.User.ID, DeviceID: first.Auth.DeviceID, RestoreEpoch: first.Auth.RestoreEpoch}
	fixture.store, fixture.snapshot = first.Store, first.Snapshot
	if fixture.seed.PGPassFile != "" {
		t.Setenv("PGPASSFILE", fixture.seed.PGPassFile)
	}
	roles := map[string]string{}
	for i, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance", "sync"} {
		roles[role] = fmt.Sprintf("http://127.0.0.1:%d", fixture.seed.InternalBasePort+i)
	}
	configuration, err := PrepareConfig(Config{StateDir: fixture.seed.StateDir, DatabaseURL: fixture.seed.BusinessURL, JournalDatabaseURL: fixture.seed.JournalURL, PublicURL: fixture.seed.ServerURL, RoleURLs: roles, NATSURL: fixture.seed.NATSURL, NATSTokenFile: fixture.seed.NATSTokenFile})
	if err == nil {
		configuration, err = normalizeAdmissionState(configuration)
	}
	if err != nil {
		t.Fatal("same-instance private RPC configuration unavailable; details redacted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	poolConfiguration, err := pgxpool.ParseConfig(fixture.seed.BusinessURL)
	if err != nil {
		t.Fatal("dedicated business connection invalid; details redacted")
	}
	poolConfiguration.MaxConns = 3
	poolConfiguration.ConnConfig.RuntimeParams["application_name"] = "jobsim-sync-snapshot-contract-test"
	poolConfiguration.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	poolConfiguration.ConnConfig.RuntimeParams["lock_timeout"] = "5000"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfiguration)
	if err != nil {
		t.Fatal("dedicated business pool unavailable; details redacted")
	}
	t.Cleanup(pool.Close)
	fixture.app = &App{Config: configuration, Role: "sync", DB: pool, Client: &http.Client{Timeout: 5 * time.Second}}
	var scope, epoch string
	if pool.QueryRow(ctx, "SELECT deployment_id,restore_epoch::text FROM sync.instance WHERE singleton").Scan(&scope, &epoch) != nil || scope != configuration.DeploymentID || epoch != fixture.user.RestoreEpoch {
		t.Fatal("dedicated business scope or restore epoch does not match the private fixture")
	}
	return ctx, fixture
}

func syncSnapshotAdminURL(t *testing.T, business string) string {
	t.Helper()
	address, err := url.Parse(business)
	if err != nil || address.Host == "" || address.Scheme != "postgres" && address.Scheme != "postgresql" {
		t.Fatal("explicit PostgreSQL URL required for owned database creation")
	}
	address.Path, address.RawPath = "/postgres", ""
	return address.String()
}

func newSyncSnapshotAuthOwner(t *testing.T, ctx context.Context, fixture syncSnapshotTestFixture) (*pgxpool.Pool, *httptest.Server) {
	t.Helper()
	configuration, err := pgx.ParseConfig(syncSnapshotAdminURL(t, fixture.seed.BusinessURL))
	if err != nil {
		t.Fatal("owned identity database creation configuration invalid")
	}
	admin, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("owned identity database creation connection unavailable")
	}
	name := "jobsim_sync_capture_" + strings.ReplaceAll(newID(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		admin.Close(ctx)
		t.Fatal("owned identity database could not be created")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted); err != nil {
			t.Error("owned identity database cleanup failed; no shared connection was terminated")
		}
		admin.Close(cleanup)
	})
	configuration.Database = name
	poolConfiguration, err := pgxpool.ParseConfig(fixture.seed.BusinessURL)
	if err != nil {
		t.Fatal("owned identity pool configuration unavailable")
	}
	poolConfiguration.ConnConfig = configuration.Copy()
	poolConfiguration.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(ctx, poolConfiguration)
	if err != nil {
		t.Fatal("owned identity pool unavailable")
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("owned identity migration transaction unavailable")
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, migrations["identity"]+"CREATE TABLE auth.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,restore_epoch bigint NOT NULL)"); err != nil {
		t.Fatal("owned identity schema unavailable")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO auth.instance VALUES(true,$1,$2)", fixture.app.Config.DeploymentID, fixture.user.RestoreEpoch); err != nil {
		t.Fatal("owned identity scope unavailable")
	}
	if err = migrateSyncOwner(ctx, tx, "auth", fixture.app.Config.DeploymentID); err != nil {
		t.Fatal("owned identity change index unavailable")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO auth.users(id,username,display_name,password_hash) VALUES($1,$2,'isolated public fixture','NON_LOGIN_SYNC_CAPTURE_FIXTURE')", fixture.user.ID, "sync_capture_"+strings.ReplaceAll(newID(), "-", "")[:8]); err != nil || tx.Commit(ctx) != nil {
		t.Fatal("owned public identity fixture unavailable")
	}
	owner := &App{Config: fixture.app.Config, Role: "identity", DB: pool, Client: &http.Client{Timeout: 5 * time.Second}}
	server := httptest.NewServer(owner.Handler())
	t.Cleanup(server.Close)
	return pool, server
}

func TestSyncSnapshotCaptureRejectsChangingCommittedOwners(t *testing.T) {
	ctx, fixture := openSyncSnapshotTestFixture(t)
	a := fixture.app
	identityDB, identityServer := newSyncSnapshotAuthOwner(t, ctx, fixture)
	var original string
	if identityDB.QueryRow(ctx, "SELECT display_name FROM auth.users WHERE id=$1", fixture.user.ID).Scan(&original) != nil {
		t.Fatal("owned actor display name unavailable")
	}
	t.Cleanup(func() {
		restore, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		result, err := identityDB.Exec(restore, "UPDATE auth.users SET display_name=$2 WHERE id=$1", fixture.user.ID, original)
		if err != nil || result.RowsAffected() != 1 {
			t.Error("owned actor display name restoration failed; no unrelated account was changed")
		}
	})
	var before int64
	if a.DB.QueryRow(ctx, "SELECT count(*) FROM sync.snapshots WHERE user_id=$1 AND device_id=$2", fixture.user.ID, fixture.user.DeviceID).Scan(&before) != nil {
		t.Fatal("initial snapshot count unavailable")
	}
	target, err := url.Parse(identityServer.URL)
	if err != nil {
		t.Fatal("identity role endpoint invalid")
	}
	var captures atomic.Int64
	var mutationFailed atomic.Bool
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "isolated identity proxy unavailable", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Request.URL.Path != "/internal/sync/capture" || response.StatusCode != http.StatusOK {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		if err != nil || len(data) > 1<<20 {
			return errors.New("isolated capture proxy response unavailable")
		}
		response.Body = io.NopCloser(bytes.NewReader(data))
		response.ContentLength = int64(len(data))
		// Preserve the real role's admission proof headers and canonical StateDir.
		// Commit after its old watermark was captured, before the final double collect.
		iteration := captures.Add(1)
		result, err := identityDB.Exec(ctx, "UPDATE auth.users SET display_name=$2 WHERE id=$1", fixture.user.ID, fmt.Sprintf("sync-capture-controlled-change-%d", iteration))
		if err != nil || result.RowsAffected() != 1 {
			mutationFailed.Store(true)
			return errors.New("isolated committed owner mutation failed")
		}
		return nil
	}
	wrapper := httptest.NewServer(proxy)
	defer wrapper.Close()
	oldIdentity := a.Config.RoleURLs["identity"]
	a.Config.RoleURLs["identity"] = wrapper.URL
	defer func() { a.Config.RoleURLs["identity"] = oldIdentity }()
	_, _, _, err = a.syncCapture(ctx, fixture.user)
	var api *apiError
	if mutationFailed.Load() || captures.Load() != 3 || !errors.As(err, &api) || api.Status != 503 || api.Code != "SYNC_SNAPSHOT_BUSY" {
		t.Fatal("three committed owner changes did not prevent a mixed-cut snapshot; response details redacted")
	}
	var after int64
	if a.DB.QueryRow(ctx, "SELECT count(*) FROM sync.snapshots WHERE user_id=$1 AND device_id=$2", fixture.user.ID, fixture.user.DeviceID).Scan(&after) != nil || after != before {
		t.Fatal("failed snapshot capture unexpectedly persisted a new snapshot")
	}
}

func TestSyncSnapshotNATSDuplicateAndBodyRejection(t *testing.T) {
	ctx, fixture := openSyncSnapshotTestFixture(t)
	a := fixture.app
	wire := syncWireEvent{Owner: "auth", Ref: syncRef{Owner: "auth"}}
	err := a.DB.QueryRow(ctx, `SELECT origin_event_id::text,scope_id::text,restore_epoch::text,revision::text,kind,object_id::text,COALESCE(subject_user_id::text,''),COALESCE(conversation_id::text,''),COALESCE(target_user_id::text,''),revision_epoch::text,state FROM auth.sync_outbox WHERE kind='account' AND object_id=$1 AND restore_epoch=$2 AND EXISTS(SELECT 1 FROM sync.origins imported WHERE imported.origin_event_id=auth.sync_outbox.origin_event_id) ORDER BY revision DESC LIMIT 1`, fixture.user.ID, fixture.user.RestoreEpoch).Scan(&wire.OriginEventID, &wire.ScopeID, &wire.RestoreEpoch, &wire.Revision, &wire.Ref.Kind, &wire.Ref.ID, &wire.Ref.SubjectUserID, &wire.Ref.ConversationID, &wire.Ref.TargetUserID, &wire.Ref.RevisionEpoch, &wire.Ref.State)
	if err != nil {
		t.Fatal("an already imported real committed owner outbox reference is required")
	}
	wire.Ref.Revision = wire.Revision
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal("real metadata envelope could not be encoded")
	}
	message := &nats.Msg{Subject: "jobsim." + wire.ScopeID + "." + wire.RestoreEpoch + "." + wire.Owner, Header: nats.Header{}, Data: data}
	message.Header.Set(nats.MsgIdHdr, wire.OriginEventID)
	for range 2 {
		if a.syncNATSStore(ctx, message) != nil {
			t.Fatal("real committed metadata envelope was not accepted idempotently")
		}
		var count int
		if a.DB.QueryRow(ctx, "SELECT count(*) FROM sync.origins WHERE origin_event_id=$1", wire.OriginEventID).Scan(&count) != nil || count != 1 {
			t.Fatal("repeated NATS storage created more than one origin fact")
		}
	}
	changed := wire
	if changed.Ref.State == "active" {
		changed.Ref.State = "removed"
	} else {
		changed.Ref.State = "active"
	}
	message.Data, _ = json.Marshal(changed)
	if !errors.Is(a.syncNATSStore(ctx, message), errSyncNATSReference) {
		t.Fatal("a duplicate origin with different immutable reference content was accepted")
	}
	var injected map[string]any
	if json.Unmarshal(data, &injected) != nil {
		t.Fatal("non-secret negative envelope setup failed")
	}
	injected["body"] = "NON_SECRET_REJECTED_BODY_PROBE"
	message.Data, _ = json.Marshal(injected)
	if !errors.Is(a.syncNATSStore(ctx, message), errSyncNATSReference) {
		t.Fatal("transport accepted a field outside the body-free envelope")
	}
	originalRef, _ := json.Marshal(wire.Ref)
	var unchanged bool
	if a.DB.QueryRow(ctx, "SELECT count(*)=1 AND bool_and(ref=$2::jsonb) FROM sync.origins WHERE origin_event_id=$1", wire.OriginEventID, originalRef).Scan(&unchanged) != nil || !unchanged {
		t.Fatal("duplicate or rejected transport traffic changed its original stored reference")
	}
}

func assertSyncSnapshotReferencesOnly(t *testing.T, raw []byte) {
	t.Helper()
	var value any
	if json.Unmarshal(raw, &value) != nil {
		t.Fatal("stored synchronization metadata is not valid JSON")
	}
	forbidden := map[string]bool{"object": true, "body": true, "bodydigest": true, "text": true, "password": true, "passwordhash": true, "password_hash": true, "token": true, "refreshtoken": true, "privatekey": true, "ciphertext": true}
	var walk func(any)
	walk = func(node any) {
		switch node := node.(type) {
		case map[string]any:
			for key, child := range node {
				if forbidden[strings.ToLower(key)] {
					t.Fatal("a body or secret field was persisted in synchronization metadata; contents redacted")
				}
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(value)
}

func TestSyncSnapshotMetadataAndEncryptedPagesContainReferencesOnly(t *testing.T) {
	ctx, fixture := openSyncSnapshotTestFixture(t)
	a := fixture.app
	for _, owner := range syncOwners {
		rows, err := a.DB.Query(ctx, "SELECT metadata FROM "+pgx.Identifier{owner}.Sanitize()+".sync_outbox ORDER BY revision LIMIT 500")
		if err != nil {
			t.Fatal("owner metadata inspection unavailable")
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if rows.Scan(&raw) != nil {
				rows.Close()
				t.Fatal("owner metadata inspection failed")
			}
			assertSyncSnapshotReferencesOnly(t, raw)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal("owner metadata stream inspection failed")
		}
	}
	rows, err := a.DB.Query(ctx, "SELECT ref FROM sync.origins ORDER BY owner,revision LIMIT 1000")
	if err != nil {
		t.Fatal("transport origin reference inspection unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			rows.Close()
			t.Fatal("transport origin inspection failed")
		}
		assertSyncSnapshotReferencesOnly(t, raw)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal("transport origin stream inspection failed")
	}
	var manifestRaw []byte
	if a.DB.QueryRow(ctx, "SELECT manifest FROM sync.snapshots WHERE id=$1 AND user_id=$2 AND device_id=$3", fixture.snapshot.ID, fixture.user.ID, fixture.user.DeviceID).Scan(&manifestRaw) != nil {
		t.Fatal("first owned snapshot metadata unavailable")
	}
	assertSyncSnapshotReferencesOnly(t, manifestRaw)
	var manifest syncSnapshot
	if json.Unmarshal(manifestRaw, &manifest) != nil {
		t.Fatal("first owned snapshot manifest invalid")
	}
	rows, err = a.DB.Query(ctx, "SELECT page,refs_ciphertext,refs_digest FROM sync.snapshot_pages WHERE snapshot_id=$1 ORDER BY page", manifest.ID)
	if err != nil {
		t.Fatal("owned encrypted snapshot pages unavailable")
	}
	defer rows.Close()
	pages := 0
	for rows.Next() {
		var page int
		var ciphertext []byte
		var digest string
		if rows.Scan(&page, &ciphertext, &digest) != nil {
			rows.Close()
			t.Fatal("encrypted snapshot page inspection failed")
		}
		plain, err := a.unseal(ciphertext, "sync-refs:"+manifest.ID+":"+fmt.Sprint(page))
		if err != nil || page != pages || page >= len(manifest.PageManifest) || digest != manifest.PageManifest[page].Digest || syncDigest([]byte(plain)) != digest {
			rows.Close()
			t.Fatal("encrypted reference page failed its actual AAD or digest contract")
		}
		assertSyncSnapshotReferencesOnly(t, []byte(plain))
		var refs []syncRef
		if json.Unmarshal([]byte(plain), &refs) != nil || len(refs) != manifest.PageManifest[page].Count {
			rows.Close()
			t.Fatal("encrypted reference page does not match its complete manifest")
		}
		pages++
	}
	err = rows.Err()
	rows.Close()
	if err != nil || pages == 0 || pages != len(manifest.PageManifest) {
		t.Fatal("owned snapshot page manifest is incomplete")
	}
}

func TestSyncSnapshotCursorPreservesIntegersAboveFloatPrecision(t *testing.T) {
	if os.Getenv("JOBS_IM_SYNC_SNAPSHOT_READY") != "1" {
		t.Skip("explicit source execution window required")
	}
	a := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x53}, 32)}}
	user := User{ID: newID(), DeviceID: newID(), RestoreEpoch: "9007199254740993"}
	reg := syncRegistration{Store: newID(), Stream: newID()}
	permission := syncDigest([]byte("non-secret local permission test vector"))
	for _, position := range []string{"9007199254740992", "9007199254740993", "9223372036854775807"} {
		encoded := a.syncEncodeCursor(user, reg, position, permission)
		decoded, err := a.syncDecodeCursor(encoded, user, reg)
		if err != nil || decoded.Position != position {
			t.Fatal("signed cursor rounded or changed an exact integer position")
		}
		other := user
		other.DeviceID = newID()
		if _, err := a.syncDecodeCursor(encoded, other, reg); err == nil {
			t.Fatal("signed cursor crossed its original device namespace")
		}
	}
}

func TestSyncNATSTokenLocalPermission(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix permission probe runs on macOS/Linux only; real Windows DACL not verified")
	}
	directory := t.TempDir()
	const probe = "NON_SECRET_LOCAL_NATS_TOKEN_PROBE"
	writeOwned := func(name string, data []byte, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(directory, name)
		if os.WriteFile(path, data, 0600) != nil || os.Chmod(path, mode) != nil {
			t.Fatal("owned non-secret token fixture could not be prepared")
		}
		return path
	}
	private := writeOwned("private", []byte(probe), 0600)
	t.Run("current_owner_0600", func(t *testing.T) {
		value, err := syncNATSToken(private)
		if err != nil || value != probe {
			t.Fatal("current-owner 0600 token file was not accepted; contents redacted")
		}
	})
	for _, fixture := range []struct {
		name string
		path string
	}{
		{name: "group_world_0664_rejected", path: writeOwned("public", []byte(probe), 0664)},
		{name: "over_4KiB_rejected", path: writeOwned("oversize", bytes.Repeat([]byte{'X'}, 4097), 0600)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			value, err := syncNATSToken(fixture.path)
			if !errors.Is(err, errSyncNATSConfiguration) || value != "" {
				t.Fatal("unsafe token file was not rejected without returning its contents")
			}
		})
	}
	t.Run("symlink_rejected", func(t *testing.T) {
		link := filepath.Join(directory, "link")
		if os.Symlink(private, link) != nil {
			t.Fatal("owned token symlink fixture could not be prepared")
		}
		value, err := syncNATSToken(link)
		if !errors.Is(err, errSyncNATSConfiguration) || value != "" {
			t.Fatal("symlink token file was not rejected without returning its contents")
		}
	})
}

// The overlay runner includes sync_outbox_test.go alongside this file. Its
// existing suite creates its own database and never mutates the live realm's schema.
func TestSyncSnapshotOwnedOutboxTransactions(t *testing.T) {
	_, fixture := openSyncSnapshotTestFixture(t)
	t.Setenv("JOBS_IM_SYNC_OUTBOX_TEST", "1")
	t.Setenv("JOBS_IM_SYNC_OUTBOX_ADMIN_URL", syncSnapshotAdminURL(t, fixture.seed.BusinessURL))
	TestSyncOwnerOutboxContracts(t)
}

func TestSyncSnapshotReplayDoesNotCrossMissingCommittedOrigin(t *testing.T) {
	_, fixture := openSyncSnapshotTestFixture(t)
	t.Setenv("JOBS_IM_SYNC_OUTBOX_TEST", "1")
	t.Setenv("JOBS_IM_SYNC_OUTBOX_ADMIN_URL", syncSnapshotAdminURL(t, fixture.seed.BusinessURL))
	ctx, database, configuration, scope := newSyncOutboxTestRealm(t)
	var err error
	if _, err = database.Exec(ctx, migrations["sync"]); err != nil {
		t.Fatal("owned gap fixture sync schema unavailable")
	}
	if _, err = database.Exec(ctx, "CREATE TABLE sync.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,restore_epoch bigint NOT NULL)"); err != nil {
		t.Fatal("owned gap fixture scope table unavailable")
	}
	if _, err = database.Exec(ctx, "INSERT INTO sync.instance VALUES(true,$1,1)", scope); err != nil {
		t.Fatal("owned gap fixture scope unavailable")
	}
	// Only this new database is made deliberately incomplete; no live realm row is removed.
	profile := newID()
	for index := 0; index < 3; index++ {
		if _, err = database.Exec(ctx, "INSERT INTO directory.profiles(user_id,display_name) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET display_name=EXCLUDED.display_name", profile, fmt.Sprintf("owned gap reference %d", index)); err != nil {
			t.Fatal("owned gap fixture source change failed")
		}
	}
	if _, err = database.Exec(ctx, "DELETE FROM directory.sync_outbox WHERE revision=2"); err != nil {
		t.Fatal("owned gap fixture could not omit its middle pointer")
	}
	poolConfiguration, err := pgxpool.ParseConfig(fixture.seed.BusinessURL)
	if err != nil {
		t.Fatal("owned gap fixture pool configuration invalid")
	}
	poolConfiguration.ConnConfig = configuration.Copy()
	poolConfiguration.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, poolConfiguration)
	if err != nil {
		t.Fatal("owned gap fixture pool unavailable")
	}
	t.Cleanup(pool.Close)
	local, err := normalizeAdmissionState(Config{DeploymentID: scope, StateDir: t.TempDir(), Key: bytes.Repeat([]byte{0x47}, 32), RoleURLs: map[string]string{}})
	if err != nil {
		t.Fatal("owned gap fixture local proof unavailable")
	}
	for _, owner := range syncOwners {
		role := syncOwnerRole(owner)
		app := &App{Config: local, Role: role, DB: pool, Client: &http.Client{Timeout: 5 * time.Second}}
		server := httptest.NewServer(app.Handler())
		t.Cleanup(server.Close)
		local.RoleURLs[role] = server.URL
	}
	central := &App{Config: local, Role: "sync", DB: pool, Client: &http.Client{Timeout: 5 * time.Second}}
	err = central.syncImportOwners(ctx, User{ID: profile, RestoreEpoch: "1"})
	var rejection *apiError
	if !errors.As(err, &rejection) || rejection.Status != 503 || rejection.Code != "SYNC_OWNER_INCOMPLETE" {
		t.Fatal("owner replay crossed a missing committed middle pointer")
	}
	var imported int64
	var skipped int
	if database.QueryRow(ctx, "SELECT revision FROM sync.owner_import WHERE owner='directory'").Scan(&imported) != nil || imported > 1 {
		t.Fatal("owner replay advanced its durable boundary beyond the committed pointer gap")
	}
	if database.QueryRow(ctx, "SELECT count(*) FROM sync.origins WHERE owner='directory' AND revision=3").Scan(&skipped) != nil || skipped != 0 {
		t.Fatal("owner replay persisted a pointer after the unproved gap")
	}
}
