package jobsim

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["directory"] += `CREATE TABLE IF NOT EXISTS directory.contact_rate_windows(subject text NOT NULL,window_start timestamptz NOT NULL,count int NOT NULL,PRIMARY KEY(subject,window_start));
UPDATE directory.contacts SET request_gate='open' WHERE request_gate<>'open';`
}

// Settled rounds never permanently prohibit a new request; only new pending
// responsibilities consume this bounded rate window. Exact retries do not.
func contactRequestRate(ctx context.Context, tx pgx.Tx, requester, recipient string) error {
	for _, rule := range []struct {
		subject string
		limit   int
	}{{"pair:" + requester + ":" + recipient, 5}, {"owner:" + requester, 30}} {
		var count int
		if err := tx.QueryRow(ctx, `INSERT INTO directory.contact_rate_windows(subject,window_start,count) VALUES($1,date_trunc('minute',now()),1) ON CONFLICT(subject,window_start) DO UPDATE SET count=directory.contact_rate_windows.count+1 RETURNING count`, rule.subject).Scan(&count); err != nil {
			return err
		}
		if count > rule.limit {
			return fail(429, "CONTACT_RATE_LIMITED", "Too many new requests; retry after the current minute")
		}
	}
	return nil
}
