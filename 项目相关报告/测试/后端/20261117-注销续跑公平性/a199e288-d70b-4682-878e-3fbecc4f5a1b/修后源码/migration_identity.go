package jobsim

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

func (a *App) migrationIdentityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	if a.Role != "identity" {
		return
	}
	h("POST /internal/migrations/principal", internal(a.migrationIdentityPrincipal))
	h("POST /internal/migrations/reauthenticate", internal(a.migrationIdentityReauthenticate))
}
func (a *App) readMigrationPrincipal(ctx context.Context, q migrationReauthentication) (migrationPrincipal, error) {
	value := migrationPrincipal{User: q.User, Device: q.Device, Epoch: q.Epoch}
	if !syncValidationID(q.User) || !syncValidationID(q.Device) || !syncValidationCounter(q.Epoch, false) {
		return value, fail(400, "INVALID_MIGRATION_PRINCIPAL", "Canonical personal device and current epoch required")
	}
	epoch, err := a.epoch(ctx)
	if err != nil {
		return value, err
	}
	if str(epoch) != q.Epoch {
		return value, nil
	}
	err = a.DB.QueryRow(ctx, `SELECT u.security_version::text,i.personal_source_generation::text,EXISTS(SELECT 1 FROM auth.sessions s WHERE s.user_id=u.id AND s.device_id=$2 AND s.family_id IS NOT NULL AND s.source_context_id='personal' AND s.source_generation=i.personal_source_generation AND s.restore_epoch=i.restore_epoch AND s.generation=u.credential_generation AND s.security_version=u.security_version AND NOT s.web_pending AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.access_expires_at>clock_timestamp() AND s.export_pending_operation_id IS NULL) AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL FROM auth.users u CROSS JOIN auth.instance i WHERE u.id=$1 AND i.singleton AND i.restore_epoch=$3`, q.User, q.Device, q.Epoch).Scan(&value.Security, &value.SourceGeneration, &value.Active)
	if err == pgx.ErrNoRows {
		return value, nil
	}
	return value, err
}
func (a *App) migrationIdentityPrincipal(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "sync" {
		return fail(403, "MIGRATION_OWNER_REQUIRED", "Only the controlled migration owner may inspect its device principal")
	}
	var q migrationReauthentication
	if err := decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if q.Password != "" || q.OTP != "" {
		return fail(400, "MIGRATION_PURPOSE_MISMATCH", "A principal lookup cannot consume reauthentication material")
	}
	value, err := a.readMigrationPrincipal(r.Context(), q)
	if err != nil {
		return err
	}
	respond(w, 200, value)
	return nil
}
func (a *App) migrationIdentityReauthenticate(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-JobsIM-Caller") != "sync" {
		return fail(403, "MIGRATION_OWNER_REQUIRED", "Only the controlled migration owner may request reauthentication")
	}
	var q migrationReauthentication
	if err := decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if err := a.rate(r.Context(), "migration-reauth:"+q.User, 10); err != nil {
		return err
	}
	value, err := a.readMigrationPrincipal(r.Context(), q)
	if err != nil {
		return err
	}
	if !value.Active || len(q.Password) == 0 || len(q.Password) > 512 {
		return fail(403, "MIGRATION_REAUTHENTICATION_REJECTED", "Current personal account reauthentication required")
	}
	var password string
	var admin bool
	if err = a.DB.QueryRow(r.Context(), `SELECT password_hash,admin FROM auth.users WHERE id=$1`, q.User).Scan(&password, &admin); err != nil {
		return err
	}
	if err = a.passwordSlot(r.Context()); err != nil {
		return err
	}
	valid := verifyPassword(password, norm.NFC.String(q.Password))
	<-a.hashSlots
	if !valid {
		return fail(403, "MIGRATION_REAUTHENTICATION_REJECTED", "Current-password proof was rejected")
	}
	if admin {
		if err = a.consumeMFA(r.Context(), q.User, q.OTP); err != nil {
			return err
		}
	}
	after, err := a.readMigrationPrincipal(r.Context(), q)
	if err != nil {
		return err
	}
	if !after.Active || after.Security != value.Security || after.SourceGeneration != value.SourceGeneration {
		return fail(409, "MIGRATION_PRINCIPAL_CHANGED", "Account authorization changed during reauthentication")
	}
	respond(w, 200, after)
	return nil
}
