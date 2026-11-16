package jobsim

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The existing feature gates authorize execution; this guard only confines every
// connection, including libpq fallbacks, to the separately approved S2 cluster.
func guardS2PGConnection(t *testing.T, configuration *pgx.ConnConfig, databasePrefix string) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal("S2 PostgreSQL guard requires the current home directory")
	}
	owner, valid := s2PGPassfileOwner(os.Getenv("PGPASSFILE"), home)
	if !valid || !s2PGConnectionMatches(configuration, databasePrefix, owner) {
		t.Fatal("S2 PostgreSQL connection rejected by private-run, endpoint, owner or database guard; details redacted")
	}
}

func s2PGPassfileOwner(passfile, home string) (string, bool) {
	if passfile == "" || !filepath.IsAbs(passfile) || filepath.Clean(passfile) != passfile {
		return "", false
	}
	base := filepath.Join(home, "Library", "Application Support", "JobsIM", "Integration")
	relative, err := filepath.Rel(base, passfile)
	if err != nil {
		return "", false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 4 || !syncValidationID(parts[0]) || parts[1] != "Safety" || parts[2] != "Secrets" || parts[3] != "pgpass" {
		return "", false
	}
	info, err := os.Lstat(passfile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(passfile)
	if err != nil || resolved != passfile {
		return "", false
	}
	run := strings.ReplaceAll(parts[0], "-", "")
	return "s2_owner_" + run[:12], true
}

func s2PGConnectionMatches(configuration *pgx.ConnConfig, prefix, owner string) bool {
	if configuration == nil || owner == "" || configuration.Host != "127.0.0.1" || configuration.Port != 62381 || configuration.User != owner {
		return false
	}
	for _, fallback := range configuration.Fallbacks {
		if fallback == nil || fallback.Host != "127.0.0.1" || fallback.Port != 62381 {
			return false
		}
	}
	if prefix == "" {
		return configuration.Database == "postgres"
	}
	switch prefix {
	case "jobsim_source_preferences_", "jobsim_read_point_", "jobsim_sync_schemas_", "jobsim_self_read_":
	default:
		return false
	}
	if !strings.HasPrefix(configuration.Database, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(configuration.Database, prefix)
	decoded, err := hex.DecodeString(suffix)
	return err == nil && len(decoded) == 16 && strings.ToLower(suffix) == suffix
}

func TestS2PGGuardRejectsForeignConnections(t *testing.T) {
	owner := "s2_owner_" + strings.ReplaceAll(newID(), "-", "")[:12]
	configuration, err := pgx.ParseConfig("host=127.0.0.1 port=62381 user=" + owner + " dbname=postgres sslmode=disable")
	if err != nil || !s2PGConnectionMatches(configuration, "", owner) {
		t.Fatal("nonsecret S2 maintenance configuration was rejected")
	}
	for _, mutate := range []func(*pgx.ConnConfig){
		func(c *pgx.ConnConfig) { c.Host = "localhost" },
		func(c *pgx.ConnConfig) { c.Port = 62382 },
		func(c *pgx.ConnConfig) { c.User += "_foreign" },
		func(c *pgx.ConnConfig) { c.Database = "shared_business" },
		func(c *pgx.ConnConfig) { c.Fallbacks = []*pgconn.FallbackConfig{{Host: "127.0.0.1", Port: 62382}} },
		func(c *pgx.ConnConfig) { c.Fallbacks = []*pgconn.FallbackConfig{{Host: "localhost", Port: 62381}} },
		func(c *pgx.ConnConfig) { c.Fallbacks = []*pgconn.FallbackConfig{nil} },
	} {
		changed := configuration.Copy()
		mutate(changed)
		if s2PGConnectionMatches(changed, "", owner) {
			t.Fatal("foreign endpoint, fallback, owner or maintenance database bypassed the S2 guard")
		}
	}
	for _, prefix := range []string{"jobsim_source_preferences_", "jobsim_read_point_", "jobsim_sync_schemas_", "jobsim_self_read_"} {
		child := configuration.Copy()
		child.Database = prefix + strings.ReplaceAll(newID(), "-", "")
		if !s2PGConnectionMatches(child, prefix, owner) || s2PGConnectionMatches(child, "", owner) || s2PGConnectionMatches(child, "jobsim_arbitrary_", owner) {
			t.Fatal("owned child database confinement was not exact")
		}
		child.Database = prefix + "not_a_generated_uuid"
		if s2PGConnectionMatches(child, prefix, owner) {
			t.Fatal("non-generated child database suffix was accepted")
		}
	}
}

func TestS2PGGuardRequiresCanonicalPrivatePassfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix 0600 and symlink guard semantics require the approved native S2 host")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("nonsecret guard fixture directory unavailable")
	}
	run := newID()
	directory := filepath.Join(home, "Library", "Application Support", "JobsIM", "Integration", run, "Safety", "Secrets")
	if os.MkdirAll(directory, 0700) != nil {
		t.Fatal("nonsecret guard fixture directory could not be created")
	}
	passfile := filepath.Join(directory, "pgpass")
	if os.WriteFile(passfile, nil, 0600) != nil {
		t.Fatal("empty nonsecret guard fixture could not be created")
	}
	expected := "s2_owner_" + strings.ReplaceAll(run, "-", "")[:12]
	if owner, valid := s2PGPassfileOwner(passfile, home); !valid || owner != expected {
		t.Fatal("canonical private run did not derive its exact S2 owner")
	}
	if os.Chmod(passfile, 0664) != nil {
		t.Fatal("nonsecret guard fixture mode could not be changed")
	}
	if _, valid := s2PGPassfileOwner(passfile, home); valid {
		t.Fatal("group/world-accessible passfile bypassed the S2 guard")
	}
	if os.Chmod(passfile, 0600) != nil || os.Rename(passfile, passfile+".target") != nil || os.Symlink("pgpass.target", passfile) != nil {
		t.Fatal("nonsecret symlink rejection fixture unavailable")
	}
	if _, valid := s2PGPassfileOwner(passfile, home); valid {
		t.Fatal("symlink passfile bypassed the S2 guard")
	}
}
