package jobsim

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type syncOwnerRequest struct {
	UserID string  `json:"userId"`
	Schema string  `json:"schemaVersion,omitempty"`
	After  string  `json:"after,omitempty"`
	Kind   string  `json:"kind,omitempty"`
	ID     string  `json:"id,omitempty"`
	Ref    syncRef `json:"ref,omitempty"`
}

func (a *App) syncOwnerRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	for _, path := range []string{"capture", "watermark", "events", "hydrate"} {
		p := path
		h("POST /internal/sync/"+p, internal(func(w http.ResponseWriter, r *http.Request) error {
			if r.Header.Get("X-JobsIM-Caller") != "sync" {
				return fail(403, "SYNC_SERVICE_REQUIRED", "Synchronization owner protocol is private")
			}
			var q syncOwnerRequest
			if e := decode(r, &q); e != nil {
				return e
			}
			if !validObjectID(q.UserID) {
				return fail(400, "INVALID_PRINCIPAL", "Current source principal required")
			}
			schema, schemaError := syncProtocolSchema(q.Schema)
			if schemaError != nil {
				return schemaError
			}
			if p == "hydrate" {
				v, e := a.syncHydrate(r, q.UserID, q.Ref)
				if e != nil {
					return e
				}
				respond(w, 200, v)
				return nil
			}
			owner := syncOwnerSchema(a.Role)
			tx, e := a.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if e != nil {
				return e
			}
			defer tx.Rollback(r.Context())
			v := syncOwnerCapture{Owner: owner, Refs: []syncRef{}}
			if e = tx.QueryRow(r.Context(), "SELECT i.restore_epoch::text,c.revision::text,COALESCE((SELECT version FROM "+owner+".sync_permission_versions WHERE user_id=$1),0)::text FROM "+owner+".instance i CROSS JOIN "+owner+".sync_clock c WHERE i.singleton AND c.singleton", q.UserID).Scan(&v.RestoreEpoch, &v.Watermark, &v.PermissionVersion); e != nil {
				return e
			}
			if p == "watermark" {
				respond(w, 200, v)
				return nil
			}
			if p == "events" {
				after, er := strconv.ParseInt(q.After, 10, 64)
				if er != nil || after < 0 {
					return fail(400, "INVALID_CURSOR", "Owner event position required")
				}
				rows, er := tx.Query(r.Context(), "SELECT origin_event_id::text,revision::text,restore_epoch::text,kind,object_id::text,COALESCE(subject_user_id::text,''),COALESCE(conversation_id::text,''),COALESCE(target_user_id::text,''),revision_epoch::text,state FROM "+owner+".sync_outbox WHERE revision>$1 ORDER BY "+owner+".sync_outbox.revision LIMIT 500", after)
				if er != nil {
					return er
				}
				defer rows.Close()
				items := []syncWireEvent{}
				for rows.Next() {
					item := syncWireEvent{Owner: owner, ScopeID: a.Config.DeploymentID, Ref: syncRef{Owner: owner}}
					if er = rows.Scan(&item.OriginEventID, &item.Revision, &item.RestoreEpoch, &item.Ref.Kind, &item.Ref.ID, &item.Ref.SubjectUserID, &item.Ref.ConversationID, &item.Ref.TargetUserID, &item.Ref.RevisionEpoch, &item.Ref.State); er != nil {
						return er
					}
					item.Ref.Revision = item.Revision
					items = append(items, item)
				}
				if er = rows.Err(); er != nil {
					return er
				}
				respond(w, 200, map[string]any{"items": items, "watermark": v.Watermark, "restoreEpoch": v.RestoreEpoch})
				return nil
			}
			base, args, er := a.syncCaptureSQL(r.Context(), owner, q.UserID)
			if er != nil {
				return er
			}
			if schema == "2" && owner == "directory" {
				base += ` UNION ALL SELECT 'preferences',$1::uuid,$1::uuid,NULL::uuid,NULL::uuid,'active'`
			}
			args = append(args, q.Kind, q.ID)
			// Stable keyset pages are accepted only while every page and the final
			// double collect retain the original committed owner watermark.
			sql := fmt.Sprintf(`WITH objects AS (%s) SELECT o.kind,o.id::text,COALESCE(v.revision,0)::text,i.restore_epoch::text,COALESCE(o.subject::text,''),COALESCE(o.conversation::text,''),COALESCE(o.target::text,''),o.state FROM objects o CROSS JOIN %s.instance i LEFT JOIN %s.sync_object_versions v ON v.kind=o.kind AND v.object_id=o.id WHERE (o.kind,o.id::text)>($%d::text,$%d::text) ORDER BY o.kind,o.id LIMIT 501`, base, owner, owner, len(args)-1, len(args))
			rows, er := tx.Query(r.Context(), sql, args...)
			if er != nil {
				return er
			}
			defer rows.Close()
			for rows.Next() {
				ref := syncRef{Owner: owner}
				if er = rows.Scan(&ref.Kind, &ref.ID, &ref.Revision, &ref.RevisionEpoch, &ref.SubjectUserID, &ref.ConversationID, &ref.TargetUserID, &ref.State); er != nil {
					return er
				}
				v.Refs = append(v.Refs, ref)
			}
			if er = rows.Err(); er != nil {
				return er
			}
			hasMore := len(v.Refs) > 500
			if hasMore {
				v.Refs = v.Refs[:500]
			}
			respond(w, 200, map[string]any{"capture": v, "hasMore": hasMore})
			return nil
		}))
	}
}

