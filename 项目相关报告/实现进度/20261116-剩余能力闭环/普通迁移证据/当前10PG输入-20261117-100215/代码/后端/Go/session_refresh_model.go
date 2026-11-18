package jobsim

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const accessLifetime = 15 * time.Minute
const refreshIdleLifetime = 30 * 24 * time.Hour
const refreshAbsoluteLifetime = 90 * 24 * time.Hour

func authenticationRetirementOperation(user, request string) string {
	return "auth-retire:" + user + ":" + hex.EncodeToString(tokenDigest(request))
}

func (a *App) authenticationRetirement(ctx context.Context, user, request string) (string, error) {
	var result struct {
		Retired     bool   `json:"retired"`
		Replacement string `json:"replacementKey"`
	}
	e := a.rpc(ctx, "journal", "/internal/auth-retirement", map[string]string{"userId": user, "requestKey": request}, &result)
	if e != nil {
		return "", e
	}
	if result.Retired {
		return result.Replacement, nil
	}
	return "", nil
}

func migrateSessionRefresh(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, `
ALTER TABLE auth.instance ADD COLUMN IF NOT EXISTS personal_source_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS family_id uuid;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS refresh_digest bytea;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS refresh_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS access_expires_at timestamptz;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS idle_expires_at timestamptz;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS absolute_expires_at timestamptz;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS source_context_id text NOT NULL DEFAULT 'personal';
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS source_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE auth.sessions ADD COLUMN IF NOT EXISTS auth_kind text NOT NULL DEFAULT 'legacy';
ALTER TABLE auth.local_auth_requests ADD COLUMN IF NOT EXISTS session_id uuid REFERENCES auth.sessions(id);
ALTER TABLE auth.local_auth_requests ADD COLUMN IF NOT EXISTS family_id uuid;
UPDATE auth.local_auth_requests a SET session_id=s.id,family_id=s.family_id FROM auth.sessions s WHERE a.session_id IS NULL AND a.user_id=s.user_id AND a.device_id=s.device_id AND (SELECT count(*) FROM auth.sessions x WHERE x.user_id=a.user_id AND x.device_id=a.device_id)=1;
CREATE UNIQUE INDEX IF NOT EXISTS session_family ON auth.sessions(family_id) WHERE family_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS session_refresh_digest ON auth.sessions(refresh_digest) WHERE refresh_digest IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS session_family_binding ON auth.sessions(id,family_id);
CREATE TABLE IF NOT EXISTS auth.refresh_token_history(scope_id uuid NOT NULL,session_id uuid NOT NULL REFERENCES auth.sessions(id),family_id uuid NOT NULL,refresh_generation bigint NOT NULL CHECK(refresh_generation>0),token_digest bytea NOT NULL UNIQUE,issued_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,state text NOT NULL CHECK(state IN('active','used','revoked','expired')),consumed_at timestamptz,revoked_at timestamptz,PRIMARY KEY(scope_id,session_id,refresh_generation));
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='auth.refresh_token_history'::regclass AND conname='refresh_session_family_binding') THEN
  ALTER TABLE auth.refresh_token_history ADD CONSTRAINT refresh_session_family_binding FOREIGN KEY(session_id,family_id) REFERENCES auth.sessions(id,family_id);
 END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS refresh_one_active ON auth.refresh_token_history(session_id) WHERE state='active';
CREATE INDEX IF NOT EXISTS refresh_history_family_expiry ON auth.refresh_token_history(scope_id,family_id,expires_at);
CREATE TABLE IF NOT EXISTS auth.refresh_operations(session_id uuid NOT NULL REFERENCES auth.sessions(id),request_id text NOT NULL,refresh_generation bigint NOT NULL,state text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(session_id,request_id));
CREATE TABLE IF NOT EXISTS auth.authentication_retirements(user_id uuid NOT NULL REFERENCES auth.users(id),request_id text NOT NULL,retired_by_request_id text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,request_id));
CREATE TABLE IF NOT EXISTS auth.bff_sessions(id uuid PRIMARY KEY,cookie_digest bytea NOT NULL UNIQUE,user_id uuid NOT NULL,device_id uuid NOT NULL,session_id uuid NOT NULL UNIQUE REFERENCES auth.sessions(id),family_id uuid NOT NULL,refresh_generation bigint NOT NULL,security_version bigint NOT NULL,restore_epoch bigint NOT NULL,source_context_id text NOT NULL,source_generation bigint NOT NULL,upstream_credentials_ciphertext bytea,state text NOT NULL CHECK(state IN('awaiting_browser_confirmation','active','updating','reauth_required','revoked','expired','cleared')),expires_at timestamptz NOT NULL,browser_confirmed_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS auth.session_revocations(session_id uuid PRIMARY KEY REFERENCES auth.sessions(id),user_id uuid NOT NULL,device_id uuid NOT NULL,operation_id text NOT NULL,state text NOT NULL DEFAULT 'pending',created_at timestamptz NOT NULL DEFAULT now());
ALTER TABLE auth.session_revocations ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz NOT NULL DEFAULT '-infinity';
CREATE OR REPLACE FUNCTION auth.revoke_refresh_family() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.revoked_at IS NOT NULL AND NEW.family_id IS NOT NULL THEN
  UPDATE auth.refresh_token_history SET state=CASE WHEN state='used' THEN 'used' ELSE 'revoked' END,revoked_at=COALESCE(revoked_at,NEW.revoked_at) WHERE session_id=NEW.id;
  UPDATE auth.bff_sessions SET state='revoked',upstream_credentials_ciphertext=NULL WHERE session_id=NEW.id;
  UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now()) WHERE session_id=NEW.id OR (session_id IS NULL AND user_id=NEW.user_id AND device_id=NEW.device_id);
  INSERT INTO auth.session_revocations(session_id,user_id,device_id,operation_id) VALUES(NEW.id,NEW.user_id,NEW.device_id,'family:'||NEW.family_id::text) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS revoke_refresh_family ON auth.sessions;
CREATE TRIGGER revoke_refresh_family AFTER UPDATE OF revoked_at ON auth.sessions FOR EACH ROW EXECUTE FUNCTION auth.revoke_refresh_family();
UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()),expires_at=LEAST(expires_at,now()) WHERE family_id IS NULL;
UPDATE auth.local_auth_requests SET delivery_ciphertext=decode('','hex'),expires_at=LEAST(expires_at,now()) WHERE delivery_version<>'refresh-auth-v1';
`)
	return e
}

