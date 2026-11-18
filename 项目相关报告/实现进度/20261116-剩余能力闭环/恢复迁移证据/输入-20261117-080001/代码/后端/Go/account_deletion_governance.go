package jobsim

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func deletionInternalCaller(r *http.Request, role string) error {
	if r.Header.Get("X-JobsIM-Caller") != role {
		return fail(403, "DELETION_OWNER_REQUIRED", "Original fact owner required")
	}
	return nil
}
func validDeletionBinding(d accountDeletion) bool {
	return validObjectID(d.ID) && validObjectID(d.User) && validObjectID(d.Device) && validReceiptNonce(d.Request) && syncValidationCounter(d.Epoch, false)
}
func (a *App) prepareDeletionInternal(w http.ResponseWriter, r *http.Request) error {
	if err := deletionInternalCaller(r, "identity"); err != nil {
		return err
	}
	var q accountDeletion
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validDeletionBinding(q) {
		return fail(400, "INVALID_ACCOUNT_DELETION", "Original deletion binding required")
	}
	days := 7
	if a.Config.AccountDeletionCooldownDays != nil {
		days = *a.Config.AccountDeletionCooldownDays
	}
	if days < 0 || days > 30 {
		return fail(503, "INVALID_DELETION_COOLDOWN", "Configured deletion cooldown must be zero to thirty days")
	}
	if _, err := a.DB.Exec(r.Context(), `INSERT INTO governance.account_deletions(id,user_id,request_id,device_id,restore_epoch,cancel_until) VALUES($1,$2,$3,$4,$5,clock_timestamp()+$6*interval '1 day') ON CONFLICT DO NOTHING`, q.ID, q.User, q.Request, q.Device, q.Epoch, days); err != nil {
		return err
	}
	d, err := scanGovernanceDeletion(a.DB.QueryRow(r.Context(), `SELECT `+deletionGovernanceColumns+` FROM governance.account_deletions WHERE id=$1 AND user_id=$2 AND request_id=$3 AND device_id=$4 AND restore_epoch=$5`, q.ID, q.User, q.Request, q.Device, q.Epoch))
	if err != nil {
		return fail(409, "DELETION_BINDING_CHANGED", "Original deletion differs")
	}
	respond(w, 200, d)
	return nil
}
func (a *App) governanceDeletionState(w http.ResponseWriter, r *http.Request) error {
	if err := deletionInternalCaller(r, "identity"); err != nil {
		return err
	}
	var q accountDeletion
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	d, err := scanGovernanceDeletion(a.DB.QueryRow(r.Context(), `SELECT `+deletionGovernanceColumns+` FROM governance.account_deletions WHERE id=$1 AND user_id=$2`, q.ID, q.User))
	if err == pgx.ErrNoRows {
		return fail(404, "ACCOUNT_DELETION_NOT_FOUND", "Original deletion is not yet acknowledged")
	}
	if err != nil {
		return err
	}
	respond(w, 200, d)
	return nil
}
func (a *App) activateDeletionGovernance(w http.ResponseWriter, r *http.Request) error {
	if err := deletionInternalCaller(r, "identity"); err != nil {
		return err
	}
	var q accountDeletion
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE governance.account_deletions SET state='requested',updated_at=now() WHERE id=$1 AND user_id=$2 AND state IN('prepared','requested') AND row_version=1`, q.ID, q.User)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "DELETION_BINDING_CHANGED", "Original requested deletion changed")
	}
	respond(w, 200, map[string]bool{"confirmed": true})
	return nil
}
func (a *App) beginDeletionCancel(w http.ResponseWriter, r *http.Request) error {
	if err := deletionInternalCaller(r, "identity"); err != nil {
		return err
	}
	var q accountDeletion
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	if !validObjectID(q.ID) || !validObjectID(q.User) || !validReceiptNonce(q.Cancel) || !syncValidationCounter(q.Version, false) {
		return fail(400, "INVALID_DELETION_CANCEL", "Original cancellation and request version required")
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	d, err := scanGovernanceDeletion(tx.QueryRow(r.Context(), `SELECT `+deletionGovernanceColumns+` FROM governance.account_deletions WHERE id=$1 AND user_id=$2 FOR UPDATE`, q.ID, q.User))
	if err != nil {
		return err
	}
	if d.Cancel == q.Cancel && (d.State == "cancel_pending" || d.State == "cancelled") {
		respond(w, 200, d)
		return nil
	}
	var eligible bool
	if err = tx.QueryRow(r.Context(), `SELECT state='requested' AND row_version=$2 AND clock_timestamp()<cancel_until FROM governance.account_deletions WHERE id=$1`, d.ID, q.Version).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return fail(409, "DELETION_CANCEL_CLOSED", "Only the original requested deletion inside its cooldown can be cancelled")
	}
	if _, err = tx.Exec(r.Context(), `UPDATE governance.account_deletions SET state='cancel_pending',cancel_request_id=$2,row_version=row_version+1,updated_at=now() WHERE id=$1`, d.ID, q.Cancel); err != nil {
		return err
	}
	d, err = scanGovernanceDeletion(tx.QueryRow(r.Context(), `SELECT `+deletionGovernanceColumns+` FROM governance.account_deletions WHERE id=$1`, d.ID))
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, d)
	return nil
}
func (a *App) confirmDeletionCancel(w http.ResponseWriter, r *http.Request) error {
	if err := deletionInternalCaller(r, "identity"); err != nil {
		return err
	}
	var q accountDeletion
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE governance.account_deletions SET state='cancelled',updated_at=now() WHERE id=$1 AND user_id=$2 AND row_version=$3 AND cancel_request_id=$4 AND state IN('cancel_pending','cancelled')`, q.ID, q.User, q.Version, q.Cancel)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "DELETION_CANCEL_CONTEXT_CHANGED", "Original cancellation no longer owns the request")
	}
	respond(w, 200, map[string]bool{"confirmed": true})
	return nil
}
func (a *App) qualifyDeletion(w http.ResponseWriter, r *http.Request) error {
	var q accountDeletion
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	d, err := scanGovernanceDeletion(a.DB.QueryRow(r.Context(), `SELECT `+deletionGovernanceColumns+` FROM governance.account_deletions WHERE id=$1 AND user_id=$2 AND row_version=$3 AND state='processing' AND cancel_until<=clock_timestamp()`, q.ID, q.User, q.Version))
	if err != nil {
		return fail(409, "DELETION_NOT_QUALIFIED", "Cleanup requires the current irreversible request version after cooldown")
	}
	respond(w, 200, d)
	return nil
}
func (a *App) resumeAccountDeletions(ctx context.Context, limit int) error {
	if limit < 1 || limit > 10 {
		return fail(400, "INVALID_DELETION_BATCH", "Bounded deletion recovery batch required")
	}
	if a.Role == "identity" {
		rows, err := a.DB.Query(ctx, `SELECT `+deletionIdentityColumns+` FROM auth.account_deletions WHERE state='prepared' ORDER BY updated_at LIMIT $1`, limit)
		if err != nil {
			return err
		}
		items := []accountDeletion{}
		for rows.Next() {
			d, e := scanIdentityDeletion(rows)
			if e != nil {
				rows.Close()
				return e
			}
			items = append(items, d)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		var pending error
		for _, d := range items {
			pending = errors.Join(pending, a.finishDeletionRequest(ctx, d))
		}
		return pending
	}
	if a.Role != "governance" {
		return nil
	}
	rows, err := a.DB.Query(ctx, `SELECT `+deletionGovernanceColumns+` FROM governance.account_deletions WHERE state IN('prepared','cancel_pending','processing') OR state='requested' AND cancel_until<=clock_timestamp() ORDER BY updated_at,id LIMIT $1`, limit)
	if err != nil {
		return err
	}
	items := []accountDeletion{}
	for rows.Next() {
		d, e := scanGovernanceDeletion(rows)
		if e != nil {
			rows.Close()
			return e
		}
		items = append(items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var pending error
	for _, d := range items {
		pending = errors.Join(pending, a.advanceAccountDeletion(ctx, d))
		if ctx.Err() != nil {
			break
		}
	}
	return pending
}
func (a *App) advanceAccountDeletion(ctx context.Context, d accountDeletion) error {
	switch d.State {
	case "prepared":
		var ack struct {
			State string `json:"status"`
		}
		if err := a.rpc(ctx, "identity", "/internal/deletion/activate", d, &ack); err != nil {
			return err
		}
		if ack.State == "retired" {
			_, err := a.DB.Exec(ctx, `UPDATE governance.account_deletions SET state='retired',updated_at=now() WHERE id=$1 AND state='prepared'`, d.ID)
			return err
		}
		return nil
	case "cancel_pending":
		return a.rpc(ctx, "identity", "/internal/deletion/cancel", d, nil)
	case "requested":
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		tag, err := tx.Exec(ctx, `UPDATE governance.account_deletions SET state='processing',row_version=row_version+1,updated_at=now() WHERE id=$1 AND state='requested' AND row_version=$2 AND cancel_until<=clock_timestamp()`, d.ID, d.Version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return nil
		}
		d, err = scanGovernanceDeletion(tx.QueryRow(ctx, `SELECT `+deletionGovernanceColumns+` FROM governance.account_deletions WHERE id=$1`, d.ID))
		if err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	if d.State != "processing" {
		return nil
	}
	if err := a.rpc(ctx, "identity", "/internal/deletion/identity-stage", d, nil); err != nil {
		return err
	}
	for _, owner := range []string{"message", "media", "directory", "conversation", "sync", "governance"} {
		var acknowledged bool
		if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM governance.account_deletion_acks WHERE deletion_id=$1 AND owner=$2 AND request_version=$3)`, d.ID, owner, d.Version).Scan(&acknowledged); err != nil {
			return err
		}
		if acknowledged {
			continue
		}
		if owner == "governance" {
			if err := a.eraseOwnerData(ctx, d); err != nil {
				return err
			}
		} else if err := a.rpc(ctx, owner, "/internal/deletion/erase", d, nil); err != nil {
			return err
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO governance.account_deletion_acks(deletion_id,owner,request_version) VALUES($1,$2,$3) ON CONFLICT(deletion_id,owner) DO UPDATE SET request_version=EXCLUDED.request_version`, d.ID, owner, d.Version); err != nil {
			return err
		}
	}
	d.State = "completed"
	if err := a.rpc(ctx, "identity", "/internal/deletion/identity-stage", d, nil); err != nil {
		return err
	}
	_, err := a.DB.Exec(ctx, `UPDATE governance.account_deletions SET state='completed',updated_at=now() WHERE id=$1 AND state='processing' AND row_version=$2`, d.ID, d.Version)
	return err
}
func (a *App) StartAccountDeletionWorkers(ctx context.Context) {
	if a.Role != "identity" && a.Role != "governance" {
		return
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
			fence, err := a.admissionFence(attempt, true)
			if err == nil {
				scoped, expire := a.heldAdmissionScope(attempt, "exclusive")
				_, gateErr := a.epoch(scoped)
				markers, markerErr := a.admissionMarkers()
				if gateErr == nil && markerErr == nil && len(markers) > 0 {
					markerErr = a.rpc(scoped, "message", "/internal/admission/recover", map[string]string{}, nil)
				}
				if gateErr == nil && markerErr == nil {
					_ = a.resumeAccountDeletions(scoped, 2)
				}
				expire()
				closeExportFence(fence)
			}
			cancel()
		}
	}()
}
