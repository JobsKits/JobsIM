package jobsim

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

func syncProtocolSchema(value string) (string, error) {
	if value == "" || value == "1" {
		return "1", nil
	}
	if value == "2" {
		return "2", nil
	}
	return "", fail(400, "UNSUPPORTED_SYNC_SCHEMA", "Synchronization schema must be canonical 1 or 2")
}

type syncSchemaConstraint struct {
	Name       string
	Kind       string
	Columns    []string
	Definition string
	Validated  bool
}

func syncSchemaConstraints(ctx context.Context, tx pgx.Tx, table string) ([]syncSchemaConstraint, error) {
	rows, err := tx.Query(ctx, `SELECT c.conname,c.contype::text,
ARRAY(SELECT a.attname::text FROM unnest(c.conkey) WITH ORDINALITY k(attnum,ordinal) JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=k.attnum ORDER BY k.ordinal),
pg_get_constraintdef(c.oid),c.convalidated FROM pg_constraint c WHERE c.conrelid=$1::regclass ORDER BY c.conname`, "sync."+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	constraints := []syncSchemaConstraint{}
	for rows.Next() {
		var constraint syncSchemaConstraint
		if err = rows.Scan(&constraint.Name, &constraint.Kind, &constraint.Columns, &constraint.Definition, &constraint.Validated); err != nil {
			return nil, err
		}
		constraints = append(constraints, constraint)
	}
	return constraints, rows.Err()
}

func syncSchemaEnumDefinition(definition string) bool {
	definition = strings.ToLower(definition)
	definition = strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "(", "", ")", "", "\"", "", "::text", "").Replace(definition)
	definition = strings.TrimSuffix(definition, "notvalid")
	return definition == "checkschema_version=anyarray['1','2']" || definition == "checkschema_versionin'1','2'"
}

// Repeated migration preserves an existing valid schema constraint and its OID.
func syncSchemaColumn(ctx context.Context, tx pgx.Tx, table string) error {
	identifier := pgx.Identifier{"sync", table}.Sanitize()
	if _, err := tx.Exec(ctx, "ALTER TABLE "+identifier+" ADD COLUMN IF NOT EXISTS schema_version text NOT NULL DEFAULT '1'"); err != nil {
		return err
	}
	var textColumn, required bool
	var defaultValue string
	err := tx.QueryRow(ctx, `SELECT a.atttypid='text'::regtype,a.attnotnull,COALESCE(pg_get_expr(d.adbin,d.adrelid),'')
FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
WHERE a.attrelid=$1::regclass AND a.attname='schema_version' AND NOT a.attisdropped`, "sync."+table).Scan(&textColumn, &required, &defaultValue)
	if err != nil {
		return err
	}
	if !textColumn {
		return fmt.Errorf("sync schema version column has an incompatible type")
	}
	if defaultValue != "'1'::text" {
		if _, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" ALTER COLUMN schema_version SET DEFAULT '1'"); err != nil {
			return err
		}
	}
	if !required {
		if _, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" ALTER COLUMN schema_version SET NOT NULL"); err != nil {
			return err
		}
	}
	constraints, err := syncSchemaConstraints(ctx, tx, table)
	if err != nil {
		return err
	}
	name := table + "_protocol_schema_check"
	for _, constraint := range constraints {
		matches := constraint.Kind == "c" && slices.Equal(constraint.Columns, []string{"schema_version"}) && syncSchemaEnumDefinition(constraint.Definition)
		if constraint.Name == name && !matches {
			return fmt.Errorf("sync schema constraint name is already used by another definition")
		}
		if matches {
			if !constraint.Validated {
				_, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" VALIDATE CONSTRAINT "+pgx.Identifier{constraint.Name}.Sanitize())
			}
			return err
		}
	}
	_, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" ADD CONSTRAINT "+pgx.Identifier{name}.Sanitize()+" CHECK(schema_version IN('1','2'))")
	return err
}

// Only the original default constraint with its exact old column list is dropped.
// Unrelated constraints and already-expanded keys are retained unchanged.
func syncSchemaExpandKey(ctx context.Context, tx pgx.Tx, table, originalName, kind string, oldColumns, newColumns []string) error {
	constraints, err := syncSchemaConstraints(ctx, tx, table)
	if err != nil {
		return err
	}
	oldFound, newFound := false, false
	for _, constraint := range constraints {
		oldKey := constraint.Kind == kind && slices.Equal(constraint.Columns, oldColumns)
		newKey := constraint.Kind == kind && slices.Equal(constraint.Columns, newColumns)
		if constraint.Name == originalName && !oldKey && !newKey {
			return fmt.Errorf("original sync key name has an incompatible catalog definition")
		}
		if oldKey {
			if constraint.Name != originalName {
				return fmt.Errorf("an unknown legacy sync key requires explicit migration review")
			}
			oldFound = true
		}
		newFound = newFound || newKey
	}
	identifier := pgx.Identifier{"sync", table}.Sanitize()
	if oldFound {
		if _, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" DROP CONSTRAINT "+pgx.Identifier{originalName}.Sanitize()); err != nil {
			return err
		}
	}
	if !newFound {
		columns := make([]string, len(newColumns))
		for index, column := range newColumns {
			columns[index] = pgx.Identifier{column}.Sanitize()
		}
		key := "UNIQUE"
		if kind == "p" {
			key = "PRIMARY KEY"
		}
		_, err = tx.Exec(ctx, "ALTER TABLE "+identifier+" ADD CONSTRAINT "+pgx.Identifier{originalName}.Sanitize()+" "+key+"("+strings.Join(columns, ",")+")")
	}
	return err
}