type sessionMetadata struct {
	FamilyID          string    `json:"familyId"`
	RefreshGeneration string    `json:"refreshGeneration"`
	AccessExpiresAt   time.Time `json:"accessExpiresAt"`
	IdleExpiresAt     time.Time `json:"idleExpiresAt"`
	AbsoluteExpiresAt time.Time `json:"absoluteExpiresAt"`
	SourceContextID   string    `json:"sourceContextId"`
}

type initialAuthDelivery struct {
	Result        authResult `json:"result"`
	BrowserCookie string     `json:"browserCookie,omitempty"`
	SessionID     string     `json:"sessionId"`
}

func (a *App) issueSession(ctx context.Context, tx pgx.Tx, u User, deviceName string, generation, security, epoch int64, web bool) (initialAuthDelivery, error) {
	var delivery initialAuthDelivery
	now := time.Now().UTC().Truncate(time.Microsecond)
	id, family, device := newID(), newID(), newID()
	access, refresh := randomToken(32), randomToken(48)
	meta := sessionMetadata{family, "1", now.Add(accessLifetime), now.Add(refreshIdleLifetime), now.Add(refreshAbsoluteLifetime), "personal"}
	var source int64
	if e := tx.QueryRow(ctx, `SELECT personal_source_generation FROM auth.instance WHERE singleton`).Scan(&source); e != nil {
		return delivery, e
	}
	kind := "native"
	if web {
		kind = "web"
	}
	_, e := tx.Exec(ctx, `INSERT INTO auth.sessions(id,user_id,device_id,device_name,token_digest,generation,expires_at,web_pending,restore_epoch,security_version,family_id,refresh_digest,refresh_generation,access_expires_at,idle_expires_at,absolute_expires_at,source_context_id,source_generation,auth_kind,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,1,$13,$7,$14,'personal',$15,$16,$17)`, id, u.ID, device, deviceName, tokenDigest(access), generation, meta.IdleExpiresAt, web, epoch, security, family, tokenDigest(refresh), meta.AccessExpiresAt, meta.AbsoluteExpiresAt, source, kind, now)
	if e != nil {
		return delivery, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO auth.refresh_token_history(scope_id,session_id,family_id,refresh_generation,token_digest,issued_at,expires_at,state) VALUES($1,$2,$3,1,$4,$5,$6,'active')`, a.Config.DeploymentID, id, family, tokenDigest(refresh), now, meta.IdleExpiresAt); e != nil {
		return delivery, e
	}
	delivery.Result = authResult{Token: access, RefreshToken: refresh, User: u, DeviceID: device, RestoreEpoch: str(epoch), sessionMetadata: meta}
	delivery.SessionID = id
	if web {
		bffID := newID()
		delivery.BrowserCookie = randomToken(40)
		sealed, e := a.sealSessionCredentials(delivery.Result, bffID)
		if e != nil {
			return delivery, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO auth.bff_sessions(id,cookie_digest,user_id,device_id,session_id,family_id,refresh_generation,security_version,restore_epoch,source_context_id,source_generation,upstream_credentials_ciphertext,state,expires_at) VALUES($1,$2,$3,$4,$5,$6,1,$7,$8,'personal',$9,$10,'awaiting_browser_confirmation',$11)`, bffID, tokenDigest(delivery.BrowserCookie), u.ID, device, id, family, security, epoch, source, sealed, meta.IdleExpiresAt)
		if e != nil {
			return delivery, e
		}
	}
	return delivery, nil
}

