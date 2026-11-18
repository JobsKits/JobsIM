package jobsim

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

const migrationSQL = `CREATE TABLE IF NOT EXISTS sync.migration_pairings(
id uuid PRIMARY KEY,scope_id uuid NOT NULL,user_id uuid NOT NULL,target_device uuid NOT NULL,target_store uuid NOT NULL,restore_epoch bigint NOT NULL,security_version bigint NOT NULL,source_generation bigint NOT NULL,request_id text NOT NULL,expires_at timestamptz NOT NULL,transfer_id uuid,UNIQUE(scope_id,user_id,target_device,restore_epoch,request_id));
CREATE TABLE IF NOT EXISTS sync.migrations(
id uuid PRIMARY KEY,scope_id uuid NOT NULL,user_id uuid NOT NULL,request_id text NOT NULL,request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),source_device uuid NOT NULL,source_store uuid NOT NULL,target_device uuid NOT NULL,target_store uuid NOT NULL,restore_epoch bigint NOT NULL,security_version bigint NOT NULL,source_generation bigint NOT NULL,pairing_id uuid NOT NULL UNIQUE,
state text NOT NULL CHECK(state IN('building','ready','installed_reported','revoking','retired')),manifest jsonb NOT NULL,manifest_digest text NOT NULL,key_ciphertext bytea NOT NULL DEFAULT decode('','hex'),capsule_digest text NOT NULL DEFAULT '',size_bytes bigint NOT NULL DEFAULT 0,expires_at timestamptz NOT NULL,retired_reason text,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(scope_id,user_id,source_device,restore_epoch,request_id));
CREATE INDEX IF NOT EXISTS sync_migration_live ON sync.migrations(scope_id,state,expires_at);`

func migrateSyncMigrations(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, migrationSQL)
	return err
}
func (a *App) syncMigrationRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/sync/migration-capabilities", a.migrationCapabilities)
	h("POST /api/sync/migration-pairings", a.createMigrationPairing)
	h("POST /api/sync/migration-pairing-results", a.migrationPairingResults)
	h("POST /api/sync/migrations", a.createMigration)
	h("POST /api/sync/migration-results", a.migrationResults)
	h("GET /api/sync/migrations/{id}", a.migrationStatus)
	h("GET /api/sync/migrations/{id}/capsule", a.migrationCapsule)
	h("POST /api/sync/migrations/{id}/key", a.migrationKey)
	h("POST /api/sync/migrations/{id}/validate", a.validateMigration)
	h("POST /api/sync/migrations/{id}/installed", a.migrationInstalled)
	h("POST /api/sync/migrations/{id}/cancel", a.cancelMigration)
	h("POST /internal/migrations/drain", internal(a.migrationDrain))
	h("POST /internal/migrations/retire", internal(a.migrationRetire))
}

func (a *App) migrationCapabilities(w http.ResponseWriter, r *http.Request) error {
	u, err := a.migrationUser(r)
	if err != nil {
		return err
	}
	if err = a.migrationStoreCurrent(r.Context(), u, r.Header.Get("X-JobsIM-Store-Generation")); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"enabled": true, "format": migrationFormat, "sourceContextId": "personal", "maxMessages": migrationMaxMessages, "maxObjects": migrationMaxObjects, "maxRequestBytes": migrationMaxBytes, "maxObjectBytes": 64 << 20, "maxLifetimeSeconds": 600, "pairingLifetimeSeconds": 300, "installLifetimeSeconds": 5, "qualifiedAck": false, "targetQualified": false})
	return nil
}

