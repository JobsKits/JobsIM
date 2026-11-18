package jobsim

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	schemaFor["sync"] = "sync"
	migrations["sync"] = `CREATE SCHEMA IF NOT EXISTS sync;
CREATE TABLE IF NOT EXISTS sync.origins(origin_event_id uuid PRIMARY KEY,scope_id uuid NOT NULL,owner text NOT NULL,revision bigint NOT NULL CHECK(revision>0),restore_epoch bigint NOT NULL CHECK(restore_epoch>0),ref jsonb NOT NULL,UNIQUE(owner,restore_epoch,revision));
CREATE TABLE IF NOT EXISTS sync.owner_import(owner text PRIMARY KEY,revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0));
CREATE TABLE IF NOT EXISTS sync.streams(id uuid PRIMARY KEY,user_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),restore_epoch bigint NOT NULL,head bigint NOT NULL DEFAULT 0 CHECK(head>=0),UNIQUE(user_id,source_context_id,restore_epoch));
CREATE TABLE IF NOT EXISTS sync.registrations(user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL,store_generation uuid NOT NULL,request_id text NOT NULL,stream_id uuid NOT NULL REFERENCES sync.streams(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,device_id,restore_epoch));
CREATE TABLE IF NOT EXISTS sync.stream_import(stream_id uuid NOT NULL REFERENCES sync.streams(id),owner text NOT NULL,revision bigint NOT NULL DEFAULT 0,PRIMARY KEY(stream_id,owner));
CREATE TABLE IF NOT EXISTS sync.events(stream_id uuid NOT NULL REFERENCES sync.streams(id),position bigint NOT NULL CHECK(position>0),origin_event_id uuid NOT NULL REFERENCES sync.origins(origin_event_id),ref jsonb NOT NULL,event_digest text NOT NULL,PRIMARY KEY(stream_id,position),UNIQUE(stream_id,origin_event_id));
CREATE TABLE IF NOT EXISTS sync.snapshots(id uuid PRIMARY KEY,user_id uuid NOT NULL,device_id uuid NOT NULL,restore_epoch bigint NOT NULL,request_id text NOT NULL,request_digest bytea NOT NULL,manifest jsonb NOT NULL,expires_at timestamptz NOT NULL,UNIQUE(user_id,device_id,restore_epoch,request_id));
CREATE TABLE IF NOT EXISTS sync.snapshot_pages(snapshot_id uuid NOT NULL REFERENCES sync.snapshots(id),page integer NOT NULL,refs_ciphertext bytea NOT NULL,refs_digest text NOT NULL,PRIMARY KEY(snapshot_id,page));
CREATE INDEX IF NOT EXISTS sync_origins_revision ON sync.origins(owner,revision);
CREATE INDEX IF NOT EXISTS sync_snapshot_expiry ON sync.snapshots(expires_at);
ALTER TABLE sync.origins DROP CONSTRAINT IF EXISTS sync_ref_metadata;
ALTER TABLE sync.origins ADD CONSTRAINT sync_ref_metadata CHECK(jsonb_typeof(ref)='object' AND (ref-ARRAY['owner','kind','id','revision','revisionEpoch','subjectUserId','conversationId','targetUserId','state']::text[])='{}'::jsonb);
ALTER TABLE sync.events DROP CONSTRAINT IF EXISTS sync_ref_metadata;
ALTER TABLE sync.events ADD CONSTRAINT sync_ref_metadata CHECK(jsonb_typeof(ref)='object' AND (ref-ARRAY['owner','kind','id','revision','revisionEpoch','subjectUserId','conversationId','targetUserId','state']::text[])='{}'::jsonb);`
}

func syncDigest(raw []byte) string { d := sha256.Sum256(raw); return hex.EncodeToString(d[:]) }
func syncJSONDigest(v any) string  { raw, _ := json.Marshal(v); return syncDigest(raw) }

type syncCursor struct {
	Scope      string `json:"scope"`
	User       string `json:"user"`
	Source     string `json:"source"`
	Device     string `json:"device"`
	Store      string `json:"store"`
	Epoch      string `json:"epoch"`
	Stream     string `json:"stream"`
	Position   string `json:"position"`
	Permission string `json:"permission"`
	Schema     string `json:"schema,omitempty"`
}
type syncRegistration struct {
	Store  string
	Stream string
	Schema string
}
type syncView struct {
	Permission string
	Watermarks map[string]string
	Scopes     []readableScope
}

