package jobsim

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func deploymentArchiveJournalAnchor(ctx context.Context, s DeploymentRestoreSpec, archive string, targets map[string]string) (backupManifest, error) {
	var baseline backupManifest
	password, err := deploymentPassword(archive, "operator")
	if err != nil {
		return baseline, err
	}
	adminURL := deploymentURL(s.JournalPort, "jobsim_operator", password, "postgres")
	admin, err := operatorConnect(ctx, adminURL)
	if err != nil {
		return baseline, err
	}
	defer admin.Close(ctx)
	name := "jobsim_restore_anchor_" + strings.ReplaceAll(newID(), "-", "")
	if _, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		return baseline, err
	}
	defer admin.Exec(context.Background(), `DROP DATABASE `+pgx.Identifier{name}.Sanitize())
	address := deploymentURL(s.JournalPort, "jobsim_operator", password, name)
	prepare, err := operatorConnect(ctx, address)
	if err != nil {
		return baseline, err
	}
	_, err = prepare.Exec(ctx, `CREATE SCHEMA journal`)
	prepare.Close(ctx)
	if err != nil {
		return baseline, err
	}
	if err = pgCommand(ctx, s.PGBin, address, "pg_restore", "--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges", "--schema=journal", "--dbname="+name, filepath.Join(archive, "journal.dump")); err != nil {
		return baseline, err
	}
	db, err := operatorConnect(ctx, address)
	if err != nil {
		return baseline, err
	}
	defer db.Close(ctx)
	if err = db.QueryRow(ctx, `SELECT deployment_id,restore_epoch FROM journal.instance WHERE singleton`).Scan(&baseline.DeploymentID, &baseline.RestoreEpoch); err != nil {
		return baseline, err
	}
	if err = db.QueryRow(ctx, `SELECT COALESCE(max(seq),0) FROM journal.entries`).Scan(&baseline.JournalThrough); err != nil {
		return baseline, err
	}
	var head []byte
	if err = db.QueryRow(ctx, `SELECT digest FROM journal.chain_head WHERE singleton`).Scan(&head); err != nil || len(head) != 32 {
		return baseline, fmt.Errorf("archived journal snapshot anchor is incomplete")
	}
	baseline.JournalHead = hex.EncodeToString(head)
	expected, err := deploymentReadInstance(filepath.Join(archive, "Application", "instance.json"))
	if err != nil || expected != baseline.DeploymentID {
		return baseline, fmt.Errorf("archived journal snapshot belongs to another deployment")
	}
	return baseline, nil
}
func deploymentSealJournal(ctx context.Context, journal *pgx.Conn, s DeploymentRestoreSpec, binding, target string) (int64, error) {
	tx, err := journal.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS journal.deployment_handoff(singleton boolean PRIMARY KEY CHECK(singleton),operation_id text NOT NULL,binding text NOT NULL,target_identity text NOT NULL,restore_epoch bigint NOT NULL,sealed_at timestamptz NOT NULL DEFAULT clock_timestamp())`); err != nil {
		return 0, err
	}
	var operation, priorBinding, priorTarget string
	var epoch int64
	err = tx.QueryRow(ctx, `SELECT operation_id,binding,target_identity,restore_epoch FROM journal.deployment_handoff WHERE singleton FOR UPDATE`).Scan(&operation, &priorBinding, &priorTarget, &epoch)
	if err == nil {
		var current string
		if err = tx.QueryRow(ctx, `SELECT system_identifier::text||':'||(SELECT oid::text FROM pg_database WHERE datname=current_database()) FROM pg_control_system()`).Scan(&current); err != nil {
			return 0, err
		}
		decision := journalHandoffDecision(operation, priorBinding, priorTarget, current, s.Operation, binding, target)
		if decision == "resume" {
			return epoch, tx.Commit(ctx)
		}
		if decision != "next" {
			return 0, fmt.Errorf("latest journal handoff is immutable; only its original operation can resume")
		}
		if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS journal.deployment_handoff_history(operation_id text NOT NULL,binding text NOT NULL,target_identity text NOT NULL,restore_epoch bigint NOT NULL,PRIMARY KEY(operation_id,target_identity));INSERT INTO journal.deployment_handoff_history SELECT operation_id,binding,target_identity,restore_epoch FROM journal.deployment_handoff WHERE singleton ON CONFLICT DO NOTHING`); err != nil {
			return 0, err
		}
		if err = tx.QueryRow(ctx, `UPDATE journal.restore_epochs SET restore_epoch=restore_epoch+1 WHERE singleton RETURNING restore_epoch`).Scan(&epoch); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `UPDATE journal.deployment_handoff SET operation_id=$1,binding=$2,target_identity=$3,restore_epoch=$4,sealed_at=clock_timestamp() WHERE singleton`, s.Operation, binding, target, epoch); err != nil {
			return 0, err
		}
		return epoch, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}
	if err = tx.QueryRow(ctx, `UPDATE journal.restore_epochs SET restore_epoch=restore_epoch+1 WHERE singleton RETURNING restore_epoch`).Scan(&epoch); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO journal.deployment_handoff(singleton,operation_id,binding,target_identity,restore_epoch) VALUES(true,$1,$2,$3,$4)`, s.Operation, binding, target, epoch); err != nil {
		return 0, err
	}
	return epoch, tx.Commit(ctx)
}
func deploymentRestoreFiles(ctx context.Context, s DeploymentRestoreSpec, archive string) error {
	application := filepath.Join(s.TargetState, "Application")
	skip := func(path string) bool {
		top := strings.Split(path, "/")[0]
		return top == "JetStream" || top == "Admission" || top == "Exports" || top == "ExportFences" || top == "restore.pending" || top == "restore.retired" || top == "send-admission.lock"
	}
	if err := deploymentCopyTree(ctx, filepath.Join(archive, "Application"), application, skip); err != nil {
		return err
	}
	if err := deploymentCopyTree(ctx, filepath.Join(archive, "TLS"), filepath.Join(s.TargetState, "TLS"), nil); err != nil {
		return err
	}
	for _, name := range []string{"JetStream", "Admission"} {
		source := filepath.Join(archive, "Application", name)
		if _, err := os.Lstat(source); err == nil {
			if err = deploymentCopyTree(ctx, source, filepath.Join(s.TargetState, "Quarantine", name+"-old-epoch"), nil); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, name := range []string{"nats.conf", "nats.token"} {
		raw, err := os.ReadFile(filepath.Join(archive, "Secrets", name))
		if err != nil {
			return fmt.Errorf("original stopped broker material is incomplete")
		}
		if err = privateRestoreBytes(filepath.Join(s.TargetState, "Quarantine", "BrokerSecrets", name), raw); err != nil {
			return err
		}
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	return privateRestoreBytes(filepath.Join(s.TargetState, "Secrets", "nats.token"), []byte(hex.EncodeToString(token)+"\n"))
}
func deploymentRetireOldAuthority(ctx context.Context, s DeploymentRestoreSpec, targets map[string]string, domains map[string]*pgx.Conn, epoch int64) error {
	state := filepath.Join(s.TargetState, "Application")
	if err := CancelRestoredMigrations(ctx, domains["sync"], state); err != nil {
		return err
	}
	if err := ClearRestoredAccountMedia(ctx, domains["media"], state); err != nil {
		return err
	}
	if err := ClearRestoredRetainedMedia(ctx, domains["media"], state); err != nil {
		return err
	}
	if err := deploymentVerifyMedia(ctx, domains["media"], state); err != nil {
		return err
	}
	if err := ClearRestoredGroupRecoveryGrants(ctx, domains["conversation"]); err != nil {
		return err
	}
	if _, err := CancelRestoredExports(ctx, targets["identity"], state, epoch); err != nil {
		return err
	}
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "sync"} {
		if err := CancelRestoredPersonalExportHolders(ctx, domains[role]); err != nil {
			return err
		}
		if role == "message" || role == "media" {
			if _, err := domains[role].Exec(ctx, `UPDATE `+role+`.export_holders SET state='cleared' WHERE state<>'cleared'`); err != nil {
				return err
			}
		}
	}
	for _, role := range []string{"message", "sync"} {
		if _, err := CancelRestoredDrafts(ctx, targets[role], epoch); err != nil {
			return err
		}
	}
	_, err := domains["identity"].Exec(ctx, `UPDATE auth.users SET recovery_state=CASE WHEN pending_operation_id IS NOT NULL THEN 'recovery_required' ELSE recovery_state END,pending_operation_id=NULL;UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now());UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now());UPDATE auth.step_up_authorizations SET expires_at=LEAST(expires_at,now());UPDATE auth.reset_grants SET state='revoked',revoked_at=COALESCE(revoked_at,now()) WHERE state IN('active','prepared');`)
	return err
}
func deploymentWriteRuntimeLayout(s DeploymentRestoreSpec) error {
	if err := privateRestoreBytes(filepath.Join(s.TargetState, "owner"), []byte(s.Repository+"\n"+strconv.Itoa(os.Getuid())+"\n")); err != nil {
		return err
	}
	layout := fmt.Sprintf("format=1\nmain_port=%d\njournal_port=%d\nhttps_port=%d\nrole_port=%d\nnats_port=%d\npublic_host=%s\n", s.MainPort, s.JournalPort, s.HTTPSPort, s.RolePort, s.NATSPort, s.PublicHost)
	return privateRestoreBytes(filepath.Join(s.TargetState, "restore-layout.conf"), []byte(layout))
}

func deploymentVerifyMedia(ctx context.Context, db *pgx.Conn, state string) error {
	rows, err := db.Query(ctx, `SELECT id::text,storage_key,size_bytes,sha256 FROM media.objects`)
	if err != nil {
		return err
	}
	type item struct {
		id, key string
		size    int64
		digest  []byte
	}
	items := []item{}
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.id, &value.key, &value.size, &value.digest); err != nil {
			rows.Close()
			return err
		}
		items = append(items, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, value := range items {
		needed, e := MediaObjectNeedsBackup(ctx, db, value.id)
		if e != nil {
			return e
		}
		if !needed {
			continue
		}
		if value.id != value.key || !syncValidationID(value.key) {
			return fmt.Errorf("restored primary media has an unsafe original binding")
		}
		path := filepath.Join(state, "Media", value.key)
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("restored primary media is missing or not regular")
		}
		actual, e := fileDigest(path)
		if e != nil || actual.Size != value.size || actual.Digest != hex.EncodeToString(value.digest) {
			return fmt.Errorf("restored primary media does not match its original length and digest")
		}
	}
	return nil
}