func (a *App) syncCaptureSQL(ctx context.Context, owner, user string) (string, []any, error) {
	args := []any{user}
	switch owner {
	case "auth":
		return `SELECT 'account'::text kind,id,id subject,NULL::uuid conversation,NULL::uuid target,'active'::text state FROM auth.users WHERE id=$1 UNION ALL SELECT 'devices',id,id,NULL::uuid,NULL::uuid,'active' FROM auth.users WHERE id=$1`, args, nil
	case "directory":
		return `SELECT 'profile'::text kind,user_id id,user_id subject,NULL::uuid conversation,NULL::uuid target,'active'::text state FROM directory.profiles WHERE user_id=$1 UNION ALL SELECT kind,$1::uuid,$1::uuid,NULL::uuid,NULL::uuid,'active' FROM (VALUES ('contacts'),('relationship')) k(kind) UNION ALL SELECT 'contact_request',id,requester_id,NULL::uuid,recipient_id,'active' FROM directory.contact_requests WHERE requester_id=$1 OR recipient_id=$1`, args, nil
	case "conversation":
		return `SELECT k.kind,c.id,NULL::uuid subject,c.id conversation,NULL::uuid target,'active'::text state FROM conversation.conversations c JOIN conversation.members m ON m.conversation_id=c.id CROSS JOIN (VALUES('conversation'::text),('announcement'::text)) k(kind) WHERE m.user_id=$1 AND m.state='active' AND m.export_pending_operation_id IS NULL AND (k.kind='conversation' OR c.kind='group') AND (c.state IN('active','owner_unavailable') OR c.state='dissolved' AND k.kind='conversation')`, args, nil
	case "message":
		var scopes struct {
			Items []readableScope `json:"items"`
		}
		if e := a.rpc(ctx, "conversation", "/internal/read-scopes", map[string]string{"userId": user}, &scopes); e != nil {
			return "", nil, e
		}
		raw, _ := json.Marshal(scopes.Items)
		var historical struct {
			Items []syncMembershipPeriod `json:"items"`
		}
		if e := a.rpc(ctx, "conversation", "/internal/sync/historical-scopes", map[string]string{"userId": user}, &historical); e != nil {
			return "", nil, e
		}
		periods, _ := json.Marshal(historical.Items)
		return `SELECT 'message'::text kind,m.id,m.sender_id subject,m.conversation_id conversation,NULL::uuid target,CASE WHEN m.deleted THEN 'removed' ELSE 'active' END::text state FROM message.messages m WHERE EXISTS(SELECT 1 FROM jsonb_to_recordset($1::jsonb) AS s(id uuid,floor timestamptz) WHERE s.id=m.conversation_id AND m.created_at>=s.floor) OR (m.deleted AND EXISTS(SELECT 1 FROM jsonb_to_recordset($2::jsonb) AS h(id uuid,"joinedAt" timestamptz,"leftAt" timestamptz) WHERE h.id=m.conversation_id AND m.created_at>=h."joinedAt" AND(h."leftAt" IS NULL OR m.created_at<h."leftAt")))`, []any{raw, periods}, nil
	case "sync":
		var scopes struct {
			Items []readableScope `json:"items"`
		}
		if err := a.rpc(ctx, "conversation", "/internal/read-scopes", map[string]string{"userId": user}, &scopes); err != nil {
			return "", nil, err
		}
		defaults := []map[string]string{}
		for _, scope := range scopes.Items {
			defaults = append(defaults, map[string]string{"id": stableID(a.Config.Key, "personal-view:"+a.Config.DeploymentID+":"+user+":"+scope.ID), "readId": stableID(a.Config.Key, "self-read-cursor:"+a.Config.DeploymentID+":personal:"+user+":"+scope.ID), "conversation": scope.ID})
		}
		raw, err := json.Marshal(defaults)
		if err != nil {
			return "", nil, err
		}
		return `SELECT 'conversation_view'::text kind,id,user_id subject,conversation_id conversation,NULL::uuid target,'active'::text state FROM sync.conversation_views WHERE user_id=$1 AND source_context_id='personal' AND scope_id=$3
UNION SELECT 'conversation_view',d.id,$1::uuid,d.conversation,NULL::uuid,'active' FROM jsonb_to_recordset($2::jsonb) AS d(id uuid,conversation uuid)
UNION SELECT 'read_state',id,user_id,conversation_id,NULL::uuid,'active' FROM sync.read_cursors WHERE user_id=$1 AND source_context_id='personal' AND scope_id=$3
UNION SELECT 'read_state',d."readId",$1::uuid,d.conversation,NULL::uuid,'active' FROM jsonb_to_recordset($2::jsonb) AS d("readId" uuid,conversation uuid)
UNION SELECT 'read_observation',id,user_id,conversation_id,NULL::uuid,'active' FROM sync.read_observations WHERE user_id=$1 AND source_context_id='personal' AND scope_id=$3
UNION SELECT 'draft',id,user_id,conversation_id,NULL::uuid,'active' FROM sync.conversation_views WHERE user_id=$1 AND source_context_id='personal' AND scope_id=$3 AND draft_version>0`, []any{user, raw, a.Config.DeploymentID}, nil
	}
	return "", nil, fail(400, "INVALID_SYNC_OWNER", "Unknown synchronization owner")
}