func (a *App) syncRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("PUT /api/sync/device", a.syncRegister)
	h("GET /api/sync/context", a.syncContext)
	h("POST /api/sync/snapshots", a.syncCreateSnapshot)
	h("GET /api/sync/snapshots/{id}/pages/{page}", a.syncSnapshotPage)
	h("POST /api/sync/snapshots/{id}/validate", a.syncValidateSnapshot)
	h("GET /api/sync/changes", a.syncChanges)
	h("POST /api/sync/classifications", a.syncClassifications)
}
func (a *App) syncUser(r *http.Request) (User, error) {
	if a.Config.NATSURL == "" {
		return User{}, fail(503, "SYNC_NOT_CONFIGURED", "Self-hosted JetStream is required for this synchronization service")
	}
	u, e := a.current(r)
	if e != nil {
		return u, e
	}
	var epoch string
	if e = a.DB.QueryRow(r.Context(), `SELECT restore_epoch::text FROM sync.instance WHERE singleton`).Scan(&epoch); e != nil {
		return u, e
	}
	if epoch != u.RestoreEpoch {
		return u, fail(409, "RESTORE_EPOCH_CHANGED", "Owner restore epochs do not agree")
	}
	return u, nil
}
func (a *App) syncRegister(w http.ResponseWriter, r *http.Request) error {
	u, e := a.syncUser(r)
	if e != nil {
		return e
	}
	var q struct {
		RequestID string `json:"requestId"`
		Store     string `json:"storeGeneration"`
		Schema    string `json:"schemaVersion,omitempty"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if !syncValidationID(q.Store) || len(q.RequestID) < 8 || len(q.RequestID) > 128 {
		return fail(400, "INVALID_SYNC_DEVICE", "Stable request and store generation required")
	}
	schema, e := syncProtocolSchema(q.Schema)
	if e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), `INSERT INTO sync.device_stores(scope_id,source_context_id,user_id,device_id,restore_epoch,store_generation) VALUES($1,'personal',$2,$3,$4,$5) ON CONFLICT DO NOTHING`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch, q.Store); e != nil {
		return e
	}
	var physicalStore string
	if e = tx.QueryRow(r.Context(), `SELECT store_generation::text FROM sync.device_stores WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND device_id=$3 AND restore_epoch=$4 FOR UPDATE`, a.Config.DeploymentID, u.ID, u.DeviceID, u.RestoreEpoch).Scan(&physicalStore); e != nil {
		return e
	}
	if physicalStore != q.Store {
		return fail(409, "STORE_GENERATION_CHANGED", "A protocol schema cannot replace the current physical local store")
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO sync.streams(id,user_id,source_context_id,restore_epoch,schema_version) VALUES($1,$2,'personal',$3,$4) ON CONFLICT(user_id,source_context_id,restore_epoch,schema_version) DO NOTHING`, newID(), u.ID, u.RestoreEpoch, schema); e != nil {
		return e
	}
	var stream string
	if e = tx.QueryRow(r.Context(), `SELECT id::text FROM sync.streams WHERE user_id=$1 AND source_context_id='personal' AND restore_epoch=$2 AND schema_version=$3`, u.ID, u.RestoreEpoch, schema).Scan(&stream); e != nil {
		return e
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO sync.registrations(user_id,device_id,restore_epoch,store_generation,request_id,stream_id,schema_version) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, u.ID, u.DeviceID, u.RestoreEpoch, q.Store, q.RequestID, stream, schema); e != nil {
		return e
	}
	var store string
	if e = tx.QueryRow(r.Context(), `SELECT store_generation::text FROM sync.registrations WHERE user_id=$1 AND device_id=$2 AND restore_epoch=$3 AND schema_version=$4`, u.ID, u.DeviceID, u.RestoreEpoch, schema).Scan(&store); e != nil {
		return e
	}
	if store != q.Store {
		return fail(409, "STORE_GENERATION_CHANGED", "An empty or replaced local store requires a newly authenticated device identity")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	response := map[string]any{"scopeId": a.Config.DeploymentID, "sourceContextId": "personal", "principalId": u.ID, "deviceId": u.DeviceID, "storeGeneration": store, "restoreEpoch": u.RestoreEpoch, "streamId": stream, "qualification": "transport_only", "qualifiedAck": false}
	if schema == "2" {
		response["schemaVersion"] = schema
	}
	respond(w, 200, response)
	return nil
}
func (a *App) syncRegistered(ctx context.Context, u User, requested ...string) (syncRegistration, error) {
	var v syncRegistration
	schema := ""
	if len(requested) > 0 {
		schema = requested[0]
	}
	var e error
	v.Schema, e = syncProtocolSchema(schema)
	if e != nil {
		return v, e
	}
	e = a.DB.QueryRow(ctx, `SELECT r.store_generation::text,r.stream_id::text FROM sync.registrations r JOIN sync.streams s ON s.id=r.stream_id JOIN sync.device_stores d ON d.scope_id=$5 AND d.source_context_id='personal' AND d.user_id=r.user_id AND d.device_id=r.device_id AND d.restore_epoch=r.restore_epoch AND d.store_generation=r.store_generation WHERE r.user_id=$1 AND r.device_id=$2 AND r.restore_epoch=$3 AND r.schema_version=$4 AND s.user_id=r.user_id AND s.restore_epoch=r.restore_epoch AND s.source_context_id='personal' AND s.schema_version=r.schema_version`, u.ID, u.DeviceID, u.RestoreEpoch, v.Schema, a.Config.DeploymentID).Scan(&v.Store, &v.Stream)
	if e == pgx.ErrNoRows {
		return v, fail(409, "SYNC_DEVICE_REQUIRED", "Register the current physical local store before synchronization")
	}
	return v, e
}
func (a *App) syncCurrentView(ctx context.Context, u User, schema ...string) (syncView, error) {
	v := syncView{Watermarks: map[string]string{}, Scopes: []readableScope{}}
	permissions := map[string]string{}
	selected := "1"
	if len(schema) > 0 && schema[0] == "2" {
		selected = "2"
	}
	for _, owner := range syncSchemaOwners(selected) {
		var capture syncOwnerCapture
		if e := a.rpc(ctx, syncOwnerRole(owner), "/internal/sync/watermark", syncOwnerRequest{UserID: u.ID}, &capture); e != nil {
			return v, e
		}
		if capture.RestoreEpoch != u.RestoreEpoch {
			return v, fail(409, "RESTORE_EPOCH_CHANGED", "Owner restore epochs do not agree")
		}
		v.Watermarks[owner] = capture.Watermark
		if owner != "message" && owner != "sync" {
			permissions[owner] = capture.PermissionVersion
		}
	}
	var scopes struct {
		Items []readableScope `json:"items"`
	}
	if e := a.rpc(ctx, "conversation", "/internal/read-scopes", map[string]string{"userId": u.ID}, &scopes); e != nil {
		return v, e
	}
	v.Scopes = scopes.Items
	v.Permission = syncJSONDigest(permissions)
	return v, nil
}
func (a *App) syncContext(w http.ResponseWriter, r *http.Request) error {
	u, e := a.syncUser(r)
	if e != nil {
		return e
	}
	reg, e := a.syncRegistered(r.Context(), u, r.URL.Query().Get("schemaVersion"))
	if e != nil {
		return e
	}
	view, e := a.syncCurrentView(r.Context(), u, reg.Schema)
	if e != nil {
		return e
	}
	var head string
	if e = a.DB.QueryRow(r.Context(), `SELECT head::text FROM sync.streams WHERE id=$1`, reg.Stream).Scan(&head); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"scopeId": a.Config.DeploymentID, "sourceContextId": "personal", "principalId": u.ID, "deviceId": u.DeviceID, "storeGeneration": reg.Store, "restoreEpoch": u.RestoreEpoch, "streamId": reg.Stream, "head": head, "minimumPosition": "0", "permissionGeneration": view.Permission, "authorizedScopes": view.Scopes, "snapshotRequired": true, "qualification": "transport_only", "qualifiedAck": false, "afterReceiptEnabled": false, "transport": map[string]any{"kind": "nats_jetstream", "connected": a.syncTransportConnected.Load(), "httpReplay": "compensation"}, "coverage": syncCoverage(reg.Schema)})
	return nil
}
func syncCoverage(schema ...string) map[string]any {
	if len(schema) > 0 && schema[0] == "2" {
		return map[string]any{"included": []string{"own_account", "own_device_metadata", "own_profile", "own_contacts_and_relationships", "own_contact_requests", "current_conversations", "current_member_ids", "authorized_messages_and_interactions", "historically_entitled_global_tombstones", "announcements", "attachment_references", "own_font_preference", "own_conversation_views", "own_read_points", "own_read_observation_metadata", "own_text_drafts"}, "excluded": []string{"exact_unread_counts", "cloud_attachment_upload_queue", "attachment_bytes", "qualified_ack", "ordinary_local_migration", "formal_export_package", "account_deletion", "peer_public_read_receipts", "peer_presence"}, "classification": "explicit_known_only", "absence": "unknown", "schemaVersion": "2"}
	}
	return map[string]any{"included": []string{"own_account", "own_device_metadata", "own_profile", "own_contacts_and_relationships", "own_contact_requests", "current_conversations", "current_member_ids", "authorized_messages_and_interactions", "historically_entitled_global_tombstones", "announcements", "attachment_references"}, "excluded": []string{"server_read_state", "cloud_drafts", "server_preferences", "attachment_bytes", "qualified_ack", "ordinary_local_migration", "formal_export_package", "account_deletion"}, "classification": "explicit_known_only", "absence": "unknown", "schemaVersion": "1"}
}
func (a *App) syncEncodeCursor(u User, reg syncRegistration, position, permission string) string {
	v := syncCursor{Scope: a.Config.DeploymentID, User: u.ID, Source: "personal", Device: u.DeviceID, Store: reg.Store, Epoch: u.RestoreEpoch, Stream: reg.Stream, Position: position, Permission: permission}
	if reg.Schema == "2" {
		v.Schema = reg.Schema
	}
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(keyedDigest(a.Config.Key, "sync-cursor-v1", raw))
}
func (a *App) syncDecodeCursor(raw string, u User, reg syncRegistration) (syncCursor, error) {
	var v syncCursor
	left, right, ok := strings.Cut(raw, ".")
	if !ok || len(raw) > 4096 {
		return v, fail(400, "INVALID_SYNC_CURSOR", "Signed synchronization boundary required")
	}
	b, e := base64.RawURLEncoding.DecodeString(left)
	if e != nil {
		return v, fail(400, "INVALID_SYNC_CURSOR", "Invalid synchronization boundary")
	}
	sig, e := base64.RawURLEncoding.DecodeString(right)
	if e != nil || !hmac.Equal(sig, keyedDigest(a.Config.Key, "sync-cursor-v1", b)) || json.Unmarshal(b, &v) != nil {
		return v, fail(400, "INVALID_SYNC_CURSOR", "Invalid synchronization boundary")
	}
	if v.Epoch != u.RestoreEpoch {
		return v, fail(409, "RESTORE_EPOCH_CHANGED", "Boundary belongs to an older restore epoch")
	}
	if v.Scope != a.Config.DeploymentID || v.User != u.ID || v.Source != "personal" || v.Device != u.DeviceID || v.Store != reg.Store || v.Stream != reg.Stream {
		return v, fail(409, "SYNC_CONTEXT_CHANGED", "Boundary belongs to another principal, source, device, or store")
	}
	schema, schemaError := syncProtocolSchema(v.Schema)
	registeredSchema, registrationError := syncProtocolSchema(reg.Schema)
	if schemaError != nil || registrationError != nil || schema != registeredSchema {
		return v, fail(409, "SYNC_CONTEXT_CHANGED", "Boundary belongs to another synchronization protocol stream")
	}
	n, e := strconv.ParseInt(v.Position, 10, 64)
	if e != nil || n < 0 {
		return v, fail(400, "INVALID_SYNC_CURSOR", "Invalid exact position")
	}
	return v, nil
}

