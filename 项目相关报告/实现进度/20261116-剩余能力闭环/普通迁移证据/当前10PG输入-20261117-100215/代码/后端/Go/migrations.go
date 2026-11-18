package jobsim

import (
	"context"
	"fmt"
)

// Each role migrates and queries only its own schema. No cross-role FK or transaction.
var migrations = map[string]string{
	"identity": `
CREATE SCHEMA IF NOT EXISTS auth;
CREATE TABLE IF NOT EXISTS auth.users(id uuid PRIMARY KEY,username text NOT NULL UNIQUE CHECK(username~'^[a-z0-9_]{3,32}$'),display_name text NOT NULL,password_hash text NOT NULL,credential_generation bigint NOT NULL DEFAULT 1,admin boolean NOT NULL DEFAULT false,state text NOT NULL DEFAULT 'active',created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS auth.sessions(id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES auth.users(id),device_id uuid NOT NULL,device_name text NOT NULL,token_digest bytea NOT NULL UNIQUE,generation bigint NOT NULL,restore_epoch bigint NOT NULL DEFAULT 1,expires_at timestamptz NOT NULL,revoked_at timestamptz,web_pending boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS auth.local_auth_requests(request_id text PRIMARY KEY,purpose text NOT NULL,request_digest bytea NOT NULL,user_id uuid NOT NULL,device_id uuid NOT NULL,delivery_ciphertext bytea NOT NULL,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS auth.rate_windows(subject text NOT NULL,window_start timestamptz NOT NULL,count int NOT NULL,PRIMARY KEY(subject,window_start));
CREATE TABLE IF NOT EXISTS auth.recovery_code_batches(id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES auth.users(id),generation bigint NOT NULL,active boolean NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE UNIQUE INDEX IF NOT EXISTS recovery_active ON auth.recovery_code_batches(user_id) WHERE active;
CREATE TABLE IF NOT EXISTS auth.recovery_codes(id uuid PRIMARY KEY,batch_id uuid NOT NULL REFERENCES auth.recovery_code_batches(id),code_digest bytea NOT NULL UNIQUE,consumed_at timestamptz);
CREATE TABLE IF NOT EXISTS auth.recovery_operations(request_id text PRIMARY KEY,user_id uuid NOT NULL,request_digest bytea NOT NULL,kind text NOT NULL,result jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now());`,
	"directory": `
CREATE SCHEMA IF NOT EXISTS directory;
CREATE TABLE IF NOT EXISTS directory.profiles(user_id uuid PRIMARY KEY,display_name text NOT NULL,updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS directory.contacts(owner_id uuid NOT NULL,peer_id uuid NOT NULL,state text NOT NULL,request_generation bigint NOT NULL DEFAULT 1,request_gate text NOT NULL DEFAULT 'open',updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(owner_id,peer_id),CHECK(owner_id<>peer_id));
CREATE TABLE IF NOT EXISTS directory.contact_requests(id uuid PRIMARY KEY,request_id text NOT NULL,requester_id uuid NOT NULL,recipient_id uuid NOT NULL,state text NOT NULL,generation bigint NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(requester_id,request_id),CHECK(requester_id<>recipient_id));
CREATE TABLE IF NOT EXISTS directory.blocks(owner_id uuid NOT NULL,peer_id uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(owner_id,peer_id),CHECK(owner_id<>peer_id));`,
	"conversation": `
CREATE SCHEMA IF NOT EXISTS conversation;
CREATE TABLE IF NOT EXISTS conversation.conversations(id uuid PRIMARY KEY,kind text NOT NULL CHECK(kind IN('direct','group','self')),pair_key text UNIQUE,title text NOT NULL,owner_id uuid NOT NULL,version bigint NOT NULL DEFAULT 1,state text NOT NULL DEFAULT 'active',created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS conversation.members(conversation_id uuid NOT NULL REFERENCES conversation.conversations(id),user_id uuid NOT NULL,role text NOT NULL,state text NOT NULL DEFAULT 'active',membership_generation bigint NOT NULL DEFAULT 1,joined_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(conversation_id,user_id));
CREATE TABLE IF NOT EXISTS conversation.membership_periods(conversation_id uuid NOT NULL,user_id uuid NOT NULL,generation bigint NOT NULL,joined_at timestamptz NOT NULL DEFAULT now(),left_at timestamptz,PRIMARY KEY(conversation_id,user_id,generation));`,
	"message": `
CREATE SCHEMA IF NOT EXISTS message;
CREATE TABLE IF NOT EXISTS message.heads(conversation_id uuid PRIMARY KEY,sequence bigint NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS message.messages(id uuid PRIMARY KEY,conversation_id uuid NOT NULL,sender_id uuid NOT NULL,client_msg_id text NOT NULL,payload_digest bytea NOT NULL,type text NOT NULL,text text NOT NULL,reply_to_id uuid,attachment_id uuid,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),version bigint NOT NULL DEFAULT 1,sequence bigint NOT NULL,deleted boolean NOT NULL DEFAULT false,UNIQUE(sender_id,client_msg_id));
CREATE INDEX IF NOT EXISTS message_history ON message.messages(conversation_id,sequence);
CREATE TABLE IF NOT EXISTS message.events(conversation_id uuid NOT NULL,sequence bigint NOT NULL,message_id uuid NOT NULL REFERENCES message.messages(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(conversation_id,sequence));
CREATE TABLE IF NOT EXISTS message.reactions(message_id uuid NOT NULL REFERENCES message.messages(id),user_id uuid NOT NULL,emoji text NOT NULL,PRIMARY KEY(message_id,user_id,emoji));
CREATE TABLE IF NOT EXISTS message.pins(message_id uuid PRIMARY KEY REFERENCES message.messages(id),actor_id uuid NOT NULL);
CREATE TABLE IF NOT EXISTS message.favorites(message_id uuid NOT NULL REFERENCES message.messages(id),user_id uuid NOT NULL,PRIMARY KEY(message_id,user_id));`,
	"media": `
CREATE SCHEMA IF NOT EXISTS media;
CREATE TABLE IF NOT EXISTS media.objects(id uuid PRIMARY KEY,owner_id uuid NOT NULL,name text NOT NULL,content_type text NOT NULL,size_bytes bigint NOT NULL,sha256 bytea NOT NULL,storage_key text NOT NULL UNIQUE,conversation_id uuid,message_id uuid,created_at timestamptz NOT NULL DEFAULT now());`,
	"control": `
CREATE SCHEMA IF NOT EXISTS control;
CREATE TABLE IF NOT EXISTS control.capabilities(name text PRIMARY KEY,enabled boolean NOT NULL,updated_at timestamptz NOT NULL DEFAULT now());`,
}

