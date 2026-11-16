package jobsim

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func syncValidationCounter(value string, zero bool) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && strconv.FormatInt(n, 10) == value && (n > 0 || zero && n == 0)
}

func syncValidationID(value string) bool {
	return validObjectID(value) && strings.ToLower(value) == value
}

func syncValidationDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}

func syncValidationRef(owner, user string, ref syncRef) bool {
	if syncOwnerSchema(owner) != owner || owner == "" || ref.Owner != owner || syncKindOwner(ref.Kind) != owner || !syncValidationID(ref.ID) || !syncValidationCounter(ref.Revision, true) || !syncValidationCounter(ref.RevisionEpoch, false) || ref.State != "active" && ref.State != "removed" {
		return false
	}
	for _, id := range []string{ref.SubjectUserID, ref.ConversationID, ref.TargetUserID} {
		if id != "" && !syncValidationID(id) {
			return false
		}
	}
	switch owner {
	case "auth":
		return ref.ID == user && ref.SubjectUserID == user && ref.ConversationID == "" && ref.TargetUserID == "" && (ref.Kind != "devices" || ref.State == "active")
	case "directory":
		if ref.ConversationID != "" {
			return false
		}
		if ref.Kind == "contact_request" {
			return ref.SubjectUserID != "" && ref.TargetUserID != "" && ref.SubjectUserID != ref.TargetUserID && (ref.SubjectUserID == user || ref.TargetUserID == user)
		}
		if ref.Kind == "profile" || ref.Kind == "preferences" {
			return ref.ID == user && ref.SubjectUserID == user && ref.TargetUserID == ""
		}
		return ref.ID == user && ref.SubjectUserID == user && ref.TargetUserID != user && ref.State == "active"
	case "conversation":
		return ref.ConversationID == ref.ID && ref.SubjectUserID == ""
	case "message":
		return ref.SubjectUserID != "" && ref.ConversationID != "" && ref.TargetUserID == ""
	case "sync":
		return ref.SubjectUserID == user && ref.ConversationID != "" && ref.TargetUserID == ""
	}
	return false
}

