package jobsim

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["conversation"] += `CREATE TABLE IF NOT EXISTS conversation.group_recovery_audit(user_id uuid NOT NULL,operation_id text NOT NULL,granted boolean NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,operation_id));`
}

// Only the local operator possessing the private instance identity and database
// credentials can grant this permission. No ordinary administrator API grants it.
func SetGroupRecoveryGrant(ctx context.Context, c Config, identityURL, user, operation string, revoke bool) error {
	if !validObjectID(user) || len(operation) < 8 || len(operation) > 128 {
		return fmt.Errorf("explicit principal and stable local operator identity required")
	}
	lock, err := maintenanceAdmissionGuard(ctx, c.StateDir)
	if err != nil {
		return err
	}
	defer closeExportFence(lock)
	db, err := operatorConnect(ctx, c.DatabaseURL)
	if err != nil {
		return fmt.Errorf("original conversation database unavailable")
	}
	defer db.Close(ctx)
	var deployment string
	if err = db.QueryRow(ctx, `SELECT deployment_id::text FROM conversation.instance WHERE singleton`).Scan(&deployment); err != nil || deployment != c.DeploymentID {
		return fmt.Errorf("original conversation deployment identity differs")
	}
	if !revoke {
		identity, e := operatorConnect(ctx, identityURL)
		if e != nil {
			return fmt.Errorf("original identity database unavailable")
		}
		var valid bool
		e = identity.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.users u JOIN auth.instance i ON i.singleton WHERE u.id=$1 AND u.admin AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL AND i.deployment_id=$2)`, user, c.DeploymentID).Scan(&valid)
		identity.Close(ctx)
		if e != nil || !valid {
			return fmt.Errorf("current active authenticated administrator identity required for independent grant")
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old bool
	err = tx.QueryRow(ctx, `SELECT granted FROM conversation.group_recovery_audit WHERE user_id=$1 AND operation_id=$2`, user, operation).Scan(&old)
	if err == nil {
		if old == revoke {
			return fmt.Errorf("original local operator operation differs")
		}
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	a := &App{Role: "conversation", Config: c, Client: &http.Client{Timeout: 5 * time.Second}}
	if revoke {
		payload := map[string]any{"userId": user, "requestKey": operation}
		if err = a.protect(ctx, "group-recovery-revoke:"+user+":"+operation, "intent", "conversation.group_recovery_revoke", payload); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM conversation.governance_grants WHERE user_id=$1`, user)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO conversation.governance_grants(user_id,permission,operation_id) VALUES($1,'recover_group',$2) ON CONFLICT(user_id) DO UPDATE SET operation_id=$2,created_at=now()`, user, operation)
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO conversation.group_recovery_audit(user_id,operation_id,granted) VALUES($1,$2,$3)`, user, operation, !revoke); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Historical backups do not carry an independent live governance grant.
func ClearRestoredGroupRecoveryGrants(ctx context.Context, db *pgx.Conn) error {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('conversation.governance_grants') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		_, err := db.Exec(ctx, `DELETE FROM conversation.governance_grants`)
		return err
	}
	return nil
}
