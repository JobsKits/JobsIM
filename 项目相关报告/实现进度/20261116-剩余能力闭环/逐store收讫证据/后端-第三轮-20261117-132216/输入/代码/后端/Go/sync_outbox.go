package jobsim

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// migrateSyncOwner installs only the owner's change index. Snapshot baselines
// and delivery publication belong to their callers; no historical events are invented.
func migrateSyncOwner(ctx context.Context, tx pgx.Tx, owner, scope string) error {
	switch owner {
	case "auth", "directory", "conversation", "message", "sync":
	default:
		return fmt.Errorf("unsupported sync owner")
	}
	if !validObjectID(scope) {
		return fmt.Errorf("invalid sync deployment scope")
	}
	schema := pgx.Identifier{owner}.Sanitize()
	var deployment string
	var epoch int64
	if err := tx.QueryRow(ctx, "SELECT deployment_id,restore_epoch FROM "+schema+".instance WHERE singleton").Scan(&deployment, &epoch); err != nil {
		return fmt.Errorf("sync owner instance unavailable")
	}
	if deployment != scope || epoch < 1 {
		return fmt.Errorf("sync owner instance does not match deployment")
	}
	kinds := syncOriginalKinds
	if owner == "sync" {
		kinds = "'read_state','read_observation','conversation_view','draft'"
	}
	ddl := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %[1]s.sync_clock(
 singleton boolean PRIMARY KEY CHECK(singleton),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0)
);
INSERT INTO %[1]s.sync_clock(singleton,revision) VALUES(true,0) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS %[1]s.sync_object_versions(
 kind text NOT NULL CHECK(kind IN(%[3]s)),
 object_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 revision_epoch bigint NOT NULL CHECK(revision_epoch>0),
 subject_user_id uuid,
 conversation_id uuid,
 target_user_id uuid,
 state text NOT NULL CHECK(state IN('active','removed')),
 PRIMARY KEY(kind,object_id)
);
CREATE INDEX IF NOT EXISTS sync_object_subject ON %[1]s.sync_object_versions(subject_user_id,kind,revision);
CREATE INDEX IF NOT EXISTS sync_object_conversation ON %[1]s.sync_object_versions(conversation_id,revision);
CREATE TABLE IF NOT EXISTS %[1]s.sync_outbox(
 revision bigint PRIMARY KEY CHECK(revision>0),
 origin_event_id uuid NOT NULL UNIQUE,
 scope_id uuid NOT NULL CHECK(scope_id='%[2]s'::uuid),
 kind text NOT NULL CHECK(kind IN(%[3]s)),
 object_id uuid NOT NULL,
 subject_user_id uuid,
 conversation_id uuid,
 target_user_id uuid,
 restore_epoch bigint NOT NULL CHECK(restore_epoch>0),
 revision_epoch bigint NOT NULL CHECK(revision_epoch>0),
 state text NOT NULL CHECK(state IN('active','removed')),
 metadata jsonb NOT NULL CHECK(
  jsonb_typeof(metadata)='object'
  AND (metadata-ARRAY['action','sourceVersion','createdAt']::text[])='{}'::jsonb
  AND metadata ? 'action'
  AND jsonb_typeof(metadata->'action')='string'
  AND metadata->>'action' IN('insert','update','delete')
  AND (NOT(metadata ? 'sourceVersion') OR (jsonb_typeof(metadata->'sourceVersion')='string' AND metadata->>'sourceVersion' ~ '^[1-9][0-9]{0,18}$'))
  AND (NOT(metadata ? 'createdAt') OR (jsonb_typeof(metadata->'createdAt')='string' AND metadata->>'createdAt' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[.][0-9]{6}Z$'))
 ),
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS sync_outbox_epoch_revision ON %[1]s.sync_outbox(restore_epoch,revision);
CREATE TABLE IF NOT EXISTS %[1]s.sync_permission_versions(
 user_id uuid PRIMARY KEY,
 version bigint NOT NULL CHECK(version>0)
);
CREATE OR REPLACE FUNCTION %[1]s.sync_emit(
 p_kind text,p_object uuid,p_subject uuid,p_conversation uuid,p_target uuid,p_state text,p_metadata jsonb
) RETURNS bigint LANGUAGE plpgsql SET search_path=pg_catalog AS $jobsim_sync$
DECLARE v_revision bigint; v_epoch bigint;
BEGIN
 SELECT restore_epoch INTO STRICT v_epoch FROM %[1]s.instance WHERE singleton AND deployment_id='%[2]s';
 -- The row lock survives to commit, so a later visible revision cannot overtake an uncommitted one.
 UPDATE %[1]s.sync_clock SET revision=revision+1 WHERE singleton RETURNING revision INTO STRICT v_revision;
 INSERT INTO %[1]s.sync_object_versions(kind,object_id,revision,revision_epoch,subject_user_id,conversation_id,target_user_id,state)
 VALUES(p_kind,p_object,v_revision,v_epoch,p_subject,p_conversation,p_target,p_state)
 ON CONFLICT(kind,object_id) DO UPDATE SET revision=EXCLUDED.revision,revision_epoch=EXCLUDED.revision_epoch,
 subject_user_id=EXCLUDED.subject_user_id,conversation_id=EXCLUDED.conversation_id,target_user_id=EXCLUDED.target_user_id,state=EXCLUDED.state;
 INSERT INTO %[1]s.sync_outbox(revision,origin_event_id,scope_id,kind,object_id,subject_user_id,conversation_id,target_user_id,restore_epoch,revision_epoch,state,metadata)
 VALUES(v_revision,gen_random_uuid(),'%[2]s'::uuid,p_kind,p_object,p_subject,p_conversation,p_target,v_epoch,v_epoch,p_state,p_metadata);
 RETURN v_revision;
END $jobsim_sync$;
CREATE OR REPLACE FUNCTION %[1]s.sync_bump_permissions(p_users uuid[])
RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog AS $jobsim_sync$
DECLARE v_user uuid;
BEGIN
 FOR v_user IN SELECT DISTINCT u FROM unnest(p_users) AS users(u) WHERE u IS NOT NULL ORDER BY u LOOP
  INSERT INTO %[1]s.sync_permission_versions AS permissions(user_id,version) VALUES(v_user,1)
  ON CONFLICT(user_id) DO UPDATE SET version=permissions.version+1;
 END LOOP;
END $jobsim_sync$;
`, schema, scope, kinds)
	if _, err := tx.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("install sync owner change index: %w", err)
	}
	if err := migrateSyncPreferenceKind(ctx, tx, owner); err != nil {
		return err
	}
	var capture string
	switch owner {
	case "auth":
		capture = `
CREATE OR REPLACE FUNCTION auth.sync_capture() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $jobsim_sync$
DECLARE v_user uuid; v_state text := 'active';
BEGIN
 IF TG_TABLE_NAME='users' THEN
  IF TG_OP='DELETE' THEN v_user:=OLD.id; v_state:='removed'; ELSE v_user:=NEW.id; END IF;
  PERFORM auth.sync_emit('account',v_user,v_user,NULL,NULL,v_state,jsonb_build_object('action',lower(TG_OP)));
  PERFORM auth.sync_bump_permissions(ARRAY[v_user]);
 ELSE
  IF TG_OP='DELETE' THEN v_user:=OLD.user_id; ELSE v_user:=NEW.user_id; END IF;
  PERFORM auth.sync_emit('devices',v_user,v_user,NULL,NULL,'active',jsonb_build_object('action',lower(TG_OP)));
 END IF;
 RETURN NULL;
END $jobsim_sync$;
DROP TRIGGER IF EXISTS sync_capture ON auth.users;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON auth.users FOR EACH ROW EXECUTE FUNCTION auth.sync_capture();
DROP TRIGGER IF EXISTS sync_capture ON auth.sessions;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON auth.sessions FOR EACH ROW EXECUTE FUNCTION auth.sync_capture();`
	case "directory":
		capture = `
CREATE OR REPLACE FUNCTION directory.sync_capture() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $jobsim_sync$
DECLARE v_user uuid; v_peer uuid; v_id uuid; v_state text := 'active';
BEGIN
 IF TG_TABLE_NAME='profiles' THEN
  IF TG_OP='DELETE' THEN v_user:=OLD.user_id; v_state:='removed'; ELSE v_user:=NEW.user_id; END IF;
  PERFORM directory.sync_emit('profile',v_user,v_user,NULL,NULL,v_state,jsonb_build_object('action',lower(TG_OP)));
 ELSIF TG_TABLE_NAME='contact_requests' THEN
  IF TG_OP='DELETE' THEN v_id:=OLD.id; v_user:=OLD.requester_id; v_peer:=OLD.recipient_id; v_state:='removed';
  ELSE v_id:=NEW.id; v_user:=NEW.requester_id; v_peer:=NEW.recipient_id; END IF;
  PERFORM directory.sync_emit('contact_request',v_id,v_user,NULL,v_peer,v_state,jsonb_build_object('action',lower(TG_OP)));
 ELSE
  IF TG_OP='DELETE' THEN v_user:=OLD.owner_id; v_peer:=OLD.peer_id;
  ELSE v_user:=NEW.owner_id; v_peer:=NEW.peer_id; END IF;
  IF TG_TABLE_NAME='contacts' THEN
   PERFORM directory.sync_emit('contacts',v_user,v_user,NULL,v_peer,'active',jsonb_build_object('action',lower(TG_OP)));
  ELSE
   PERFORM directory.sync_emit('relationship',v_user,v_user,NULL,v_peer,'active',jsonb_build_object('action',lower(TG_OP)));
  END IF;
  PERFORM directory.sync_bump_permissions(ARRAY[v_user,v_peer]);
 END IF;
 RETURN NULL;
END $jobsim_sync$;
DROP TRIGGER IF EXISTS sync_capture ON directory.profiles;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON directory.profiles FOR EACH ROW EXECUTE FUNCTION directory.sync_capture();
DROP TRIGGER IF EXISTS sync_capture ON directory.contacts;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON directory.contacts FOR EACH ROW EXECUTE FUNCTION directory.sync_capture();
DROP TRIGGER IF EXISTS sync_capture ON directory.blocks;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON directory.blocks FOR EACH ROW EXECUTE FUNCTION directory.sync_capture();
DROP TRIGGER IF EXISTS sync_capture ON directory.contact_requests;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON directory.contact_requests FOR EACH ROW EXECUTE FUNCTION directory.sync_capture();`
	case "conversation":
		capture = `
CREATE OR REPLACE FUNCTION conversation.sync_capture() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $jobsim_sync$
DECLARE v_id uuid; v_user uuid; v_owner uuid; v_previous_owner uuid; v_state text := 'active'; v_announcement boolean := false; v_permission boolean := false; v_users uuid[];
BEGIN
 IF TG_TABLE_NAME='members' THEN
  IF TG_OP='DELETE' THEN v_id:=OLD.conversation_id; v_user:=OLD.user_id;
  ELSE v_id:=NEW.conversation_id; v_user:=NEW.user_id; END IF;
  PERFORM conversation.sync_emit('conversation',v_id,NULL,v_id,v_user,'active',jsonb_build_object('action',lower(TG_OP)));
  PERFORM conversation.sync_bump_permissions(ARRAY[v_user]);
 ELSE
  IF TG_OP='DELETE' THEN
   v_id:=OLD.id; v_owner:=OLD.owner_id; v_state:='removed'; v_permission:=true; v_announcement:=true;
  ELSIF TG_OP='INSERT' THEN
   v_id:=NEW.id; v_owner:=NEW.owner_id;
   v_announcement:=NEW.announcement_message_id IS NOT NULL;
  ELSE
   v_id:=NEW.id; v_owner:=NEW.owner_id; v_previous_owner:=OLD.owner_id;
   v_permission:=OLD.state IS DISTINCT FROM NEW.state OR OLD.owner_id IS DISTINCT FROM NEW.owner_id;
   v_announcement:=OLD.announcement_message_id IS DISTINCT FROM NEW.announcement_message_id
    OR OLD.announcement_source_version IS DISTINCT FROM NEW.announcement_source_version
    OR OLD.announcement_expires_at IS DISTINCT FROM NEW.announcement_expires_at
    OR OLD.announcement_version IS DISTINCT FROM NEW.announcement_version
    OR OLD.announcement_updated_at IS DISTINCT FROM NEW.announcement_updated_at;
  END IF;
  PERFORM conversation.sync_emit('conversation',v_id,NULL,v_id,NULL,v_state,jsonb_build_object('action',lower(TG_OP)));
  IF v_announcement THEN
   PERFORM conversation.sync_emit('announcement',v_id,NULL,v_id,NULL,v_state,jsonb_build_object('action',lower(TG_OP)));
  END IF;
  IF v_permission THEN
   SELECT array_agg(user_id ORDER BY user_id) INTO v_users FROM conversation.members WHERE conversation_id=v_id;
   PERFORM conversation.sync_bump_permissions(coalesce(v_users,ARRAY[]::uuid[])||ARRAY[v_owner,v_previous_owner]);
  END IF;
 END IF;
 RETURN NULL;
END $jobsim_sync$;
DROP TRIGGER IF EXISTS sync_capture ON conversation.conversations;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON conversation.conversations FOR EACH ROW EXECUTE FUNCTION conversation.sync_capture();
DROP TRIGGER IF EXISTS sync_capture ON conversation.members;
CREATE TRIGGER sync_capture AFTER INSERT OR UPDATE OR DELETE ON conversation.members FOR EACH ROW EXECUTE FUNCTION conversation.sync_capture();`
	case "message":
		capture = `
CREATE OR REPLACE FUNCTION message.sync_capture() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $jobsim_sync$
DECLARE v_user uuid; v_conversation uuid; v_version bigint; v_created timestamptz; v_deleted boolean;
BEGIN
 SELECT sender_id,conversation_id,content_version,created_at,deleted INTO STRICT v_user,v_conversation,v_version,v_created,v_deleted
 FROM message.messages WHERE id=NEW.message_id;
 IF v_conversation<>NEW.conversation_id THEN RAISE EXCEPTION 'sync message event conversation mismatch'; END IF;
 PERFORM message.sync_emit('message',NEW.message_id,v_user,v_conversation,NULL,CASE WHEN v_deleted THEN 'removed' ELSE 'active' END,
  jsonb_build_object('action','insert','sourceVersion',v_version::text,'createdAt',to_char(v_created AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')));
 RETURN NULL;
END $jobsim_sync$;
DROP TRIGGER IF EXISTS sync_capture ON message.events;
CREATE TRIGGER sync_capture AFTER INSERT ON message.events FOR EACH ROW EXECUTE FUNCTION message.sync_capture();`
	case "sync":
		capture = `CREATE OR REPLACE FUNCTION sync.sync_capture_self() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $self$
DECLARE v_id uuid; v_user uuid; v_conversation uuid; v_state text := 'active'; v_kind text;
BEGIN
 IF TG_TABLE_NAME='conversation_views' THEN
  v_kind:='conversation_view';
  IF TG_OP='UPDATE' AND ROW(NEW.id,NEW.scope_id,NEW.source_context_id,NEW.user_id,NEW.conversation_id,NEW.revision_epoch,NEW.row_version,NEW.pinned_rank,NEW.muted_until,NEW.archived,NEW.hidden,NEW.manual_unread,NEW.manual_unread_generation,NEW.manual_unread_marked_at,NEW.manual_unread_from_epoch,NEW.manual_unread_from_seq)
   IS NOT DISTINCT FROM ROW(OLD.id,OLD.scope_id,OLD.source_context_id,OLD.user_id,OLD.conversation_id,OLD.revision_epoch,OLD.row_version,OLD.pinned_rank,OLD.muted_until,OLD.archived,OLD.hidden,OLD.manual_unread,OLD.manual_unread_generation,OLD.manual_unread_marked_at,OLD.manual_unread_from_epoch,OLD.manual_unread_from_seq) THEN RETURN NULL; END IF;
 ELSIF TG_TABLE_NAME='read_cursors' THEN v_kind:='read_state';
 ELSIF TG_TABLE_NAME='read_observations' THEN v_kind:='read_observation';
 ELSE RAISE EXCEPTION 'unknown personal sync source'; END IF;
 IF TG_OP='DELETE' THEN
  IF OLD.scope_id::text<>(SELECT deployment_id FROM sync.instance WHERE singleton) OR OLD.source_context_id<>'personal' THEN RAISE EXCEPTION 'personal sync scope mismatch'; END IF;
  IF OLD.row_version=0 THEN RETURN NULL; END IF;
  v_id:=OLD.id; v_user:=OLD.user_id; v_conversation:=OLD.conversation_id; v_state:='removed';
 ELSE
  IF NEW.scope_id::text<>(SELECT deployment_id FROM sync.instance WHERE singleton) OR NEW.source_context_id<>'personal' THEN RAISE EXCEPTION 'personal sync scope mismatch'; END IF;
  IF NEW.row_version=0 THEN RETURN NULL; END IF;
  v_id:=NEW.id; v_user:=NEW.user_id; v_conversation:=NEW.conversation_id;
 END IF;
 PERFORM sync.sync_emit(v_kind,v_id,v_user,v_conversation,NULL,v_state,jsonb_build_object('action',lower(TG_OP)));
 RETURN NULL;
END $self$;
DROP TRIGGER IF EXISTS sync_capture_self ON sync.conversation_views;
CREATE TRIGGER sync_capture_self AFTER INSERT OR UPDATE OR DELETE ON sync.conversation_views FOR EACH ROW EXECUTE FUNCTION sync.sync_capture_self();`
		capture += `DROP TRIGGER IF EXISTS sync_capture_self ON sync.read_cursors;
CREATE TRIGGER sync_capture_self AFTER INSERT OR UPDATE OR DELETE ON sync.read_cursors FOR EACH ROW EXECUTE FUNCTION sync.sync_capture_self();
DROP TRIGGER IF EXISTS sync_capture_self ON sync.read_observations;
CREATE TRIGGER sync_capture_self AFTER INSERT OR UPDATE OR DELETE ON sync.read_observations FOR EACH ROW EXECUTE FUNCTION sync.sync_capture_self();`
	}
	if _, err := tx.Exec(ctx, capture); err != nil {
		return fmt.Errorf("install sync owner capture triggers: %w", err)
	}
	return migrateSyncPreferenceCapture(ctx, tx, owner)
}
