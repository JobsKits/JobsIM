package jobsim

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// This is the frozen 57a867 table shape, independent of future migrations["sync"].
const syncSchema57Fixture = `CREATE SCHEMA sync;
CREATE TABLE sync.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,migration_version int NOT NULL,restore_epoch bigint NOT NULL DEFAULT 1);
CREATE TABLE sync.origins(origin_event_id uuid PRIMARY KEY,scope_id uuid NOT NULL,owner text NOT NULL,revision bigint NOT NULL CHECK(revision>0),restore_epoch bigint NOT NULL CHECK(restore_epoch>0),ref jsonb NOT NULL,UNIQUE(owner,restore_epoch,revision));
CREATE TABLE sync.owner_import(owner text PRIMARY KEY,revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0));
CREATE TABLE sync.streams(id uuid PRIMARY KEY,user_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),restore_epoch bigint NOT NULL,head bigint NOT NULL DEFAULT 0 CHECK(head>=0),UNIQUE(user_id,source_context_id,restore_epoch));
CREATE TABLE sync.registrations(user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL,store_generation uuid NOT NULL,request_id text NOT NULL,stream_id uuid NOT NULL REFERENCES sync.streams(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,device_id,restore_epoch));
CREATE TABLE sync.stream_import(stream_id uuid NOT NULL REFERENCES sync.streams(id),owner text NOT NULL,revision bigint NOT NULL DEFAULT 0,PRIMARY KEY(stream_id,owner));
CREATE TABLE sync.events(stream_id uuid NOT NULL REFERENCES sync.streams(id),position bigint NOT NULL CHECK(position>0),origin_event_id uuid NOT NULL REFERENCES sync.origins(origin_event_id),ref jsonb NOT NULL,event_digest text NOT NULL,PRIMARY KEY(stream_id,position),UNIQUE(stream_id,origin_event_id));
CREATE TABLE sync.snapshots(id uuid PRIMARY KEY,user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL,request_id text NOT NULL,request_digest bytea NOT NULL,manifest jsonb NOT NULL,expires_at timestamptz NOT NULL,UNIQUE(user_id,device_id,restore_epoch,request_id));
CREATE TABLE sync.snapshot_pages(snapshot_id uuid NOT NULL REFERENCES sync.snapshots(id),page integer NOT NULL,refs_ciphertext bytea NOT NULL,refs_digest text NOT NULL,PRIMARY KEY(snapshot_id,page));
CREATE INDEX sync_origins_revision ON sync.origins(owner,revision);
CREATE INDEX sync_snapshot_expiry ON sync.snapshots(expires_at);
ALTER TABLE sync.origins ADD CONSTRAINT sync_ref_metadata CHECK(jsonb_typeof(ref)='object' AND (ref-ARRAY['owner','kind','id','revision','revisionEpoch','subjectUserId','conversationId','targetUserId','state']::text[])='{}'::jsonb);
ALTER TABLE sync.events ADD CONSTRAINT sync_ref_metadata CHECK(jsonb_typeof(ref)='object' AND (ref-ARRAY['owner','kind','id','revision','revisionEpoch','subjectUserId','conversationId','targetUserId','state']::text[])='{}'::jsonb);`