func syncUnavailableProjection(ref syncRef, state string) syncProjection {
	return syncProjection{Ref: ref, State: state}
}

func (a *App) syncHydrate(r *http.Request, user string, ref syncRef) (syncProjection, error) {
	if ref.Owner != syncOwnerSchema(a.Role) || syncKindOwner(ref.Kind) != ref.Owner || !validObjectID(ref.ID) {
		return syncProjection{}, fail(400, "INVALID_SYNC_REFERENCE", "Invalid owner reference")
	}
	for attempt := 0; attempt < 3; attempt++ {
		before, epoch, e := a.syncObjectRevision(r.Context(), ref)
		if e != nil {
			return syncProjection{}, e
		}
		v, e := a.syncHydrateOnce(r, user, ref)
		if e != nil {
			return v, e
		}
		after, afterEpoch, e := a.syncObjectRevision(r.Context(), ref)
		if e != nil {
			return v, e
		}
		if before == after && epoch == afterEpoch {
			v.CurrentRevision, v.CurrentRevisionEpoch = after, afterEpoch
			v.RevisionMatch = ref.Revision == after && ref.RevisionEpoch == afterEpoch
			return v, nil
		}
	}
	return syncUnavailableProjection(ref, "unknown"), nil
}

func (a *App) syncObjectRevision(ctx context.Context, ref syncRef) (string, string, error) {
	var revision, epoch string
	e := a.DB.QueryRow(ctx, "SELECT COALESCE(v.revision,0)::text,i.restore_epoch::text FROM "+ref.Owner+".instance i LEFT JOIN "+ref.Owner+".sync_object_versions v ON v.kind=$1 AND v.object_id=$2 WHERE i.singleton", ref.Kind, ref.ID).Scan(&revision, &epoch)
	return revision, epoch, e
}