func (a *App) migrationUser(r *http.Request) (User, error) {
	if err := a.requireMigrationTLS(r); err != nil {
		return User{}, err
	}
	u, err := a.syncUser(r)
	if err != nil {
		return u, err
	}
	_, err = a.migrationPrincipal(r.Context(), u, "", "")
	return u, err
}
func (a *App) readMigrationPairing(ctx context.Context, u User, request, store string) (migrationPairingProof, error) {
	var p migrationPairingProof
	p.Scope = a.Config.DeploymentID
	p.User = u.ID
	p.Device = u.DeviceID
	p.Store = store
	p.Epoch = u.RestoreEpoch
	err := a.DB.QueryRow(ctx, `SELECT id::text,security_version::text,source_generation::text,extract(epoch FROM expires_at)::bigint FROM sync.migration_pairings WHERE scope_id=$1 AND user_id=$2 AND target_device=$3 AND target_store=$4 AND restore_epoch=$5 AND request_id=$6`, p.Scope, p.User, p.Device, p.Store, p.Epoch, request).Scan(&p.ID, &p.Security, &p.SourceGeneration, &p.Expires)
	return p, err
}
func (a *App) pairingResponse(w http.ResponseWriter, p migrationPairingProof) {
	respond(w, 200, map[string]any{"pairingId": p.ID, "pairingProof": migrationProof(a, "mp1", p), "targetDeviceId": p.Device, "targetStoreGeneration": p.Store, "restoreEpoch": p.Epoch, "expiresAt": time.Unix(p.Expires, 0).UTC(), "sourcePreserved": true})
}
func (a *App) createMigrationPairing(w http.ResponseWriter, r *http.Request) error {
	u, err := a.migrationUser(r)
	if err != nil {
		return err
	}
	var q migrationPairingRequest
	if err = decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if !validReceiptNonce(q.Request) || q.Epoch != u.RestoreEpoch {
		return fail(400, "INVALID_MIGRATION_PAIRING", "Original target request and current restore epoch required")
	}
	if err = a.migrationStoreCurrent(r.Context(), u, q.Store); err != nil {
		return err
	}
	p, err := a.readMigrationPairing(r.Context(), u, q.Request, q.Store)
	if err == nil {
		a.pairingResponse(w, p)
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	principal, err := a.migrationPrincipal(r.Context(), u, q.Password, q.OTP)
	if err != nil {
		return err
	}
	if q.Password == "" {
		return fail(403, "MIGRATION_REAUTHENTICATION_REQUIRED", "Explicit current-password proof required")
	}
	p = migrationPairingProof{ID: newID(), Scope: a.Config.DeploymentID, User: u.ID, Device: u.DeviceID, Store: q.Store, Epoch: u.RestoreEpoch, Security: principal.Security, SourceGeneration: principal.SourceGeneration, Expires: time.Now().Add(5 * time.Minute).Unix()}
	_, err = a.DB.Exec(r.Context(), `INSERT INTO sync.migration_pairings(id,scope_id,user_id,target_device,target_store,restore_epoch,security_version,source_generation,request_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, p.ID, p.Scope, p.User, p.Device, p.Store, p.Epoch, p.Security, p.SourceGeneration, q.Request, time.Unix(p.Expires, 0))
	if err != nil {
		return err
	}
	p, err = a.readMigrationPairing(r.Context(), u, q.Request, q.Store)
	if err == pgx.ErrNoRows {
		return fail(409, "MIGRATION_ORIGINAL_CHANGED", "The original target pairing is already bound to a different physical store")
	}
	if err != nil {
		return err
	}
	a.pairingResponse(w, p)
	return nil
}
func (a *App) migrationPairingResults(w http.ResponseWriter, r *http.Request) error {
	u, err := a.migrationUser(r)
	if err != nil {
		return err
	}
	var q struct {
		Request string `json:"requestId"`
		Store   string `json:"storeGeneration"`
	}
	if err = decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if !validReceiptNonce(q.Request) || !syncValidationID(q.Store) {
		return fail(400, "INVALID_MIGRATION_REQUEST", "Original target request and store required")
	}
	p, err := a.readMigrationPairing(r.Context(), u, q.Request, q.Store)
	if err == pgx.ErrNoRows {
		respond(w, 200, map[string]string{"status": "not_found"})
		return nil
	}
	if err != nil {
		return err
	}
	a.pairingResponse(w, p)
	return nil
}

const migrationColumns = `id::text,user_id::text,request_id,request_digest,source_device::text,source_store::text,target_device::text,target_store::text,restore_epoch::text,security_version::text,source_generation::text,state,manifest,manifest_digest,key_ciphertext,capsule_digest,size_bytes,expires_at`

func scanMigration(row pgx.Row) (migrationRecord, error) {
	var v migrationRecord
	var raw []byte
	err := row.Scan(&v.ID, &v.User, &v.Request, &v.Digest, &v.SourceDevice, &v.SourceStore, &v.TargetDevice, &v.TargetStore, &v.Epoch, &v.Security, &v.SourceGeneration, &v.State, &raw, &v.ManifestDigest, &v.Key, &v.CapsuleDigest, &v.Bytes, &v.Expires)
	if err == nil {
		err = json.Unmarshal(raw, &v.Manifest)
	}
	return v, err
}
func (a *App) readMigration(ctx context.Context, id string) (migrationRecord, error) {
	return scanMigration(a.DB.QueryRow(ctx, `SELECT `+migrationColumns+` FROM sync.migrations WHERE scope_id=$1 AND id=$2`, a.Config.DeploymentID, id))
}
func (a *App) readMigrationRequest(ctx context.Context, u User, request, store string) (migrationRecord, error) {
	return scanMigration(a.DB.QueryRow(ctx, `SELECT `+migrationColumns+` FROM sync.migrations WHERE scope_id=$1 AND user_id=$2 AND source_device=$3 AND source_store=$4 AND restore_epoch=$5 AND request_id=$6`, a.Config.DeploymentID, u.ID, u.DeviceID, store, u.RestoreEpoch, request))
}
func (a *App) migrationResponse(w http.ResponseWriter, rec migrationRecord) {
	raw, _, _ := migrationManifestBytes(rec.Manifest)
	respond(w, 200, map[string]any{"id": rec.ID, "requestId": rec.Request, "status": rec.State, "manifest": migrationPublicManifest(rec.Manifest), "manifestEnvelope": base64.RawURLEncoding.EncodeToString(raw), "manifestDigest": rec.ManifestDigest, "capsuleSHA256": rec.CapsuleDigest, "sizeBytes": str(rec.Bytes), "expiresAt": rec.Expires, "sourcePreserved": true, "qualifiedAck": false, "targetQualified": false})
}

func (a *App) createMigration(w http.ResponseWriter, r *http.Request) error {
	select {
	case migrationCreationSlot <- struct{}{}:
		defer func() { <-migrationCreationSlot }()
	default:
		return fail(429, "MIGRATION_BUSY", "Another bounded personal migration build is active")
	}
	u, err := a.migrationUser(r)
	if err != nil {
		return err
	}
	var q migrationCreateRequest
	if err = decodeMigrationRequest(r, &q, migrationMaxBytes); err != nil {
		return err
	}
	if !validReceiptNonce(q.Request) || q.Context != "personal" || q.Epoch != u.RestoreEpoch || !syncValidationID(q.Store) {
		return fail(400, "INVALID_MIGRATION_SCOPE", "Same-instance personal source and original request required")
	}
	if err = a.migrationStoreCurrent(r.Context(), u, q.Store); err != nil {
		return err
	}
	digest := a.migrationInputDigest(q)
	old, err := a.readMigrationRequest(r.Context(), u, q.Request, q.Store)
	if err == nil {
		if !migrationDigestMatches(old.Digest, digest) {
			return fail(409, "MIGRATION_ORIGINAL_CHANGED", "Retry must retain the complete original manifest and target")
		}
		a.migrationResponse(w, old)
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	principal, err := a.migrationPrincipal(r.Context(), u, q.Password, q.OTP)
	if err != nil {
		return err
	}
	if q.Password == "" {
		return fail(403, "MIGRATION_REAUTHENTICATION_REQUIRED", "Source creation requires current-password proof")
	}
	var pairing migrationPairingProof
	if err = migrationDecodeProof(a, "mp1", q.Pairing, &pairing); err != nil {
		return err
	}
	if pairing.Scope != a.Config.DeploymentID || pairing.User != u.ID || pairing.Epoch != u.RestoreEpoch || pairing.Security != principal.Security || pairing.SourceGeneration != principal.SourceGeneration || pairing.Expires <= time.Now().Unix() || pairing.Device == u.DeviceID || !syncValidationID(pairing.ID) || !syncValidationID(pairing.Device) || !syncValidationID(pairing.Store) {
		return fail(403, "MIGRATION_TARGET_MISMATCH", "A different current personal target store of this same account is required")
	}
	target := User{ID: u.ID, DeviceID: pairing.Device, RestoreEpoch: u.RestoreEpoch}
	if err = a.migrationStoreCurrent(r.Context(), target, pairing.Store); err != nil {
		return err
	}
	targetPrincipal, err := a.migrationPrincipal(r.Context(), target, "", "")
	if err != nil {
		return err
	}
	if targetPrincipal.Security != principal.Security || targetPrincipal.SourceGeneration != principal.SourceGeneration {
		return fail(409, "MIGRATION_TARGET_CHANGED", "Target authorization changed after pairing")
	}
	rows, refs, err := a.migrationPrepareMessages(r.Context(), u, q)
	if err != nil {
		return err
	}
	objects, objectRefs, err := a.migrationPrepareObjects(r.Context(), u, q, refs)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	expires := migrationDeadline(now, refs, objectRefs).Truncate(time.Microsecond)
	for _, deadline := range []*time.Time{principal.AccessExpires, targetPrincipal.AccessExpires} {
		if deadline != nil && deadline.Before(expires) {
			expires = deadline.UTC().Truncate(time.Microsecond)
		}
	}
	if !expires.After(now) {
		return fail(409, "MIGRATION_SOURCE_EXPIRED", "No remaining source delivery window")
	}
	manifest := migrationManifest{Format: migrationFormat, ID: newID(), Scope: a.Config.DeploymentID, User: u.ID, Context: "personal", SourceDevice: u.DeviceID, SourceStore: q.Store, TargetDevice: pairing.Device, TargetStore: pairing.Store, Epoch: u.RestoreEpoch, Security: principal.Security, SourceGeneration: principal.SourceGeneration, Messages: refs, Objects: objectRefs, Expires: expires}
	raw, manifestDigest, err := migrationManifestBytes(manifest)
	if err != nil {
		return err
	}
	raw, err = json.Marshal(manifest)
	if err != nil {
		return err
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext($1))`, "jobsim:migration-build:"+a.Config.DeploymentID); err != nil {
		return err
	}
	var total, personal int
	if err = tx.QueryRow(r.Context(), `SELECT count(*),count(*) FILTER(WHERE user_id=$2) FROM sync.migrations WHERE scope_id=$1 AND state<>'retired'`, a.Config.DeploymentID, u.ID).Scan(&total, &personal); err != nil {
		return err
	}
	if total >= 1024 || personal >= 4 {
		return fail(429, "MIGRATION_CAPACITY", "Retire the original bounded personal transfers before building another")
	}
	var claimed bool
	err = tx.QueryRow(r.Context(), `UPDATE sync.migration_pairings SET transfer_id=$1 WHERE id=$2 AND scope_id=$3 AND user_id=$4 AND target_device=$5 AND target_store=$6 AND restore_epoch=$7 AND security_version=$8 AND source_generation=$9 AND expires_at>clock_timestamp() AND transfer_id IS NULL RETURNING true`, manifest.ID, pairing.ID, manifest.Scope, u.ID, pairing.Device, pairing.Store, manifest.Epoch, manifest.Security, manifest.SourceGeneration).Scan(&claimed)
	if err != nil || !claimed {
		return fail(409, "MIGRATION_PAIRING_USED", "Original pairing is expired or already assigned; query the source request before pairing again")
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO sync.migrations(id,scope_id,user_id,request_id,request_digest,source_device,source_store,target_device,target_store,restore_epoch,security_version,source_generation,pairing_id,state,manifest,manifest_digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'building',$14,$15,$16)`, manifest.ID, manifest.Scope, u.ID, q.Request, digest, u.DeviceID, q.Store, pairing.Device, pairing.Store, manifest.Epoch, manifest.Security, manifest.SourceGeneration, pairing.ID, raw, manifestDigest, expires)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	if err = a.registerMigrationInterests(r.Context(), manifest); err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return err
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	plain, err := json.Marshal(migrationPackage{Format: migrationFormat, ManifestDigest: manifestDigest, Messages: rows, Objects: objects})
	if err != nil {
		return err
	}
	defer func() {
		for i := range plain {
			plain[i] = 0
		}
	}()
	capsule, err := migrationSeal(key, plain, a.migrationAAD(manifest, manifestDigest))
	if err != nil {
		return err
	}
	if err = deploymentRequireSpace(r.Context(), a.Config.StateDir, int64(len(capsule))); err != nil {
		return fail(507, "MIGRATION_STORAGE_RESERVE", "Private migration build must preserve the instance disk reserve")
	}
	path, err := a.migrationPath(manifest.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(capsule)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err = syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	sealed, err := a.seal(base64.RawURLEncoding.EncodeToString(key), a.migrationKeyAAD(manifest))
	if err != nil {
		return err
	}
	rec, err := a.readMigration(r.Context(), manifest.ID)
	if err != nil {
		return err
	}
	rec.State = "ready"
	if err = a.migrationValidateRecord(r.Context(), rec); err != nil {
		return err
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE sync.migrations SET state='ready',key_ciphertext=$3,capsule_digest=$4,size_bytes=$5,updated_at=now() WHERE scope_id=$1 AND id=$2 AND state='building'`, manifest.Scope, manifest.ID, sealed, syncDigest(capsule), len(capsule))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "MIGRATION_BUILD_RETIRED", "Old builders cannot republish a retired transfer")
	}
	rec, err = a.readMigration(r.Context(), manifest.ID)
	if err != nil {
		return err
	}
	a.migrationResponse(w, rec)
	return nil
}

