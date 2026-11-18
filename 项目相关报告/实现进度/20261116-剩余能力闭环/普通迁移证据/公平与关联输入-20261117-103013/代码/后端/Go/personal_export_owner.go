package jobsim

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type personalExportHolder struct {
	JobID           string    `json:"jobId"`
	ArtifactID      string    `json:"artifactId"`
	UserID          string    `json:"userId"`
	RestoreEpoch    string    `json:"restoreEpoch"`
	BuildGeneration string    `json:"buildGeneration"`
	ExpiresAt       time.Time `json:"expiresAt"`
	Ref             syncRef   `json:"ref"`
	ObjectID        string    `json:"objectId,omitempty"`
}

func personalExportOwner(role string) string {
	switch role {
	case "identity":
		return "auth"
	case "directory", "conversation", "sync", "governance":
		return role
	}
	return ""
}

func personalExportOwners() []string {
	return []string{"auth", "directory", "conversation", "sync", "governance"}
}

func personalExportRefOwner(kind string) string {
	if kind == "report" {
		return "governance"
	}
	return syncKindOwner(kind)
}

func personalExportVersionTable(owner string) string {
	if owner == "governance" {
		return "governance.personal_export_report_versions"
	}
	return owner + ".sync_object_versions"
}

func migratePersonalExportOwner(ctx context.Context, tx pgx.Tx, role string) error {
	owner := personalExportOwner(role)
	if owner == "" {
		return nil
	}
	if owner == "governance" {
		if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS governance.personal_export_report_versions(kind text NOT NULL DEFAULT 'report',object_id uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),PRIMARY KEY(kind,object_id));
INSERT INTO governance.personal_export_report_versions(object_id,revision) SELECT id,row_version FROM governance.reports ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION governance.personal_export_report_changed() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $personal_report$
DECLARE report_id uuid;
BEGIN
IF TG_OP='UPDATE' AND (OLD.id,OLD.reporter_id,OLD.target_type,OLD.target_id,OLD.reason,OLD.state,OLD.row_version,OLD.updated_at) IS NOT DISTINCT FROM (NEW.id,NEW.reporter_id,NEW.target_type,NEW.target_id,NEW.reason,NEW.state,NEW.row_version,NEW.updated_at) THEN RETURN NEW; END IF;
IF TG_OP='DELETE' THEN report_id:=OLD.id; ELSE report_id:=NEW.id; END IF;
INSERT INTO governance.personal_export_report_versions(object_id,revision) VALUES(report_id,1) ON CONFLICT(kind,object_id) DO UPDATE SET revision=governance.personal_export_report_versions.revision+1;
IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $personal_report$;
DROP TRIGGER IF EXISTS personal_export_report_changed ON governance.reports;
CREATE TRIGGER personal_export_report_changed AFTER INSERT OR UPDATE OR DELETE ON governance.reports FOR EACH ROW EXECUTE FUNCTION governance.personal_export_report_changed();`); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %[1]s.personal_export_holders(
job_id uuid NOT NULL,artifact_id uuid NOT NULL,user_id uuid NOT NULL,restore_epoch bigint NOT NULL,build_generation bigint NOT NULL,
kind text NOT NULL,object_id uuid NOT NULL,ref jsonb NOT NULL,source_revision bigint NOT NULL,revision_epoch bigint NOT NULL,
state text NOT NULL CHECK(state IN('prepared','active','revoking','cleared')),journal_seq bigint,expires_at timestamptz NOT NULL,
PRIMARY KEY(job_id,kind,object_id));
CREATE INDEX IF NOT EXISTS personal_export_active_subject ON %[1]s.personal_export_holders(user_id) WHERE state IN('prepared','active','revoking');
CREATE TABLE IF NOT EXISTS %[1]s.personal_export_cleanups(job_id uuid PRIMARY KEY,artifact_id uuid NOT NULL,user_id uuid NOT NULL,state text NOT NULL CHECK(state IN('pending','cleared')),updated_at timestamptz NOT NULL DEFAULT now());
INSERT INTO %[1]s.personal_export_cleanups(job_id,artifact_id,user_id,state) SELECT DISTINCT job_id,artifact_id,user_id,'pending' FROM %[1]s.personal_export_holders WHERE state IN('revoking','cleared') ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION %[1]s.personal_export_ref_changed() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $personal_export$
BEGIN
WITH revoked AS(UPDATE %[1]s.personal_export_holders SET state='revoking' WHERE kind=NEW.kind AND object_id=NEW.object_id AND state IN('prepared','active') AND source_revision<>NEW.revision AND NOT('%[1]s'='auth' AND NEW.kind='devices') RETURNING job_id,artifact_id,user_id)
INSERT INTO %[1]s.personal_export_cleanups(job_id,artifact_id,user_id,state) SELECT DISTINCT job_id,artifact_id,user_id,'pending' FROM revoked ON CONFLICT(job_id) DO UPDATE SET state='pending',updated_at=now();
RETURN NEW; END $personal_export$;
DROP TRIGGER IF EXISTS personal_export_ref_changed ON %[2]s;
CREATE TRIGGER personal_export_ref_changed AFTER INSERT OR UPDATE ON %[2]s FOR EACH ROW EXECUTE FUNCTION %[1]s.personal_export_ref_changed();`, owner, personalExportVersionTable(owner)))
	if err != nil {
		return err
	}
	if owner == "auth" {
		_, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS auth.personal_export_device_versions(user_id uuid PRIMARY KEY,revision bigint NOT NULL DEFAULT 1 CHECK(revision>0));
CREATE OR REPLACE FUNCTION auth.personal_export_device_changed() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $personal_export$
DECLARE u uuid; v bigint;
BEGIN
IF TG_OP='UPDATE' AND (OLD.device_id,OLD.device_name,OLD.created_at,OLD.revoked_at) IS NOT DISTINCT FROM (NEW.device_id,NEW.device_name,NEW.created_at,NEW.revoked_at) THEN RETURN NEW; END IF;
IF TG_OP='DELETE' THEN u:=OLD.user_id; ELSE u:=NEW.user_id; END IF;
INSERT INTO auth.personal_export_device_versions(user_id) VALUES(u) ON CONFLICT(user_id) DO UPDATE SET revision=auth.personal_export_device_versions.revision+1 RETURNING revision INTO v;
WITH revoked AS(UPDATE auth.personal_export_holders SET state='revoking' WHERE user_id=u AND kind='devices' AND state IN('prepared','active') AND source_revision<>v RETURNING job_id,artifact_id,user_id)
INSERT INTO auth.personal_export_cleanups(job_id,artifact_id,user_id,state) SELECT DISTINCT job_id,artifact_id,user_id,'pending' FROM revoked ON CONFLICT(job_id) DO UPDATE SET state='pending',updated_at=now();
IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $personal_export$;
DROP TRIGGER IF EXISTS personal_export_device_changed ON auth.sessions;
CREATE TRIGGER personal_export_device_changed AFTER INSERT OR UPDATE OR DELETE ON auth.sessions FOR EACH ROW EXECUTE FUNCTION auth.personal_export_device_changed();`)
	}
	return err
}

func (a *App) personalExportOwnerRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	owner := personalExportOwner(a.Role)
	if owner == "" {
		return
	}
	for _, path := range []string{"capture", "register", "read", "check", "check-avatar", "clear-holders"} {
		path := path
		h("POST /internal/personal-exports/"+path, internal(func(w http.ResponseWriter, r *http.Request) error {
			caller := r.Header.Get("X-JobsIM-Caller")
			if caller != "governance" && !(owner == "directory" && path == "check-avatar" && caller == "media") {
				return fail(403, "EXPORT_SERVICE_REQUIRED", "Personal export owner protocol is private")
			}
			var q personalExportHolder
			if err := decode(r, &q); err != nil {
				return err
			}
			if !validAssetID(q.UserID) || !validAssetID(q.JobID) || !validAssetID(q.ArtifactID) || !syncValidationCounter(q.RestoreEpoch, false) || !syncValidationCounter(q.BuildGeneration, false) {
				return fail(400, "INVALID_EXPORT_HOLDER", "Bound personal export holder required")
			}
			if path == "clear-holders" {
				if err := a.rpc(r.Context(), "governance", "/internal/exports/cleared", map[string]string{"jobId": q.JobID, "artifactId": q.ArtifactID}, nil); err != nil {
					return err
				}
				_, err := a.DB.Exec(r.Context(), "UPDATE "+owner+".personal_export_holders SET state='cleared' WHERE job_id=$1 AND artifact_id=$2", q.JobID, q.ArtifactID)
				if err == nil {
					respond(w, 200, map[string]bool{"cleared": true})
				}
				return err
			}
			if err := a.rpc(r.Context(), "governance", "/internal/personal-exports/build", q, nil); err != nil {
				return err
			}
			if path == "capture" {
				refs, err := a.capturePersonalExportRefs(r.Context(), q.UserID)
				if err == nil {
					respond(w, 200, map[string]any{"refs": refs})
				}
				return err
			}
			if !validObjectID(q.Ref.ID) || q.Ref.Owner != owner || personalExportRefOwner(q.Ref.Kind) != owner || !syncValidationCounter(q.Ref.Revision, true) || q.Ref.RevisionEpoch != q.RestoreEpoch {
				return fail(400, "INVALID_EXPORT_REFERENCE", "Original owner reference required")
			}
			if path == "register" {
				return a.registerPersonalExportRef(w, r, q)
			}
			if err := a.checkPersonalExportRef(r.Context(), q); err != nil {
				return err
			}
			if path == "check-avatar" {
				if owner != "directory" || q.Ref.Kind != "profile" || q.Ref.ID != q.UserID || !validAssetID(q.ObjectID) {
					return fail(403, "EXPORT_AVATAR_CHANGED", "Original own profile avatar required")
				}
				profile, err := a.avatarProfileFields(r.Context(), q.UserID)
				if err != nil {
					return err
				}
				if profile.AvatarURL != "/api/avatars/"+q.ObjectID && profile.AvatarThumbnailURL != "/api/avatars/"+q.ObjectID {
					return fail(403, "EXPORT_AVATAR_CHANGED", "Avatar is no longer owned by the original profile version")
				}
				respond(w, 200, map[string]bool{"valid": true})
				return nil
			}
			if path == "check" {
				respond(w, 200, map[string]bool{"valid": true})
				return nil
			}
			value, err := a.readPersonalExportObject(r, q.UserID, q.Ref)
			if err != nil {
				return err
			}
			if err = a.checkPersonalExportRef(r.Context(), q); err != nil {
				return err
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if len(raw) > 512<<10 {
				return fail(413, "EXPORT_METADATA_CAPACITY", "Personal metadata object exceeds 512 KiB; no partial package published")
			}
			respond(w, 200, map[string]any{"ref": q.Ref, "bodyEnvelope": base64.RawURLEncoding.EncodeToString(raw), "bodyDigest": syncDigest(raw)})
			return nil
		}))
	}
}

func (a *App) capturePersonalExportRefs(ctx context.Context, user string) ([]syncRef, error) {
	owner := personalExportOwner(a.Role)
	var base string
	var args []any
	var err error
	if owner == "governance" {
		base = `SELECT 'report'::text kind,id,reporter_id subject,NULL::uuid conversation,NULL::uuid target,'active'::text state FROM governance.reports WHERE reporter_id=$1`
		args = []any{user}
	} else {
		base, args, err = a.syncCaptureSQL(ctx, owner, user)
	}
	if err != nil {
		return nil, err
	}
	if owner == "conversation" {
		// Announcement objects belong only to groups. A normal self/direct
		// conversation must not turn a complete personal export into a failure.
		base = `SELECT k.kind,c.id,NULL::uuid subject,c.id conversation,NULL::uuid target,'active'::text state FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id CROSS JOIN(VALUES('conversation'::text),('announcement'::text))k(kind) WHERE m.user_id=$1 AND m.state='active' AND m.export_pending_operation_id IS NULL AND c.state IN('active','owner_unavailable') AND(k.kind<>'announcement' OR c.kind='group')`
	}
	if owner == "directory" {
		base += ` UNION SELECT 'profile',$1::uuid,$1::uuid,NULL::uuid,NULL::uuid,'active' UNION SELECT 'preferences',$1::uuid,$1::uuid,NULL::uuid,NULL::uuid,'active'`
	}
	query := fmt.Sprintf(`WITH objects AS (%s) SELECT o.kind,o.id::text,COALESCE(v.revision,0)::text,i.restore_epoch::text,COALESCE(o.subject::text,''),COALESCE(o.conversation::text,''),COALESCE(o.target::text,''),o.state FROM objects o CROSS JOIN %s.instance i LEFT JOIN %s v ON v.kind=o.kind AND v.object_id=o.id ORDER BY o.kind,o.id LIMIT 20001`, base, owner, personalExportVersionTable(owner))
	rows, err := a.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	refs := []syncRef{}
	for rows.Next() {
		ref := syncRef{Owner: owner}
		if err = rows.Scan(&ref.Kind, &ref.ID, &ref.Revision, &ref.RevisionEpoch, &ref.SubjectUserID, &ref.ConversationID, &ref.TargetUserID, &ref.State); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(refs) > 20000 {
		return nil, fail(413, "EXPORT_METADATA_CAPACITY", "Export metadata scope exceeds 20000 references; no partial package published")
	}
	for i := range refs {
		if refs[i].Kind == "devices" {
			refs[i].Revision, err = a.personalExportRevision(ctx, refs[i])
			if err != nil {
				return nil, err
			}
		}
	}
	return refs, nil
}

func (a *App) personalExportRevision(ctx context.Context, ref syncRef) (string, error) {
	if ref.Owner == "governance" && ref.Kind == "report" {
		var revision, epoch string
		err := a.DB.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM governance.personal_export_report_versions WHERE kind='report' AND object_id=$1),0)::text,restore_epoch::text FROM governance.instance`, ref.ID).Scan(&revision, &epoch)
		if err == nil && epoch != ref.RevisionEpoch {
			return "", fail(409, "RESTORE_EPOCH_CHANGED", "Export source epoch changed")
		}
		return revision, err
	}
	if ref.Owner == "auth" && ref.Kind == "devices" {
		var revision string
		err := a.DB.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM auth.personal_export_device_versions WHERE user_id=$1),0)::text`, ref.ID).Scan(&revision)
		return revision, err
	}
	revision, epoch, err := a.syncObjectRevision(ctx, ref)
	if err == nil && epoch != ref.RevisionEpoch {
		return "", fail(409, "RESTORE_EPOCH_CHANGED", "Export source epoch changed")
	}
	return revision, err
}

func (a *App) registerPersonalExportRef(w http.ResponseWriter, r *http.Request, q personalExportHolder) error {
	owner := personalExportOwner(a.Role)
	current, err := a.personalExportRevision(r.Context(), q.Ref)
	if err != nil {
		return err
	}
	if current != q.Ref.Revision {
		return fail(409, "EXPORT_SOURCE_CHANGED", "Personal export source version changed")
	}
	raw, err := json.Marshal(q.Ref)
	if err != nil {
		return err
	}
	_, err = a.DB.Exec(r.Context(), "INSERT INTO "+owner+`.personal_export_holders(job_id,artifact_id,user_id,restore_epoch,build_generation,kind,object_id,ref,source_revision,revision_epoch,state,expires_at) VALUES($1,$2,$3,$4::bigint,$5::bigint,$6,$7,$8,$9::bigint,$10::bigint,'prepared',$11) ON CONFLICT DO NOTHING`, q.JobID, q.ArtifactID, q.UserID, q.RestoreEpoch, q.BuildGeneration, q.Ref.Kind, q.Ref.ID, raw, q.Ref.Revision, q.Ref.RevisionEpoch, q.ExpiresAt)
	if err != nil {
		return err
	}
	if err = a.checkPersonalExportRefState(r.Context(), q, false); err != nil {
		return err
	}
	payload := map[string]any{"jobId": q.JobID, "artifactId": q.ArtifactID, "userId": q.UserID, "restoreEpoch": q.RestoreEpoch, "buildGeneration": q.BuildGeneration, "owner": owner, "kind": q.Ref.Kind, "objectId": q.Ref.ID, "sourceRevision": q.Ref.Revision, "revisionEpoch": q.Ref.RevisionEpoch, "expiresAt": q.ExpiresAt.UTC().Format(time.RFC3339Nano)}
	operation := "personal-export-holder:" + q.JobID + ":" + q.Ref.Kind + ":" + q.Ref.ID
	var ack struct {
		JournalSeq string `json:"journalSeq"`
	}
	kind := owner + ".personal_export_holder"
	if err = a.rpc(r.Context(), "journal", "/internal/append", journalRecord{a.Role, operation, "intent", kind, payload}, &ack); err != nil {
		return err
	}
	if !syncValidationCounter(ack.JournalSeq, false) {
		return fail(503, "EXPORT_JOURNAL_REQUIRED", "Durable personal holder journal result required")
	}
	_, err = a.DB.Exec(r.Context(), "UPDATE "+owner+`.personal_export_holders SET state='active',journal_seq=$4::bigint WHERE job_id=$1 AND kind=$2 AND object_id=$3 AND state IN('prepared','active')`, q.JobID, q.Ref.Kind, q.Ref.ID, ack.JournalSeq)
	if err != nil {
		return err
	}
	if err = a.checkPersonalExportRef(r.Context(), q); err != nil {
		return err
	}
	if err = a.protect(r.Context(), operation, "result", kind, payload); err != nil {
		return err
	}
	respond(w, 200, map[string]string{"registrationId": operation, "journalSeq": ack.JournalSeq})
	return nil
}

func (a *App) checkPersonalExportRefState(ctx context.Context, q personalExportHolder, active bool) error {
	owner := personalExportOwner(a.Role)
	var valid bool
	raw, _ := json.Marshal(q.Ref)
	err := a.DB.QueryRow(ctx, "SELECT artifact_id=$4 AND user_id=$5 AND restore_epoch=$6::bigint AND build_generation=$7::bigint AND ref=$8::jsonb AND source_revision=$9::bigint AND revision_epoch=$10::bigint AND state IN('prepared','active') AND (NOT $11 OR(state='active' AND journal_seq IS NOT NULL)) AND expires_at>now() FROM "+owner+".personal_export_holders WHERE job_id=$1 AND kind=$2 AND object_id=$3", q.JobID, q.Ref.Kind, q.Ref.ID, q.ArtifactID, q.UserID, q.RestoreEpoch, q.BuildGeneration, raw, q.Ref.Revision, q.Ref.RevisionEpoch, active).Scan(&valid)
	if err != nil || !valid {
		return fail(409, "EXPORT_SOURCE_CHANGED", "Personal source holder is unavailable")
	}
	return nil
}

func (a *App) checkPersonalExportRef(ctx context.Context, q personalExportHolder) error {
	if err := a.checkPersonalExportRefState(ctx, q, true); err != nil {
		return err
	}
	revision, err := a.personalExportRevision(ctx, q.Ref)
	if err != nil {
		return err
	}
	if revision != q.Ref.Revision {
		return fail(409, "EXPORT_SOURCE_CHANGED", "Personal source version no longer matches")
	}
	if q.Ref.Owner == "governance" {
		var owned bool
		if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM governance.reports WHERE id=$1 AND reporter_id=$2)`, q.Ref.ID, q.UserID).Scan(&owned); err != nil {
			return err
		}
		if !owned {
			return fail(409, "EXPORT_SOURCE_CHANGED", "Original personal report no longer available")
		}
	}
	if q.Ref.Owner == "conversation" && q.Ref.ConversationID != "" {
		if _, err = a.member(ctx, q.UserID, q.Ref.ConversationID); err != nil {
			return err
		}
	}
	return nil
}

