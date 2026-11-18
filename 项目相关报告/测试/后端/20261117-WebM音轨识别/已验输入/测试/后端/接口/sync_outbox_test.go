package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// This suite creates and drops only its own new database. It never wraps shared
// runtime writes in a rollback transaction or connects to an HTTP service.
func newSyncOutboxTestRealm(t *testing.T) (context.Context, *pgx.Conn, *pgx.ConnConfig, string) {
	t.Helper()
	if os.Getenv("JOBS_IM_SYNC_OUTBOX_TEST") != "1" {
		t.Skip("explicit isolated sync outbox test switch required")
	}
	dsn := os.Getenv("JOBS_IM_SYNC_OUTBOX_ADMIN_URL")
	if dsn == "" {
		t.Fatal("explicit database creation connection required")
	}
	configuration, err := pgx.ParseConfig(dsn)
	if err != nil || configuration.Database == "" {
		t.Fatal("explicit database creation connection invalid; details redacted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("database creation connection unavailable; details redacted")
	}
	name := "jobsim_sync_outbox_" + strings.ReplaceAll(newID(), "-", "")
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier+" TEMPLATE template0"); err != nil {
		admin.Close(ctx)
		t.Fatal("fresh isolated test database could not be created; details redacted")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier); err != nil {
			t.Error("isolated sync test database cleanup failed; no shared connections were terminated")
		}
		admin.Close(cleanup)
	})
	isolated := configuration.Copy()
	isolated.Database = name
	isolated.RuntimeParams["application_name"] = "jobsim-sync-outbox-contract-test"
	isolated.RuntimeParams["statement_timeout"] = "15000"
	isolated.RuntimeParams["lock_timeout"] = "10000"
	db, err := pgx.ConnectConfig(ctx, isolated)
	if err != nil {
		t.Fatal("fresh isolated database unavailable; details redacted")
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	scope := newID()
	for _, pair := range [][2]string{{"auth", "identity"}, {"directory", "directory"}, {"conversation", "conversation"}, {"message", "message"}} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal("isolated migration transaction unavailable")
		}
		schema := pgx.Identifier{pair[0]}.Sanitize()
		_, err = tx.Exec(ctx, migrations[pair[1]])
		if err == nil {
			_, err = tx.Exec(ctx, "CREATE TABLE "+schema+".instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,restore_epoch bigint NOT NULL CHECK(restore_epoch>0))")
		}
		if err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO "+schema+".instance VALUES(true,$1,1)", scope)
		}
		if err == nil {
			err = migrateSyncOwner(ctx, tx, pair[0], scope)
		}
		if err != nil {
			tx.Rollback(ctx)
			t.Fatal("isolated sync owner migration failed; details redacted")
		}
		if tx.Commit(ctx) != nil {
			t.Fatal("isolated sync owner migration did not commit")
		}
	}
	return ctx, db, isolated, scope
}

func syncOutboxScalar(t *testing.T, ctx context.Context, db *pgx.Conn, query string, args ...any) int64 {
	t.Helper()
	var result int64
	if db.QueryRow(ctx, query, args...).Scan(&result) != nil {
		t.Fatal("isolated sync assertion query failed; details redacted")
	}
	return result
}

func syncOutboxExec(t *testing.T, ctx context.Context, db *pgx.Conn, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(ctx, query, args...); err != nil {
		t.Fatal("isolated sync fixture operation failed; details redacted")
	}
}

