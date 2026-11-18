package jobsim

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// A journal handed to another physical database remains readable to the offline
// operator, but can never serve an ordinary runtime again under the old identity.
func (a *App) journalHandoffAllowed(ctx context.Context) error {
	if a.Role != "journal" {
		return nil
	}
	var exists bool
	if err := a.DB.QueryRow(ctx, `SELECT to_regclass('journal.deployment_handoff') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var expected, actual string
	err := a.DB.QueryRow(ctx, `SELECT target_identity FROM journal.deployment_handoff WHERE singleton`).Scan(&expected)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if err = a.DB.QueryRow(ctx, `SELECT system_identifier::text||':'||(SELECT oid::text FROM pg_database WHERE datname=current_database()) FROM pg_control_system()`).Scan(&actual); err != nil {
		return err
	}
	if actual != expected {
		return fail(503, "JOURNAL_HANDED_OFF", "The latest ledger was sealed for its recorded isolated target; resume the original restore operation")
	}
	return nil
}
func requireJournalHandoffTarget(ctx context.Context, db *pgx.Conn, operation, binding, target string) error {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('journal.deployment_handoff') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var priorOperation, priorBinding, priorTarget string
	err := db.QueryRow(ctx, `SELECT operation_id,binding,target_identity FROM journal.deployment_handoff WHERE singleton`).Scan(&priorOperation, &priorBinding, &priorTarget)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	actual, err := databaseIdentity(ctx, db)
	if err != nil {
		return err
	}
	if journalHandoffDecision(priorOperation, priorBinding, priorTarget, actual, operation, binding, target) == "deny" {
		return fmt.Errorf("current ledger is sealed; only its original operation or its current authoritative target may hand off")
	}
	return nil
}
func journalHandoffDecision(priorOperation, priorBinding, priorTarget, current, operation, binding, target string) string {
	if priorOperation == operation {
		if priorBinding == binding && priorTarget == target {
			return "resume"
		}
		return "deny"
	}
	if priorTarget == current {
		return "next"
	}
	return "deny"
}
