// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func syncSchemaPath(path, schema string) string {
	if schema != "2" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "schemaVersion=2"
}

// This is the sole schema transition. Ordinary guard updates never replace a nonce/stream.
func (s *localStore) selectSyncSchema(expected syncControl, schema string) (syncControl, error) {
	if schema != "2" {
		return expected, syncRefuse("SYNC_SCHEMA_DOWNGRADE_FORBIDDEN")
	}
	if expected.schema() == schema {
		return expected, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return expected, err
	}
	defer tx.Rollback()
	actual, present, err := syncLoadControl(tx, s.epoch)
	if err != nil || !present {
		return expected, syncRefuse("SYNC_STORE_INVALID")
	}
	if err = syncCheckControl(actual, expected); err != nil {
		return expected, err
	}
	if err = s.checkSyncFile(); err != nil {
		return expected, err
	}
	raw, _ := json.Marshal(actual)
	if _, err = tx.Exec("INSERT INTO sync_stream_archive VALUES(?,?,?,?,?)", s.epoch, actual.StreamID, "control", actual.schema(), string(raw)); err != nil {
		return expected, err
	}
	for _, entry := range []struct{ table, key string }{{"sync_refs", "id"}, {"sync_events", "id"}, {"sync_objects", "id"}, {"sync_pages", "snapshot||':'||CAST(page AS TEXT)"}} {
		if _, err = tx.Exec("INSERT INTO sync_stream_archive SELECT epoch,?, ?, "+entry.key+",payload FROM "+entry.table+" WHERE epoch=?", actual.StreamID, entry.table, s.epoch); err != nil {
			return expected, err
		}
	}
	if err = archiveSyncCloud(tx, s.epoch); err != nil {
		return expected, err
	}
	for _, table := range []string{"sync_refs", "sync_events", "sync_objects", "sync_pages"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE epoch=?", s.epoch); err != nil {
			return expected, err
		}
	}
	request, err := syncNewUUID()
	if err != nil {
		return expected, err
	}
	guard, err := syncNewUUID()
	if err != nil {
		return expected, err
	}
	next := syncControl{syncBinding: actual.syncBinding, SchemaVersion: "2", RegistrationRequestID: request, Guard: guard, Position: "0", Classification: "0", Restriction: "SYNC_SCHEMA_BASELINE_REQUIRED"}
	next.StreamID = ""
	if err = syncSaveControl(tx, s.epoch, next); err != nil {
		return expected, err
	}
	return next, tx.Commit()
}

type selfScope struct {
	ID           string `json:"id,omitempty"`
	ScopeID      string `json:"scopeId"`
	Source       string `json:"sourceContextId"`
	User         string `json:"userId"`
	Conversation string `json:"conversationId,omitempty"`
	Epoch        string `json:"revisionEpoch"`
	Version      string `json:"version"`
}

func (v selfScope) check(binding syncBinding, conversation string) error {
	if v.ScopeID != binding.ScopeID || v.Source != "personal" || v.User != binding.PrincipalID || v.Conversation != conversation || v.Epoch != binding.RestoreEpoch {
		return syncRefuse("SELF_SCOPE_MISMATCH")
	}
	if _, err := syncCounter(v.Version); err != nil {
		return err
	}
	if conversation != "" && (!syncUUIDPattern.MatchString(v.ID) || !syncUUIDPattern.MatchString(conversation)) {
		return syncRefuse("SELF_OBJECT_INVALID")
	}
	return nil
}

type selfPreferences struct {
	selfScope
	Font     string `json:"fontPreset"`
	Explicit *bool  `json:"explicit"`
}
type selfView struct {
	selfScope
	Pinned           *int       `json:"pinnedRank"`
	Muted            *string    `json:"mutedUntil"`
	Archived         *bool      `json:"archived"`
	Hidden           *bool      `json:"hidden"`
	Manual           *bool      `json:"manualUnread"`
	ManualGeneration string     `json:"manualUnreadGeneration"`
	Marked           *string    `json:"manualUnreadMarkedAt"`
	From             *selfPoint `json:"manualUnreadFrom"`
}
type selfPoint struct {
	Epoch    string `json:"epoch"`
	Sequence string `json:"sequence"`
}

func (v selfPoint) check() error {
	if _, err := syncCounter(v.Epoch); err != nil {
		return err
	}
	_, err := syncCounter(v.Sequence)
	return err
}