func (a *App) migrationResults(w http.ResponseWriter, r *http.Request) error {
	u, err := a.migrationUser(r)
	if err != nil {
		return err
	}
	var q struct {
		Request string `json:"requestId"`
		Store   string `json:"sourceStoreGeneration"`
	}
	if err = decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if !validReceiptNonce(q.Request) || !syncValidationID(q.Store) {
		return fail(400, "INVALID_MIGRATION_REQUEST", "Original source request and store required")
	}
	rec, err := a.readMigrationRequest(r.Context(), u, q.Request, q.Store)
	if err == pgx.ErrNoRows {
		respond(w, 200, map[string]string{"status": "not_found"})
		return nil
	}
	if err != nil {
		return err
	}
	a.migrationResponse(w, rec)
	return nil
}
func (a *App) migrationAuthorized(r *http.Request, targetOnly bool) (migrationRecord, User, error) {
	u, err := a.migrationUser(r)
	if err != nil {
		return migrationRecord{}, u, err
	}
	id := r.PathValue("id")
	if !syncValidationID(id) {
		return migrationRecord{}, u, fail(400, "INVALID_MIGRATION_ID", "Original server transfer identity required")
	}
	rec, err := a.readMigration(r.Context(), id)
	if err != nil {
		return rec, u, fail(404, "MIGRATION_NOT_FOUND", "Original personal transfer unavailable")
	}
	store := r.Header.Get("X-JobsIM-Store-Generation")
	target := u.DeviceID == rec.TargetDevice && store == rec.TargetStore
	source := u.DeviceID == rec.SourceDevice && store == rec.SourceStore
	if rec.User != u.ID || rec.Epoch != u.RestoreEpoch || (!target && (targetOnly || !source)) {
		return rec, u, fail(403, "MIGRATION_DEVICE_MISMATCH", "Only the exact original personal source or paired target store is authorized")
	}
	if err = a.migrationStoreCurrent(r.Context(), u, store); err != nil {
		return rec, u, err
	}
	return rec, u, nil
}
func (a *App) migrationStatus(w http.ResponseWriter, r *http.Request) error {
	rec, _, err := a.migrationAuthorized(r, false)
	if err != nil {
		return err
	}
	a.migrationResponse(w, rec)
	return nil
}
