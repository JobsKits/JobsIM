package jobsim

import (
	"context"
	"sync"
	"time"
)

type retentionClockObservation struct{ Database, Monotonic time.Time }

var retentionClockObservations sync.Map

// Database UTC determines deadlines. Runtime monotonic time detects steps in
// either direction; a durable alarm prevents a restart from clearing a detected
// clock fault. Clearing an alarm requires an operator's independent clock audit.
func (a *App) retentionClockGuard(ctx context.Context) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO message.lifecycle_clock(singleton,last_database_time,alarmed) VALUES(true,clock_timestamp(),false) ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	var previous, current time.Time
	var alarm bool
	if err = tx.QueryRow(ctx, `SELECT last_database_time,clock_timestamp(),alarmed FROM message.lifecycle_clock WHERE singleton FOR UPDATE`).Scan(&previous, &current, &alarm); err != nil {
		return err
	}
	sampled := time.Now()
	bad := alarm || current.Before(previous.Add(-time.Second)) || current.Sub(sampled) > 5*time.Second || sampled.Sub(current) > 5*time.Second
	if value, ok := retentionClockObservations.Load(a); ok {
		last := value.(retentionClockObservation)
		drift := current.Sub(last.Database) - sampled.Sub(last.Monotonic)
		bad = bad || drift > 5*time.Second || drift < -5*time.Second
	}
	if bad {
		if _, err = tx.Exec(ctx, `UPDATE message.lifecycle_clock SET alarmed=true WHERE singleton`); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return fail(503, "RETENTION_CLOCK_UNVERIFIED", "An independently checked stable database clock is required before irreversible cleanup")
	}
	if _, err = tx.Exec(ctx, `UPDATE message.lifecycle_clock SET last_database_time=$1 WHERE singleton`, current); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	retentionClockObservations.Store(a, retentionClockObservation{current, sampled})
	return nil
}