func (a *App) syncCapture(ctx context.Context, u User, schema ...string) ([]syncRef, map[string]string, string, error) {
	selected := "1"
	if len(schema) > 0 && schema[0] == "2" {
		selected = "2"
	}
	for attempt := 0; attempt < 3; attempt++ {
		refs := []syncRef{}
		watermarks := map[string]string{}
		permissions := map[string]string{}
		stable := true
		for _, owner := range syncSchemaOwners(selected) {
			q := syncOwnerRequest{UserID: u.ID}
			if len(schema) > 0 && schema[0] == "2" {
				q.Schema = "2"
			}
			for {
				var page struct {
					Capture syncOwnerCapture `json:"capture"`
					HasMore bool             `json:"hasMore"`
				}
				if e := a.rpc(ctx, syncOwnerRole(owner), "/internal/sync/capture", q, &page); e != nil {
					return nil, nil, "", e
				}
				v := page.Capture
				if v.RestoreEpoch != u.RestoreEpoch {
					return nil, nil, "", fail(409, "RESTORE_EPOCH_CHANGED", "Owner restore epochs do not agree")
				}
				if prior, ok := watermarks[owner]; ok && prior != v.Watermark {
					stable = false
					break
				}
				watermarks[owner] = v.Watermark
				if owner != "message" && owner != "sync" {
					permissions[owner] = v.PermissionVersion
				}
				refs = append(refs, v.Refs...)
				if len(refs) > 200000 {
					return nil, nil, "", fail(503, "SYNC_CAPACITY_EXCEEDED", "Snapshot exceeds the configured basic resource bound; no incomplete manifest was published")
				}
				if !page.HasMore {
					break
				}
				if len(v.Refs) == 0 {
					return nil, nil, "", fail(503, "SYNC_OWNER_INCOMPLETE", "Owner capture did not advance")
				}
				last := v.Refs[len(v.Refs)-1]
				q.Kind, q.ID = last.Kind, last.ID
			}
			if !stable {
				break
			}
		}
		if !stable {
			continue
		}
		// Every capture finishes before any final read. Equal monotonic clocks
		// prove a common committed cut across independent owner databases.
		for _, owner := range syncSchemaOwners(selected) {
			var now syncOwnerCapture
			if e := a.rpc(ctx, syncOwnerRole(owner), "/internal/sync/watermark", syncOwnerRequest{UserID: u.ID}, &now); e != nil {
				return nil, nil, "", e
			}
			if now.RestoreEpoch != u.RestoreEpoch {
				return nil, nil, "", fail(409, "RESTORE_EPOCH_CHANGED", "Owner restore epochs do not agree")
			}
			if now.Watermark != watermarks[owner] || owner != "message" && owner != "sync" && now.PermissionVersion != permissions[owner] {
				stable = false
				break
			}
		}
		if stable {
			return refs, watermarks, syncJSONDigest(permissions), nil
		}
	}
	return nil, nil, "", fail(503, "SYNC_SNAPSHOT_BUSY", "Owners changed during capture; retain the current local store and retry the original request")
}

