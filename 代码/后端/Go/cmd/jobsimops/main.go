package main

import (
	"context"
	"flag"
	"fmt"
	jobsim "jobsim/backend"
	"os"
	"strconv"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	username := flag.String("username", "admin", "Controlled initial administrator username")
	passwordEnv := flag.String("password-env", "JOBS_IM_BOOTSTRAP_PASSWORD", "Environment variable containing the new administrator password")
	totpFile := flag.String("totp-secret-file", "", "Explicit private administrator authenticator enrollment file")
	internalBase := flag.Int("internal-base-port", 8101, "First internal domain role port")
	mode := flag.String("mode", "backup", "backup|restore|reconcile-admissions|bootstrap-admin")
	pgbin := flag.String("pg-bin", "", "Matching PostgreSQL tool directory")
	file := flag.String("file", "", "Encrypted backup file; never overwritten")
	key := flag.String("key-file", "", "Separate private 32-byte recovery key")
	state := flag.String("state-dir", os.Getenv("JOBS_IM_STATE_DIR"), "Source private state directory")
	tls := flag.String("tls-dir", "", "Source TLS certificate/private key directory")
	target := flag.String("target-database-url", os.Getenv("JOBS_IM_RESTORE_DATABASE_URL"), "Separate empty target URL; environment recommended")
	targetState := flag.String("target-state-dir", "", "Separate empty restored state directory")
	offline := flag.Bool("offline", false, "Confirm source roles are stopped; database sessions are checked")
	op := flag.String("operation-id", "", "Stable operator restore request identity")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	o := jobsim.BackupOptions{BusinessURL: os.Getenv("JOBS_IM_DATABASE_URL"), JournalURL: os.Getenv("JOBS_IM_JOURNAL_DATABASE_URL"), StateDir: *state, TLSDir: *tls, PGBin: *pgbin, Output: *file, KeyFile: *key}
	if *mode == "reconcile-admissions" {
		messageURL := os.Getenv("JOBS_IM_MESSAGE_DATABASE_URL")
		if messageURL == "" {
			messageURL = o.BusinessURL
		}
		config, e := jobsim.PrepareConfig(jobsim.Config{DatabaseURL: messageURL, StateDir: *state})
		if e != nil {
			return e
		}
		if e = jobsim.ReconcileSendAdmissions(ctx, config); e != nil {
			return e
		}
		fmt.Println("Original send-admission transaction outcomes reconciled; no messages resent")
		return nil
	}
	if *mode == "bootstrap-admin" {
		password, ok := os.LookupEnv(*passwordEnv)
		if !ok || password == "" {
			return fmt.Errorf("explicit bootstrap password environment variable required")
		}
		urls := map[string]string{}
		for i, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
			urls[role] = "http://127.0.0.1:" + strconv.Itoa(*internalBase+i)
		}
		config, e := jobsim.PrepareConfig(jobsim.Config{DatabaseURL: o.BusinessURL, JournalDatabaseURL: o.JournalURL, StateDir: *state, RoleURLs: urls})
		if e != nil {
			return e
		}
		if e = jobsim.BootstrapAdministrator(ctx, config, *username, password, *totpFile, *op); e != nil {
			return e
		}
		fmt.Println("Controlled administrator initialized with mandatory authenticator; enrollment remains in the explicit private file")
		return nil
	}
	if *mode == "backup" {
		if e := jobsim.Backup(ctx, o); e != nil {
			return e
		}
		fmt.Println("Encrypted PostgreSQL/media/instance/TLS backup completed")
		return nil
	}
	if *mode == "restore" {
		if *op == "" {
			return fmt.Errorf("restore requires an explicit stable --operation-id")
		}
		if e := jobsim.Restore(ctx, jobsim.RestoreOptions{BackupOptions: o, TargetURL: *target, TargetStateDir: *targetState, Offline: *offline, OperationID: *op}); e != nil {
			return e
		}
		fmt.Println("Isolated restore and minimum safety replay completed; old sessions revoked")
		return nil
	}
	return fmt.Errorf("unknown operation")
}