type selfRead struct {
	selfScope
	Membership     string    `json:"membershipGeneration"`
	Point          selfPoint `json:"readPoint"`
	CountStatus    string    `json:"countStatus"`
	Continuity     string    `json:"continuityStatus"`
	Writable       *bool     `json:"writable"`
	Unread         *string   `json:"unreadCount,omitempty"`
	CountWatermark string    `json:"countSourceWatermark,omitempty"`
}
type selfObservation struct {
	selfScope
	Message          string `json:"messageId"`
	Membership       string `json:"membershipGeneration"`
	ObservedEpoch    string `json:"observedInEpoch"`
	ObservedAt       string `json:"observedAt"`
	OrderKnown       *bool  `json:"orderKnown"`
	Eligible         *bool  `json:"eligibleAtObservation"`
	Applicable       *bool  `json:"currentlyApplicable"`
	CountStatus      string `json:"countStatus"`
	CreationEpoch    string `json:"creationEpoch"`
	CreationSequence string `json:"creationSequence"`
}
type selfReply struct {
	MessageID string `json:"messageId"`
	Version   string `json:"sourceVersion"`
	Admission string `json:"replyAdmission,omitempty"`
}
type selfDraft struct {
	selfScope
	ContentState string        `json:"contentState"`
	Owned        *string       `json:"ownedText"`
	ReplyState   string        `json:"replyState"`
	Reply        *selfReply    `json:"reply"`
	Admission    string        `json:"draftAdmission,omitempty"`
	Writable     *bool         `json:"writable"`
	Pending      string        `json:"pendingOperationId,omitempty"`
	Recovery     *selfRecovery `json:"recovery,omitempty"`
}
type selfRecovery struct {
	Operation string `json:"operationId"`
	Text      string `json:"ownedText"`
}