func (a *App) syncCreateSnapshot(w http.ResponseWriter, r *http.Request) error {
	u, e := a.syncUser(r)
	if e != nil {
		return e
	}
	var q struct {
		RequestID string `json:"requestId"`
		Store     string `json:"storeGeneration"`
		Schema    string `json:"schemaVersion"`
		PageSize  int    `json:"pageSize"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if q.Schema != "1" && q.Schema != "2" || len(q.RequestID) < 8 || len(q.RequestID) > 128 || q.PageSize < 1 || q.PageSize > 100 {
		return fail(400, "INVALID_SNAPSHOT_REQUEST", "Original request, current store, explicit schema 1 or 2, and pageSize 1–100 required")
	}
	reg, e := a.syncRegistered(r.Context(), u, q.Schema)
	if e != nil {
		return e
	}
	if q.Store != reg.Store {
		return fail(400, "INVALID_SNAPSHOT_REQUEST", "Original request must use the current registered physical store")
	}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "sync-snapshot-request-v1", raw)
	if original, found, er := a.syncOriginalSnapshot(r.Context(), u, q.RequestID, digest); er != nil {
		return er
	} else if found {
		respond(w, 200, original)
		return nil
	}
	// Keep the personal stream boundary fixed throughout capture. Another
	// device must not advance this snapshot past objects absent from its cut.
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var boundary string
	if e = tx.QueryRow(r.Context(), `SELECT head::text FROM sync.streams WHERE id=$1 FOR UPDATE`, reg.Stream).Scan(&boundary); e != nil {
		return e
	}
	refs, watermarks, permission, e := a.syncCapture(r.Context(), u, reg.Schema)
	if e != nil {
		return e
	}
	view, e := a.syncCurrentView(r.Context(), u, reg.Schema)
	if e != nil {
		return e
	}
	if view.Permission != permission {
		return fail(409, "SNAPSHOT_INVALIDATED", "Permission changed during capture; retain existing and staging data")
	}
	if e = a.syncImportOwners(r.Context(), u); e != nil {
		return e
	}
	if e = a.syncAllocateTx(r.Context(), tx, u, reg, view, watermarks); e != nil {
		return e
	}
	if e = tx.QueryRow(r.Context(), `SELECT head::text FROM sync.streams WHERE id=$1`, reg.Stream).Scan(&boundary); e != nil {
		return e
	}
	v := syncSnapshot{ID: newID(), ScopeID: a.Config.DeploymentID, PrincipalID: u.ID, SourceContextID: "personal", DeviceID: u.DeviceID, StoreGeneration: reg.Store, RestoreEpoch: u.RestoreEpoch, StreamID: reg.Stream, SchemaVersion: reg.Schema, Boundary: boundary, BoundaryToken: a.syncEncodeCursor(u, reg, boundary, permission), OwnerWatermarks: watermarks, PermissionGeneration: permission, ClassificationWatermark: watermarks["message"], Coverage: syncCoverage(reg.Schema), PageManifest: []syncPageManifest{}, ExpiresAt: time.Now().UTC().Add(1800 * time.Second)}
	if len(refs) == 0 {
		refs = []syncRef{}
	}
	pages := [][]syncRef{}
	for i := 0; i < len(refs); i += q.PageSize {
		end := i + q.PageSize
		if end > len(refs) {
			end = len(refs)
		}
		pages = append(pages, refs[i:end])
	}
	if len(pages) == 0 {
		pages = append(pages, []syncRef{})
	}
	for i, page := range pages {
		v.PageManifest = append(v.PageManifest, syncPageManifest{Page: i, Count: len(page), Digest: syncJSONDigest(page)})
	}
	manifest, _ := json.Marshal(v)
	var inserted string
	e = tx.QueryRow(r.Context(), `INSERT INTO sync.snapshots(id,user_id,device_id,restore_epoch,request_id,request_digest,manifest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING RETURNING id::text`, v.ID, u.ID, u.DeviceID, u.RestoreEpoch, q.RequestID, digest, manifest, v.ExpiresAt).Scan(&inserted)
	if e == pgx.ErrNoRows {
		tx.Rollback(r.Context())
		original, _, er := a.syncOriginalSnapshot(r.Context(), u, q.RequestID, digest)
		if er != nil {
			return er
		}
		respond(w, 200, original)
		return nil
	}
	if e != nil {
		return e
	}
	for i, page := range pages {
		b, _ := json.Marshal(page)
		cipher, er := a.seal(string(b), "sync-refs:"+v.ID+":"+strconv.Itoa(i))
		if er != nil {
			return er
		}
		if _, er = tx.Exec(r.Context(), `INSERT INTO sync.snapshot_pages(snapshot_id,page,refs_ciphertext,refs_digest) VALUES($1,$2,$3,$4)`, v.ID, i, cipher, v.PageManifest[i].Digest); er != nil {
			return er
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 201, v)
	return nil
}
func (a *App) syncOriginalSnapshot(ctx context.Context, u User, request string, digest []byte) (syncSnapshot, bool, error) {
	var v syncSnapshot
	var old, manifest []byte
	var expires time.Time
	e := a.DB.QueryRow(ctx, `SELECT request_digest,manifest,expires_at FROM sync.snapshots WHERE user_id=$1 AND device_id=$2 AND restore_epoch=$3 AND request_id=$4`, u.ID, u.DeviceID, u.RestoreEpoch, request).Scan(&old, &manifest, &expires)
	if e == pgx.ErrNoRows {
		return v, false, nil
	}
	if e != nil {
		return v, false, e
	}
	if !hmac.Equal(digest, old) {
		return v, true, fail(409, "IDEMPOTENCY_CONFLICT", "Original snapshot request body changed")
	}
	if !expires.After(time.Now()) {
		return v, true, fail(410, "SNAPSHOT_EXPIRED", "Original snapshot expired; retain local and staging data, then explicitly request a new snapshot")
	}
	e = json.Unmarshal(manifest, &v)
	return v, true, e
}
func (a *App) syncSnapshotPage(w http.ResponseWriter, r *http.Request) error {
	u, e := a.syncUser(r)
	if e != nil {
		return e
	}
	page, e := strconv.Atoi(r.PathValue("page"))
	if e != nil || page < 0 || !validObjectID(r.PathValue("id")) {
		return fail(400, "INVALID_SNAPSHOT_PAGE", "Valid snapshot and page required")
	}
	var raw, cipher []byte
	var expected string
	var expires time.Time
	e = a.DB.QueryRow(r.Context(), `SELECT s.manifest,p.refs_ciphertext,p.refs_digest,s.expires_at FROM sync.snapshots s JOIN sync.snapshot_pages p ON p.snapshot_id=s.id WHERE s.id=$1 AND s.user_id=$2 AND s.device_id=$3 AND s.restore_epoch=$4 AND p.page=$5`, r.PathValue("id"), u.ID, u.DeviceID, u.RestoreEpoch, page).Scan(&raw, &cipher, &expected, &expires)
	if e == pgx.ErrNoRows {
		return fail(404, "SNAPSHOT_PAGE_NOT_FOUND", "Snapshot page unavailable; do not install a partial baseline")
	}
	if e != nil {
		return e
	}
	if !expires.After(time.Now()) {
		return fail(410, "SNAPSHOT_EXPIRED", "Snapshot expired; retain old and staging data")
	}
	var manifest syncSnapshot
	if e = json.Unmarshal(raw, &manifest); e != nil {
		return e
	}
	reg, e := a.syncRegistered(r.Context(), u, manifest.SchemaVersion)
	if e != nil {
		return e
	}
	if manifest.StoreGeneration != reg.Store || manifest.StreamID != reg.Stream {
		return fail(409, "STORE_GENERATION_CHANGED", "Snapshot belongs to another local store")
	}
	view, e := a.syncCurrentView(r.Context(), u, reg.Schema)
	if e != nil {
		return e
	}
	if view.Permission != manifest.PermissionGeneration {
		return fail(409, "SNAPSHOT_INVALIDATED", "Permission changed; restrict affected cloud content and retain data classifications")
	}
	plain, e := a.unseal(cipher, "sync-refs:"+manifest.ID+":"+strconv.Itoa(page))
	if e != nil || syncDigest([]byte(plain)) != expected {
		return fail(503, "SNAPSHOT_INTEGRITY_FAILED", "Stored reference page failed integrity verification")
	}
	var refs []syncRef
	if e = json.Unmarshal([]byte(plain), &refs); e != nil {
		return e
	}
	items := []syncProjection{}
	installable := true
	for _, ref := range refs {
		projection, er := a.syncProject(r.Context(), u, ref)
		if er != nil {
			return er
		}
		items = append(items, projection)
		if !projection.RevisionMatch || projection.State == "unknown" || projection.State == "redacted" {
			installable = false
		}
	}
	respond(w, 200, map[string]any{"snapshotId": manifest.ID, "page": page, "refs": refs, "referenceEnvelope": base64.RawURLEncoding.EncodeToString([]byte(plain)), "pageDigest": expected, "items": items, "permissionGeneration": view.Permission, "restoreEpoch": u.RestoreEpoch, "digestScope": "immutable_references_only", "installable": installable, "requiresFinalValidation": true, "requiresAtomicInstall": true, "qualifiedAck": false})
	return nil
}
func (a *App) syncProject(ctx context.Context, u User, ref syncRef) (syncProjection, error) {
	var projection syncProjection
	e := a.rpc(ctx, syncOwnerRole(ref.Owner), "/internal/sync/hydrate", syncOwnerRequest{UserID: u.ID, Ref: ref}, &projection)
	return projection, e
}

func (a *App) syncImportOwners(ctx context.Context, u User) error {
	for _, owner := range syncSchemaOwners("2") {
		if _, e := a.DB.Exec(ctx, `INSERT INTO sync.owner_import(owner) VALUES($1) ON CONFLICT DO NOTHING`, owner); e != nil {
			return e
		}
		for {
			tx, e := a.DB.Begin(ctx)
			if e != nil {
				return e
			}
			var after string
			e = tx.QueryRow(ctx, `SELECT revision::text FROM sync.owner_import WHERE owner=$1 FOR UPDATE`, owner).Scan(&after)
			if e != nil {
				tx.Rollback(ctx)
				return e
			}
			var page struct {
				Items     []syncWireEvent `json:"items"`
				Watermark string          `json:"watermark"`
				Epoch     string          `json:"restoreEpoch"`
			}
			e = a.rpc(ctx, syncOwnerRole(owner), "/internal/sync/events", syncOwnerRequest{UserID: u.ID, After: after}, &page)
			if e != nil {
				tx.Rollback(ctx)
				return e
			}
			if page.Epoch != u.RestoreEpoch {
				tx.Rollback(ctx)
				return fail(409, "RESTORE_EPOCH_CHANGED", "Owner restore epochs do not agree")
			}
			next := after
			for _, event := range page.Items {
				if event.Owner != owner || event.ScopeID != a.Config.DeploymentID {
					tx.Rollback(ctx)
					return fail(503, "SYNC_OWNER_MISMATCH", "Owner event scope mismatch")
				}
				if e = a.syncStoreOrigin(ctx, tx, event); e != nil {
					tx.Rollback(ctx)
					return e
				}
				previous, _ := strconv.ParseInt(next, 10, 64)
				current, er := strconv.ParseInt(event.Revision, 10, 64)
				if er != nil || current != previous+1 {
					tx.Rollback(ctx)
					return fail(503, "SYNC_OWNER_INCOMPLETE", "Owner committed pointer range has a gap; source progress was retained")
				}
				next = event.Revision
			}
			if len(page.Items) < 500 && next != page.Watermark {
				tx.Rollback(ctx)
				return fail(503, "SYNC_OWNER_INCOMPLETE", "Owner committed watermark has a missing reference tail")
			}
			if _, e = tx.Exec(ctx, `UPDATE sync.owner_import SET revision=$2 WHERE owner=$1`, owner, next); e != nil {
				tx.Rollback(ctx)
				return e
			}
			if e = tx.Commit(ctx); e != nil {
				return e
			}
			if len(page.Items) < 500 {
				break
			}
		}
	}
	return nil
}
func syncRelevant(ref syncRef, user string, scopes map[string]bool) bool {
	switch ref.Owner {
	case "auth":
		return ref.SubjectUserID == user
	case "directory":
		return ref.SubjectUserID == user || ref.Kind == "contact_request" && ref.TargetUserID == user
	case "conversation":
		return scopes[ref.ConversationID] || ref.TargetUserID == user
	case "message":
		return scopes[ref.ConversationID]
	case "sync":
		return ref.SubjectUserID == user
	}
	return false
}
func (a *App) syncAllocate(ctx context.Context, u User, reg syncRegistration, view syncView, watermarks map[string]string) error {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = a.syncAllocateTx(ctx, tx, u, reg, view, watermarks); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (a *App) syncAllocateTx(ctx context.Context, tx pgx.Tx, u User, reg syncRegistration, view syncView, watermarks map[string]string) error {
	scopes := map[string]bool{}
	for _, scope := range view.Scopes {
		scopes[scope.ID] = true
	}
	var head int64
	e := tx.QueryRow(ctx, `SELECT head FROM sync.streams WHERE id=$1 FOR UPDATE`, reg.Stream).Scan(&head)
	if e != nil {
		return e
	}
	for _, owner := range syncSchemaOwners(reg.Schema) {
		if _, e = tx.Exec(ctx, `INSERT INTO sync.stream_import(stream_id,owner) VALUES($1,$2) ON CONFLICT DO NOTHING`, reg.Stream, owner); e != nil {
			return e
		}
		var after int64
		if e = tx.QueryRow(ctx, `SELECT revision FROM sync.stream_import WHERE stream_id=$1 AND owner=$2`, reg.Stream, owner).Scan(&after); e != nil {
			return e
		}
		rows, er := tx.Query(ctx, `SELECT origin_event_id::text,ref FROM sync.origins WHERE owner=$1 AND restore_epoch=$2 AND revision>$3 AND revision<=$4 ORDER BY revision`, owner, u.RestoreEpoch, after, watermarks[owner])
		if er != nil {
			return er
		}
		type candidate struct {
			id  string
			raw []byte
		}
		items := []candidate{}
		for rows.Next() {
			var c candidate
			if er = rows.Scan(&c.id, &c.raw); er != nil {
				rows.Close()
				return er
			}
			items = append(items, c)
		}
		er = rows.Err()
		rows.Close()
		if er != nil {
			return er
		}
		for _, c := range items {
			var ref syncRef
			if er = json.Unmarshal(c.raw, &ref); er != nil {
				return er
			}
			if !syncSchemaIncludes(reg.Schema, ref.Kind) {
				continue
			}
			if !syncRelevant(ref, u.ID, scopes) {
				// A former member still needs a global deletion classification for
				// a legitimately retained local copy, without regaining cloud read.
				if ref.Owner != "message" || ref.State != "removed" {
					continue
				}
				p, er := a.syncProject(ctx, u, ref)
				if er != nil {
					return er
				}
				if p.State != "global" {
					continue
				}
			}
			var exists bool
			if er = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.events WHERE stream_id=$1 AND origin_event_id=$2)`, reg.Stream, c.id).Scan(&exists); er != nil {
				return er
			}
			if exists {
				continue
			}
			head++
			eventDigest := syncJSONDigest(map[string]any{"position": str(head), "originEventId": c.id, "ref": ref})
			if _, er = tx.Exec(ctx, `INSERT INTO sync.events(stream_id,position,origin_event_id,ref,event_digest) VALUES($1,$2,$3,$4,$5)`, reg.Stream, head, c.id, c.raw, eventDigest); er != nil {
				return er
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE sync.stream_import SET revision=GREATEST(revision,$3) WHERE stream_id=$1 AND owner=$2`, reg.Stream, owner, watermarks[owner]); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE sync.streams SET head=$2 WHERE id=$1`, reg.Stream, head); e != nil {
		return e
	}
	return nil
}
func (a *App) syncChanges(w http.ResponseWriter, r *http.Request) error {
	u, e := a.syncUser(r)
	if e != nil {
		return e
	}
	reg, e := a.syncRegistered(r.Context(), u, r.URL.Query().Get("schemaVersion"))
	if e != nil {
		return e
	}
	cursor, e := a.syncDecodeCursor(r.URL.Query().Get("cursor"), u, reg)
	if e != nil {
		return e
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, e = strconv.Atoi(v)
		if e != nil || limit < 1 || limit > 500 {
			return fail(400, "INVALID_LIMIT", "Limit must be 1–500")
		}
	}
	view, e := a.syncCurrentView(r.Context(), u, reg.Schema)
	if e != nil {
		return e
	}
	if e = a.syncImportOwners(r.Context(), u); e != nil {
		return e
	}
	if e = a.syncAllocate(r.Context(), u, reg, view, view.Watermarks); e != nil {
		return e
	}
	var head int64
	if e = a.DB.QueryRow(r.Context(), `SELECT head FROM sync.streams WHERE id=$1`, reg.Stream).Scan(&head); e != nil {
		return e
	}
	position, _ := strconv.ParseInt(cursor.Position, 10, 64)
	if position > head {
		return fail(409, "SYNC_CURSOR_AHEAD", "Boundary is ahead of the authoritative stream")
	}
	rows, e := a.DB.Query(r.Context(), `SELECT position,origin_event_id::text,ref,event_digest FROM sync.events WHERE stream_id=$1 AND position>$2 ORDER BY position LIMIT $3`, reg.Stream, position, limit)
	if e != nil {
		return e
	}
	type event struct {
		position int64
		id       string
		raw      []byte
		digest   string
	}
	events := []event{}
	for rows.Next() {
		var v event
		if e = rows.Scan(&v.position, &v.id, &v.raw, &v.digest); e != nil {
			rows.Close()
			return e
		}
		events = append(events, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	items := []map[string]any{}
	redacted := []string{}
	var from, to int64
	flush := func() {
		if len(redacted) > 0 {
			items = append(items, map[string]any{"kind": "redacted", "from": str(from), "to": str(to), "rangeDigest": syncJSONDigest(redacted)})
			redacted = nil
		}
	}
	for _, v := range events {
		var ref syncRef
		if e = json.Unmarshal(v.raw, &ref); e != nil {
			return e
		}
		projection, er := a.syncProject(r.Context(), u, ref)
		if er != nil {
			return er
		}
		position = v.position
		if projection.State == "redacted" {
			if len(redacted) == 0 {
				from = v.position
			}
			to = v.position
			redacted = append(redacted, v.digest)
			continue
		}
		flush()
		envelope, _ := json.Marshal(map[string]any{"position": str(v.position), "originEventId": v.id, "ref": ref})
		if syncDigest(envelope) != v.digest {
			return fail(503, "SYNC_EVENT_INTEGRITY_FAILED", "Immutable event envelope failed integrity verification")
		}
		items = append(items, map[string]any{"kind": "reference", "position": str(v.position), "originEventId": v.id, "eventDigest": v.digest, "eventEnvelope": base64.RawURLEncoding.EncodeToString(envelope), "ref": ref, "projection": projection})
	}
	flush()
	respond(w, 200, map[string]any{"items": items, "cursor": a.syncEncodeCursor(u, reg, str(position), cursor.Permission), "position": str(position), "head": str(head), "hasMore": position < head, "restoreEpoch": u.RestoreEpoch, "permissionGeneration": view.Permission, "authorizedScopes": view.Scopes, "snapshotRequired": cursor.Permission != view.Permission, "qualifiedAck": false})
	return nil
}
func (a *App) syncClassifications(w http.ResponseWriter, r *http.Request) error {
	u, e := a.syncUser(r)
	if e != nil {
		return e
	}
	if _, e = a.syncRegistered(r.Context(), u, r.URL.Query().Get("schemaVersion")); e != nil {
		return e
	}
	var q struct {
		IDs []string `json:"messageIds"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.IDs) < 1 || len(q.IDs) > 100 {
		return fail(400, "INVALID_CLASSIFICATION_REQUEST", "Supply 1–100 local message identities")
	}
	items := []map[string]any{}
	for _, id := range q.IDs {
		if !validObjectID(id) {
			return fail(400, "INVALID_MESSAGE_ID", "Stable message identity required")
		}
		projection, er := a.syncProject(r.Context(), u, syncRef{Owner: "message", Kind: "message", ID: id, Revision: "0", RevisionEpoch: u.RestoreEpoch, State: "active"})
		if er != nil {
			return er
		}
		classification := "unknown"
		if projection.State == "global" {
			classification = "global"
		} else if projection.State == "available" {
			classification = "available"
		}
		items = append(items, map[string]any{"messageId": id, "classification": classification, "restoreEpoch": u.RestoreEpoch})
	}
	respond(w, 200, map[string]any{"items": items, "missing": "unknown", "cloudOnlyInferred": false})
	return nil
}