// Only rows persistently marked by the source transaction are drained. A newer
// task captured after that transaction must not be swept with an older task.
func (a *App) freezePersonalMetadataExports(ctx context.Context, users []string, operation string) error {
	owner := personalExportOwner(a.Role)
	if owner == "" || len(users) == 0 {
		return nil
	}
	rows, err := a.DB.Query(ctx, "SELECT job_id::text FROM "+owner+`.personal_export_cleanups WHERE user_id=ANY($1::uuid[]) AND state='pending'`, users)
	if err != nil {
		return err
	}
	jobs := []string{}
	seen := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !seen[id] {
			seen[id] = true
			jobs = append(jobs, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	if len(operation) > 150 {
		operation = stringBase64(keyedDigest(a.Config.Key, "personal-export-invalidation", []byte(operation)))
	}
	var ack struct {
		State string `json:"state"`
	}
	if err = a.rpc(ctx, "governance", "/internal/exports/invalidate", map[string]any{"jobIds": jobs, "operationId": "personal-source:" + owner + ":" + operation}, &ack); err != nil {
		return err
	}
	if ack.State != "cleared" {
		return fail(503, "EXPORT_CLEANUP_PENDING", "Governance has not acknowledged the complete original handoff")
	}
	_, err = a.DB.Exec(ctx, "UPDATE "+owner+".personal_export_cleanups SET state='cleared',updated_at=now() WHERE job_id=ANY($1::uuid[]) AND state='pending'", jobs)
	return err
}

// Persisted trigger fallback also fences accidental/unobserved SQL changes.
// It supplements, rather than replaces, the source action's synchronous hook.
func (a *App) resumePersonalExportInvalidations(ctx context.Context) error {
	owner := personalExportOwner(a.Role)
	if owner == "" {
		return nil
	}
	rows, err := a.DB.Query(ctx, "SELECT DISTINCT user_id::text FROM "+owner+".personal_export_cleanups WHERE state='pending' LIMIT 100")
	if err != nil {
		return err
	}
	users := []string{}
	for rows.Next() {
		var user string
		if err = rows.Scan(&user); err != nil {
			rows.Close()
			return err
		}
		users = append(users, user)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	return a.freezePersonalMetadataExports(ctx, users, "resume")
}

func personalExportRole(owner string) string {
	if owner == "auth" {
		return "identity"
	}
	return owner
}

func (a *App) readPersonalExportObject(r *http.Request, user string, ref syncRef) (any, error) {
	if ref.Owner == "governance" && ref.Kind == "report" {
		// Public reporter fields only: report_operations.reason belongs to the
		// administrator and must never enter a reporter's personal package.
		return scanReport(a.DB.QueryRow(r.Context(), `SELECT `+reportColumns+` FROM governance.reports WHERE id=$1 AND reporter_id=$2`, ref.ID, user))
	}
	if ref.Owner == "auth" && ref.Kind == "devices" {
		rows, err := a.DB.Query(r.Context(), `SELECT device_id::text,device_name,created_at,revoked_at IS NOT NULL FROM auth.sessions WHERE user_id=$1 ORDER BY device_id`, user)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, name string
			var created time.Time
			var revoked bool
			if err = rows.Scan(&id, &name, &created, &revoked); err != nil {
				return nil, err
			}
			items = append(items, map[string]any{"deviceId": id, "name": name, "createdAt": created, "revoked": revoked})
		}
		return map[string]any{"items": items}, rows.Err()
	}
	v, err := a.syncHydrateOnce(r, user, ref)
	if err != nil {
		return nil, err
	}
	if v.State != "available" {
		return nil, fail(409, "EXPORT_SOURCE_UNAVAILABLE", "Personal source is not currently available")
	}
	switch value := v.Object.(type) {
	case User:
		return map[string]any{"id": value.ID, "username": value.Username, "displayName": value.DisplayName, "state": value.State}, nil
	case Conversation:
		value.SendAdmission = ""
		rows, err := a.DB.Query(r.Context(), `SELECT user_id::text,role,membership_generation::text,role_generation::text,joined_at FROM conversation.members WHERE conversation_id=$1 AND state='active' ORDER BY user_id`, ref.ID)
		if err != nil {
			return nil, err
		}
		members := []map[string]any{}
		for rows.Next() {
			var id, role, generation, roleGeneration string
			var joined time.Time
			if err = rows.Scan(&id, &role, &generation, &roleGeneration, &joined); err != nil {
				rows.Close()
				return nil, err
			}
			members = append(members, map[string]any{"userId": id, "role": role, "membershipGeneration": generation, "roleGeneration": roleGeneration, "joinedAt": joined})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		return map[string]any{"conversation": value, "members": members}, nil
	case announcement:
		value.Message = nil
		return value, nil
	case selfDraftView:
		value.DraftAdmission = ""
		value.Recovery = nil
		value.PendingOperationID = ""
		return value, nil
	}
	// Recursive removal is a final defensive boundary, not a broad DB dump.
	raw, err := json.Marshal(v.Object)
	if err != nil {
		return nil, err
	}
	var clean any
	if err = json.Unmarshal(raw, &clean); err != nil {
		return nil, err
	}
	stripPersonalExportAuthority(clean)
	return clean, nil
}

func stripPersonalExportAuthority(value any) {
	switch object := value.(type) {
	case map[string]any:
		for key, child := range object {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "admission") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "ciphertext") || lower == "recovery" || lower == "pendingoperationid" {
				delete(object, key)
				continue
			}
			stripPersonalExportAuthority(child)
		}
	case []any:
		for _, child := range object {
			stripPersonalExportAuthority(child)
		}
	}
}