func init() {
	schemaFor["journal"] = "journal"
	migrations["journal"] = `CREATE SCHEMA IF NOT EXISTS journal;
CREATE TABLE IF NOT EXISTS journal.restore_epochs(singleton boolean PRIMARY KEY CHECK(singleton),restore_epoch bigint NOT NULL CHECK(restore_epoch>0));
INSERT INTO journal.restore_epochs VALUES(true,1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS journal.chain_head(singleton boolean PRIMARY KEY CHECK(singleton),digest bytea NOT NULL);
INSERT INTO journal.chain_head VALUES(true,decode(repeat('00',32),'hex')) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS journal.entries(seq bigserial PRIMARY KEY,owner text NOT NULL,operation_key text NOT NULL,phase text NOT NULL,kind text NOT NULL,payload jsonb NOT NULL,metadata_digest bytea NOT NULL,previous_digest bytea NOT NULL,entry_digest bytea NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(owner,operation_key,phase));`
}

var schemaFor = map[string]string{"identity": "auth", "directory": "directory", "conversation": "conversation", "message": "message", "media": "media", "control": "control"}

func (a *App) migrate(ctx context.Context) error {
	schema := schemaFor[a.Role]
	if schema == "" {
		return nil
	}
	conn, err := a.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock(hashtext($1))", "jobsim:migrate:"+a.Role); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", "jobsim:migrate:"+a.Role)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, migrations[a.Role]); err != nil {
		return fmt.Errorf("%s migration: %w", a.Role, err)
	}
	if _, err = tx.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,migration_version int NOT NULL,restore_epoch bigint NOT NULL DEFAULT 1);`, schema)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s.instance(singleton,deployment_id,migration_version) VALUES(true,$1,1) ON CONFLICT DO NOTHING`, schema), a.Config.DeploymentID); err != nil {
		return err
	}
	var id string
	if err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT deployment_id FROM %s.instance WHERE singleton`, schema)).Scan(&id); err != nil {
		return err
	}
	if id != a.Config.DeploymentID {
		return fmt.Errorf("database belongs to another deployment")
	}
	if a.Role == "control" {
		if _, err = tx.Exec(ctx, `ALTER TABLE control.capabilities ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1; CREATE TABLE IF NOT EXISTS control.operation_results(request_id text PRIMARY KEY,actor_id uuid NOT NULL,request_digest bytea NOT NULL,state text NOT NULL,result jsonb NOT NULL DEFAULT '{}');`); err != nil {
			return err
		}
		for name, enabled := range initialFeatures {
			if _, err = tx.Exec(ctx, `INSERT INTO control.capabilities(name,enabled) VALUES($1,$2) ON CONFLICT DO NOTHING`, name, enabled); err != nil {
				return err
			}
		}
	}
	if a.Role == "identity" {
		if _, err = tx.Exec(ctx, `ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS restore_epoch bigint NOT NULL DEFAULT 1; ALTER TABLE auth.recovery_operations ADD COLUMN IF NOT EXISTS state text NOT NULL DEFAULT 'completed'; ALTER TABLE auth.recovery_operations ADD COLUMN IF NOT EXISTS planned_metadata jsonb; ALTER TABLE auth.local_auth_requests ADD COLUMN IF NOT EXISTS delivery_version text NOT NULL DEFAULT 'legacy'; UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now()) WHERE delivery_version='legacy'; CREATE TABLE IF NOT EXISTS auth.identity_reservations(username text PRIMARY KEY,user_id uuid NOT NULL); ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS recovery_generation bigint NOT NULL DEFAULT 0; ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS security_version bigint NOT NULL DEFAULT 1; ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS row_version bigint NOT NULL DEFAULT 1; ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS recovery_state text NOT NULL DEFAULT 'normal'; ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS deletion_state text NOT NULL DEFAULT 'none'; ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS state_before_tombstone text; ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS ban_until timestamptz; ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS pending_operation_id text; ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS security_version bigint NOT NULL DEFAULT 1; CREATE TABLE IF NOT EXISTS auth.mfa_factors(id uuid PRIMARY KEY,user_id uuid NOT NULL UNIQUE REFERENCES auth.users(id),secret_ciphertext bytea NOT NULL,last_accepted_step bigint NOT NULL,revoked_at timestamptz); CREATE TABLE IF NOT EXISTS auth.step_up_authorizations(id uuid PRIMARY KEY,user_id uuid NOT NULL,device_id uuid NOT NULL,action text NOT NULL,target_id text NOT NULL,request_id text NOT NULL,request_digest bytea NOT NULL,security_version bigint NOT NULL,restore_epoch bigint NOT NULL,expires_at timestamptz NOT NULL,consumed_at timestamptz,UNIQUE(user_id,request_id)); CREATE TABLE IF NOT EXISTS auth.admin_operations(request_id text PRIMARY KEY,actor_id uuid NOT NULL,target_id uuid NOT NULL,action text NOT NULL,request_digest bytea NOT NULL,state text NOT NULL,result jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now()); ALTER TABLE auth.admin_operations ADD COLUMN IF NOT EXISTS journal_payload jsonb; ALTER TABLE auth.admin_operations ADD COLUMN IF NOT EXISTS request_metadata jsonb NOT NULL DEFAULT '{}'; UPDATE auth.users u SET recovery_generation=COALESCE((SELECT max(generation) FROM auth.recovery_code_batches b WHERE b.user_id=u.id AND b.active),0) WHERE recovery_generation=0 AND EXISTS(SELECT 1 FROM auth.recovery_code_batches b WHERE b.user_id=u.id AND b.active AND b.generation>0);`); err != nil {
			return err
		}
		if err = migrateSessionRefresh(ctx, tx); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s.instance ADD COLUMN IF NOT EXISTS restore_epoch bigint NOT NULL DEFAULT 1`, schema)); err != nil {
		return err
	}
	if a.Role == "sync" {
		if err = migrateSyncSchemas(ctx, tx, a.Config.DeploymentID); err != nil {
			return err
		}
		if err = migrateSyncReceipts(ctx, tx); err != nil {
			return err
		}
		if err = migrateSyncMigrations(ctx, tx); err != nil {
			return err
		}
		if err = migrateSelfViews(ctx, tx, a.Config.DeploymentID); err != nil {
			return err
		}
		if err = migrateSelfRead(ctx, tx, a.Config.DeploymentID); err != nil {
			return err
		}
		if err = migrateSelfDraftFloor(ctx, tx, a.Config.DeploymentID); err != nil {
			return err
		}
		if err = migrateSelfDraftOperations(ctx, tx, a.Config.DeploymentID); err != nil {
			return err
		}
	}
	if err = migrateMigrationInterests(ctx, tx, a.Role); err != nil {
		return err
	}
	if owner := syncOwnerSchema(a.Role); owner != "" {
		if owner == "message" {
			if err = migrateDraftReplyHolders(ctx, tx, a.Config.DeploymentID); err != nil {
				return err
			}
			if err = migrateDraftReplyRevocations(ctx, tx, a.Config.DeploymentID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `ALTER TABLE message.messages ADD COLUMN IF NOT EXISTS sequence_epoch bigint;
ALTER TABLE message.messages ADD COLUMN IF NOT EXISTS creation_sequence bigint;
CREATE OR REPLACE FUNCTION message.sync_creation_epoch() RETURNS trigger LANGUAGE plpgsql AS $creation$ BEGIN
IF TG_OP='INSERT' THEN SELECT restore_epoch INTO STRICT NEW.sequence_epoch FROM message.instance WHERE singleton; NEW.creation_sequence:=NEW.sequence;
ELSE NEW.sequence_epoch:=OLD.sequence_epoch; NEW.creation_sequence:=OLD.creation_sequence; END IF; RETURN NEW; END $creation$;
DROP TRIGGER IF EXISTS sync_creation_epoch ON message.messages;
CREATE TRIGGER sync_creation_epoch BEFORE INSERT OR UPDATE ON message.messages FOR EACH ROW EXECUTE FUNCTION message.sync_creation_epoch();`); err != nil {
				return err
			}
		}
		if err = migrateSyncOwner(ctx, tx, owner, a.Config.DeploymentID); err != nil {
			return err
		}
		if owner == "sync" {
			if err = migrateSelfDraftCapture(ctx, tx); err != nil {
				return err
			}
		}
		if err = migrateSyncPublication(ctx, tx, owner); err != nil {
			return err
		}
	}
	if err = migrateRetentionLegacy(ctx, tx, a.Role); err != nil {
		return err
	}
	if err = migratePersonalExports(ctx, tx, a.Role); err != nil {
		return err
	}
	if err = migrateAccountDeletion(ctx, tx, a.Role); err != nil {
		return err
	}
	if err = migrateMigrationInvalidations(ctx, tx, a.Role); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
