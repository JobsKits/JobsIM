package jobsim

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type deploymentRestoreCheckpoint struct {
	Operation, Binding, Archive, Deployment, TargetJournal string
	Epoch                                                  int64
	State                                                  string
}

func privateRestoreBytes(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("restore refuses a nonregular existing private file")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".restore-write-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(raw); err == nil {
		err = tmp.Sync()
	}
	closed := tmp.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func deploymentReadInstance(path string) (string, error) {
	raw, err := os.ReadFile(path)
	var instance struct {
		ID string `json:"deploymentId"`
	}
	if err != nil || json.Unmarshal(raw, &instance) != nil || !syncValidationID(instance.ID) {
		return "", fmt.Errorf("original deployment identity is unavailable")
	}
	return instance.ID, nil
}
func deploymentCheckOwner(s DeploymentRestoreSpec) error {
	raw, err := os.ReadFile(filepath.Join(s.SourceState, "owner"))
	if err != nil {
		return fmt.Errorf("source deployment ownership receipt required")
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(parts) != 2 || parts[0] == "" || parts[1] != strconv.Itoa(os.Getuid()) {
		return fmt.Errorf("source deployment receipt belongs to another user")
	}
	if _, err = os.Stat(filepath.Join(s.Repository, "代码", "后端", "Go", "go.mod")); err != nil {
		return fmt.Errorf("target repository does not contain the owned backend")
	}
	return nil
}
func deploymentCopyTree(ctx context.Context, source, target string, skip func(string) bool) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(target, 0700)
		}
		if skip != nil && skip(filepath.ToSlash(rel)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dest := filepath.Join(target, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("restore copy refuses symbolic links")
		}
		if entry.IsDir() {
			info, e := os.Lstat(dest)
			if e == nil && !info.IsDir() {
				return fmt.Errorf("restore directory conflicts with a retained file")
			}
			return os.MkdirAll(dest, 0700)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("restore copy requires regular private input")
		}
		if err = deploymentRequireSpace(ctx, filepath.Dir(target), info.Size()); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		raw, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		return privateRestoreBytes(dest, raw)
	})
}
func deploymentClusterIdentity(ctx context.Context, bin, data string) (string, error) {
	cmd := exec.CommandContext(ctx, filepath.Join(bin, "pg_controldata"), data)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("target PostgreSQL identity cannot be verified")
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "Database system identifier:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Database system identifier:")), nil
		}
	}
	return "", fmt.Errorf("target PostgreSQL identity missing")
}
func deploymentStartCluster(ctx context.Context, s DeploymentRestoreSpec, label string, port int) error {
	data := filepath.Join(s.TargetState, label+"Database")
	receipt := filepath.Join(s.TargetState, "Processes", label+".cluster")
	if _, err := os.Stat(filepath.Join(data, "PG_VERSION")); os.IsNotExist(err) {
		entries, e := os.ReadDir(data)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if len(entries) != 0 {
			return fmt.Errorf("partial target cluster is preserved; no initialization over existing files")
		}
		if err = deploymentRequireSpace(ctx, s.TargetState, 256<<20); err != nil {
			return err
		}
		if err = deploymentTool(ctx, filepath.Join(s.PGBin, "initdb"), []string{"-D", data, "-U", "jobsim_operator", "--pwfile=" + filepath.Join(s.TargetState, "Secrets", "operator.password"), "--auth-local=trust", "--auth-host=scram-sha-256", "--encoding=UTF8", "--locale=C"}, nil); err != nil {
			return err
		}
		identity, e := deploymentClusterIdentity(ctx, s.PGBin, data)
		if e != nil {
			return e
		}
		if e = privateRestoreBytes(receipt, []byte(identity+"\n")); e != nil {
			return e
		}
	} else if err != nil {
		return err
	}
	identity, err := deploymentClusterIdentity(ctx, s.PGBin, data)
	if err != nil {
		return err
	}
	expected, err := os.ReadFile(receipt)
	if err != nil || strings.TrimSpace(string(expected)) != identity {
		return fmt.Errorf("existing target cluster has no matching physical identity receipt")
	}
	status := exec.CommandContext(ctx, filepath.Join(s.PGBin, "pg_ctl"), "-D", data, "status")
	status.Stdout, status.Stderr = io.Discard, io.Discard
	if status.Run() == nil {
		if !deploymentClusterPort(data, port) {
			return fmt.Errorf("owned target cluster is running on a different port")
		}
		return nil
	}
	options := fmt.Sprintf("-p %d -h 127.0.0.1 -k '' -c max_connections=100 -c shared_buffers=64MB -c fsync=on -c full_page_writes=on -c synchronous_commit=on", port)
	return deploymentTool(ctx, filepath.Join(s.PGBin, "pg_ctl"), []string{"-D", data, "-l", filepath.Join(s.TargetState, "Logs", "pg-"+label+".log"), "-w", "-t", "30", "-o", options, "start"}, nil)
}
func deploymentStopClusters(s DeploymentRestoreSpec) {
	for _, label := range []string{"Journal", "Main"} {
		data := filepath.Join(s.TargetState, label+"Database")
		if _, err := os.Stat(filepath.Join(data, "PG_VERSION")); err != nil {
			continue
		}
		identity, e := deploymentClusterIdentity(context.Background(), s.PGBin, data)
		receipt, re := os.ReadFile(filepath.Join(s.TargetState, "Processes", label+".cluster"))
		if e != nil || re != nil || strings.TrimSpace(string(receipt)) != identity {
			continue
		}
		_ = deploymentTool(context.Background(), filepath.Join(s.PGBin, "pg_ctl"), []string{"-D", data, "-m", "fast", "-w", "-t", "30", "stop"}, nil)
	}
}
func deploymentCreateTargets(ctx context.Context, s DeploymentRestoreSpec, archive string) (map[string]string, error) {
	targets := map[string]string{}
	operator, err := deploymentPassword(archive, "operator")
	if err != nil {
		return targets, err
	}
	for _, label := range []struct {
		name string
		port int
	}{{"Main", s.MainPort}, {"Journal", s.JournalPort}} {
		if err = deploymentStartCluster(ctx, s, label.name, label.port); err != nil {
			return targets, err
		}
	}
	for _, role := range deploymentPhysicalDomains {
		port := s.MainPort
		if role == "journal" {
			port = s.JournalPort
		}
		admin, err := operatorConnect(ctx, deploymentURL(port, "jobsim_operator", operator, "postgres"))
		if err != nil {
			return targets, fmt.Errorf("private target operator unavailable")
		}
		password, e := deploymentPassword(archive, role)
		if e != nil {
			admin.Close(ctx)
			return targets, e
		}
		var exists bool
		if e = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, "jobsim_"+role).Scan(&exists); e == nil && !exists {
			_, e = admin.Exec(ctx, `CREATE ROLE `+pgx.Identifier{"jobsim_" + role}.Sanitize()+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '`+password+`'`)
		}
		if e == nil {
			e = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, "jobsim_"+role).Scan(&exists)
		}
		if e == nil && !exists {
			_, e = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{"jobsim_" + role}.Sanitize()+` OWNER `+pgx.Identifier{"jobsim_" + role}.Sanitize())
		}
		admin.Close(ctx)
		if e != nil {
			return targets, fmt.Errorf("target domain preparation failed")
		}
		target := deploymentURL(port, "jobsim_"+role, password, "jobsim_"+role)
		if supplied := s.Targets[role]; supplied != "" && supplied != target {
			return targets, fmt.Errorf("explicit target URL does not match the isolated deployable role layout")
		}
		targets[role] = target
		grant, err := operatorConnect(ctx, deploymentURL(port, "jobsim_operator", operator, "jobsim_"+role))
		if err != nil {
			return targets, err
		}
		_, err = grant.Exec(ctx, `GRANT EXECUTE ON FUNCTION pg_control_system() TO `+pgx.Identifier{"jobsim_" + role}.Sanitize())
		grant.Close(ctx)
		if err != nil {
			return targets, err
		}
	}
	targets["governance"] = targets["identity"]
	return targets, nil
}
func deploymentImportDomain(ctx context.Context, s DeploymentRestoreSpec, role, address, dump, binding string) error {
	db, err := operatorConnect(ctx, address)
	if err != nil {
		return fmt.Errorf("isolated target domain unavailable")
	}
	defer db.Close(ctx)
	var imported bool
	schema := schemaFor[role]
	if err = db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, schema+".instance").Scan(&imported); err != nil {
		return err
	}
	if !imported {
		var count int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN('information_schema','pg_catalog','jobsim_restore')`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("nonempty target has no owned atomic import checkpoint")
		}
		namespaces := []string{schema}
		if role == "identity" {
			namespaces = append(namespaces, "governance")
		}
		for _, namespace := range namespaces {
			if _, err = db.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+pgx.Identifier{namespace}.Sanitize()); err != nil {
				return err
			}
		}
		args := []string{"--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges", "--schema=" + schema}
		if role == "identity" {
			args = append(args, "--schema=governance")
		}
		args = append(args, "--dbname=jobsim_"+role, dump)
		if err = pgCommand(ctx, s.PGBin, address, "pg_restore", args...); err != nil {
			return err
		}
	}
	if _, err = db.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS jobsim_restore;CREATE TABLE IF NOT EXISTS jobsim_restore.deployment_import(singleton boolean PRIMARY KEY CHECK(singleton),binding text NOT NULL)`); err != nil {
		return err
	}
	if _, err = db.Exec(ctx, `INSERT INTO jobsim_restore.deployment_import VALUES(true,$1) ON CONFLICT DO NOTHING`, binding); err != nil {
		return err
	}
	var original string
	if err = db.QueryRow(ctx, `SELECT binding FROM jobsim_restore.deployment_import WHERE singleton`).Scan(&original); err != nil || original != binding {
		return fmt.Errorf("target import belongs to another immutable restore operation")
	}
	return nil
}

// DeploymentRestore restores the native eight-database CMS package. The old
// journal dump is an integrity anchor only; authority comes from the locked live
// ledger, handed once to the new physical journal identity.
func DeploymentRestore(ctx context.Context, specPath string, planOnly bool) (map[string]any, error) {
	s, err := loadDeploymentRestoreSpec(specPath)
	if err != nil {
		return nil, err
	}
	if !planOnly {
		path := filepath.Join(s.TargetState, "deployment-restore-operation.json")
		if raw, e := os.ReadFile(path); e == nil {
			var completed deploymentRestoreCheckpoint
			if json.Unmarshal(raw, &completed) == nil && completed.Binding == deploymentBinding(s) && completed.State == "completed" {
				var result map[string]any
				receipt, e := os.ReadFile(filepath.Join(s.TargetState, "deployment-restore-receipt.json"))
				if e != nil || json.Unmarshal(receipt, &result) != nil {
					return nil, fmt.Errorf("completed restore receipt is incomplete")
				}
				result["resultIsHistorical"] = true
				return result, nil
			}
		}
	}
	if err = deploymentCheckOwner(s); err != nil {
		return nil, err
	}
	if err = deploymentRequireSpace(ctx, filepath.Dir(s.TargetState), 0); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp("", "jobsim-deployment-restore-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	archive, err := extractDeploymentCMS(ctx, s, work)
	if err != nil {
		return nil, err
	}
	deployment, err := deploymentReadInstance(filepath.Join(archive, "Application", "instance.json"))
	if err != nil {
		return nil, err
	}
	sourceDeployment, err := deploymentReadInstance(filepath.Join(s.SourceState, "Application", "instance.json"))
	if err != nil || sourceDeployment != deployment {
		return nil, fmt.Errorf("archive and current source deployment differ")
	}
	if err = deploymentTool(ctx, s.OpenSSL, append([]string{"verify", "-x509_strict", "-purpose", "sslserver", "-CAfile", filepath.Join(archive, "TLS", "ca.crt")}, deploymentTLSHostArgs(s.PublicHost, filepath.Join(archive, "TLS", "server.crt"))...), nil); err != nil {
		return nil, fmt.Errorf("original TLS chain requires controlled repair; trust root was preserved")
	}
	result := map[string]any{"operationId": s.Operation, "deploymentId": deployment, "physicalDomains": deploymentPhysicalDomains, "targetState": s.TargetState, "broker": "quarantine old data; rebuild with a new token under the new epoch", "journal": "handoff latest locked ledger; archived journal is only a snapshot anchor", "planOnly": planOnly}
	if planOnly {
		return result, nil
	}
	sourceLock, err := maintenanceAdmissionGuard(ctx, filepath.Join(s.SourceState, "Application"))
	if err != nil {
		return nil, err
	}
	defer closeExportFence(sourceLock)
	if err = deploymentAvailablePorts(s); err != nil {
		return nil, err
	}
	binding := deploymentBinding(s)
	checkpointPath := filepath.Join(s.TargetState, "deployment-restore-operation.json")
	checkpoint := deploymentRestoreCheckpoint{Operation: s.Operation, Binding: binding, Archive: s.BackupSHA, Deployment: deployment, State: "prepared"}
	if raw, e := os.ReadFile(checkpointPath); e == nil {
		var prior deploymentRestoreCheckpoint
		if json.Unmarshal(raw, &prior) != nil || prior.Binding != binding {
			return nil, fmt.Errorf("target state is bound to another restore operation")
		}
		checkpoint = prior
	} else if !os.IsNotExist(e) {
		return nil, e
	} else {
		entries, e := os.ReadDir(s.TargetState)
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		if len(entries) != 0 {
			return nil, fmt.Errorf("target state must be empty; existing files were preserved")
		}
		if e = os.MkdirAll(s.TargetState, 0700); e != nil {
			return nil, e
		}
		if e = writePrivateJSON(checkpointPath, checkpoint); e != nil {
			return nil, e
		}
	}
	for _, dir := range []string{"Processes", "Logs", "Secrets", "Application", "TLS", "Quarantine"} {
		if err = os.MkdirAll(filepath.Join(s.TargetState, dir), 0700); err != nil {
			return nil, err
		}
	}
	targetLock, err := maintenanceAdmissionGuard(ctx, filepath.Join(s.TargetState, "Application"))
	if err != nil {
		return nil, err
	}
	defer closeExportFence(targetLock)
	if err = writePrivateJSON(filepath.Join(s.TargetState, "Application", "restore.pending"), map[string]string{"operationId": s.Operation}); err != nil {
		return nil, err
	}
	if err = deploymentCopyTree(ctx, filepath.Join(archive, "Secrets"), filepath.Join(s.TargetState, "Secrets"), func(path string) bool { return path == "nats.conf" || path == "nats.token" }); err != nil {
		return nil, err
	}
	defer deploymentStopClusters(s)
	targets, err := deploymentCreateTargets(ctx, s, archive)
	if err != nil {
		return nil, err
	}
	sources := map[string]*pgx.Conn{}
	domains := map[string]*pgx.Conn{}
	defer func() {
		for _, db := range sources {
			db.Close(context.Background())
		}
		for role, db := range domains {
			if role != "governance" {
				db.Close(context.Background())
			}
		}
	}()
	sourceIdentities := map[string]bool{}
	for _, role := range deploymentPhysicalDomains {
		db, e := operatorConnect(ctx, s.Sources[role])
		if e != nil {
			return nil, fmt.Errorf("current source domain unavailable")
		}
		sources[role] = db
		identity, e := databaseIdentity(ctx, db)
		if e != nil || sourceIdentities[identity] {
			return nil, fmt.Errorf("eight physically distinct source domain databases required")
		}
		sourceIdentities[identity] = true
		var active int
		if e = db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name LIKE 'jobsim-%' AND application_name<>'jobsim-ops'`).Scan(&active); e != nil || active != 0 {
			return nil, fmt.Errorf("all source runtime roles must be offline before journal handoff")
		}
		var id string
		if e = db.QueryRow(ctx, `SELECT deployment_id FROM `+schemaFor[role]+`.instance WHERE singleton`).Scan(&id); e != nil || id != deployment {
			return nil, fmt.Errorf("current source domain deployment differs")
		}
	}
	if err = VerifyIndependentJournal(ctx, s.Sources["identity"], s.Sources["journal"]); err != nil {
		return nil, err
	}
	journal := sources["journal"]
	if _, err = journal.Exec(ctx, `SELECT pg_advisory_lock(hashtext('jobsim-isolated-restore'))`); err != nil {
		return nil, err
	}
	defer journal.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('jobsim-isolated-restore'))`)
	journalTarget, err := operatorConnect(ctx, targets["journal"])
	if err != nil {
		return nil, err
	}
	targetIdentity, err := databaseIdentity(ctx, journalTarget)
	journalTarget.Close(ctx)
	if err != nil {
		return nil, err
	}
	if checkpoint.TargetJournal != "" && checkpoint.TargetJournal != targetIdentity {
		return nil, fmt.Errorf("target journal physical identity changed")
	}
	checkpoint.TargetJournal = targetIdentity
	if err = requireJournalHandoffTarget(ctx, journal, s.Operation, binding, targetIdentity); err != nil {
		return nil, err
	}
	// Stage only business domains. Archived journal data is never imported into
	// either authoritative ledger, even when it contains a plausible newer row.
	for _, role := range deploymentPhysicalDomains {
		if role == "journal" {
			continue
		}
		if err = deploymentImportDomain(ctx, s, role, targets[role], filepath.Join(archive, role+".dump"), binding); err != nil {
			return nil, err
		}
	}
	configuration, err := PrepareConfig(Config{DatabaseURL: targets["identity"], JournalDatabaseURL: targets["journal"], StateDir: filepath.Join(archive, "Application")})
	if err != nil {
		return nil, err
	}
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "governance", "sync"} {
		cfg := configuration
		cfg.DatabaseURL = targets[role]
		app, e := NewApp(ctx, cfg, role)
		if e != nil {
			return nil, e
		}
		app.Close()
		if role == "governance" {
			continue
		}
		db, e := operatorConnect(ctx, targets[role])
		if e != nil {
			return nil, e
		}
		domains[role] = db
	}
	domains["governance"] = domains["identity"]
	// The archive anchor is read in a throwaway database; it cannot allocate an
	// epoch or become the runtime journal.
	anchor, err := deploymentArchiveJournalAnchor(ctx, s, archive, targets)
	if err != nil {
		return nil, err
	}
	if _, err = replaySafetyCoveredDomains(ctx, domains["identity"], journal, anchor, domains); err != nil {
		return nil, err
	}
	epoch, err := deploymentSealJournal(ctx, journal, s, binding, targetIdentity)
	if err != nil {
		return nil, err
	}
	checkpoint.Epoch, checkpoint.State = epoch, "journal_sealed"
	if err = writePrivateJSON(checkpointPath, checkpoint); err != nil {
		return nil, err
	}
	liveDump := filepath.Join(work, "current-journal.dump")
	if err = pgCommand(ctx, s.PGBin, s.Sources["journal"], "pg_dump", "--format=custom", "--file="+liveDump); err != nil {
		return nil, err
	}
	if err = deploymentImportDomain(ctx, s, "journal", targets["journal"], liveDump, binding); err != nil {
		return nil, err
	}
	targetJournal, err := operatorConnect(ctx, targets["journal"])
	if err != nil {
		return nil, err
	}
	defer targetJournal.Close(ctx)
	var targetEpoch int64
	if err = targetJournal.QueryRow(ctx, `SELECT restore_epoch FROM journal.restore_epochs WHERE singleton`).Scan(&targetEpoch); err != nil || targetEpoch != epoch {
		return nil, fmt.Errorf("target journal has a different authority epoch; never roll it backward")
	}
	coverage, err := replaySafetyCoveredDomains(ctx, domains["identity"], targetJournal, anchor, domains)
	if err != nil {
		return nil, err
	}
	if err = deploymentRestoreFiles(ctx, s, archive); err != nil {
		return nil, err
	}
	if err = deploymentRetireOldAuthority(ctx, s, targets, domains, epoch); err != nil {
		return nil, err
	}
	for role, db := range domains {
		if role == "governance" {
			continue
		}
		if _, err = db.Exec(ctx, `UPDATE `+schemaFor[role]+`.instance SET restore_epoch=$1 WHERE singleton`, epoch); err != nil {
			return nil, err
		}
	}
	if _, err = domains["identity"].Exec(ctx, `UPDATE governance.instance SET restore_epoch=$1 WHERE singleton`, epoch); err != nil {
		return nil, err
	}
	if err = deploymentWriteRuntimeLayout(s); err != nil {
		return nil, err
	}
	checkpoint.State = "completed"
	if err = writePrivateJSON(checkpointPath, checkpoint); err != nil {
		return nil, err
	}
	result["restoreEpoch"], result["journalCoverage"], result["planOnly"], result["status"] = str(epoch), coverage, false, "isolated_deployment_restored_stopped"
	if err = writePrivateJSON(filepath.Join(s.TargetState, "deployment-restore-receipt.json"), result); err != nil {
		return nil, err
	}
	if err = os.Remove(filepath.Join(s.TargetState, "Application", "restore.pending")); err != nil {
		return nil, err
	}
	if err = syncDirectory(filepath.Join(s.TargetState, "Application")); err != nil {
		return nil, err
	}
	return result, nil
}
