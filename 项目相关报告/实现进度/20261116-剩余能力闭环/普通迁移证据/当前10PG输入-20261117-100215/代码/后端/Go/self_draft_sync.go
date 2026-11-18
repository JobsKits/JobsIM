package jobsim

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func migrateSelfDraftCapture(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS sync.draft_reply_invalidations(holder_id uuid PRIMARY KEY,operation_id text NOT NULL,floor jsonb NOT NULL,state text NOT NULL CHECK(state IN('prepared','applied')));
CREATE OR REPLACE FUNCTION sync.sync_capture_draft() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $draft$
DECLARE v_id uuid; v_user uuid; v_conversation uuid;
BEGIN
 IF TG_OP='UPDATE' AND ROW(NEW.draft_revision_epoch,NEW.draft_version,NEW.draft_operation_id,NEW.draft_content_state,NEW.draft_owned_digest,NEW.draft_reply_state,NEW.draft_reply_reference)
 IS NOT DISTINCT FROM ROW(OLD.draft_revision_epoch,OLD.draft_version,OLD.draft_operation_id,OLD.draft_content_state,OLD.draft_owned_digest,OLD.draft_reply_state,OLD.draft_reply_reference) THEN RETURN NULL; END IF;
 IF TG_OP='DELETE' THEN
  IF OLD.draft_version=0 THEN RETURN NULL; END IF;
  v_id:=OLD.id; v_user:=OLD.user_id; v_conversation:=OLD.conversation_id;
 ELSE
  IF NEW.draft_version=0 THEN RETURN NULL; END IF;
  IF NEW.scope_id::text<>(SELECT deployment_id FROM sync.instance WHERE singleton) OR NEW.source_context_id<>'personal' THEN RAISE EXCEPTION 'draft scope mismatch'; END IF;
  v_id:=NEW.id; v_user:=NEW.user_id; v_conversation:=NEW.conversation_id;
 END IF;
 PERFORM sync.sync_emit('draft',v_id,v_user,v_conversation,NULL,'active',jsonb_build_object('action',lower(TG_OP)));
 RETURN NULL;
END $draft$;
DROP TRIGGER IF EXISTS sync_capture_draft ON sync.conversation_views;
CREATE TRIGGER sync_capture_draft AFTER INSERT OR UPDATE OR DELETE ON sync.conversation_views FOR EACH ROW EXECUTE FUNCTION sync.sync_capture_draft();`)
	return err
}
