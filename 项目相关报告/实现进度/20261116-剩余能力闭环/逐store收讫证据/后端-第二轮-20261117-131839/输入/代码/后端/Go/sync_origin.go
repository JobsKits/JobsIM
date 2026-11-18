package jobsim

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// Both transport and compensating replay must preserve the same immutable
// source fact. A uniqueness collision is never permission to skip a change.
func (a *App) syncStoreOrigin(ctx context.Context, tx pgx.Tx, wire syncWireEvent) error {
	subject := "jobsim." + a.Config.DeploymentID + "." + wire.RestoreEpoch + "." + wire.Owner
	if syncNATSValidate(wire, subject, a.Config.DeploymentID, wire.RestoreEpoch) != nil {
		return fail(503, "SYNC_OWNER_MISMATCH", "Owner event is not a valid immutable reference")
	}
	ref, e := json.Marshal(wire.Ref)
	if e != nil {
		return e
	}
	result, e := tx.Exec(ctx, `INSERT INTO sync.origins(origin_event_id,scope_id,owner,revision,restore_epoch,ref) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, wire.OriginEventID, wire.ScopeID, wire.Owner, wire.Revision, wire.RestoreEpoch, ref)
	if e != nil {
		return e
	}
	if result.RowsAffected() == 1 {
		return nil
	}
	var same bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.origins WHERE origin_event_id=$1 AND scope_id=$2 AND owner=$3 AND revision=$4 AND restore_epoch=$5 AND ref=$6::jsonb)`, wire.OriginEventID, wire.ScopeID, wire.Owner, wire.Revision, wire.RestoreEpoch, ref).Scan(&same)
	if e != nil {
		return e
	}
	if !same {
		return fail(503, "SYNC_ORIGIN_CONFLICT", "Immutable origin or owner position conflicts; source progress was retained")
	}
	return nil
}