func (a *App) syncHydrateOnce(r *http.Request, user string, ref syncRef) (syncProjection, error) {
	if ref.Owner != syncOwnerSchema(a.Role) || syncKindOwner(ref.Kind) != ref.Owner || !validObjectID(ref.ID) {
		return syncProjection{}, fail(400, "INVALID_SYNC_REFERENCE", "Invalid owner reference")
	}
	v := syncProjection{Ref: ref, State: "available"}
	var e error
	switch ref.Kind {
	case "account":
		if ref.ID != user {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		v.Object, e = a.getUser(r.Context(), user)
	case "devices":
		if ref.ID != user {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		rows, er := a.DB.Query(r.Context(), `SELECT device_id::text,device_name,created_at,access_expires_at,idle_expires_at,absolute_expires_at,revoked_at IS NOT NULL FROM auth.sessions WHERE user_id=$1 ORDER BY device_id`, user)
		if er != nil {
			return v, er
		}
		items := []map[string]any{}
		for rows.Next() {
			var id, name string
			var created time.Time
			var access, idle, absolute *time.Time
			var revoked bool
			if er = rows.Scan(&id, &name, &created, &access, &idle, &absolute, &revoked); er != nil {
				rows.Close()
				return v, er
			}
			items = append(items, map[string]any{"deviceId": id, "name": name, "createdAt": created, "accessExpiresAt": access, "idleExpiresAt": idle, "absoluteExpiresAt": absolute, "revoked": revoked})
		}
		er = rows.Err()
		rows.Close()
		if er != nil {
			return v, er
		}
		v.Object = map[string]any{"items": items}
	case "profile":
		if ref.ID != user {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		v.Object, e = a.avatarProfileFields(r.Context(), user)
	case "preferences":
		if ref.ID != user || ref.SubjectUserID != user {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		var epoch string
		if e = a.DB.QueryRow(r.Context(), `SELECT restore_epoch::text FROM directory.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch); e == nil {
			v.Object, e = a.sourcePreferencesRead(r.Context(), User{ID: user, RestoreEpoch: epoch})
		}
	case "conversation_view":
		if ref.SubjectUserID != user || !syncValidationID(ref.ConversationID) || ref.ID != stableID(a.Config.Key, "personal-view:"+a.Config.DeploymentID+":"+user+":"+ref.ConversationID) {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		var epoch string
		if e = a.DB.QueryRow(r.Context(), `SELECT restore_epoch::text FROM sync.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch); e == nil {
			var found bool
			v.Object, found, e = a.selfViewRead(r.Context(), a.DB, User{ID: user, RestoreEpoch: epoch}, ref.ConversationID)
			if e == nil && !found && ref.State != "removed" {
				if _, e = a.member(r.Context(), user, ref.ConversationID); e != nil {
					return syncUnavailableProjection(ref, "redacted"), nil
				}
			}
		}
	case "draft":
		if ref.SubjectUserID != user || !syncValidationID(ref.ConversationID) {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		v.Object, e = a.syncPersonalDraftProjection(r.Context(), user, ref)
	case "read_state", "read_observation":
		if ref.SubjectUserID != user || !syncValidationID(ref.ConversationID) {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		v.Object, e = a.syncPersonalReadProjection(r.Context(), user, ref)
	case "contacts", "relationship":
		if ref.ID != user {
			return syncUnavailableProjection(ref, "redacted"), nil
		}
		items := []map[string]any{}
		if ref.Kind == "contacts" {
			rows, er := a.DB.Query(r.Context(), `SELECT peer_id::text,state,request_generation::text,request_gate FROM directory.contacts WHERE owner_id=$1 ORDER BY peer_id`, user)
			if er != nil {
				return v, er
			}
			for rows.Next() {
				var id, state, generation, gate string
				if er = rows.Scan(&id, &state, &generation, &gate); er != nil {
					rows.Close()
					return v, er
				}
				items = append(items, map[string]any{"peerId": id, "state": state, "generation": generation, "inboundGate": gate})
			}
			er = rows.Err()
			rows.Close()
			if er != nil {
				return v, er
			}
		} else {
			rows, er := a.DB.Query(r.Context(), `SELECT peer_id::text FROM directory.blocks WHERE owner_id=$1 ORDER BY peer_id`, user)
			if er != nil {
				return v, er
			}
			for rows.Next() {
				var id string
				if er = rows.Scan(&id); er != nil {
					rows.Close()
					return v, er
				}
				items = append(items, map[string]any{"peerId": id, "blockedByMe": true})
			}
			er = rows.Err()
			rows.Close()
			if er != nil {
				return v, er
			}
		}
		v.Object = map[string]any{"items": items}
	case "contact_request":
		var requester, recipient, state, generation string
		var created time.Time
		e = a.DB.QueryRow(r.Context(), `SELECT requester_id::text,recipient_id::text,state,generation::text,created_at FROM directory.contact_requests WHERE id=$1 AND (requester_id=$2 OR recipient_id=$2)`, ref.ID, user).Scan(&requester, &recipient, &state, &generation, &created)
		v.Object = map[string]any{"id": ref.ID, "requesterId": requester, "recipientId": recipient, "state": state, "generation": generation, "createdAt": created}
	case "conversation":
		if _, e = a.member(r.Context(), user, ref.ID); e == nil {
			var c Conversation
			c, e = a.conversationByID(r, ref.ID, user)
			v.Object = c
		}
	case "announcement":
		request := r.Clone(r.Context())
		request.SetPathValue("id", ref.ID)
		var item announcement
		item, e = a.projectAnnouncement(request, user)
		if item.Message != nil {
			item.Message.ReplyAdmission = ""
		}
		v.Object = item
	case "message":
		var m Message
		m, e = a.loadMessage(r.Context(), ref.ID, user)
		if e == nil {
			m.ReplyAdmission = ""
			v.Object = m
			if m.Deleted {
				v.State = "global"
			} else if m.ContentUnavailable || m.ForwardUnavailable {
				v.State = "unavailable"
			}
		}
	default:
		return v, fail(400, "INVALID_SYNC_REFERENCE", "Unknown reference kind")
	}
	if e != nil {
		var ae *apiError
		if errors.Is(e, pgx.ErrNoRows) {
			return syncUnavailableProjection(ref, "unknown"), nil
		}
		if errors.As(e, &ae) {
			if ae.Status == 403 || ae.Status == 404 {
				if ref.Kind == "message" {
					global, known, er := a.syncHistoricalGlobal(r.Context(), user, ref.ID)
					if er != nil {
						return syncUnavailableProjection(ref, "unknown"), nil
					}
					if known {
						v.State = "global"
						v.Object = global
						e = nil
					} else {
						return syncUnavailableProjection(ref, "redacted"), nil
					}
				} else {
					return syncUnavailableProjection(ref, "redacted"), nil
				}
			}
			if ae.Status == 503 {
				return syncUnavailableProjection(ref, "unknown"), nil
			}
		}
		if e != nil {
			return v, e
		}
	}
	raw, er := json.Marshal(v.Object)
	if er != nil {
		return v, er
	}
	v.BodyDigest = syncDigest(raw)
	v.BodyEnvelope = base64.RawURLEncoding.EncodeToString(raw)
	return v, nil
}
