package jobsim

func init() {
	migrations["directory"] += `
ALTER TABLE directory.profiles ADD COLUMN IF NOT EXISTS row_version bigint NOT NULL DEFAULT 1;
ALTER TABLE directory.profiles ADD COLUMN IF NOT EXISTS revision_epoch bigint NOT NULL DEFAULT 1;
ALTER TABLE directory.profiles ADD COLUMN IF NOT EXISTS avatar_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE directory.profiles ADD COLUMN IF NOT EXISTS avatar_object_id uuid;
ALTER TABLE directory.profiles ADD COLUMN IF NOT EXISTS avatar_thumbnail_id uuid;
ALTER TABLE directory.profiles ADD COLUMN IF NOT EXISTS pending_avatar_operation_id text;
CREATE TABLE IF NOT EXISTS directory.avatar_operations(user_id uuid NOT NULL,request_id text NOT NULL,request_digest bytea NOT NULL,source_digest bytea NOT NULL,restore_epoch bigint NOT NULL,generation bigint NOT NULL,state text NOT NULL,result jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,request_id));`
	migrations["media"] += `
ALTER TABLE media.objects ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT 'chat_attachment';
ALTER TABLE media.objects ADD COLUMN IF NOT EXISTS owner_reference_id uuid;
ALTER TABLE media.objects ADD COLUMN IF NOT EXISTS owner_generation bigint;
ALTER TABLE media.objects ADD COLUMN IF NOT EXISTS parent_object_id uuid;
CREATE TABLE IF NOT EXISTS media.avatar_jobs(owner_id uuid NOT NULL,operation_id text NOT NULL,source_digest bytea NOT NULL,restore_epoch bigint NOT NULL,generation bigint NOT NULL,root_id uuid NOT NULL,thumbnail_id uuid NOT NULL,state text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(owner_id,operation_id));
CREATE TABLE IF NOT EXISTS media.avatar_processing_items(job_id uuid PRIMARY KEY,owner_id uuid NOT NULL,publication_operation_id text NOT NULL,kind text NOT NULL,profile_version text NOT NULL,output_id uuid NOT NULL,state text NOT NULL,UNIQUE(owner_id,publication_operation_id,kind,profile_version));`
	schemaFor["governance"] = "governance"
	migrations["governance"] = `CREATE SCHEMA IF NOT EXISTS governance;
CREATE TABLE IF NOT EXISTS governance.reports(id uuid PRIMARY KEY,reporter_id uuid NOT NULL,target_type text NOT NULL CHECK(target_type IN('message','user','conversation')),target_id uuid NOT NULL,reason text NOT NULL,state text NOT NULL DEFAULT 'open' CHECK(state IN('open','reviewed','dismissed','action_required')),row_version bigint NOT NULL DEFAULT 1,request_id text NOT NULL,request_digest bytea NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(reporter_id,request_id));
CREATE TABLE IF NOT EXISTS governance.report_operations(request_id text PRIMARY KEY,report_id uuid NOT NULL REFERENCES governance.reports(id),actor_id uuid NOT NULL,request_digest bytea NOT NULL,state text NOT NULL,result jsonb NOT NULL DEFAULT '{}',reason text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS governance.rate_windows(subject text NOT NULL,window_start timestamptz NOT NULL,count int NOT NULL,PRIMARY KEY(subject,window_start));`
}