// The savepoint also rolls back logical backfill failures before returning an
// error, so a caller cannot accidentally commit a partially expanded schema.
func migrateSyncSchemas(ctx context.Context, tx pgx.Tx, scope string) error {
	if !validObjectID(scope) || scope != strings.ToLower(scope) {
		return fmt.Errorf("canonical deployment UUID required for sync schema migration")
	}
	work, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer work.Rollback(ctx)
	var deployment string
	if err = work.QueryRow(ctx, "SELECT deployment_id FROM sync.instance WHERE singleton FOR SHARE").Scan(&deployment); err != nil {
		return err
	}
	if deployment != scope {
		return fmt.Errorf("sync schema migration belongs to another deployment")
	}
	if _, err = work.Exec(ctx, "LOCK TABLE sync.streams,sync.registrations IN ACCESS EXCLUSIVE MODE"); err != nil {
		return err
	}
	for _, table := range []string{"streams", "registrations"} {
		if err = syncSchemaColumn(ctx, work, table); err != nil {
			return err
		}
	}
	if err = syncSchemaExpandKey(ctx, work, "streams", "streams_user_id_source_context_id_restore_epoch_key", "u", []string{"user_id", "source_context_id", "restore_epoch"}, []string{"user_id", "source_context_id", "restore_epoch", "schema_version"}); err != nil {
		return err
	}
	if err = syncSchemaExpandKey(ctx, work, "registrations", "registrations_pkey", "p", []string{"user_id", "device_id", "restore_epoch"}, []string{"user_id", "device_id", "restore_epoch", "schema_version"}); err != nil {
		return err
	}
	if _, err = work.Exec(ctx, `CREATE TABLE IF NOT EXISTS sync.device_stores(
scope_id uuid NOT NULL,source_context_id text NOT NULL DEFAULT 'personal' CHECK(source_context_id='personal'),
user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL CHECK(restore_epoch>0),
store_generation uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,source_context_id,user_id,device_id,restore_epoch));
LOCK TABLE sync.device_stores IN ACCESS EXCLUSIVE MODE;`); err != nil {
		return err
	}
	constraints, err := syncSchemaConstraints(ctx, work, "device_stores")
	if err != nil {
		return err
	}
	physicalKey := false
	for _, constraint := range constraints {
		physicalKey = physicalKey || constraint.Kind == "p" && slices.Equal(constraint.Columns, []string{"scope_id", "source_context_id", "user_id", "device_id", "restore_epoch"})
	}
	if !physicalKey {
		return fmt.Errorf("sync device store has an incompatible physical device key")
	}
	var conflict bool
	if err = work.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.registrations r LEFT JOIN sync.streams s ON s.id=r.stream_id WHERE s.id IS NULL OR s.user_id<>r.user_id OR s.restore_epoch<>r.restore_epoch OR s.schema_version<>r.schema_version OR s.source_context_id<>'personal')`).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("sync registration and stream ownership do not agree")
	}
	if err = work.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.registrations r JOIN sync.streams s ON s.id=r.stream_id GROUP BY s.source_context_id,r.user_id,r.device_id,r.restore_epoch HAVING count(DISTINCT r.store_generation)>1)
OR EXISTS(SELECT 1 FROM sync.registrations r JOIN sync.streams s ON s.id=r.stream_id JOIN sync.device_stores d ON d.scope_id=$1 AND d.source_context_id=s.source_context_id AND d.user_id=r.user_id AND d.device_id=r.device_id AND d.restore_epoch=r.restore_epoch WHERE d.store_generation<>r.store_generation)`, scope).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("conflicting physical device stores prevent sync schema migration")
	}
	if _, err = work.Exec(ctx, `INSERT INTO sync.device_stores(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation,created_at)
SELECT $1,s.source_context_id,r.user_id,r.device_id,r.restore_epoch,r.store_generation,min(r.created_at)
FROM sync.registrations r JOIN sync.streams s ON s.id=r.stream_id
GROUP BY s.source_context_id,r.user_id,r.device_id,r.restore_epoch,r.store_generation
ON CONFLICT(scope_id,source_context_id,user_id,device_id,restore_epoch) DO NOTHING`, scope); err != nil {
		return err
	}
	return work.Commit(ctx)
}