func newSyncSchemaTestRealm(t *testing.T) (context.Context, *pgx.Conn, string) {
	t.Helper()
	if os.Getenv("JOBS_IM_SYNC_SCHEMAS_TEST") != "1" {
		t.Skip("explicit future isolated sync-schema test window required")
	}
	address, err := url.Parse(os.Getenv("JOBS_IM_SYNC_SCHEMAS_REALM_URL"))
	if err != nil || address.Scheme != "https" || address.User != nil || address.Port() == "" || address.Port() == "44443" || net.ParseIP(address.Hostname()) == nil || !net.ParseIP(address.Hostname()).IsLoopback() {
		t.Fatal("future loopback TLS realm distinct from frozen 44443 required")
	}
	dsn := os.Getenv("JOBS_IM_SYNC_SCHEMAS_ADMIN_URL")
	configuration, err := pgx.ParseConfig(dsn)
	if dsn == "" || err != nil || configuration.Database != "postgres" {
		t.Fatal("explicit PostgreSQL maintenance URL required; details redacted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	guardS2PGConnection(t, configuration, "")
	admin, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("explicit sync-schema creation connection unavailable")
	}
	name := "jobsim_sync_schemas_" + strings.ReplaceAll(newID(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		admin.Close(ctx)
		t.Fatal("owned sync-schema database could not be created")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted); err != nil {
			t.Error("owned sync-schema cleanup failed; no shared connection was terminated")
		}
		admin.Close(cleanup)
	})
	child := configuration.Copy()
	child.Database = name
	child.RuntimeParams["application_name"] = "jobsim-sync-schemas-test"
	child.RuntimeParams["statement_timeout"] = "15000"
	guardS2PGConnection(t, child, "jobsim_sync_schemas_")
	db, err := pgx.ConnectConfig(ctx, child)
	if err != nil {
		t.Fatal("owned sync-schema database unavailable")
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	if _, err = db.Exec(ctx, syncSchema57Fixture); err != nil {
		t.Fatal("owned frozen-57 sync schema fixture failed")
	}
	scope := newID()
	if _, err = db.Exec(ctx, "INSERT INTO sync.instance VALUES(true,$1,1,1)", scope); err != nil {
		t.Fatal("owned frozen-57 instance fixture failed")
	}
	return ctx, db, scope
}

func syncSchemaTestExec(t *testing.T, ctx context.Context, db *pgx.Conn, statement string, args ...any) {
	t.Helper()
	if _, err := db.Exec(ctx, statement, args...); err != nil {
		t.Fatal("owned sync-schema fixture mutation failed; details redacted")
	}
}

func syncSchemaTestMigration(t *testing.T, ctx context.Context, db *pgx.Conn, scope string) {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal("owned sync-schema migration transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if migrateSyncSchemas(ctx, tx, scope) != nil || tx.Commit(ctx) != nil {
		t.Fatal("owned sync-schema migration failed; details redacted")
	}
}

func syncSchemaTestOID(t *testing.T, ctx context.Context, db *pgx.Conn, table, name string) uint32 {
	t.Helper()
	var oid uint32
	if db.QueryRow(ctx, "SELECT oid FROM pg_constraint WHERE conrelid=$1::regclass AND conname=$2", "sync."+table, name).Scan(&oid) != nil {
		t.Fatal("owned expected sync constraint unavailable")
	}
	return oid
}

func TestSyncProtocolSchemaCanonical(t *testing.T) {
	for input, expected := range map[string]string{"": "1", "1": "1", "2": "2"} {
		actual, err := syncProtocolSchema(input)
		if err != nil || actual != expected {
			t.Fatal("canonical sync protocol schema was not retained")
		}
	}
	for _, input := range []string{"0", "01", "02", "3", " 2", "2 ", "2.0", "profile", "personal"} {
		_, err := syncProtocolSchema(input)
		var rejection *apiError
		if !errors.As(err, &rejection) || rejection.Status != 400 || rejection.Code != "UNSUPPORTED_SYNC_SCHEMA" {
			t.Fatal("unsupported sync schema was silently normalized or accepted")
		}
	}
}

func TestSyncSchemasPreserve57StateAndRepeatConstraintIDs(t *testing.T) {
	ctx, db, scope := newSyncSchemaTestRealm(t)
	user, device, store, stream, origin, snapshot := newID(), newID(), newID(), newID(), newID(), newID()
	const head = "9007199254740993"
	key := bytes.Repeat([]byte{0x5a}, 32)
	oldBody, _ := json.Marshal(map[string]string{"scope": scope, "user": user, "source": "personal", "device": device, "store": store, "epoch": "1", "stream": stream, "position": head, "permission": "non-secret-frozen-57-permission"})
	oldCursor := base64.RawURLEncoding.EncodeToString(oldBody) + "." + base64.RawURLEncoding.EncodeToString(keyedDigest(key, "sync-cursor-v1", oldBody))
	manifest, _ := json.Marshal(map[string]string{"schemaVersion": "1", "boundaryToken": oldCursor})
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.streams VALUES($1,$2,'personal',1,$3)", stream, user, head)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.registrations VALUES($1,$2,1,$3,'frozen57-registration',$4,'2026-11-17T00:00:00Z')", user, device, store, stream)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.origins VALUES($1,$2,'auth',1,1,'{}')", origin, scope)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.events VALUES($1,$2,$3,'{}','non-secret-existing-event-digest')", stream, head, origin)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.stream_import VALUES($1,'auth',1)", stream)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.snapshots VALUES($1,$2,$3,1,'frozen57-snapshot',$4,$5,'2040-01-01T00:00:00Z')", snapshot, user, device, bytes.Repeat([]byte{0x17}, 32), manifest)
	pageBytes := []byte("NON_SECRET_EXISTING_REFERENCE_CIPHERTEXT")
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.snapshot_pages VALUES($1,0,$2,'non-secret-existing-page-digest')", snapshot, pageBytes)
	syncSchemaTestExec(t, ctx, db, "ALTER TABLE sync.streams ADD CONSTRAINT user_stream_head_check CHECK(head>=0); ALTER TABLE sync.registrations ADD CONSTRAINT user_registration_request_key UNIQUE(request_id)")
	customStream := syncSchemaTestOID(t, ctx, db, "streams", "user_stream_head_check")
	customRegistration := syncSchemaTestOID(t, ctx, db, "registrations", "user_registration_request_key")
	syncSchemaTestMigration(t, ctx, db, scope)
	var gotStream, gotHead, gotStore, gotRequest, gotSchema, gotCursor string
	if db.QueryRow(ctx, "SELECT id::text,head::text,schema_version FROM sync.streams WHERE id=$1", stream).Scan(&gotStream, &gotHead, &gotSchema) != nil || gotStream != stream || gotHead != head || gotSchema != "1" {
		t.Fatal("migration replaced an existing stream identity, exact head or default schema")
	}
	if db.QueryRow(ctx, "SELECT store_generation::text,request_id,schema_version FROM sync.registrations WHERE stream_id=$1", stream).Scan(&gotStore, &gotRequest, &gotSchema) != nil || gotStore != store || gotRequest != "frozen57-registration" || gotSchema != "1" {
		t.Fatal("migration replaced an existing device registration or store")
	}
	var keptPage []byte
	if db.QueryRow(ctx, "SELECT s.manifest->>'boundaryToken',p.refs_ciphertext FROM sync.snapshots s JOIN sync.snapshot_pages p ON p.snapshot_id=s.id WHERE s.id=$1", snapshot).Scan(&gotCursor, &keptPage) != nil || gotCursor != oldCursor || !bytes.Equal(keptPage, pageBytes) {
		t.Fatal("migration changed an existing frozen-57 cursor or opaque page bytes")
	}
	var keptEvent, keptImport bool
	if db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sync.events WHERE stream_id=$1 AND position=$2 AND origin_event_id=$3),EXISTS(SELECT 1 FROM sync.stream_import WHERE stream_id=$1 AND owner='auth' AND revision=1)", stream, head, origin).Scan(&keptEvent, &keptImport) != nil || !keptEvent || !keptImport {
		t.Fatal("migration lost original event or owner-import rows")
	}
	var deviceCount int
	if db.QueryRow(ctx, "SELECT count(*) FROM sync.device_stores WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=1 AND store_generation=$4", scope, user, device, store).Scan(&deviceCount) != nil || deviceCount != 1 {
		t.Fatal("schema-one physical device store was not backfilled exactly once")
	}
	streamKey := syncSchemaTestOID(t, ctx, db, "streams", "streams_user_id_source_context_id_restore_epoch_key")
	registrationKey := syncSchemaTestOID(t, ctx, db, "registrations", "registrations_pkey")
	streamCheck := syncSchemaTestOID(t, ctx, db, "streams", "streams_protocol_schema_check")
	registrationCheck := syncSchemaTestOID(t, ctx, db, "registrations", "registrations_protocol_schema_check")
	syncSchemaTestMigration(t, ctx, db, scope)
	for _, expectation := range []struct {
		table string
		name  string
		oid   uint32
	}{{"streams", "streams_user_id_source_context_id_restore_epoch_key", streamKey}, {"registrations", "registrations_pkey", registrationKey}, {"streams", "streams_protocol_schema_check", streamCheck}, {"registrations", "registrations_protocol_schema_check", registrationCheck}, {"streams", "user_stream_head_check", customStream}, {"registrations", "user_registration_request_key", customRegistration}} {
		if syncSchemaTestOID(t, ctx, db, expectation.table, expectation.name) != expectation.oid {
			t.Fatal("repeat migration recreated a new key/check or touched an unrelated user constraint")
		}
	}
	secondStream := newID()
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.streams(id,user_id,source_context_id,restore_epoch,schema_version) VALUES($1,$2,'personal',1,'2')", secondStream, user)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.registrations(user_id,device_id,restore_epoch,store_generation,request_id,stream_id,schema_version) VALUES($1,$2,1,$3,'schema-two-registration',$4,'2')", user, device, store, secondStream)
	syncSchemaTestMigration(t, ctx, db, scope)
	var streams, registrations int
	if db.QueryRow(ctx, "SELECT (SELECT count(*) FROM sync.streams WHERE user_id=$1),(SELECT count(*) FROM sync.registrations WHERE user_id=$1),(SELECT count(*) FROM sync.device_stores WHERE user_id=$1)", user).Scan(&streams, &registrations, &deviceCount) != nil || streams != 2 || registrations != 2 || deviceCount != 1 {
		t.Fatal("schema-one/two streams did not share one physical device and original store")
	}
}

