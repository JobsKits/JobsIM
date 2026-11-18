package jobsim

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"time"
)

func (a *App) migrationCapsule(w http.ResponseWriter, r *http.Request) error {
	rec, _, err := a.migrationAuthorized(r, true)
	if err != nil {
		return err
	}
	if err = a.migrationValidateRecord(r.Context(), rec); err != nil {
		return err
	}
	path, err := a.migrationPath(rec.ID)
	if err != nil {
		return err
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() != rec.Bytes {
		return fail(503, "MIGRATION_CAPSULE_INTEGRITY", "Original encrypted capsule unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !syncPrivateTokenFile(opened, file) {
		return fail(503, "MIGRATION_CAPSULE_INTEGRITY", "Private original encrypted capsule required")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, rec.Bytes+1))
	if err != nil || n != rec.Bytes || hex.EncodeToString(hash.Sum(nil)) != rec.CapsuleDigest {
		return fail(503, "MIGRATION_CAPSULE_INTEGRITY", "Encrypted capsule digest differs")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "private,no-store")
	w.Header().Set("X-JobsIM-Capsule-SHA256", rec.CapsuleDigest)
	w.Header().Set("X-JobsIM-Manifest-Digest", rec.ManifestDigest)
	w.Header().Set("ETag", `"`+rec.CapsuleDigest+`"`)
	http.ServeContent(w, r, rec.ID+".jimmig", opened.ModTime(), &mediaRetainedReader{File: file, ctx: r.Context(), due: rec.Expires})
	return nil
}
func (a *App) migrationKey(w http.ResponseWriter, r *http.Request) error {
	rec, u, err := a.migrationAuthorized(r, true)
	if err != nil {
		return err
	}
	var q struct {
		Password string `json:"password"`
		OTP      string `json:"otp,omitempty"`
		Digest   string `json:"manifestDigest"`
	}
	if err = decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if q.Digest != rec.ManifestDigest || q.Password == "" {
		return fail(403, "MIGRATION_REAUTHENTICATION_REQUIRED", "Separate current-password handoff and original manifest digest required")
	}
	if _, err = a.migrationPrincipal(r.Context(), u, q.Password, q.OTP); err != nil {
		return err
	}
	if err = a.migrationValidateRecord(r.Context(), rec); err != nil {
		return err
	}
	key, err := a.unseal(rec.Key, a.migrationKeyAAD(rec.Manifest))
	if err != nil {
		return fail(503, "MIGRATION_KEY_UNAVAILABLE", "Private handoff material unavailable")
	}
	respond(w, 200, map[string]any{"id": rec.ID, "manifestDigest": rec.ManifestDigest, "algorithm": "AES-256-GCM", "key": key, "aad": base64.RawURLEncoding.EncodeToString(a.migrationAAD(rec.Manifest, rec.ManifestDigest)), "expiresAt": rec.Expires, "targetQualified": false})
	return nil
}

type migrationInstallProof struct {
	ID      string `json:"id"`
	Digest  string `json:"manifestDigest"`
	User    string `json:"userId"`
	Device  string `json:"deviceId"`
	Store   string `json:"storeGeneration"`
	Epoch   string `json:"restoreEpoch"`
	Expires int64  `json:"expires"`
}

func (a *App) validateMigration(w http.ResponseWriter, r *http.Request) error {
	rec, u, err := a.migrationAuthorized(r, true)
	if err != nil {
		return err
	}
	var q struct {
		Digest string `json:"manifestDigest"`
	}
	if err = decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if q.Digest != rec.ManifestDigest {
		return fail(409, "MIGRATION_MANIFEST_CHANGED", "The entire original manifest must validate as one batch")
	}
	if err = a.migrationValidateRecord(r.Context(), rec); err != nil {
		return err
	}
	expires := time.Now().Add(5 * time.Second)
	if rec.Expires.Before(expires) {
		expires = rec.Expires
	}
	p := migrationInstallProof{rec.ID, rec.ManifestDigest, u.ID, u.DeviceID, rec.TargetStore, u.RestoreEpoch, expires.Unix()}
	respond(w, 200, map[string]any{"id": rec.ID, "status": "validated", "manifestDigest": rec.ManifestDigest, "installToken": migrationProof(a, "mi1", p), "installBefore": time.Unix(p.Expires, 0).UTC(), "requiresAtomicLocalCommit": true, "targetQualified": false, "qualifiedAck": false})
	return nil
}
func (a *App) migrationInstalled(w http.ResponseWriter, r *http.Request) error {
	rec, u, err := a.migrationAuthorized(r, true)
	if err != nil {
		return err
	}
	var q struct {
		Digest string `json:"manifestDigest"`
		Token  string `json:"installToken"`
	}
	if err = decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	var p migrationInstallProof
	if err = migrationDecodeProof(a, "mi1", q.Token, &p); err != nil {
		return err
	}
	if p.ID != rec.ID || p.Digest != rec.ManifestDigest || q.Digest != p.Digest || p.User != u.ID || p.Device != u.DeviceID || p.Store != rec.TargetStore || p.Epoch != u.RestoreEpoch || p.Expires <= time.Now().Unix() {
		return fail(409, "MIGRATION_INSTALL_PROOF_EXPIRED", "Atomic publication requires a current target-bound whole-manifest validation")
	}
	if err = a.migrationValidateRecord(r.Context(), rec); err != nil {
		return err
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE sync.migrations SET state='installed_reported',updated_at=now() WHERE scope_id=$1 AND id=$2 AND state IN('ready','installed_reported')`, a.Config.DeploymentID, rec.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "MIGRATION_RETIRED", "A retired transfer cannot be reported installed")
	}
	respond(w, 200, map[string]any{"id": rec.ID, "status": "installed_reported", "manifestDigest": rec.ManifestDigest, "sourcePreserved": true, "qualifiedAck": false, "targetQualified": false, "serverVerifiedLocalDurability": false})
	return nil
}
func (a *App) cancelMigration(w http.ResponseWriter, r *http.Request) error {
	rec, _, err := a.migrationAuthorized(r, false)
	if err != nil {
		return err
	}
	var q struct {
		Digest string `json:"manifestDigest"`
	}
	if err = decodeMigrationRequest(r, &q, 16<<10); err != nil {
		return err
	}
	if q.Digest != rec.ManifestDigest {
		return fail(409, "MIGRATION_MANIFEST_CHANGED", "Cancellation must retain the original manifest")
	}
	if err = a.retireMigration(r.Context(), rec.ID, "cancelled"); err != nil {
		return err
	}
	rec, err = a.readMigration(r.Context(), rec.ID)
	if err != nil {
		return err
	}
	a.migrationResponse(w, rec)
	return nil
}
