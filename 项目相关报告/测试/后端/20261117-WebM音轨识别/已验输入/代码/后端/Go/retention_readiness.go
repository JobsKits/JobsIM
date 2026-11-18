package jobsim

import (
	"context"
	"net/http"
)

// Capability is derived from the current physical store and live production
// owners. An administrator flag or native self-declaration cannot supply these.
func (a *App) receiptLifecycleReady(ctx context.Context, q storageQualification) bool {
	if !q.Qualified || a.Config.NATSURL == "" || !a.syncPublishConnected.Load() || !a.syncConsumeConnected.Load() {
		return false
	}
	var epoch string
	if a.DB.QueryRow(ctx, `SELECT restore_epoch::text FROM sync.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch) != nil || epoch != q.Epoch {
		return false
	}
	if a.requireFeature(ctx, "after_receipt") != nil {
		return false
	}
	var result struct {
		Ready bool `json:"ready"`
	}
	return a.rpc(ctx, "message", "/internal/retention/readiness", map[string]string{"restoreEpoch": epoch}, &result) == nil && result.Ready
}
func (a *App) retentionReadiness(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "sync" {
		return fail(403, "RETENTION_OWNER_REQUIRED", "Only the qualified delivery owner may check readiness")
	}
	var q struct {
		Epoch string `json:"restoreEpoch"`
	}
	if err := decodeReceiptRequest(r, &q); err != nil {
		return err
	}
	var epoch string
	var durable bool
	if err := a.DB.QueryRow(r.Context(), `SELECT restore_epoch::text,current_setting('fsync')='on' AND current_setting('full_page_writes')='on' AND current_setting('synchronous_commit')='on' FROM message.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch, &durable); err != nil {
		return err
	}
	verified, err := a.epoch(r.Context())
	if err != nil {
		return err
	}
	respond(w, 200, map[string]bool{"ready": durable && a.Config.JournalDatabaseURL != "" && epoch == q.Epoch && epoch == str(verified)})
	return nil
}