func TestSyncOwnerOutboxContracts(t *testing.T) {
	ctx, db, configuration, scope := newSyncOutboxTestRealm(t)
	alice, bob, conversation, message := newID(), newID(), newID(), newID()
	const marker = "NON_SECRET_SYNC_PAYLOAD_SENTINEL"
	for _, user := range []string{alice, bob} {
		syncOutboxExec(t, ctx, db, "INSERT INTO auth.users(id,username,display_name,password_hash) VALUES($1,$2,$3,$3)", user, "sync_"+strings.ReplaceAll(user, "-", "")[:12], marker)
		syncOutboxExec(t, ctx, db, "INSERT INTO directory.profiles(user_id,display_name) VALUES($1,$2)", user, marker)
	}

	t.Run("minimal_metadata_and_related_objects", func(t *testing.T) {
		session := newID()
		syncOutboxExec(t, ctx, db, "INSERT INTO auth.sessions(id,user_id,device_id,device_name,token_digest,generation,expires_at) VALUES($1,$2,$3,$4,$5,1,now()+interval '1 hour')", session, alice, newID(), marker, []byte(marker))
		syncOutboxExec(t, ctx, db, "INSERT INTO directory.contacts(owner_id,peer_id,state) VALUES($1,$2,'accepted')", alice, bob)
		syncOutboxExec(t, ctx, db, "INSERT INTO directory.blocks(owner_id,peer_id) VALUES($1,$2)", bob, alice)
		syncOutboxExec(t, ctx, db, "INSERT INTO directory.contact_requests(id,request_id,requester_id,recipient_id,state,generation) VALUES($1,$2,$3,$4,'pending',1)", newID(), newID(), alice, bob)
		syncOutboxExec(t, ctx, db, "INSERT INTO conversation.conversations(id,kind,title,owner_id) VALUES($1,'group',$2,$3)", conversation, marker, alice)
		for _, user := range []string{alice, bob} {
			syncOutboxExec(t, ctx, db, "INSERT INTO conversation.members(conversation_id,user_id,role) VALUES($1,$2,'member')", conversation, user)
		}
		permission := syncOutboxScalar(t, ctx, db, "SELECT version FROM conversation.sync_permission_versions WHERE user_id=$1", bob)
		syncOutboxExec(t, ctx, db, "UPDATE conversation.conversations SET title='another non-secret title' WHERE id=$1", conversation)
		if syncOutboxScalar(t, ctx, db, "SELECT version FROM conversation.sync_permission_versions WHERE user_id=$1", bob) != permission {
			t.Fatal("a title change incorrectly changed member permission generations")
		}
		syncOutboxExec(t, ctx, db, "UPDATE conversation.conversations SET announcement_message_id=$2,announcement_source_version=1,announcement_version=1,announcement_updated_at=now() WHERE id=$1", conversation, message)
		if syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM conversation.sync_outbox WHERE kind='announcement' AND object_id=$1 AND conversation_id=$1", conversation) != 1 {
			t.Fatal("an announcement change did not produce exactly one related reference")
		}
		syncOutboxExec(t, ctx, db, "UPDATE conversation.conversations SET state='dissolved',version=version+1 WHERE id=$1", conversation)
		if syncOutboxScalar(t, ctx, db, "SELECT version FROM conversation.sync_permission_versions WHERE user_id=$1", bob) <= permission {
			t.Fatal("a conversation state change did not invalidate member permissions")
		}
		syncOutboxExec(t, ctx, db, "INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text',$6,1)", message, conversation, alice, newID(), []byte(marker), marker)
		if syncOutboxScalar(t, ctx, db, "SELECT revision FROM message.sync_clock WHERE singleton") != 0 {
			t.Fatal("message storage without its owner event invented a sync event")
		}
		syncOutboxExec(t, ctx, db, "INSERT INTO message.events(conversation_id,sequence,message_id) VALUES($1,1,$2)", conversation, message)
		var metadata []byte
		if db.QueryRow(ctx, "SELECT metadata FROM message.sync_outbox WHERE kind='message' AND object_id=$1", message).Scan(&metadata) != nil {
			t.Fatal("message reference metadata unavailable")
		}
		var fields map[string]string
		if json.Unmarshal(metadata, &fields) != nil || len(fields) != 3 || fields["sourceVersion"] != "1" || fields["action"] != "insert" {
			t.Fatal("message event metadata does not match the minimal versioned contract")
		}
		if _, err := time.Parse("2006-01-02T15:04:05.000000Z", fields["createdAt"]); err != nil {
			t.Fatal("message source timestamp is not an explicit UTC timestamp")
		}
		for _, owner := range []string{"auth", "directory", "conversation", "message"} {
			rows, err := db.Query(ctx, "SELECT to_jsonb(o) FROM "+pgx.Identifier{owner}.Sanitize()+".sync_outbox o")
			if err != nil {
				t.Fatal("isolated outbox inspection failed")
			}
			count := 0
			for rows.Next() {
				var raw []byte
				if rows.Scan(&raw) != nil || bytes.Contains(raw, []byte(marker)) || bytes.Contains(raw, []byte("password_hash")) || bytes.Contains(raw, []byte("token_digest")) || bytes.Contains(raw, []byte("attachment_id")) || bytes.Contains(raw, []byte("ciphertext")) {
					rows.Close()
					t.Fatal("owner outbox leaked payload or credential fields; contents redacted")
				}
				count++
			}
			err = rows.Err()
			rows.Close()
			if err != nil || count == 0 {
				t.Fatal("owner change references missing or unreadable")
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal("metadata constraint transaction unavailable")
			}
			_, err = tx.Exec(ctx, "UPDATE "+pgx.Identifier{owner}.Sanitize()+".sync_outbox SET metadata=metadata||jsonb_build_object('text','not a permitted field')")
			tx.Rollback(ctx)
			if err == nil {
				t.Fatal("sync outbox accepted a body field outside its SQL whitelist")
			}
		}
		for _, user := range []string{alice, bob} {
			if syncOutboxScalar(t, ctx, db, "SELECT version FROM directory.sync_permission_versions WHERE user_id=$1", user) != 2 {
				t.Fatal("contact and block changes did not invalidate both users once each")
			}
		}
		syncOutboxExec(t, ctx, db, "DELETE FROM auth.sessions WHERE id=$1", session)
		syncOutboxExec(t, ctx, db, "DELETE FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2", alice, bob)
		if syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM auth.sync_object_versions WHERE kind='devices' AND object_id=$1 AND state='active'", alice) != 1 || syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM directory.sync_object_versions WHERE kind='contacts' AND object_id=$1 AND state='active'", alice) != 1 {
			t.Fatal("deleting an aggregate member incorrectly removed its entire aggregate")
		}
		removed := newID()
		syncOutboxExec(t, ctx, db, "INSERT INTO directory.profiles(user_id,display_name) VALUES($1,'temporary')", removed)
		syncOutboxExec(t, ctx, db, "DELETE FROM directory.profiles WHERE user_id=$1", removed)
		if syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM directory.sync_object_versions WHERE kind='profile' AND object_id=$1 AND state='removed'", removed) != 1 {
			t.Fatal("independent object deletion did not retain a removal reference")
		}
	})

	t.Run("rollback_restores_clock_versions_and_outbox", func(t *testing.T) {
		before := syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton")
		version := syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_object_versions WHERE kind='profile' AND object_id=$1", alice)
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal("rollback transaction unavailable")
		}
		if _, err = tx.Exec(ctx, "UPDATE directory.profiles SET display_name='rolled back' WHERE user_id=$1", alice); err != nil {
			tx.Rollback(ctx)
			t.Fatal("rollback fixture change failed")
		}
		if tx.Rollback(ctx) != nil {
			t.Fatal("rollback did not complete")
		}
		if syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton") != before || syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM directory.sync_outbox WHERE revision>$1", before) != 0 || syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_object_versions WHERE kind='profile' AND object_id=$1", alice) != version {
			t.Fatal("rolled-back business changes escaped into the sync change index")
		}
		syncOutboxExec(t, ctx, db, "UPDATE directory.profiles SET display_name='committed after rollback' WHERE user_id=$1", alice)
		if syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton") != before+1 {
			t.Fatal("rollback left a sequence allocation hole")
		}
	})

	t.Run("uncommitted_revision_cannot_be_overtaken", func(t *testing.T) {
		first, err := pgx.ConnectConfig(ctx, configuration.Copy())
		if err != nil {
			t.Fatal("first isolated concurrency connection unavailable")
		}
		defer first.Close(context.Background())
		second, err := pgx.ConnectConfig(ctx, configuration.Copy())
		if err != nil {
			t.Fatal("second isolated concurrency connection unavailable")
		}
		defer second.Close(context.Background())
		before := syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton")
		a, err := first.Begin(ctx)
		if err != nil {
			t.Fatal("first concurrency transaction unavailable")
		}
		defer a.Rollback(context.Background())
		b, err := second.Begin(ctx)
		if err != nil {
			t.Fatal("second concurrency transaction unavailable")
		}
		if _, err = a.Exec(ctx, "UPDATE directory.profiles SET display_name='first pending' WHERE user_id=$1", alice); err != nil {
			b.Rollback(context.Background())
			t.Fatal("first concurrency change failed")
		}
		finished := make(chan error, 1)
		waitingContext, stopWaiting := context.WithCancel(ctx)
		joined := false
		go func() {
			_, err := b.Exec(waitingContext, "UPDATE directory.profiles SET display_name='second pending' WHERE user_id=$1", bob)
			finished <- err
		}()
		defer func() {
			stopWaiting()
			if !joined {
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					second.Close(context.Background())
				}
			}
			b.Rollback(context.Background())
		}()
		waiting := false
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-finished:
				joined = true
				t.Fatal("later owner revision completed before the prior transaction settled")
			default:
			}
			if db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND datname=current_database() AND wait_event_type='Lock')", second.PgConn().PID()).Scan(&waiting) != nil {
				t.Fatal("isolated lock wait evidence unavailable")
			}
			if waiting {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !waiting || syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton") != before || syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM directory.sync_outbox WHERE revision>$1", before) != 0 {
			t.Fatal("uncommitted revisions were published or clock serialization was not observed")
		}
		if a.Commit(ctx) != nil {
			t.Fatal("first concurrency commit failed")
		}
		select {
		case err = <-finished:
			joined = true
			if err != nil {
				t.Fatal("second concurrency change failed after first commit")
			}
		case <-ctx.Done():
			t.Fatal("second concurrency change did not resume")
		}
		if syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton") != before+1 {
			t.Fatal("later uncommitted revision became visible early")
		}
		if b.Commit(ctx) != nil {
			t.Fatal("second concurrency commit failed")
		}
		if syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton") != before+2 || syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM directory.sync_outbox WHERE revision>$1", before) != 2 {
			t.Fatal("committed owner revisions were lost or duplicated")
		}
	})

	t.Run("repeat_migration_preserves_revision_and_events", func(t *testing.T) {
		for _, owner := range []string{"auth", "directory", "conversation", "message"} {
			schema := pgx.Identifier{owner}.Sanitize()
			before := syncOutboxScalar(t, ctx, db, "SELECT revision FROM "+schema+".sync_clock WHERE singleton")
			count := syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM "+schema+".sync_outbox")
			for range 2 {
				tx, err := db.Begin(ctx)
				if err != nil {
					t.Fatal("repeat migration transaction unavailable")
				}
				if err = migrateSyncOwner(ctx, tx, owner, scope); err != nil {
					tx.Rollback(ctx)
					t.Fatal("repeat owner migration failed; details redacted")
				}
				if tx.Commit(ctx) != nil {
					t.Fatal("repeat owner migration commit failed")
				}
			}
			if syncOutboxScalar(t, ctx, db, "SELECT revision FROM "+schema+".sync_clock WHERE singleton") != before || syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM "+schema+".sync_outbox") != count {
				t.Fatal("repeat migration reset progress or invented baseline events")
			}
			if owner == "directory" {
				syncOutboxExec(t, ctx, db, "UPDATE directory.profiles SET display_name='one change after repeat migration' WHERE user_id=$1", alice)
				if syncOutboxScalar(t, ctx, db, "SELECT revision FROM directory.sync_clock WHERE singleton") != before+1 || syncOutboxScalar(t, ctx, db, "SELECT count(*) FROM directory.sync_outbox") != count+1 {
					t.Fatal("repeat migration duplicated or lost the capture trigger")
				}
			}
		}
	})

	t.Run("owner_and_scope_are_closed", func(t *testing.T) {
		for _, input := range [][2]string{{"media", scope}, {"directory;SELECT 1", scope}, {"auth", newID()}, {"auth", "not-a-scope"}} {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal("closed-boundary transaction unavailable")
			}
			err = migrateSyncOwner(ctx, tx, input[0], input[1])
			tx.Rollback(ctx)
			if err == nil {
				t.Fatal("owner/scope boundary accepted an invalid migration input")
			}
		}
	})
}