func selfDraftValid(v selfDraft) error {
	if v.Writable == nil || !oneOf(v.ContentState, "empty", "owned", "unavailable") || !oneOf(v.ReplyState, "none", "available", "unavailable") {
		return syncRefuse("DRAFT_RESPONSE_INVALID")
	}
	if v.Owned != nil && (!utf8.ValidString(*v.Owned) || len(*v.Owned) > 16384) || v.ContentState == "unavailable" && v.Owned != nil || v.ContentState == "owned" && v.Owned == nil || v.ContentState == "empty" && v.Owned != nil && *v.Owned != "" {
		return syncRefuse("DRAFT_RESPONSE_INVALID")
	}
	if v.Reply != nil {
		if !syncUUIDPattern.MatchString(v.Reply.MessageID) {
			return syncRefuse("DRAFT_REPLY_INVALID")
		}
		if n, e := syncCounter(v.Reply.Version); e != nil || n == 0 {
			return syncRefuse("DRAFT_REPLY_INVALID")
		}
	}
	if v.ReplyState == "available" && v.Reply == nil || v.ReplyState != "available" && v.Reply != nil {
		return syncRefuse("DRAFT_REPLY_INVALID")
	}
	if v.Recovery != nil && (!utf8.ValidString(v.Recovery.Text) || len(v.Recovery.Text) > 16384 || len(v.Recovery.Operation) < 8) {
		return syncRefuse("DRAFT_RECOVERY_INVALID")
	}
	return nil
}
func oneOf(value string, values ...string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func validateSelfProjection(p syncProjection, control syncControl) (json.RawMessage, error) {
	if p.Ref.Owner != "sync" && p.Ref.Kind != "preferences" {
		return p.Object, nil
	}
	if control.schema() != "2" {
		return nil, syncRefuse("SYNC_SCHEMA_MISMATCH")
	}
	if p.Ref.SubjectUserID != control.PrincipalID {
		return nil, syncRefuse("SELF_SCOPE_MISMATCH")
	}
	if p.Ref.Kind == "preferences" && p.Ref.ID != control.PrincipalID {
		return nil, syncRefuse("SELF_SCOPE_MISMATCH")
	}
	if len(p.Object) == 0 || string(p.Object) == "null" {
		if p.State == "available" {
			return nil, syncRefuse("SELF_OBJECT_INVALID")
		}
		return p.Object, nil
	}
	var scope selfScope
	if syncDecode(p.Object, &scope) != nil {
		return nil, syncRefuse("SELF_OBJECT_INVALID")
	}
	conversation := p.Ref.ConversationID
	if p.Ref.Kind == "preferences" {
		conversation = ""
	}
	var removed struct {
		Removed *bool `json:"removed"`
	}
	_ = syncDecode(p.Object, &removed)
	if removed.Removed != nil && *removed.Removed {
		if scope.ID != p.Ref.ID || p.Ref.State != "removed" {
			return nil, syncRefuse("SELF_OBJECT_INVALID")
		}
		scope.Version = "0"
		if err := scope.check(control.syncBinding, conversation); err != nil {
			return nil, err
		}
		return p.Object, nil
	}
	if err := scope.check(control.syncBinding, conversation); err != nil {
		return nil, err
	}
	switch p.Ref.Kind {
	case "preferences":
		var value selfPreferences
		if syncDecode(p.Object, &value) != nil || value.Explicit == nil || !oneOf(value.Font, "small", "standard", "large") {
			return nil, syncRefuse("PREFERENCE_RESPONSE_INVALID")
		}
		return json.Marshal(value)
	case "conversation_view":
		var v selfView
		if syncDecode(p.Object, &v) != nil || v.ID != p.Ref.ID || v.Archived == nil || v.Hidden == nil || v.Manual == nil || v.Pinned != nil && (*v.Pinned < 0 || *v.Pinned > 999999) {
			return nil, syncRefuse("VIEW_RESPONSE_INVALID")
		}
		if _, e := syncCounter(v.ManualGeneration); e != nil {
			return nil, e
		}
		if v.From != nil {
			if e := v.From.check(); e != nil {
				return nil, e
			}
		}
		return json.Marshal(v)
	case "read_state":
		var v selfRead
		if syncDecode(p.Object, &v) != nil || v.ID != p.Ref.ID || v.Writable == nil || v.CountStatus != "not_computed" || v.Unread != nil || v.CountWatermark != "" {
			return nil, syncRefuse("READ_RESPONSE_INVALID")
		}
		if _, e := syncCounter(v.Membership); e != nil {
			return nil, e
		}
		if e := v.Point.check(); e != nil {
			return nil, e
		}
		return json.Marshal(v)
	case "read_observation":
		var v selfObservation
		if syncDecode(p.Object, &v) != nil || v.ID != p.Ref.ID || !syncUUIDPattern.MatchString(v.Message) || v.OrderKnown == nil || v.Eligible == nil || v.Applicable == nil || v.CountStatus != "not_computed" {
			return nil, syncRefuse("READ_RESPONSE_INVALID")
		}
		if _, e := syncCounter(v.Membership); e != nil {
			return nil, e
		}
		if _, e := syncCounter(v.ObservedEpoch); e != nil {
			return nil, e
		}
		if _, e := time.Parse(time.RFC3339Nano, v.ObservedAt); e != nil {
			return nil, syncRefuse("READ_RESPONSE_INVALID")
		}
		if *v.OrderKnown {
			if e := (selfPoint{v.CreationEpoch, v.CreationSequence}).check(); e != nil {
				return nil, e
			}
		} else if v.CreationEpoch != "unknown" || v.CreationSequence != "unknown" {
			return nil, syncRefuse("READ_RESPONSE_INVALID")
		}
		return json.Marshal(v)
	case "draft":
		var v selfDraft
		if syncDecode(p.Object, &v) != nil || v.ID != p.Ref.ID {
			return nil, syncRefuse("DRAFT_RESPONSE_INVALID")
		}
		if e := selfDraftValid(v); e != nil {
			return nil, e
		}
		v.Admission = ""
		if v.Reply != nil {
			v.Reply.Admission = ""
		}
		return json.Marshal(v)
	}
	return nil, syncRefuse("SELF_OBJECT_INVALID")
}

type selfOperation struct{ ID, Kind, Conversation, Epoch, Method, Route, Body, Hash, State, Result, Created string }

func selfOperationHash(o selfOperation) string {
	raw, _ := json.Marshal([]string{o.Kind, o.Conversation, o.Epoch, o.Method, o.Route, o.Body})
	return fingerprint(string(raw))
}
func validateSelfOperation(o selfOperation) error {
	if !utf8.ValidString(o.ID) || len(o.ID) < 8 || len(o.ID) > 128 || o.Hash != selfOperationHash(o) {
		return syncRefuse("SELF_OPERATION_BODY_CHANGED")
	}
	if n, e := syncCounter(o.Epoch); e != nil || n == 0 {
		return syncRefuse("SELF_OPERATION_INVALID")
	}
	expected := "/api/conversations/" + url.PathEscape(o.Conversation) + "/"
	method := ""
	switch o.Kind {
	case "font":
		if o.Conversation != "" {
			return syncRefuse("SELF_OPERATION_INVALID")
		}
		expected = "/api/me/preferences"
		method = "PATCH"
	case "view":
		expected += "view"
		method = "PATCH"
	case "read":
		expected += "read"
		method = "PUT"
	case "draft":
		expected += "draft"
		if !oneOf(o.Method, "PUT", "DELETE") {
			return syncRefuse("SELF_OPERATION_INVALID")
		}
		method = o.Method
	default:
		return syncRefuse("SELF_OPERATION_INVALID")
	}
	if o.Kind != "font" && !syncUUIDPattern.MatchString(o.Conversation) || o.Route != expected || o.Method != method {
		return syncRefuse("SELF_OPERATION_ROUTE_CHANGED")
	}
	var body struct {
		ID    string `json:"requestId"`
		Epoch string `json:"revisionEpoch"`
	}
	if syncDecode([]byte(o.Body), &body) != nil || body.ID != o.ID || body.Epoch != o.Epoch {
		return syncRefuse("SELF_OPERATION_BODY_CHANGED")
	}
	return nil
}
func (s *localStore) saveSelfOperation(o selfOperation) error {
	if err := validateSelfOperation(o); err != nil {
		return err
	}
	if err := s.syncReady(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pending int
	if err = tx.QueryRow("SELECT count(*) FROM self_operations WHERE kind=? AND conversation=? AND epoch=? AND state='pending'", o.Kind, o.Conversation, s.epoch).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return syncRefuse("SELF_OPERATION_PENDING_USE_RESUME")
	}
	control, present, e := syncLoadControl(tx, s.epoch)
	if e != nil {
		return e
	}
	if !present || control.Restriction != "" || control.schema() != "2" {
		return syncRefuse("SYNC_NOT_READY")
	}
	if o.Epoch != s.epoch || o.Hash != selfOperationHash(o) {
		return syncRefuse("SELF_OPERATION_INVALID")
	}
	_, err = tx.Exec("INSERT INTO self_operations VALUES(?,?,?,?,?,?,?,?,?,?,?)", o.ID, o.Kind, o.Conversation, o.Epoch, o.Method, o.Route, o.Body, o.Hash, "pending", "", o.Created)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *localStore) loadSelfOperation(id string) (selfOperation, error) {
	var o selfOperation
	err := s.db.QueryRow("SELECT id,kind,conversation,epoch,method,route,body,body_hash,state,result,created FROM self_operations WHERE id=?", id).Scan(&o.ID, &o.Kind, &o.Conversation, &o.Epoch, &o.Method, &o.Route, &o.Body, &o.Hash, &o.State, &o.Result, &o.Created)
	if err != nil {
		return o, err
	}
	if err = validateSelfOperation(o); err != nil {
		return o, err
	}
	return o, nil
}
func selfResultPath(o selfOperation) (string, any) {
	route := o.Route + "-results"
	body := any(json.RawMessage(o.Body))
	if o.Kind == "font" {
		route = "/api/me/preferences-results"
	}
	if o.Kind == "draft" {
		body = struct {
			Method string          `json:"method"`
			Body   json.RawMessage `json:"body"`
		}{o.Method, json.RawMessage(o.Body)}
	}
	return route, body
}
func (c *client) selfCapabilities(draft bool) error {
	if c.store == nil {
		return syncRefuse("SYNC_STORE_REQUIRED")
	}
	var boot struct {
		Deployment string          `json:"deploymentId"`
		Epoch      string          `json:"restoreEpoch"`
		Features   map[string]bool `json:"features"`
	}
	if err := c.call("GET", "/api/bootstrap", nil, &boot); err != nil {
		return err
	}
	if boot.Deployment != c.deployment || boot.Epoch != c.epoch {
		return syncRefuse("RESTORE_EPOCH_CHANGED")
	}
	if !boot.Features["authoritative_sync"] || !boot.Features["sync_jetstream"] || !boot.Features["sync_self_state"] || draft && !boot.Features["cloud_drafts"] {
		return syncRefuse("SELF_CAPABILITY_REQUIRED")
	}
	return nil
}
func (c *client) performSelfOperation(id string, explicit bool) (json.RawMessage, error) {
	s := c.store
	var pair *authPair
	if c.auth != nil {
		p, e := c.auth.snapshot()
		if e != nil {
			return nil, e
		}
		pair = &p
	}
	current := func() bool { return c.syncCurrent(s, pair) }
	o, err := s.loadSelfOperation(id)
	if err != nil {
		return nil, err
	}
	if o.Epoch != c.epoch {
		_, err = s.db.Exec("UPDATE self_operations SET state='frozen' WHERE id=?", o.ID)
		if err != nil {
			return nil, err
		}
		return nil, syncRefuse("RESTORE_EPOCH_CHANGED")
	}
	if o.State != "pending" {
		if len(o.Result) == 0 {
			return nil, syncRefuse("SELF_OPERATION_FROZEN")
		}
		return json.RawMessage(o.Result), nil
	}
	if err = c.selfCapabilities(o.Kind == "draft"); err != nil {
		return nil, err
	}
	if !current() {
		return nil, syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	route, body := selfResultPath(o)
	var result json.RawMessage
	err = c.call("POST", route, body, &result)
	var api *apiFailure
	missing := errors.As(err, &api) && api.Status == 404
	if err != nil && !missing {
		return nil, err
	}
	var status struct {
		ID      string `json:"operationId"`
		Status  string `json:"status"`
		Current *bool  `json:"current"`
		Result  struct {
			Epoch   string `json:"revisionEpoch"`
			Version string `json:"version"`
		} `json:"result"`
	}
	if err == nil {
		if syncDecode(result, &status) != nil || status.ID != o.ID || status.Current == nil || status.Result.Epoch != o.Epoch {
			return nil, syncRefuse("SELF_RESULT_INVALID")
		}
		if _, e := syncCounter(status.Result.Version); e != nil {
			return nil, e
		}
	}
	if explicit && (missing || o.Kind == "draft" && status.Status == "prepared") {
		if !current() {
			return nil, syncRefuse("SYNC_CONTEXT_CHANGED")
		}
		if err = c.call(o.Method, o.Route, json.RawMessage(o.Body), &result); err != nil {
			var rejection *apiFailure
			if current() && errors.As(err, &rejection) && rejection.Status >= 400 && rejection.Status < 500 && rejection.Status != 429 {
				failureJSON, _ := json.Marshal(map[string]string{"error": rejection.Code})
				if _, saveErr := s.db.Exec("UPDATE self_operations SET state='frozen',result=? WHERE id=? AND body_hash=? AND state='pending'", string(failureJSON), o.ID, o.Hash); saveErr != nil {
					return nil, fmt.Errorf("%w; saving rejection failed: %v", err, saveErr)
				}
			}
			return nil, err
		}
		if syncDecode(result, &status) != nil || status.ID != o.ID || status.Current == nil || status.Result.Epoch != o.Epoch {
			return nil, syncRefuse("SELF_RESULT_INVALID")
		}
		if _, e := syncCounter(status.Result.Version); e != nil {
			return nil, e
		}
	} else if missing {
		return nil, err
	}
	state := "pending"
	if err = validateSelfResult(o, result); err != nil {
		return nil, err
	}
	switch status.Status {
	case "applied":
		state = "completed"
	case "prepared":
		if o.Kind != "draft" {
			return nil, syncRefuse("SELF_RESULT_INVALID")
		}
	case "retired":
		if o.Kind != "draft" {
			return nil, syncRefuse("SELF_RESULT_INVALID")
		}
		state = "frozen"
	default:
		return nil, syncRefuse("SELF_RESULT_INVALID")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var storedHash, storedState string
	err = tx.QueryRow("SELECT body_hash,state FROM self_operations WHERE id=?", o.ID).Scan(&storedHash, &storedState)
	if err != nil {
		return nil, err
	}
	if storedHash != o.Hash || storedState != "pending" || !current() {
		return nil, syncRefuse("SELF_OPERATION_CHANGED")
	}
	if _, err = tx.Exec("UPDATE self_operations SET state=?,result=? WHERE id=? AND body_hash=? AND state='pending'", state, string(result), o.ID, o.Hash); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func (c *client) newSelfOperation(kind, conversation, method, route string, body map[string]any) error {
	request := id()
	body["requestId"] = request
	body["revisionEpoch"] = c.epoch
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	o := selfOperation{ID: request, Kind: kind, Conversation: conversation, Epoch: c.epoch, Method: method, Route: route, Body: string(raw), Created: time.Now().UTC().Format(time.RFC3339Nano)}
	o.Hash = selfOperationHash(o)
	if err = c.store.saveSelfOperation(o); err != nil {
		return err
	}
	result, err := c.performSelfOperation(o.ID, true)
	if err != nil {
		return err
	}
	// Original completion never overwrites a later local draft; show the fact and refresh current authority.
	if err = c.authoritativeSync(); err != nil {
		return fmt.Errorf("original operation retained; refresh current state: %w", err)
	}
	return printJSON(result)
}

func (c *client) selfCommand(cmd, args string) (bool, error) {
	if !oneOf(cmd, "sync-schema", "font", "view", "read", "cloud-draft", "cloud-draft-set", "cloud-draft-reply", "cloud-draft-clear", "cloud-draft-adopt", "cloud-draft-recover", "self-operations", "resume-self") {
		return false, nil
	}
	if c.store == nil {
		return true, syncRefuse("SYNC_STORE_REQUIRED")
	}
	if cmd == "self-operations" {
		rows, err := c.store.db.Query("SELECT id,kind,conversation,epoch,state FROM self_operations ORDER BY created,id")
		if err != nil {
			return true, err
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, kind, conv, epoch, state string
			if err = rows.Scan(&id, &kind, &conv, &epoch, &state); err != nil {
				return true, err
			}
			items = append(items, map[string]string{"id": id, "kind": kind, "conversationId": conv, "revisionEpoch": epoch, "state": state})
		}
		if err = rows.Err(); err != nil {
			return true, err
		}
		return true, printJSON(items)
	}
	if err := c.selfCapabilities(strings.HasPrefix(cmd, "cloud-draft")); err != nil {
		return true, err
	}
	if err := c.authoritativeSync(); err != nil {
		return true, err
	}
	if cmd == "sync-schema" {
		if strings.TrimSpace(args) != "2" {
			return true, syncRefuse("SYNC_SCHEMA_DOWNGRADE_FORBIDDEN")
		}
		return true, printJSON(map[string]string{"schemaVersion": "2"})
	}
	if cmd == "resume-self" {
		result, err := c.performSelfOperation(strings.TrimSpace(args), true)
		if err != nil {
			return true, err
		}
		if err = c.authoritativeSync(); err != nil {
			return true, err
		}
		return true, printJSON(result)
	}
	parts := strings.Fields(args)
	if cmd == "font" {
		var current selfPreferences
		if err := c.call("GET", "/api/me/preferences", nil, &current); err != nil {
			return true, err
		}
		control, _, err := syncLoadControl(c.store.db, c.epoch)
		if err != nil {
			return true, err
		}
		if err = current.selfScope.check(control.syncBinding, ""); err != nil {
			return true, err
		}
		if !oneOf(current.Font, "small", "standard", "large") || current.Explicit == nil {
			return true, syncRefuse("PREFERENCE_RESPONSE_INVALID")
		}
		if len(parts) == 0 {
			return true, printJSON(current)
		}
		if len(parts) != 1 || !oneOf(parts[0], "small", "standard", "large") {
			return true, fmt.Errorf("font [small|standard|large] (cloud preference, not terminal emulator configuration)")
		}
		return true, c.newSelfOperation("font", "", "PATCH", "/api/me/preferences", map[string]any{"expectedVersion": current.Version, "fontPreset": parts[0]})
	}
	head, text, hasText := strings.Cut(args, "|")
	fields := strings.Fields(head)
	if len(fields) == 0 || !syncUUIDPattern.MatchString(fields[0]) {
		return true, fmt.Errorf("%s CONVERSATION_ID [arguments]", cmd)
	}
	conversation := fields[0]
	route := "/api/conversations/" + url.PathEscape(conversation)
	control, _, err := syncLoadControl(c.store.db, c.epoch)
	if err != nil {
		return true, err
	}
	if cmd == "view" {
		var current selfView
		if err = c.call("GET", route+"/view", nil, &current); err != nil {
			return true, err
		}
		if err = current.selfScope.check(control.syncBinding, conversation); err != nil {
			return true, err
		}
		_, changes, found := strings.Cut(args, " ")
		if !found || strings.TrimSpace(changes) == "" {
			return true, printJSON(current)
		}
		var change map[string]any
		if syncDecode([]byte(changes), &change) != nil || len(change) == 0 {
			return true, fmt.Errorf("view CONVERSATION_ID JSON_CHANGES")
		}
		for key := range change {
			if !oneOf(key, "pinnedRank", "mutedUntil", "archived", "hidden", "manualUnread", "expectedManualUnreadGeneration", "manualUnreadFromMessageId") {
				return true, syncRefuse("INVALID_VIEW_CHANGES")
			}
		}
		if err = validateViewChanges(change); err != nil {
			return true, err
		}
		if _, ok := change["manualUnread"]; ok {
			if _, provided := change["expectedManualUnreadGeneration"]; provided {
				return true, fmt.Errorf("manual generation is captured by this explicit new intent; use resume-self for an old one")
			}
			change["expectedManualUnreadGeneration"] = current.ManualGeneration
		}
		return true, c.newSelfOperation("view", conversation, "PATCH", route+"/view", map[string]any{"expectedVersion": current.Version, "changes": change})
	}
	if cmd == "read" {
		if len(fields) == 1 {
			var current json.RawMessage
			err = c.call("GET", route+"/read", nil, &current)
			if err == nil {
				err = validateSelfReadDirect(current, control.syncBinding, conversation)
			}
			return true, printSelfJSON(current, err)
		}
		if len(fields) != 2 {
			return true, fmt.Errorf("read CONVERSATION_ID OBSERVED_ID[,OBSERVED_ID...]")
		}
		observed := strings.Split(fields[1], ",")
		seen := map[string]bool{}
		if len(observed) > 200 {
			return true, syncRefuse("INVALID_OBSERVED_IDS")
		}
		for _, message := range observed {
			if !syncUUIDPattern.MatchString(message) || seen[message] {
				return true, syncRefuse("INVALID_OBSERVED_IDS")
			}
			seen[message] = true
		}
		var current map[string]any
		if err = c.call("GET", route, nil, &current); err != nil {
			return true, err
		}
		generation, _ := current["membershipGeneration"].(string)
		if n, e := syncCounter(generation); e != nil || n == 0 {
			return true, syncRefuse("QUALIFICATION_CHANGED")
		}
		return true, c.newSelfOperation("read", conversation, "PUT", route+"/read", map[string]any{"membershipGeneration": generation, "observedMessageIds": observed})
	}
	var current selfDraft
	if err = c.call("GET", route+"/draft", nil, &current); err != nil {
		return true, err
	}
	if err = current.selfScope.check(control.syncBinding, conversation); err != nil {
		return true, err
	}
	if err = selfDraftValid(current); err != nil {
		return true, err
	}
	if cmd == "cloud-draft" {
		current.Admission = ""
		if current.Reply != nil {
			current.Reply.Admission = ""
		}
		return true, printJSON(current)
	}
	if cmd == "cloud-draft-adopt" {
		if current.ContentState == "unavailable" || len(fields) != 2 || fields[1] != current.Version {
			return true, syncRefuse("DRAFT_CONTENT_UNAVAILABLE")
		}
		replacement := ""
		if current.Owned != nil {
			replacement = *current.Owned
		}
		return true, c.adoptSelfDraft(conversation, text, replacement)
	}
	if cmd == "cloud-draft-recover" {
		if current.Recovery == nil || len(fields) != 2 || fields[1] != current.Recovery.Operation {
			return true, syncRefuse("DRAFT_RECOVERY_REVIEW_REQUIRED")
		}
		return true, c.adoptSelfDraft(conversation, text, current.Recovery.Text)
	}
	if current.Admission == "" {
		return true, syncRefuse("DRAFT_ADMISSION_REQUIRED")
	}
	body := map[string]any{"expectedVersion": current.Version, "draftAdmission": current.Admission}
	if cmd == "cloud-draft-clear" {
		return true, c.newSelfOperation("draft", conversation, "DELETE", route+"/draft", body)
	}
	if !hasText || !utf8.ValidString(text) || len(text) > 16384 {
		return true, fmt.Errorf("%s CONVERSATION_ID [source] | ORIGINAL_TEXT (up to 16384 UTF-8 bytes)", cmd)
	}
	if !*current.Writable {
		return true, syncRefuse("DRAFT_NOT_WRITABLE")
	}
	body["ownedText"] = text
	body["reply"] = nil
	if cmd == "cloud-draft-reply" {
		if len(fields) != 3 || !syncUUIDPattern.MatchString(fields[1]) {
			return true, fmt.Errorf("cloud-draft-reply CONVERSATION_ID MESSAGE_ID SOURCE_CONTENT_VERSION | ORIGINAL_TEXT")
		}
		if n, e := syncCounter(fields[2]); e != nil || n == 0 {
			return true, syncRefuse("DRAFT_REPLY_INVALID")
		}
		var message struct {
			ID                 string `json:"id"`
			Conversation       string `json:"conversationId"`
			Version            string `json:"contentVersion"`
			Admission          string `json:"replyAdmission"`
			Deleted            bool   `json:"deleted"`
			Unavailable        bool   `json:"contentUnavailable"`
			ForwardUnavailable bool   `json:"forwardUnavailable"`
		}
		if err = c.call("GET", "/api/messages/"+url.PathEscape(fields[1]), nil, &message); err != nil {
			return true, err
		}
		if message.ID != fields[1] || message.Conversation != conversation || message.Version != fields[2] || message.Admission == "" || message.Deleted || message.Unavailable || message.ForwardUnavailable {
			return true, syncRefuse("DRAFT_REPLY_RESELECT_REQUIRED")
		}
		body["reply"] = selfReply{MessageID: message.ID, Version: message.Version, Admission: message.Admission}
	}
	return true, c.newSelfOperation("draft", conversation, "PUT", route+"/draft", body)
}
func validateSelfReadDirect(raw json.RawMessage, binding syncBinding, conversation string) error {
	var v selfRead
	if syncDecode(raw, &v) != nil || v.Writable == nil || v.CountStatus != "exact_current_source" || v.Unread == nil {
		return syncRefuse("READ_RESPONSE_INVALID")
	}
	if err := v.selfScope.check(binding, conversation); err != nil {
		return err
	}
	if err := v.Point.check(); err != nil {
		return err
	}
	if _, err := syncCounter(*v.Unread); err != nil {
		return err
	}
	if _, err := syncCounter(v.CountWatermark); err != nil {
		return err
	}
	_, err := syncCounter(v.Membership)
	return err
}
func validateSelfResult(o selfOperation, raw json.RawMessage) error {
	var operation struct {
		Result json.RawMessage `json:"result"`
		Status string          `json:"status"`
	}
	if syncDecode(raw, &operation) != nil {
		return syncRefuse("SELF_RESULT_INVALID")
	}
	var result map[string]json.RawMessage
	if syncDecode(operation.Result, &result) != nil {
		return syncRefuse("SELF_RESULT_INVALID")
	}
	if operation.Status == "applied" {
		var version string
		if syncDecode(result["version"], &version) != nil {
			return syncRefuse("SELF_RESULT_INVALID")
		}
		if n, e := syncCounter(version); e != nil || n == 0 {
			return syncRefuse("SELF_RESULT_INVALID")
		}
	}
	switch o.Kind {
	case "font":
		var value struct {
			Font string `json:"fontPreset"`
		}
		if syncDecode(operation.Result, &value) != nil || !oneOf(value.Font, "small", "standard", "large") {
			return syncRefuse("SELF_RESULT_INVALID")
		}
		var original struct {
			Font string `json:"fontPreset"`
		}
		if syncDecode([]byte(o.Body), &original) != nil || value.Font != original.Font {
			return syncRefuse("SELF_RESULT_INVALID")
		}
	case "view":
		var value selfView
		if syncDecode(operation.Result, &value) != nil || value.Conversation != o.Conversation || value.Archived == nil || value.Hidden == nil || value.Manual == nil {
			return syncRefuse("SELF_RESULT_INVALID")
		}
		if _, err := syncCounter(value.ManualGeneration); err != nil {
			return err
		}
	case "read":
		var value struct {
			Membership string    `json:"membershipGeneration"`
			Point      selfPoint `json:"readPoint"`
			IDs        []string  `json:"observedMessageIds"`
			Status     string    `json:"countStatus"`
			More       *bool     `json:"hasMoreEligible"`
			Unordered  *bool     `json:"hasUnorderedEligible"`
		}
		var original struct {
			Membership string   `json:"membershipGeneration"`
			IDs        []string `json:"observedMessageIds"`
		}
		if syncDecode(operation.Result, &value) != nil || syncDecode([]byte(o.Body), &original) != nil || value.Status != "not_computed" || value.More == nil || value.Unordered == nil || value.Membership != original.Membership || value.IDs == nil {
			return syncRefuse("SELF_RESULT_INVALID")
		}
		a := append([]string{}, value.IDs...)
		b := append([]string{}, original.IDs...)
		sort.Strings(a)
		sort.Strings(b)
		if strings.Join(a, ",") != strings.Join(b, ",") {
			return syncRefuse("SELF_RESULT_INVALID")
		}
		return value.Point.check()
	case "draft":
		var value struct {
			Content string `json:"contentState"`
			Reply   string `json:"replyState"`
		}
		if syncDecode(operation.Result, &value) != nil || !oneOf(value.Content, "empty", "owned", "unavailable") || !oneOf(value.Reply, "none", "available", "unavailable") {
			return syncRefuse("SELF_RESULT_INVALID")
		}
	default:
		return syncRefuse("SELF_OPERATION_INVALID")
	}
	return nil
}
func (c *client) adoptSelfDraft(conversation, expected, replacement string) error {
	s := c.store
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var actual string
	err = tx.QueryRow("SELECT text FROM drafts WHERE epoch=? AND conversation_id=?", s.epoch, conversation).Scan(&actual)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if actual != expected {
		return fmt.Errorf("local draft changed; repeat with the exact current local text after |")
	}
	if !c.syncCurrent(s, nil) {
		return syncRefuse("SYNC_CONTEXT_CHANGED")
	}
	if _, err = tx.Exec("INSERT INTO drafts VALUES(?,?,?,?) ON CONFLICT(epoch,conversation_id) DO UPDATE SET text=excluded.text,updated_at=excluded.updated_at", s.epoch, conversation, replacement, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
func printSelfJSON(raw json.RawMessage, err error) error {
	if err != nil {
		return err
	}
	return printJSON(raw)
}
func validateViewChanges(changes map[string]any) error {
	for key, value := range changes {
		switch key {
		case "archived", "hidden", "manualUnread":
			if _, ok := value.(bool); !ok {
				return syncRefuse("INVALID_VIEW_CHANGES")
			}
		case "pinnedRank":
			if value != nil {
				n, ok := value.(json.Number)
				if !ok {
					return syncRefuse("INVALID_PIN_RANK")
				}
				v, e := n.Int64()
				if e != nil || v < 0 || v > 999999 {
					return syncRefuse("INVALID_PIN_RANK")
				}
			}
		case "mutedUntil":
			if value != nil {
				s, ok := value.(string)
				if !ok || !strings.HasSuffix(s, "Z") {
					return syncRefuse("INVALID_MUTE_TIME")
				}
				if _, e := time.Parse(time.RFC3339Nano, s); e != nil {
					return syncRefuse("INVALID_MUTE_TIME")
				}
			}
		case "manualUnreadFromMessageId":
			s, ok := value.(string)
			if !ok || !syncUUIDPattern.MatchString(s) || changes["manualUnread"] != true {
				return syncRefuse("INVALID_VIEW_CHANGES")
			}
		}
	}
	return nil
}

// Stable order is useful to inspect every view, including archived/hidden conversations.
func (s *localStore) selfObjects(kind string) ([]json.RawMessage, error) {
	if err := s.syncReady(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query("SELECT payload FROM sync_objects WHERE epoch=? AND kind=? AND state='available' ORDER BY id", s.epoch, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		items = append(items, json.RawMessage(raw))
	}
	sort.SliceStable(items, func(i, j int) bool { return string(items[i]) < string(items[j]) })
	return items, rows.Err()
}
func (s *localStore) selfConversationListing(items []json.RawMessage, all bool) (map[string]any, error) {
	views, err := s.selfObjects("conversation_view")
	if err != nil {
		return nil, err
	}
	byConversation := map[string]selfView{}
	for _, raw := range views {
		var wrapper struct {
			Object selfView `json:"object"`
		}
		if syncDecode(raw, &wrapper) != nil || wrapper.Object.Hidden == nil || wrapper.Object.Archived == nil {
			return nil, syncRefuse("VIEW_RESPONSE_INVALID")
		}
		byConversation[wrapper.Object.Conversation] = wrapper.Object
	}
	type entry struct {
		id   string
		raw  json.RawMessage
		rank *int
	}
	ordered := []entry{}
	for _, raw := range items {
		var conversation struct {
			ID string `json:"id"`
		}
		if syncDecode(raw, &conversation) != nil {
			return nil, syncRefuse("SYNC_INVALID_CONVERSATION")
		}
		view, present := byConversation[conversation.ID]
		if present && !all && (*view.Hidden || *view.Archived) {
			continue
		}
		ordered = append(ordered, entry{conversation.ID, raw, view.Pinned})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.rank != nil && b.rank == nil {
			return true
		}
		if a.rank == nil && b.rank != nil {
			return false
		}
		if a.rank != nil && b.rank != nil && *a.rank != *b.rank {
			return *a.rank < *b.rank
		}
		return a.id < b.id
	})
	filtered := []json.RawMessage{}
	for _, value := range ordered {
		filtered = append(filtered, value.raw)
	}
	font := "standard"
	preferences, err := s.selfObjects("preferences")
	if err != nil {
		return nil, err
	}
	for _, raw := range preferences {
		var wrapper struct {
			Object selfPreferences `json:"object"`
		}
		if syncDecode(raw, &wrapper) != nil {
			return nil, syncRefuse("PREFERENCE_RESPONSE_INVALID")
		}
		font = wrapper.Object.Font
	}
	return map[string]any{"source": "authoritative_sync", "schemaVersion": "2", "fontPreset": font, "terminalFontChanged": false, "items": filtered, "ownViews": byConversation, "includesHiddenArchived": all}, nil
}
