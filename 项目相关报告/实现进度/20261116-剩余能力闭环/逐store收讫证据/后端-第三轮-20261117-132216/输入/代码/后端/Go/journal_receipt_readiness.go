package jobsim

import "net/http"

type receiptJournalProtection struct {
	Epoch   string `json:"restoreEpoch"`
	System  string `json:"systemId"`
	Durable bool   `json:"durable"`
}

func (a *App) journalReceiptReadinessRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	if a.Role != "journal" {
		return
	}
	h("POST /internal/receipts/protection", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "message" {
			return fail(403, "RECEIPT_OWNER_REQUIRED", "Only the lifecycle owner may inspect deletion protection")
		}
		if err := a.journalHandoffAllowed(r.Context()); err != nil {
			return err
		}
		var value receiptJournalProtection
		if err := a.DB.QueryRow(r.Context(), `SELECT restore_epoch::text,(SELECT system_identifier::text FROM pg_control_system()),current_setting('fsync')='on' AND current_setting('full_page_writes')='on' AND current_setting('synchronous_commit')='on' FROM journal.restore_epochs WHERE singleton`).Scan(&value.Epoch, &value.System, &value.Durable); err != nil {
			return err
		}
		respond(w, 200, value)
		return nil
	}))
}
