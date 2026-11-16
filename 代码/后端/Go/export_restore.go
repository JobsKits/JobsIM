package jobsim

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func init() {
	migrations["governance"] += `CREATE TABLE IF NOT EXISTS governance.retired_export_builds(job_id uuid NOT NULL,artifact_id uuid NOT NULL,restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,state text NOT NULL DEFAULT 'cancelled',PRIMARY KEY(job_id,artifact_id,restore_epoch));`
}

// ReplayExportSafety reconstructs denial identities only. Short-lived export
// bytes and their wrapped keys never regain authority from an older snapshot.
func ReplayExportSafety(ctx context.Context, business *pgx.Conn, record journalRecord) (bool, error) {
	if record.Kind != "message.export_holder" && record.Kind != "media.export_holder" && record.Kind != "governance.export_fence" {
		return false, nil
	}
	text := func(key string) string { value, _ := record.Payload[key].(string); return value }
	_, e := business.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS governance;CREATE TABLE IF NOT EXISTS governance.retired_export_builds(job_id uuid NOT NULL,artifact_id uuid NOT NULL,restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,state text NOT NULL DEFAULT 'cancelled',PRIMARY KEY(job_id,artifact_id,restore_epoch));`)
	if e != nil {
		return true, e
	}
	_, e = business.Exec(ctx, `INSERT INTO governance.retired_export_builds(job_id,artifact_id,restore_epoch,build_generation) VALUES($1,$2,$3::bigint,$4::bigint) ON CONFLICT(job_id,artifact_id,restore_epoch) DO UPDATE SET build_generation=GREATEST(governance.retired_export_builds.build_generation,EXCLUDED.build_generation),state='cancelled'`, text("jobId"), text("artifactId"), text("restoreEpoch"), text("buildGeneration"))
	return true, e
}

// CancelRestoredExports runs on the isolated target, with all runtime writers
// fenced offline. Export artifacts are derived and deliberately never restored
// ready. Primary chat/media assets still follow the complete backup manifest.
func CancelRestoredExports(ctx context.Context, databaseURL, stateDir string, currentEpoch int64) (int, error) {
	pool, e := pgxpool.New(ctx, databaseURL)
	if e != nil {
		return 0, e
	}
	defer pool.Close()
	var exists bool
	if e = pool.QueryRow(ctx, `SELECT to_regclass('governance.export_jobs') IS NOT NULL`).Scan(&exists); e != nil || !exists {
		return 0, e
	}
	a := &App{Config: Config{StateDir: stateDir}, DB: pool}
	rows, e := pool.Query(ctx, `SELECT id::text FROM governance.export_jobs WHERE restore_epoch<$1 ORDER BY id`, currentEpoch)
	if e != nil {
		return 0, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		ids = append(ids, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return 0, e
	}
	for _, id := range ids {
		job, e := a.exportJob(ctx, id)
		if e != nil {
			return 0, e
		}
		if _, e = pool.Exec(ctx, `UPDATE governance.export_jobs SET state='cancelled',build_generation=build_generation+1,key_ciphertext=NULL WHERE id=$1 AND state<>'cancelled'`, id); e != nil {
			return 0, e
		}
		if _, e = pool.Exec(ctx, `UPDATE governance.export_artifacts SET state='revoked',writer_state='closing',download_token_digest=NULL,download_expires_at=NULL WHERE job_id=$1 AND state<>'cleared'`, id); e != nil {
			return 0, e
		}
		lock, e := a.exportFenceFile(ctx, job.ArtifactID, true)
		if e != nil {
			return 0, e
		}
		prefix, e := a.exportPrefix(job)
		if e == nil {
			e = clearExportPrefix(prefix)
		}
		closeExportFence(lock)
		if e != nil {
			return 0, e
		}
		if _, e = pool.Exec(ctx, `UPDATE governance.export_artifacts SET state='cleared',writer_state='closed',writer_token=NULL,writer_lease_until=NULL,cleared_at=COALESCE(cleared_at,now()) WHERE job_id=$1`, id); e != nil {
			return 0, e
		}
		for _, schema := range []string{"message", "media"} {
			if e = pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, schema+".export_holders").Scan(&exists); e != nil {
				return 0, e
			}
			if exists {
				if _, e = pool.Exec(ctx, `UPDATE `+schema+`.export_holders SET state='cleared' WHERE job_id=$1`, id); e != nil {
					return 0, e
				}
			}
		}
	}
	connection, e := pool.Acquire(ctx)
	if e != nil {
		return 0, e
	}
	e = CancelRestoredPersonalExportHolders(ctx, connection.Conn())
	connection.Release()
	if e != nil {
		return 0, e
	}
	return len(ids), nil
}
