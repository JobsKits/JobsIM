package jobsim

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFreshPostgresDomainsMigrateFromEmptyDatabases(t *testing.T) {
	file := os.Getenv("JOBS_IM_MIGRATION_TEST_ENV")
	if file == "" {
		t.Skip("explicit private PostgreSQL test environment required")
	}
	text, e := os.ReadFile(file)
	if e != nil {
		t.Fatal("private test environment unavailable")
	}
	var environment struct {
		BusinessURL string `json:"businessURL"`
		JournalURL  string `json:"journalURL"`
		PGPassFile  string `json:"pgPassFile"`
	}
	if e = json.Unmarshal(text, &environment); e != nil {
		t.Fatal("invalid private environment")
	}
	if environment.PGPassFile != "" {
		t.Setenv("PGPASSFILE", environment.PGPassFile)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	targets := []string{}
	for _, source := range []string{environment.BusinessURL, environment.JournalURL} {
		admin, e := pgx.Connect(ctx, source)
		if e != nil {
			t.Fatal("test database operator unavailable")
		}
		name := "jobsim_migration_" + strings.ReplaceAll(newID(), "-", "")
		if _, e = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); e != nil {
			admin.Close(ctx)
			t.Fatal("could not create an isolated empty test database")
		}
		t.Cleanup(func() {
			cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_, err := admin.Exec(cleanup, `DROP DATABASE `+pgx.Identifier{name}.Sanitize())
			admin.Close(cleanup)
			if err != nil {
				t.Error("isolated test database cleanup failed")
			}
		})
		parsed, e := url.Parse(source)
		if e != nil {
			t.Fatal("invalid test URL")
		}
		parsed.Path = "/" + name
		targets = append(targets, parsed.String())
	}
	config, e := PrepareConfig(Config{DatabaseURL: targets[0], JournalDatabaseURL: targets[1], StateDir: filepath.Join(t.TempDir(), "State")})
	if e != nil {
		t.Fatal(e)
	}
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
		app, e := NewApp(ctx, config, role)
		if e != nil {
			t.Fatalf("fresh %s migration failed: %v", role, e)
		}
		if e = app.migrate(ctx); e != nil {
			app.Close()
			t.Fatalf("idempotent %s migration failed: %v", role, e)
		}
		if role == "identity" {
			// SQL rows with the old shape exercise invalidation only. This is
			// deliberately separate from a real old-binary migration claim.
			user, session, device, request := newID(), newID(), newID(), newID()
			if _, e = app.DB.Exec(ctx, `INSERT INTO auth.users(id,username,display_name,password_hash) VALUES($1,$2,'Legacy SQL fixture','not-a-login-hash')`, user, "legacy_"+user[:8]); e != nil {
				t.Fatal("legacy SQL principal fixture failed")
			}
			if _, e = app.DB.Exec(ctx, `INSERT INTO auth.sessions(id,user_id,device_id,device_name,token_digest,generation,expires_at) VALUES($1,$2,$3,'legacy SQL fixture',$4,1,now()+interval '7 days')`, session, user, device, tokenDigest(randomToken(32))); e != nil {
				t.Fatal("legacy SQL session fixture failed")
			}
			if _, e = app.DB.Exec(ctx, `INSERT INTO auth.local_auth_requests(request_id,purpose,request_digest,user_id,device_id,delivery_ciphertext,expires_at,delivery_version) VALUES($1,'login',$2,$3,$4,$5,now()+interval '300 seconds','bound-v1')`, request, tokenDigest(request), user, device, []byte("obsolete delivery fixture")); e != nil {
				t.Fatal("legacy SQL delivery fixture failed")
			}
			if e = app.migrate(ctx); e != nil {
				t.Fatal("legacy SQL fixture migration failed")
			}
			var safe bool
			if app.DB.QueryRow(ctx, `SELECT s.revoked_at IS NOT NULL AND s.family_id IS NULL AND s.refresh_generation=0 AND s.refresh_digest IS NULL AND a.session_id=s.id AND octet_length(a.delivery_ciphertext)=0 AND NOT EXISTS(SELECT 1 FROM auth.refresh_token_history h WHERE h.session_id=s.id) FROM auth.sessions s JOIN auth.local_auth_requests a ON a.request_id=$2 WHERE s.id=$1`, session, request).Scan(&safe) != nil || !safe {
				t.Fatal("legacy SQL token was promoted or its obsolete delivery survived")
			}
		}
		app.Close()
	}
}
