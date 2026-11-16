package jobsim

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type restoreBinding struct {
	OperationID  string `json:"operationId"`
	BackupID     string `json:"backupId"`
	BackupDigest string `json:"backupDigest"`
	DeploymentID string `json:"deploymentId"`
	Target       string `json:"target"`
	StatePath    string `json:"statePath"`
}

func writePrivateJSON(path string, value any) error {
	data, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	dir := filepath.Dir(path)
	f, e := os.CreateTemp(dir, ".write-")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, e = f.Write(data); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(temp, path); e != nil {
		return e
	}
	return syncDirectory(dir)
}
func syncDirectory(path string) error {
	d, e := os.Open(path)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func databaseIdentity(ctx context.Context, db *pgx.Conn) (string, error) {
	var identity string
	e := db.QueryRow(ctx, `SELECT system_identifier::text||':'||(SELECT oid::text FROM pg_database WHERE datname=current_database()) FROM pg_control_system()`).Scan(&identity)
	return identity, e
}

func Restore(ctx context.Context, o RestoreOptions) error {
	admissionLock, lockError := maintenanceAdmissionGuard(ctx, o.StateDir)
	if lockError != nil {
		return lockError
	}
	defer closeExportFence(admissionLock)
	if o.TargetStateDir == "" || len(o.OperationID) < 8 || len(o.OperationID) > 128 {
		return fmt.Errorf("separate target state and stable operation id of 8–128 bytes required")
	}
	if !o.Offline {
		return fmt.Errorf("restore requires --offline after stopping all source roles")
	}
	if e := VerifyIndependentJournal(ctx, o.TargetURL, o.JournalURL); e != nil {
		return e
	}
	work, e := os.MkdirTemp("", "jobsim-verified-restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(work)
	manifest, e := extractBackup(ctx, o, work)
	if e != nil {
		return e
	}
	archive, e := fileDigest(o.Output)
	if e != nil {
		return e
	}
	source, e := operatorConnect(ctx, o.BusinessURL)
	if e != nil {
		return e
	}
	defer source.Close(ctx)
	target, e := operatorConnect(ctx, o.TargetURL)
	if e != nil {
		return e
	}
	defer target.Close(ctx)
	sourceIdentity, e := databaseIdentity(ctx, source)
	if e != nil {
		return e
	}
	targetIdentity, e := databaseIdentity(ctx, target)
	if e != nil {
		return e
	}
	if sourceIdentity == targetIdentity {
		return fmt.Errorf("restore target must be a different database; URL aliases do not create isolation")
	}
	statePath, e := filepath.Abs(o.TargetStateDir)
	if e != nil {
		return e
	}
	binding := restoreBinding{o.OperationID, manifest.BackupID, archive.Digest, manifest.DeploymentID, targetIdentity, filepath.Clean(statePath)}
	encoded, _ := json.Marshal(binding)
	digest := sha256.Sum256(encoded)
	journal, e := operatorConnect(ctx, o.JournalURL)
	if e != nil {
		return e
	}
	defer journal.Close(ctx)
	// The session lock serializes restore epochs and releases automatically on a crash.
	if _, e = journal.Exec(ctx, `SELECT pg_advisory_lock(hashtext('jobsim-isolated-restore'))`); e != nil {
		return e
	}
	defer journal.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('jobsim-isolated-restore'))`)
	var deployment string
	if e = journal.QueryRow(ctx, `SELECT deployment_id FROM journal.instance WHERE singleton`).Scan(&deployment); e != nil {
		return e
	}
	if deployment != manifest.DeploymentID {
		return fmt.Errorf("backup belongs to a different journal deployment")
	}
	if _, e = journal.Exec(ctx, `CREATE TABLE IF NOT EXISTS journal.restore_operations(operation_id text PRIMARY KEY,binding_digest bytea NOT NULL,restore_epoch bigint NOT NULL,state text NOT NULL,result jsonb NOT NULL DEFAULT '{}')`); e != nil {
		return e
	}
	var original []byte
	var epoch int64
	var state string
	e = journal.QueryRow(ctx, `SELECT binding_digest,restore_epoch,state FROM journal.restore_operations WHERE operation_id=$1`, o.OperationID).Scan(&original, &epoch, &state)
	exists := e == nil
	if e != nil && e != pgx.ErrNoRows {
		return e
	}
	if exists && !equalDigest(original, digest[:]) {
		return fmt.Errorf("restore operation is bound to another backup, target database or state directory")
	}
	if exists && state == "completed" {
		return finishCompletedRestore(ctx, target, statePath, binding, digest[:])
	}
	for _, db := range []*pgx.Conn{source, target} {
		var active int
		if e = db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name LIKE 'jobsim-%' AND application_name<>'jobsim-ops'`).Scan(&active); e != nil {
			return e
		}
		if active != 0 {
			return fmt.Errorf("source/target roles still have database sessions; stop them before restore")
		}
	}
	if _, e = target.Exec(ctx, `SELECT pg_advisory_lock(hashtext('jobsim-isolated-restore-target'))`); e != nil {
		return e
	}
	defer target.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('jobsim-isolated-restore-target'))`)
	pending := filepath.Join(statePath, "restore.pending")
	marker := filepath.Join(statePath, "restore-operation.json")
	if b, e := os.ReadFile(marker); e == nil {
		var prior restoreBinding
		if e = json.Unmarshal(b, &prior); e != nil {
			return e
		}
		old, _ := json.Marshal(prior)
		if !equalDigest(old, encoded) {
			return fmt.Errorf("state directory belongs to a different restore operation")
		}
	} else if !os.IsNotExist(e) {
		return e
	} else {
		entries, e := os.ReadDir(statePath)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if len(entries) > 0 {
			return fmt.Errorf("new restore target state is not empty; existing files are preserved")
		}
		if e = os.MkdirAll(statePath, 0700); e != nil {
			return e
		}
		if e = writePrivateJSON(marker, binding); e != nil {
			return e
		}
	}
	if e = writePrivateJSON(pending, map[string]string{"operationId": o.OperationID}); e != nil {
		return e
	}
	if !exists {
		var tables int
		if e = target.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN('information_schema','pg_catalog')`).Scan(&tables); e != nil {
			return e
		}
		if tables != 0 {
			return fmt.Errorf("new restore target database is not empty; existing data is preserved")
		}
		tx, e := journal.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		if e = tx.QueryRow(ctx, `UPDATE journal.restore_epochs SET restore_epoch=restore_epoch+1 WHERE singleton RETURNING restore_epoch`).Scan(&epoch); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO journal.restore_operations(operation_id,binding_digest,restore_epoch,state) VALUES($1,$2,$3,'allocated')`, o.OperationID, digest[:], epoch)
		if e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	var currentEpoch int64
	if e = journal.QueryRow(ctx, `SELECT restore_epoch FROM journal.restore_epochs WHERE singleton`).Scan(&currentEpoch); e != nil {
		return e
	}
	if epoch != currentEpoch {
		return fmt.Errorf("a newer restore epoch superseded this unfinished operation; it cannot activate")
	}
	sourceDerivedExports, e := CancelRestoredExports(ctx, o.BusinessURL, o.StateDir, epoch)
	if e != nil {
		return e
	}
	var hasCheckpoint bool
	if e = target.QueryRow(ctx, `SELECT to_regclass('jobsim_restore.operation') IS NOT NULL`).Scan(&hasCheckpoint); e != nil {
		return e
	}
	if !hasCheckpoint {
		var foreignTables int
		if e = target.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN('information_schema','pg_catalog','jobsim_restore')`).Scan(&foreignTables); e != nil {
			return e
		}
		if foreignTables != 0 {
			return fmt.Errorf("unfinished restore has no owned checkpoint for the nonempty target; existing data is preserved")
		}
	}
	if _, e = target.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS jobsim_restore; CREATE TABLE IF NOT EXISTS jobsim_restore.operation(singleton boolean PRIMARY KEY CHECK(singleton),operation_id text NOT NULL,binding_digest bytea NOT NULL,restore_epoch bigint NOT NULL,state text NOT NULL)`); e != nil {
		return e
	}
	_, e = target.Exec(ctx, `INSERT INTO jobsim_restore.operation VALUES(true,$1,$2,$3,'allocated') ON CONFLICT DO NOTHING`, o.OperationID, digest[:], epoch)
	if e != nil {
		return e
	}
	var targetOperation, targetState string
	var targetDigest []byte
	var targetEpoch int64
	if e = target.QueryRow(ctx, `SELECT operation_id,binding_digest,restore_epoch,state FROM jobsim_restore.operation WHERE singleton`).Scan(&targetOperation, &targetDigest, &targetEpoch, &targetState); e != nil {
		return e
	}
	if targetOperation != o.OperationID || !equalDigest(targetDigest, digest[:]) || targetEpoch != epoch {
		return fmt.Errorf("target database belongs to another restore operation")
	}
	var imported bool
	if e = target.QueryRow(ctx, `SELECT to_regclass('auth.instance') IS NOT NULL`).Scan(&imported); e != nil {
		return e
	}
	if !imported {
		if targetState != "allocated" {
			return fmt.Errorf("restored database checkpoint is incomplete")
		}
		if e = pgCommand(ctx, o.PGBin, o.TargetURL, "pg_restore", "--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges", "--dbname", target.Config().Database, filepath.Join(work, "database.dump")); e != nil {
			return e
		}
	}
	// pg_restore is one transaction. An unknown response can be reconciled from
	// this separately owned target marker and the imported instance identity.
	var importedDeployment string
	if e = target.QueryRow(ctx, `SELECT deployment_id FROM auth.instance WHERE singleton`).Scan(&importedDeployment); e != nil {
		return e
	}
	if importedDeployment != manifest.DeploymentID {
		return fmt.Errorf("imported instance identity differs from the verified backup")
	}
	if _, e = target.Exec(ctx, `UPDATE jobsim_restore.operation SET state='database_restored' WHERE singleton`); e != nil {
		return e
	}
	if e = restorePrivateFiles(work, statePath, manifest); e != nil {
		return e
	}
	config, e := PrepareConfig(Config{DatabaseURL: o.TargetURL, JournalDatabaseURL: o.JournalURL, StateDir: work})
	if e != nil {
		return e
	}
	roles := []string{}
	for role := range schemaFor {
		if role != "journal" {
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)
	for _, role := range roles {
		app, e := NewApp(ctx, config, role)
		if e != nil {
			return e
		}
		app.Close()
	}
	coverage, e := replaySafetyCovered(ctx, target, journal, manifest)
	if e != nil {
		return e
	}
	retiredExports, e := CancelRestoredExports(ctx, o.TargetURL, statePath, epoch)
	if e != nil {
		return e
	}
	tx, e := target.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	for _, role := range roles {
		if _, e = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s.instance SET restore_epoch=$1 WHERE singleton`, schemaFor[role]), epoch); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE auth.users SET recovery_state=CASE WHEN pending_operation_id IS NOT NULL THEN 'recovery_required' ELSE recovery_state END,pending_operation_id=NULL; UPDATE auth.sessions SET revoked_at=now(); UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=now(); UPDATE auth.step_up_authorizations SET expires_at=LEAST(expires_at,now()); UPDATE auth.reset_grants SET state='revoked',revoked_at=COALESCE(revoked_at,now()) WHERE state IN('active','prepared'); UPDATE jobsim_restore.operation SET state='safety_replayed' WHERE singleton;`); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	receipt := map[string]any{"operationId": o.OperationID, "backupId": manifest.BackupID, "deploymentId": manifest.DeploymentID, "restoreEpoch": str(epoch), "status": "isolated_restore_completed", "oldSessionsRevoked": true, "strongProtection": false, "journalCoverage": coverage, "snapshotLSN": manifest.SnapshotLSN, "ownerCoverage": manifest.OwnerCoverage, "resumable": true, "retiredDerivedExports": retiredExports, "sourceDerivedExportsRetired": sourceDerivedExports}
	if e = writePrivateJSON(filepath.Join(statePath, "restore-receipt.json"), receipt); e != nil {
		return e
	}
	if e = journal.QueryRow(ctx, `SELECT restore_epoch FROM journal.restore_epochs WHERE singleton`).Scan(&currentEpoch); e != nil {
		return e
	}
	if epoch != currentEpoch {
		return fmt.Errorf("restore epoch changed before activation")
	}
	if _, e = target.Exec(ctx, `UPDATE jobsim_restore.operation SET state='completed' WHERE singleton`); e != nil {
		return e
	}
	result, _ := json.Marshal(receipt)
	if _, e = journal.Exec(ctx, `UPDATE journal.restore_operations SET state='completed',result=$2 WHERE operation_id=$1`, o.OperationID, result); e != nil {
		return e
	}
	if e = os.Remove(pending); e != nil && !os.IsNotExist(e) {
		return e
	}
	return syncDirectory(statePath)
}

func equalDigest(first, second []byte) bool {
	return len(first) == len(second) && hex.EncodeToString(first) == hex.EncodeToString(second)
}

func finishCompletedRestore(ctx context.Context, target *pgx.Conn, statePath string, binding restoreBinding, digest []byte) error {
	var operation, state string
	var original []byte
	if e := target.QueryRow(ctx, `SELECT operation_id,binding_digest,state FROM jobsim_restore.operation WHERE singleton`).Scan(&operation, &original, &state); e != nil {
		return e
	}
	if operation != binding.OperationID || !equalDigest(original, digest) || state != "completed" {
		return fmt.Errorf("completed journal operation does not match its target checkpoint")
	}
	meta, e := os.ReadFile(filepath.Join(statePath, "restore-operation.json"))
	if e != nil {
		return e
	}
	var prior restoreBinding
	if e = json.Unmarshal(meta, &prior); e != nil {
		return e
	}
	encoded, _ := json.Marshal(prior)
	expected, _ := json.Marshal(binding)
	if !equalDigest(encoded, expected) {
		return fmt.Errorf("completed operation state binding differs")
	}
	receipt, e := os.ReadFile(filepath.Join(statePath, "restore-receipt.json"))
	if e != nil {
		return e
	}
	var result map[string]any
	if e = json.Unmarshal(receipt, &result); e != nil {
		return e
	}
	if result["operationId"] != binding.OperationID || result["backupId"] != binding.BackupID {
		return fmt.Errorf("completed operation receipt differs")
	}
	pending := filepath.Join(statePath, "restore.pending")
	if e = os.Remove(pending); e != nil && !os.IsNotExist(e) {
		return e
	}
	return syncDirectory(statePath)
}

func restorePrivateFiles(work, target string, manifest backupManifest) error {
	for _, f := range manifest.Files {
		if f.Name == "database.dump" {
			continue
		}
		dest := filepath.Join(target, f.Name)
		if existing, e := fileDigest(dest); e == nil {
			if existing.Size != f.Size || existing.Digest != f.Digest {
				return fmt.Errorf("restored file differs from its bound backup; existing file preserved")
			}
			continue
		} else if !os.IsNotExist(e) {
			return e
		}
		if e := os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		src, e := os.Open(filepath.Join(work, f.Name))
		if e != nil {
			return e
		}
		temp := dest + ".restore-partial"
		out, e := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if e != nil {
			src.Close()
			return e
		}
		h := sha256.New()
		size, e := io.Copy(out, io.TeeReader(src, h))
		src.Close()
		if e == nil {
			e = out.Sync()
		}
		closeErr := out.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if size != f.Size || hex.EncodeToString(h.Sum(nil)) != f.Digest {
			return fmt.Errorf("verified restore file changed unexpectedly")
		}
		if e = os.Rename(temp, dest); e != nil {
			return e
		}
		if e = syncDirectory(filepath.Dir(dest)); e != nil {
			return e
		}
	}
	return nil
}