func (a *App) sealSessionCredentials(result authResult, bffID string) ([]byte, error) {
	data, e := json.Marshal(result)
	if e != nil {
		return nil, e
	}
	return a.seal(string(data), a.Config.DeploymentID+":bff:"+bffID+":"+result.FamilyID+":"+result.RefreshGeneration+":"+result.RestoreEpoch)
}

func (a *App) deliverSession(w http.ResponseWriter, r *http.Request, delivery initialAuthDelivery) error {
	w.Header().Set("Cache-Control", "no-store")
	result := delivery.Result
	result.User.DeviceID, result.User.RestoreEpoch = result.DeviceID, result.RestoreEpoch
	if delivery.BrowserCookie != "" {
		a.setSessionCookie(w, delivery.BrowserCookie, result.IdleExpiresAt)
		result.Token, result.RefreshToken = "", ""
	}
	respond(w, 200, result)
	return nil
}

func (a *App) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	name := "__Host-jobsim_session"
	if a.Config.InsecureLoopback {
		name = "jobsim_dev_session"
	}
	seconds := int(time.Until(expires).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: !a.Config.InsecureLoopback, SameSite: http.SameSiteLaxMode, MaxAge: seconds, Expires: expires})
}

func (a *App) sessionMetadataForDevice(ctx context.Context, user, device string) (sessionMetadata, error) {
	var m sessionMetadata
	e := a.DB.QueryRow(ctx, `SELECT family_id::text,refresh_generation::text,access_expires_at,idle_expires_at,absolute_expires_at,source_context_id FROM auth.sessions WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL`, user, device).Scan(&m.FamilyID, &m.RefreshGeneration, &m.AccessExpiresAt, &m.IdleExpiresAt, &m.AbsoluteExpiresAt, &m.SourceContextID)
	m.AccessExpiresAt, m.IdleExpiresAt, m.AbsoluteExpiresAt = m.AccessExpiresAt.UTC(), m.IdleExpiresAt.UTC(), m.AbsoluteExpiresAt.UTC()
	return m, e
}