func TestSyncSchemasConflictingBackfillRollsBackAllDDL(t *testing.T) {
	ctx, db, scope := newSyncSchemaTestRealm(t)
	// Model an already-expanded multi-schema deployment that lacks the new
	// physical-store table; do not use the function under test to hide a conflict.
	syncSchemaTestExec(t, ctx, db, `ALTER TABLE sync.streams ADD COLUMN schema_version text NOT NULL DEFAULT '1';
ALTER TABLE sync.registrations ADD COLUMN schema_version text NOT NULL DEFAULT '1';
ALTER TABLE sync.streams DROP CONSTRAINT streams_user_id_source_context_id_restore_epoch_key;
ALTER TABLE sync.streams ADD CONSTRAINT streams_user_id_source_context_id_restore_epoch_key UNIQUE(user_id,source_context_id,restore_epoch,schema_version);
ALTER TABLE sync.registrations DROP CONSTRAINT registrations_pkey;
ALTER TABLE sync.registrations ADD CONSTRAINT registrations_pkey PRIMARY KEY(user_id,device_id,restore_epoch,schema_version);`)
	user, device := newID(), newID()
	for _, schema := range []string{"1", "2"} {
		stream := newID()
		syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.streams(id,user_id,source_context_id,restore_epoch,schema_version) VALUES($1,$2,'personal',1,$3)", stream, user, schema)
		syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.registrations(user_id,device_id,restore_epoch,store_generation,request_id,stream_id,schema_version) VALUES($1,$2,1,$3,$4,$5,$6)", user, device, newID(), newID(), stream, schema)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal("owned conflicting migration transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if migrateSyncSchemas(ctx, tx, scope) == nil {
		t.Fatal("conflicting schema stores were silently selected or ignored")
	}
	var noTable bool
	var newChecks int
	if tx.QueryRow(ctx, "SELECT to_regclass('sync.device_stores') IS NULL,(SELECT count(*) FROM pg_constraint WHERE conrelid IN('sync.streams'::regclass,'sync.registrations'::regclass) AND conname IN('streams_protocol_schema_check','registrations_protocol_schema_check'))").Scan(&noTable, &newChecks) != nil || !noTable || newChecks != 0 {
		t.Fatal("failed logical backfill retained partial DDL or left the caller transaction unusable")
	}
	if tx.Commit(ctx) != nil {
		t.Fatal("rolled-back migration savepoint poisoned its unchanged outer transaction")
	}
}

func TestSyncSchemasExistingPhysicalStoreConflictIsNotIgnored(t *testing.T) {
	ctx, db, scope := newSyncSchemaTestRealm(t)
	user, device, stream, store := newID(), newID(), newID(), newID()
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.streams VALUES($1,$2,'personal',1,0)", stream, user)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.registrations(user_id,device_id,restore_epoch,store_generation,request_id,stream_id) VALUES($1,$2,1,$3,$4,$5)", user, device, store, newID(), stream)
	syncSchemaTestMigration(t, ctx, db, scope)
	second := newID()
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.streams(id,user_id,source_context_id,restore_epoch,schema_version) VALUES($1,$2,'personal',1,'2')", second, user)
	syncSchemaTestExec(t, ctx, db, "INSERT INTO sync.registrations(user_id,device_id,restore_epoch,store_generation,request_id,stream_id,schema_version) VALUES($1,$2,1,$3,$4,$5,'2')", user, device, newID(), newID(), second)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal("owned store-conflict migration transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if migrateSyncSchemas(ctx, tx, scope) == nil {
		t.Fatal("existing physical store conflict bypassed schema-independent identity")
	}
	var original string
	if tx.QueryRow(ctx, "SELECT store_generation::text FROM sync.device_stores WHERE scope_id=$1 AND user_id=$2 AND device_id=$3 AND restore_epoch=1", scope, user, device).Scan(&original) != nil || original != store {
		t.Fatal("failed backfill replaced the original physical store")
	}
}

func TestSyncSchemasScopeMismatchMakesNoColumns(t *testing.T) {
	ctx, db, _ := newSyncSchemaTestRealm(t)
	for _, scope := range []string{"not-a-uuid", newID()} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal("owned scope-rejection transaction unavailable")
		}
		if migrateSyncSchemas(ctx, tx, scope) == nil {
			tx.Rollback(ctx)
			t.Fatal("invalid or foreign deployment scope was accepted")
		}
		var untouched bool
		if tx.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='sync.streams'::regclass AND attname='schema_version' AND NOT attisdropped) AND to_regclass('sync.device_stores') IS NULL").Scan(&untouched) != nil || !untouched {
			tx.Rollback(ctx)
			t.Fatal("rejected scope changed the frozen sync schema")
		}
		if tx.Commit(ctx) != nil {
			t.Fatal("scope rejection poisoned the unchanged caller transaction")
		}
	}
}
