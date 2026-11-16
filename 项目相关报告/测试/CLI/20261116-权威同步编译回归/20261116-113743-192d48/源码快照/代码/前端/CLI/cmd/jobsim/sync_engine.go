// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func (c *client) syncCurrent(store *localStore, pair *authPair) bool {
	if store == nil || store.checkSyncFile() != nil {
		return false
	}
	if c.store != store || c.token == "" || c.base != store.identity.Origin || c.user != store.identity.UserID || c.deployment != store.identity.DeploymentID || c.epoch != store.epoch || c.serverDevice != store.identity.DeviceID {
		return false
	}
	return pair == nil || c.auth != nil && c.auth.owns(*pair)
}
func (c *client) authoritativeSync() (err error) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	if c.store == nil {
		return syncRefuse("SYNC_STORE_REQUIRED")
	}
	s := c.store
	var pair *authPair
	if c.auth != nil {
		p, e := c.auth.snapshot()
		if e != nil {
			return e
		}
		pair = &p
		if p.SourceContextID != "personal" || !syncUUIDPattern.MatchString(p.FamilyID) {
			return syncRefuse("SYNC_NATIVE_IDENTITY_REQUIRED")
		}
	}
	current := func() bool { return c.syncCurrent(s, pair) }
	if !current() {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	// Probe before taking durable responsibility; old servers must not strand a new store.
	var initial struct {
		Deployment string `json:"deploymentId"`
		Epoch      string `json:"restoreEpoch"`
		Features   struct {
			Authority bool `json:"authoritative_sync"`
			JetStream bool `json:"sync_jetstream"`
		} `json:"features"`
	}
	responsible, e := s.syncResponsible()
	if e != nil {
		return e
	}
	if !responsible {
		if err = c.call("GET", "/api/bootstrap", nil, &initial); err != nil {
			return err
		}
		if !current() {
			return syncRefuse("SYNC_CONTEXT_CHANGED")
		}
		if initial.Deployment != c.deployment || initial.Epoch != c.epoch {
			return syncRefuse("RESTORE_EPOCH_CHANGED")
		}
		if !initial.Features.Authority || !initial.Features.JetStream {
			return syncRefuse("SYNC_CAPABILITY_REQUIRED")
		}
	}
	control, err := s.ensureSync()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil && current() {
			code := "SYNC_UNAVAILABLE"
			var failure *syncFailure
			var api *apiFailure
			if errors.As(err, &failure) {
				code = failure.Code
			} else if errors.As(err, &api) {
				code = api.Code
			}
			obsolete := code == "SNAPSHOT_EXPIRED" || code == "SNAPSHOT_INVALIDATED" || code == "SYNC_SNAPSHOT_EXPIRED" || code == "SYNC_PERMISSION_CHANGED"
			if restrictErr := s.restrictSync(code, obsolete); restrictErr != nil {
				err = fmt.Errorf("%w; local restriction failed: %v", err, restrictErr)
			}
			c.lookup = nil
		}
	}()
	// Responsibility is already durable; broker failure cannot restore legacy reads.
	if err = s.drainSyncCleanup(); err != nil {
		return err
	}
	call := func(method, path string, body any) (json.RawMessage, error) {
		if !current() {
			return nil, syncRefuse("SYNC_CONTEXT_CHANGED")
		}
		var raw json.RawMessage
		e := c.call(method, path, body, &raw)
		if !current() {
			return nil, syncRefuse("SYNC_CONTEXT_CHANGED")
		}
		return raw, e
	}
	bootstrap, err := call("GET", "/api/bootstrap", nil)
	if err != nil {
		return err
	}
	var boot struct {
		Deployment string `json:"deploymentId"`
		Epoch      string `json:"restoreEpoch"`
		Features   struct {
			Authority bool `json:"authoritative_sync"`
			JetStream bool `json:"sync_jetstream"`
		} `json:"features"`
	}
	if syncDecode(bootstrap, &boot) != nil {
		return syncRefuse("SYNC_INVALID_BOOTSTRAP")
	}
	if boot.Deployment != c.deployment || boot.Epoch != c.epoch {
		return syncRefuse("RESTORE_EPOCH_CHANGED")
	}
	if !boot.Features.Authority || !boot.Features.JetStream {
		return syncRefuse("SYNC_CAPABILITY_REQUIRED")
	}
	registration, err := call("PUT", "/api/sync/device", map[string]string{"requestId": control.RegistrationRequestID, "storeGeneration": control.StoreGeneration})
	if err != nil {
		return err
	}
	var bound struct {
		syncBinding
		Qualification string `json:"qualification"`
		Ack           *bool  `json:"qualifiedAck"`
	}
	if syncDecode(registration, &bound) != nil || bound.Qualification != "transport_only" || bound.Ack == nil || *bound.Ack || !syncUUIDPattern.MatchString(bound.StreamID) {
		return syncRefuse("SYNC_INVALID_REGISTRATION")
	}
	if err = bound.syncBinding.check(control.syncBinding, false); err != nil {
		return err
	}
	if control.StreamID != "" && control.StreamID != bound.StreamID {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	next := control
	next.StreamID = bound.StreamID
	control, err = s.updateSync(control, next, nil)
	if err != nil {
		return err
	}
	contextRaw, err := call("GET", "/api/sync/context", nil)
	if err != nil {
		return err
	}
	var context struct {
		syncBinding
		Head          string `json:"head"`
		Permission    string `json:"permissionGeneration"`
		Qualification string `json:"qualification"`
		Ack           *bool  `json:"qualifiedAck"`
		After         *bool  `json:"afterReceiptEnabled"`
	}
	if syncDecode(contextRaw, &context) != nil || context.Qualification != "transport_only" || context.Ack == nil || *context.Ack || context.After == nil || *context.After || !syncDigestPattern.MatchString(context.Permission) {
		return syncRefuse("SYNC_INVALID_CONTEXT")
	}
	if err = context.syncBinding.check(control.syncBinding, true); err != nil {
		return err
	}
	if _, err = syncCounter(context.Head); err != nil {
		return err
	}
	if control.Permission != "" && control.Permission != context.Permission {
		if err = s.restrictSync("SYNC_PERMISSION_CHANGED", true); err != nil {
			return err
		}
		control, _, err = syncLoadControl(s.db, s.epoch)
		if err != nil {
			return err
		}
	}
	var manifest *syncManifest
	var pages []syncPage
	var projections []syncProjection
	var delta syncDelta
	classification := control.Classification
	if control.Cursor != "" && control.Restriction == "" && control.Permission == context.Permission {
		raw, e := call("GET", "/api/sync/changes?cursor="+url.QueryEscape(control.Cursor)+"&limit=500", nil)
		if e != nil {
			return e
		}
		delta, err = syncReadChanges(raw, control.Position, s.epoch, control.Permission)
		if err != nil {
			return err
		}
	} else {
		if len(control.SnapshotRequest) == 0 {
			request, e := syncNewUUID()
			if e != nil {
				return e
			}
			next = control
			next.SnapshotRequest, _ = json.Marshal(map[string]any{"requestId": request, "storeGeneration": control.StoreGeneration, "schemaVersion": "1", "pageSize": 100})
			control, err = s.updateSync(control, next, nil)
			if err != nil {
				return err
			}
		}
		raw, e := call("POST", "/api/sync/snapshots", control.SnapshotRequest)
		if e != nil {
			return e
		}
		m, e := syncReadManifest(raw, control.syncBinding)
		if e != nil {
			return e
		}
		manifest = &m
		if len(control.Manifest) != 0 && !syncEqual(control.Manifest, m.Raw) {
			return syncRefuse("SYNC_STAGE_CHANGED")
		}
		next = control
		next.Manifest = m.Raw
		control, err = s.updateSync(control, next, nil)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for index := range m.PageManifest {
			raw, e = call("GET", "/api/sync/snapshots/"+m.SnapshotID+"/pages/"+fmt.Sprint(index), nil)
			if e != nil {
				return e
			}
			page, e := syncReadPage(raw, m, index)
			if e != nil {
				return e
			}
			for _, ref := range page.Refs {
				if seen[ref.key()] {
					return syncRefuse("SYNC_DUPLICATE_REFERENCE")
				}
				seen[ref.key()] = true
			}
			control, err = s.updateSync(control, control, &page)
			if err != nil {
				return err
			}
			pages = append(pages, page)
			projections = append(projections, page.Items...)
		}
		digests := make([]string, len(m.PageManifest))
		for i, page := range m.PageManifest {
			digests[i] = page.Digest
		}
		validated, e := call("POST", "/api/sync/snapshots/"+m.SnapshotID+"/validate", map[string]any{"pageDigests": digests})
		if e != nil {
			return e
		}
		classification, err = syncValidation(validated, m)
		if err != nil {
			return err
		}
		// A final validate alone does not unlock cloud bodies: check current permission first.
		raw, e = call("GET", "/api/sync/changes?cursor="+url.QueryEscape(m.BoundaryToken)+"&limit=500", nil)
		if e != nil {
			return e
		}
		delta, err = syncReadChanges(raw, m.Boundary, s.epoch, m.PermissionGeneration)
		if err != nil {
			return err
		}
	}
	projections = append(projections, delta.Items...)
	proofs, err := c.syncOutboxProofs(call, current, control.Guard)
	if err != nil {
		return err
	}
	next = control
	next.Cursor = delta.Cursor
	next.Position = delta.Position
	next.Permission = delta.Permission
	next.Classification = classification
	if !current() {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	if err = s.commitSync(control, next, manifest, pages, projections, delta, proofs, current); err != nil {
		return err
	}
	c.lookup = nil
	return s.drainSyncCleanup()
}

func (c *client) syncOutboxProofs(call func(string, string, any) (json.RawMessage, error), current func() bool, guard string) ([]syncProof, error) {
	items, err := c.store.outbox()
	if err != nil {
		return nil, err
	}
	proofs := []syncProof{}
	for _, p := range items {
		if p.State == "confirmed" || p.RestoreEpoch != c.epoch || p.UserID != c.user || p.DeploymentID != c.deployment || !syncUUIDPattern.MatchString(p.ConversationID) {
			continue
		}
		raw, e := call("POST", "/api/conversations/"+url.PathEscape(p.ConversationID)+"/message-results", p.payload())
		if !current() {
			return nil, syncRefuse("SYNC_CONTEXT_CHANGED")
		}
		if e != nil {
			var api *apiFailure
			if errors.As(e, &api) && (api.Status == 401 || api.Code == "RESTORE_EPOCH_CHANGED" || api.Code == "STORE_GENERATION_CHANGED" || api.Code == "DEPLOYMENT_CHANGED") {
				return nil, e
			}
			continue
		}
		m, e := syncValidateMessage(raw, false)
		if e != nil || m.ClientMsgID != p.ClientMsgID || m.SenderID != p.UserID || m.ConversationID != p.ConversationID {
			continue
		}
		proofs = append(proofs, syncProof{Original: p, Result: raw, Guard: guard})
	}
	return proofs, nil
}

func (c *client) reconcileReadOnly(p pending) error {
	if c.store == nil || p.UserID != c.user || p.DeploymentID != c.deployment || p.RestoreEpoch != c.epoch || c.store.epoch != c.epoch {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	guard := ""
	if control, present, err := syncLoadControl(c.store.db, c.epoch); err != nil {
		return err
	} else if present {
		guard = control.Guard
	}
	var result json.RawMessage
	if err := c.call("POST", "/api/conversations/"+url.PathEscape(p.ConversationID)+"/message-results", p.payload(), &result); err != nil {
		return err
	}
	m, err := syncValidateMessage(result, false)
	if err != nil {
		return err
	}
	if m.ClientMsgID != p.ClientMsgID || m.SenderID != p.UserID || m.ConversationID != p.ConversationID {
		return syncRefuse("SYNC_OUTBOX_RESULT_INVALID")
	}
	if err = c.store.confirmReadOnly(p, result, guard); err != nil {
		return err
	}
	if err = c.store.drainSyncCleanup(); err != nil {
		return err
	}
	return printJSON(map[string]any{"source": "read_only_original_result", "clientMsgId": p.ClientMsgID, "messageId": m.ID, "confirmed": true})
}

func syncProtectedPath(method, path string) bool {
	if strings.HasPrefix(path, "/api/sync/") || strings.HasPrefix(path, "/api/auth/") || path == "/api/bootstrap" {
		return false
	}
	return strings.HasPrefix(path, "/api/conversations") || strings.HasPrefix(path, "/api/messages") || strings.HasPrefix(path, "/api/search") || strings.HasPrefix(path, "/api/favorites") || strings.HasPrefix(path, "/api/media")
}
func (c *client) syncHTTPGuard(method, path string) (string, error) {
	if c.store == nil || !syncProtectedPath(method, path) {
		return "", nil
	}
	responsible, err := c.store.syncResponsible()
	if err != nil || !responsible {
		return "", err
	}
	control, present, err := syncLoadControl(c.store.db, c.store.epoch)
	if err != nil {
		return "", err
	}
	if !present {
		return "", syncRefuse("SYNC_STORE_INVALID")
	}
	if !(method == "POST" && strings.HasSuffix(path, "/message-results")) {
		if err = c.store.syncReady(); err != nil {
			return "", err
		}
	}
	identity, _ := json.Marshal(c.store.identity)
	return fingerprint(string(identity)+"|"+c.base+"|"+c.user+"|"+c.epoch+"|"+c.serverDevice) + ":" + control.Guard, nil
}
func (c *client) syncHTTPAfter(stamp string) error {
	if stamp == "" {
		return nil
	}
	if c.store == nil {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	control, present, err := syncLoadControl(c.store.db, c.store.epoch)
	if err != nil {
		return err
	}
	identity, _ := json.Marshal(c.store.identity)
	expected := fingerprint(string(identity)+"|"+c.base+"|"+c.user+"|"+c.epoch+"|"+c.serverDevice) + ":" + control.Guard
	if !present || stamp != expected {
		return syncRefuse("SYNC_LOCAL_GUARD_CHANGED")
	}
	return nil
}

func (c *client) observeMessageGlobals(method, path string, request any, raw json.RawMessage) error {
	if len(raw) == 0 || c.store == nil {
		return nil
	}
	u, err := url.ParseRequestURI(path)
	if err != nil {
		return syncRefuse("SYNC_ROUTE_INVALID")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	expectedID, expectedConversation := "", ""
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "messages" {
		expectedID = parts[2]
	}
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "conversations" {
		expectedConversation = parts[2]
	}
	if u.Path == "/api/search" || u.Path == "/api/favorites" {
		expectedConversation = u.Query().Get("conversationId")
	}
	if expectedID == "" && expectedConversation == "" && u.Path != "/api/search" && u.Path != "/api/favorites" {
		return nil
	}
	var original struct {
		ClientMsgID string `json:"clientMsgId"`
	}
	resultRoute := method == "POST" && strings.HasSuffix(u.Path, "/message-results")
	if resultRoute {
		body, e := json.Marshal(request)
		if e != nil || syncDecode(body, &original) != nil || original.ClientMsgID == "" {
			return syncRefuse("SYNC_ROUTE_INVALID")
		}
	}
	var value any
	if err := syncDecode(raw, &value); err != nil {
		return err
	}
	var ids []string
	var visit func(any) error
	visit = func(value any) error {
		switch node := value.(type) {
		case []any:
			for _, item := range node {
				if err := visit(item); err != nil {
					return err
				}
			}
		case map[string]any:
			if _, isMessage := node["conversationId"]; isMessage {
				if _, hasClientID := node["clientMsgId"]; hasClientID {
					body, _ := json.Marshal(node)
					message, err := syncValidateMessage(body, false)
					if err != nil {
						return err
					}
					if expectedID != "" && message.ID != expectedID || expectedConversation != "" && message.ConversationID != expectedConversation || resultRoute && (message.ClientMsgID != original.ClientMsgID || message.SenderID != c.user) {
						return syncRefuse("SYNC_ROUTE_MISMATCH")
					}
					if node["deleted"] == true {
						ids = append(ids, message.ID)
					}
				}
			}
			for _, key := range []string{"items", "message"} {
				if child, ok := node[key]; ok {
					if err := visit(child); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := visit(value); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	tx, err := c.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if err = syncRecordGlobalTx(tx, c.epoch, id); err != nil {
			return err
		}
	}
	control, present, err := syncLoadControl(tx, c.epoch)
	if err != nil {
		return err
	}
	if present {
		control.Guard, err = syncNewUUID()
		if err != nil {
			return err
		}
		if err = syncSaveControl(tx, c.epoch, control); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	c.lookup = nil
	return c.store.drainSyncCleanup()
}

func (c *client) syncCommand(cmd, args string) (bool, error) {
	if c.store == nil {
		return false, nil
	}
	if cmd == "sync-status" {
		value, err := c.store.syncSummary()
		if err != nil {
			return true, err
		}
		return true, printJSON(value)
	}
	if cmd == "local-history" {
		parts := strings.Fields(args)
		if len(parts) != 1 {
			return true, fmt.Errorf("local-history CONVERSATION_ID")
		}
		if !c.syncCurrent(c.store, nil) {
			return true, syncRefuse("SYNC_CONTEXT_CHANGED")
		}
		items, err := c.store.localHistory(parts[0])
		if err != nil {
			return true, err
		}
		return true, printJSON(map[string]any{"source": "verified_local_history_read_only", "restoreEpoch": c.epoch, "items": items, "cloudPermission": false, "qualifiedAck": false})
	}
	if cmd != "sync" && cmd != "history" && cmd != "conversations" {
		return false, nil
	}
	responsible, err := c.store.syncResponsible()
	if err != nil {
		return true, err
	}
	if !responsible && !(cmd == "sync" && strings.TrimSpace(args) == "") {
		var boot struct {
			Deployment string `json:"deploymentId"`
			Epoch      string `json:"restoreEpoch"`
			Features   struct {
				Authority bool `json:"authoritative_sync"`
			} `json:"features"`
		}
		if err = c.call("GET", "/api/bootstrap", nil, &boot); err != nil {
			return true, err
		}
		if boot.Deployment != c.deployment || boot.Epoch != c.epoch {
			return true, syncRefuse("SYNC_CONTEXT_CHANGED")
		}
		if !boot.Features.Authority {
			return false, nil
		}
	}
	parts := strings.Fields(args)
	if len(parts) > 1 || cmd == "history" && len(parts) != 1 || cmd == "conversations" && len(parts) != 0 {
		return true, fmt.Errorf("schema1 uses sync [CONVERSATION_ID], history CONVERSATION_ID or conversations; old history cursors are not stream cursors")
	}
	if err = c.authoritativeSync(); err != nil {
		return true, err
	}
	if cmd == "conversations" {
		rows, err := c.store.db.Query("SELECT payload FROM conversations WHERE epoch=? ORDER BY id", c.epoch)
		if err != nil {
			return true, err
		}
		defer rows.Close()
		items := []json.RawMessage{}
		for rows.Next() {
			var raw string
			if err = rows.Scan(&raw); err != nil {
				return true, err
			}
			items = append(items, json.RawMessage(raw))
		}
		if err = rows.Err(); err != nil {
			return true, err
		}
		return true, printJSON(map[string]any{"source": "authoritative_sync", "items": items})
	}
	if len(parts) == 1 {
		items, err := c.store.history(parts[0])
		if err != nil {
			return true, err
		}
		return true, printJSON(map[string]any{"source": "authoritative_sync", "items": items, "restoreEpoch": c.epoch, "qualifiedAck": false})
	}
	value, err := c.store.syncSummary()
	if err != nil {
		return true, err
	}
	return true, printJSON(value)
}