func (a *App) syncValidationOwnerRoute(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/sync/validate", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "sync" {
			return fail(403, "SYNC_SERVICE_REQUIRED", "Synchronization validation is private")
		}
		var q struct {
			UserID string    `json:"userId"`
			Refs   []syncRef `json:"refs"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		owner := syncOwnerSchema(a.Role)
		if !syncValidationID(q.UserID) || len(q.Refs) < 1 || len(q.Refs) > 500 {
			return fail(400, "INVALID_SYNC_VALIDATION", "Current principal and 1–500 references required")
		}
		for _, ref := range q.Refs {
			if !syncValidationRef(owner, q.UserID, ref) {
				return fail(400, "INVALID_SYNC_REFERENCE", "References must match their owner and exact identity types")
			}
		}
		tx, err := a.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			return err
		}
		defer tx.Rollback(r.Context())
		schema := pgx.Identifier{owner}.Sanitize()
		var epoch string
		if err = tx.QueryRow(r.Context(), "SELECT restore_epoch::text FROM "+schema+".instance WHERE singleton AND deployment_id=$1", a.Config.DeploymentID).Scan(&epoch); err != nil {
			return err
		}
		raw, err := json.Marshal(q.Refs)
		if err != nil {
			return err
		}
		var valid bool
		err = tx.QueryRow(r.Context(), `SELECT NOT EXISTS(
 SELECT 1 FROM jsonb_to_recordset($1::jsonb) AS expected(kind text,id uuid,revision text,"revisionEpoch" text)
 LEFT JOIN `+schema+`.sync_object_versions v ON v.kind=expected.kind AND v.object_id=expected.id
 WHERE COALESCE(v.revision,0)::text<>expected.revision OR expected."revisionEpoch"<>$2::text
)`, raw, epoch).Scan(&valid)
		if err != nil {
			return err
		}
		w.Header().Set("Cache-Control", "no-store")
		respond(w, 200, map[string]any{"valid": valid, "restoreEpoch": epoch})
		return nil
	}))
}

func (a *App) syncValidateSnapshot(w http.ResponseWriter, r *http.Request) error {
	u, err := a.syncUser(r)
	if err != nil {
		return err
	}
	var q struct {
		PageDigests []string `json:"pageDigests"`
	}
	if err = decode(r, &q); err != nil {
		return err
	}
	incomplete := func() error {
		return fail(400, "SNAPSHOT_MANIFEST_INCOMPLETE", "All original pages and digests are required; retain current and staging data")
	}
	invalidated := func() error {
		return fail(409, "SNAPSHOT_INVALIDATED", "Original references or permissions changed; retain current and staging data")
	}
	id := strings.ToLower(r.PathValue("id"))
	if !syncValidationID(id) || len(q.PageDigests) == 0 {
		return incomplete()
	}
	var raw []byte
	var expires time.Time
	err = a.DB.QueryRow(r.Context(), `SELECT manifest,expires_at FROM sync.snapshots WHERE id=$1 AND user_id=$2 AND device_id=$3 AND restore_epoch=$4`, id, u.ID, u.DeviceID, u.RestoreEpoch).Scan(&raw, &expires)
	if err == pgx.ErrNoRows {
		return incomplete()
	}
	if err != nil {
		return err
	}
	if !expires.After(time.Now()) {
		return fail(410, "SNAPSHOT_EXPIRED", "Original snapshot expired; retain current and staging data")
	}
	var manifest syncSnapshot
	if json.Unmarshal(raw, &manifest) != nil {
		return incomplete()
	}
	reg, err := a.syncRegistered(r.Context(), u, manifest.SchemaVersion)
	if err != nil {
		return err
	}
	if manifest.ID != id || manifest.ScopeID != a.Config.DeploymentID || manifest.PrincipalID != u.ID || manifest.SourceContextID != "personal" || manifest.DeviceID != u.DeviceID || manifest.RestoreEpoch != u.RestoreEpoch || manifest.StoreGeneration != reg.Store || manifest.StreamID != reg.Stream || manifest.SchemaVersion != reg.Schema {
		return invalidated()
	}
	if len(manifest.PageManifest) != len(q.PageDigests) || !manifest.ExpiresAt.After(time.Now()) || manifest.BoundaryToken == "" {
		return incomplete()
	}
	for index, page := range manifest.PageManifest {
		if page.Page != index || page.Count < 0 || page.Count > 100 || !syncValidationDigest(page.Digest) || q.PageDigests[index] != page.Digest {
			return incomplete()
		}
	}
	view, err := a.syncCurrentView(r.Context(), u, manifest.SchemaVersion)
	if err != nil {
		return err
	}
	if view.Permission != manifest.PermissionGeneration {
		return invalidated()
	}
	rows, err := a.DB.Query(r.Context(), `SELECT page,refs_ciphertext,refs_digest FROM sync.snapshot_pages WHERE snapshot_id=$1 ORDER BY page`, id)
	if err != nil {
		return err
	}
	buckets := make(map[string][]syncRef, len(syncOwners))
	pageIndex, total := 0, 0
	for rows.Next() {
		var page int
		var cipher []byte
		var digest string
		if err = rows.Scan(&page, &cipher, &digest); err != nil {
			rows.Close()
			return err
		}
		if pageIndex >= len(manifest.PageManifest) || page != pageIndex || digest != manifest.PageManifest[pageIndex].Digest {
			rows.Close()
			return incomplete()
		}
		plain, unsealErr := a.unseal(cipher, "sync-refs:"+manifest.ID+":"+strconv.Itoa(page))
		if unsealErr != nil || syncDigest([]byte(plain)) != digest {
			rows.Close()
			return incomplete()
		}
		var refs []syncRef
		decoder := json.NewDecoder(bytes.NewReader([]byte(plain)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&refs) != nil || decoder.Decode(new(any)) != io.EOF || len(refs) != manifest.PageManifest[pageIndex].Count {
			rows.Close()
			return incomplete()
		}
		for _, ref := range refs {
			if !syncSchemaIncludes(manifest.SchemaVersion, ref.Kind) || !syncValidationRef(ref.Owner, u.ID, ref) {
				rows.Close()
				return incomplete()
			}
			buckets[ref.Owner] = append(buckets[ref.Owner], ref)
		}
		total += len(refs)
		if total > 200000 {
			rows.Close()
			return incomplete()
		}
		pageIndex++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if pageIndex != len(manifest.PageManifest) {
		return incomplete()
	}
	// Two collections prove a common valid cut for the original references.
	// Unrelated appends may advance owner clocks without invalidating these refs.
	for pass := 0; pass < 2; pass++ {
		for _, owner := range syncSchemaOwners(manifest.SchemaVersion) {
			refs := buckets[owner]
			for offset := 0; offset < len(refs); offset += 500 {
				end := offset + 500
				if end > len(refs) {
					end = len(refs)
				}
				var result struct {
					Valid        bool   `json:"valid"`
					RestoreEpoch string `json:"restoreEpoch"`
				}
				if err = a.rpc(r.Context(), syncOwnerRole(owner), "/internal/sync/validate", struct {
					UserID string    `json:"userId"`
					Refs   []syncRef `json:"refs"`
				}{u.ID, refs[offset:end]}, &result); err != nil {
					return err
				}
				if !result.Valid || result.RestoreEpoch != u.RestoreEpoch {
					return invalidated()
				}
			}
		}
	}
	latest, err := a.syncCurrentView(r.Context(), u, manifest.SchemaVersion)
	if err != nil {
		return err
	}
	if latest.Permission != manifest.PermissionGeneration {
		return invalidated()
	}
	if !expires.After(time.Now()) || !manifest.ExpiresAt.After(time.Now()) {
		return fail(410, "SNAPSHOT_EXPIRED", "Original snapshot expired during validation; retain current and staging data")
	}
	w.Header().Set("Cache-Control", "no-store")
	// Keep the classification watermark sampled before reference checks: a later
	// append is delta work, not proof that this request checked its classification.
	respond(w, 200, map[string]any{"canInstall": true, "boundaryToken": manifest.BoundaryToken, "restoreEpoch": u.RestoreEpoch, "permissionGeneration": view.Permission, "classificationWatermark": view.Watermarks["message"], "requiresLocalAtomicInstall": true, "qualifiedAck": false})
	return nil
}
