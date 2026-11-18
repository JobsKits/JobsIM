package jobsim

import (
	"context"
	"net/http"
)

// Capability is derived from the current physical store and live production
// owners. An administrator flag or native self-declaration cannot supply these.
func (a *App) receiptLifecycleReady(ctx context.Context, q storageQualification) bool {
	return q.Qualified && a.receiptRuntime(ctx, q.Epoch).Ready
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
	var epoch, system string
	var durable bool
	if err := a.DB.QueryRow(r.Context(), `SELECT restore_epoch::text,(SELECT system_identifier::text FROM pg_control_system()),current_setting('fsync')='on' AND current_setting('full_page_writes')='on' AND current_setting('synchronous_commit')='on' FROM message.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch, &system, &durable); err != nil {
		return err
	}
	var journal receiptJournalProtection
	if err := a.rpc(r.Context(), "journal", "/internal/receipts/protection", struct{}{}, &journal); err != nil {
		return err
	}
	value := receiptRuntimeReadiness{MessageDurable: durable, JournalDurable: journal.Durable, JournalIndependent: a.Config.JournalDatabaseURL != "" && journal.System != "" && system != "" && journal.System != system, EpochMatches: epoch == q.Epoch && epoch == journal.Epoch, Blockers: []string{}}
	if !value.MessageDurable {
		value.Blockers = append(value.Blockers, "message_not_durable")
	}
	if !value.JournalDurable {
		value.Blockers = append(value.Blockers, "journal_not_durable")
	}
	if !value.JournalIndependent {
		value.Blockers = append(value.Blockers, "journal_not_independent")
	}
	if !value.EpochMatches {
		value.Blockers = append(value.Blockers, "protection_epoch_changed")
	}
	value.Ready = value.MessageDurable && value.JournalDurable && value.JournalIndependent && value.EpochMatches
	respond(w, 200, value)
	return nil
}
