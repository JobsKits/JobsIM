package jobsim

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const syncOriginalKinds = "'account','devices','profile','contacts','relationship','contact_request','conversation','announcement','message'"

// Expand only the owned generated check. A differently defined constraint is
// configuration drift, not permission to erase an administrator's constraint.
func migrateSyncPreferenceKind(ctx context.Context, tx pgx.Tx, owner string) error {
	if owner != "directory" {
		return nil
	}
	normalize := func(value string) string {
		return strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "", "(", "", ")", "", "::text", "").Replace(value)
	}
	oldDefinition := normalize("CHECK(kind=ANY(ARRAY[" + syncOriginalKinds + "]))")
	newKinds := syncOriginalKinds + ",'preferences'"
	newDefinition := normalize("CHECK(kind=ANY(ARRAY[" + newKinds + "]))")
	for _, table := range []string{"sync_object_versions", "sync_outbox"} {
		name := table + "_kind_check"
		var definition, kind string
		var columns []string
		err := tx.QueryRow(ctx, `SELECT pg_get_constraintdef(c.oid),c.contype::text,
ARRAY(SELECT a.attname::text FROM unnest(c.conkey) WITH ORDINALITY k(attnum,ordinal) JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=k.attnum ORDER BY k.ordinal)
FROM pg_constraint c WHERE c.conrelid=$1::regclass AND c.conname=$2`, owner+"."+table, name).Scan(&definition, &kind, &columns)
		if err != nil {
			return fmt.Errorf("sync kind constraint unavailable: %w", err)
		}
		definition = normalize(definition)
		if kind != "c" || len(columns) != 1 || columns[0] != "kind" || definition != oldDefinition && definition != newDefinition {
			return fmt.Errorf("sync kind constraint requires explicit migration review")
		}
		if definition == newDefinition {
			continue
		}
		identifier := pgx.Identifier{owner, table}.Sanitize()
		if _, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" DROP CONSTRAINT "+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" ADD CONSTRAINT "+pgx.Identifier{name}.Sanitize()+" CHECK(kind IN("+newKinds+"))"); err != nil {
			return err
		}
	}
	return nil
}

func migrateSyncPreferenceCapture(ctx context.Context, tx pgx.Tx, owner string) error {
	if owner != "directory" {
		return nil
	}
	_, err := tx.Exec(ctx, `CREATE OR REPLACE FUNCTION directory.sync_capture_preferences() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $preferences$
DECLARE v_user uuid; v_state text := 'active';
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.scope_id::text<>(SELECT deployment_id FROM directory.instance WHERE singleton) OR OLD.source_context_id<>'personal' THEN RAISE EXCEPTION 'preference source scope mismatch'; END IF;
  IF NOT OLD.explicit THEN RETURN NULL; END IF;
  v_user:=OLD.user_id; v_state:='removed';
 ELSE
  IF NEW.scope_id::text<>(SELECT deployment_id FROM directory.instance WHERE singleton) OR NEW.source_context_id<>'personal' THEN RAISE EXCEPTION 'preference source scope mismatch'; END IF;
  IF NOT NEW.explicit OR NEW.row_version=0 THEN RETURN NULL; END IF;
  v_user:=NEW.user_id;
 END IF;
 PERFORM directory.sync_emit('preferences',v_user,v_user,NULL,NULL,v_state,jsonb_build_object('action',lower(TG_OP)));
 RETURN NULL;
END $preferences$;
DROP TRIGGER IF EXISTS sync_capture_preferences ON directory.source_preferences;
CREATE TRIGGER sync_capture_preferences AFTER INSERT OR UPDATE OR DELETE ON directory.source_preferences FOR EACH ROW EXECUTE FUNCTION directory.sync_capture_preferences();`)
	return err
}
