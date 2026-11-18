// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type collaborationCapabilities struct {
	DeploymentID string `json:"deploymentId"`
	RestoreEpoch string `json:"restoreEpoch"`
	Features     struct {
		SendAdmission bool `json:"send_admission"`
		Mentions      bool `json:"mentions"`
		RoleAdmission bool `json:"role_admission"`
		Announcements bool `json:"announcements"`
	} `json:"features"`
}

func (c *client) collaborationContext() (collaborationCapabilities, error) {
	var boot collaborationCapabilities
	if c.store == nil || c.store.identity.Origin != c.base || c.store.identity.DeploymentID != c.deployment || c.store.identity.UserID != c.user || c.store.epoch != c.epoch {
		return boot, fmt.Errorf("local context mismatch; nothing sent")
	}
	err := c.call("GET", "/api/bootstrap", nil, &boot)
	if err != nil {
		return boot, err
	}
	if boot.DeploymentID != c.deployment || boot.RestoreEpoch != c.epoch {
		return boot, fmt.Errorf("instance/restore epoch changed; original local state retained")
	}
	return boot, nil
}

type lookupSession struct {
	Kind, Query, Conversation, Cursor, Context string
	HasMore                                    bool
	Sources                                    map[string]string
}

func (c *client) lookupContext() string {
	return c.base + "|" + c.deployment + "|" + c.user + "|" + c.epoch + "|" + c.device
}
func (c *client) startLookup(kind, query, conversation string) error {
	c.lookup = nil
	if kind == "search" && (!utf8.ValidString(query) || len(query) < 1 || len(query) > 256) {
		return fmt.Errorf("search requires 1..256 UTF-8 bytes")
	}
	c.lookup = &lookupSession{Kind: kind, Query: query, Conversation: conversation, Context: c.lookupContext(), Sources: map[string]string{}}
	return c.lookupPage(false)
}
func (c *client) lookupPage(more bool) error {
	l := c.lookup
	if l == nil || l.Context != c.lookupContext() {
		c.lookup = nil
		return fmt.Errorf("start search or favorites in the current account first")
	}
	if more && !l.HasMore {
		return fmt.Errorf("no more results; run search/favorites again to reload")
	}
	if _, err := c.collaborationContext(); err != nil {
		c.lookup = nil
		return err
	}
	q := url.Values{"limit": {"50"}}
	if l.Kind == "search" {
		q.Set("q", l.Query)
	}
	if l.Conversation != "" {
		q.Set("conversationId", l.Conversation)
	}
	if more {
		q.Set("cursor", l.Cursor)
	}
	var page struct {
		Items   []json.RawMessage `json:"items"`
		Cursor  string            `json:"cursor"`
		HasMore bool              `json:"hasMore"`
	}
	if err := c.call("GET", "/api/"+l.Kind+"?"+q.Encode(), nil, &page); err != nil {
		c.lookup = nil
		return err
	}
	if _, err := c.collaborationContext(); err != nil {
		c.lookup = nil
		return err
	}
	if page.HasMore && (page.Cursor == "" || (more && page.Cursor == l.Cursor)) {
		c.lookup = nil
		return fmt.Errorf("invalid paging response")
	}
	sources := map[string]string{}
	for k, v := range l.Sources {
		sources[k] = v
	}
	for _, raw := range page.Items {
		var m cacheMessage
		if err := json.Unmarshal(raw, &m); err != nil || m.ID == "" || m.ConversationID == "" {
			c.lookup = nil
			return fmt.Errorf("invalid message result")
		}
		sources[m.ID] = m.ConversationID
	}
	l.Sources, l.Cursor, l.HasMore = sources, page.Cursor, page.HasMore
	return printJSON(page)
}
func (c *client) openLookup(id string) error {
	l := c.lookup
	if l == nil || l.Context != c.lookupContext() {
		c.lookup = nil
		return fmt.Errorf("start search/favorites first")
	}
	conversation, ok := l.Sources[id]
	if !ok {
		return fmt.Errorf("message must be selected from this search/favorites session")
	}
	if _, err := c.collaborationContext(); err != nil {
		c.lookup = nil
		return err
	}
	var current struct {
		ID string `json:"id"`
	}
	if err := c.call("GET", "/api/conversations/"+url.PathEscape(conversation), nil, &current); err != nil {
		c.lookup = nil
		return err
	}
	if current.ID != conversation {
		c.lookup = nil
		return fmt.Errorf("source conversation mismatch")
	}
	var raw json.RawMessage
	if err := c.call("GET", "/api/messages/"+url.PathEscape(id), nil, &raw); err != nil {
		c.lookup = nil
		return err
	}
	var m cacheMessage
	if err := json.Unmarshal(raw, &m); err != nil || m.ID != id || m.ConversationID != conversation {
		c.lookup = nil
		return fmt.Errorf("source message mismatch")
	}
	if _, err := c.collaborationContext(); err != nil {
		c.lookup = nil
		return err
	}
	return printJSON(map[string]any{"source": "exact_authorized_message", "message": raw})
}
func (p pending) hasMentions() bool { return p.MentionAll || len(p.MentionedUserIDs) > 0 }
func mentionPermanent(code string) bool {
	switch code {
	case "SEND_ADMISSION_CHANGED", "QUALIFICATION_CHANGED", "INVALID_MENTIONS", "MENTION_MEMBER_REQUIRED", "MENTION_FORBIDDEN", "MENTION_ALL_FORBIDDEN", "FEATURE_DISABLED", "NOT_A_MEMBER", "HISTORY_NOT_ALLOWED":
		return true
	}
	return false
}
func (s *localStore) freezeMention(p pending, code string) error {
	_, err := s.db.Exec("UPDATE outbox SET state='frozen',freeze_reason=?,updated_at=? WHERE client_msg_id=? AND state='pending'", code, time.Now().UTC().Format(time.RFC3339Nano), p.ClientMsgID)
	return err
}
func (c *client) mention(conversation, targets, text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("mention text required")
	}
	p := pending{ConversationID: conversation, Type: "text", Text: text}
	if targets == "all" {
		p.MentionAll = true
	} else {
		seen := map[string]bool{}
		for _, v := range strings.Split(targets, ",") {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				return fmt.Errorf("mention targets must be unique IDs")
			}
			seen[v] = true
			p.MentionedUserIDs = append(p.MentionedUserIDs, v)
		}
		if len(p.MentionedUserIDs) > 20 {
			return fmt.Errorf("at most 20 explicit targets in this client")
		}
	}
	return c.queueAndSend(p)
}
func (c *client) prepareMentions(p *pending) error {
	boot, err := c.collaborationContext()
	if err != nil {
		return err
	}
	if !boot.Features.SendAdmission || !boot.Features.Mentions || (p.MentionAll && !boot.Features.RoleAdmission) {
		return fmt.Errorf("server does not confirm mention/role admission; nothing queued")
	}
	var group struct {
		ID, Kind       string
		OwnerID        string `json:"ownerId"`
		ReadOnly       bool   `json:"readOnly"`
		SendAdmission  string `json:"sendAdmission"`
		RoleGeneration string `json:"roleGeneration"`
	}
	if err = c.call("GET", "/api/conversations/"+url.PathEscape(p.ConversationID), nil, &group); err != nil {
		return err
	}
	if group.ID != p.ConversationID || group.Kind != "group" || group.ReadOnly || group.SendAdmission == "" {
		return fmt.Errorf("current writable group unavailable")
	}
	if p.MentionAll && (group.OwnerID != c.user || !decimal.MatchString(group.RoleGeneration)) {
		return fmt.Errorf("current protected group owner required for @all")
	}
	if !p.MentionAll {
		var roster struct {
			Items []struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"items"`
		}
		if err = c.call("GET", "/api/conversations/"+url.PathEscape(p.ConversationID)+"/members", nil, &roster); err != nil {
			return err
		}
		active := map[string]bool{}
		for _, m := range roster.Items {
			if m.State == "" || m.State == "active" {
				active[m.ID] = true
			}
		}
		for _, v := range p.MentionedUserIDs {
			if !active[v] {
				return fmt.Errorf("mention target is not a current active member")
			}
		}
	}
	p.SendAdmission, p.RoleGeneration = group.SendAdmission, group.RoleGeneration
	return nil
}
func (c *client) transmitMentions(p pending) error {
	boot, err := c.collaborationContext()
	if err != nil {
		if boot.DeploymentID != "" && (boot.DeploymentID != c.deployment || boot.RestoreEpoch != c.epoch) {
			if freezeErr := c.store.freezeMention(p, "RESTORE_EPOCH_CHANGED"); freezeErr != nil {
				return freezeErr
			}
		}
		return err
	}
	if !boot.Features.SendAdmission {
		return fmt.Errorf("send admission unsupported; no POST sent")
	}
	var result json.RawMessage
	writable := boot.Features.Mentions && (!p.MentionAll || (boot.Features.RoleAdmission && p.RoleGeneration != "")) && p.State == "pending"
	path := "/api/conversations/" + url.PathEscape(p.ConversationID) + "/messages"
	if !writable {
		path = "/api/conversations/" + url.PathEscape(p.ConversationID) + "/message-results"
	}
	if err = c.call("POST", path, p.payload(), &result); err != nil {
		var failure *apiFailure
		if errors.As(err, &failure) && mentionPermanent(failure.Code) {
			if e := c.store.freezeMention(p, failure.Code); e != nil {
				return e
			}
		}
		return fmt.Errorf("original mention %s retained without changing targets/qualification: %w", p.ClientMsgID, err)
	}
	return c.reconcileReadOnly(p)
}

type announcementOperation struct {
	ID           string          `json:"operationId"`
	Conversation string          `json:"conversationId"`
	Epoch        string          `json:"restoreEpoch"`
	Method       string          `json:"method"`
	Payload      json.RawMessage `json:"payload"`
	Hash         string          `json:"payloadHash"`
	State        string          `json:"state"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
}

const announcementColumns = "id,conversation_id,restore_epoch,method,payload,payload_hash,state,result,error_code"

func announcementHash(op announcementOperation) string {
	raw, _ := json.Marshal(map[string]any{"conversationId": op.Conversation, "restoreEpoch": op.Epoch, "method": op.Method, "payload": op.Payload})
	return fingerprint(string(raw))
}
func scanAnnouncement(row rowScanner) (announcementOperation, error) {
	var op announcementOperation
	var raw string
	var result sql.NullString
	err := row.Scan(&op.ID, &op.Conversation, &op.Epoch, &op.Method, &raw, &op.Hash, &op.State, &result, &op.ErrorCode)
	if err != nil {
		return op, err
	}
	op.Payload = json.RawMessage(raw)
	if result.Valid {
		op.Result = json.RawMessage(result.String)
	}
	var body map[string]string
	if json.Unmarshal(op.Payload, &body) != nil || body["requestId"] != op.ID || op.Conversation == "" || !decimal.MatchString(op.Epoch) || (op.Method != "PUT" && op.Method != "DELETE") || announcementHash(op) != op.Hash {
		return op, fmt.Errorf("announcement original integrity mismatch")
	}
	if err := validateAnnouncementBody(op); err != nil {
		return op, err
	}
	return op, nil
}
func validateAnnouncementBody(op announcementOperation) error {
	var body map[string]string
	if json.Unmarshal(op.Payload, &body) != nil || body["requestId"] != op.ID || op.Conversation == "" || !decimal.MatchString(op.Epoch) || (op.Method != "PUT" && op.Method != "DELETE") {
		return fmt.Errorf("invalid original announcement")
	}
	allowed := map[string]bool{"requestId": true, "expectedVersion": true, "membershipGeneration": true, "roleGeneration": true}
	if op.Method == "PUT" {
		allowed["messageId"], allowed["sourceVersion"], allowed["expiresAt"] = true, true, true
	}
	for key := range body {
		if !allowed[key] {
			return fmt.Errorf("announcement contains unsupported payload fields")
		}
	}
	if !nonnegativeDecimal(body["expectedVersion"]) || !decimal.MatchString(body["membershipGeneration"]) || (body["roleGeneration"] != "" && !decimal.MatchString(body["roleGeneration"])) || (op.Method == "PUT" && (body["messageId"] == "" || !decimal.MatchString(body["sourceVersion"]))) {
		return fmt.Errorf("announcement original qualification invalid")
	}
	return nil
}

func (s *localStore) announcementOperation(id string) (announcementOperation, error) {
	return scanAnnouncement(s.db.QueryRow("SELECT "+announcementColumns+" FROM announcement_operations WHERE id=?", id))
}
func (s *localStore) announcementOperations() ([]announcementOperation, error) {
	rows, err := s.db.Query("SELECT " + announcementColumns + " FROM announcement_operations ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []announcementOperation{}
	for rows.Next() {
		op, e := scanAnnouncement(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, op)
	}
	return result, rows.Err()
}
func (s *localStore) prepareAnnouncement(op announcementOperation) (announcementOperation, error) {
	if err := validateAnnouncementBody(op); err != nil {
		return op, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return op, err
	}
	defer tx.Rollback()
	if old, e := scanAnnouncement(tx.QueryRow("SELECT "+announcementColumns+" FROM announcement_operations WHERE conversation_id=? AND state='pending'", op.Conversation)); e == nil {
		return old, fmt.Errorf("pending original announcement %s; use resume-announcement", old.ID)
	} else if !errors.Is(e, sql.ErrNoRows) {
		return op, e
	}
	op.Hash = announcementHash(op)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec("INSERT INTO announcement_operations(id,conversation_id,restore_epoch,method,payload,payload_hash,state,created_at,updated_at) VALUES(?,?,?,?,?,?,'pending',?,?)", op.ID, op.Conversation, op.Epoch, op.Method, string(op.Payload), op.Hash, now, now)
	if err != nil {
		return op, err
	}
	return op, tx.Commit()
}
func sanitizeAnnouncementFact(op announcementOperation, raw json.RawMessage) (json.RawMessage, error) {
	var envelope struct {
		OperationID string `json:"operationId"`
		Status      string `json:"status"`
		Current     bool   `json:"current"`
		Result      struct {
			ConversationID string `json:"conversationId"`
			Version        string `json:"version"`
			State          string `json:"state"`
			MessageID      string `json:"messageId,omitempty"`
			SourceVersion  string `json:"sourceVersion,omitempty"`
			ExpiresAt      string `json:"expiresAt,omitempty"`
			UpdatedAt      string `json:"updatedAt,omitempty"`
			CanManage      bool   `json:"canManage"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.OperationID != op.ID || envelope.Status != "applied" || envelope.Result.ConversationID != op.Conversation {
		return nil, fmt.Errorf("invalid original announcement fact")
	}
	return json.Marshal(envelope)
}
func (s *localStore) finishAnnouncement(op announcementOperation, state string, result json.RawMessage, code string) error {
	var stored any
	if len(result) > 0 {
		raw, err := sanitizeAnnouncementFact(op, result)
		if err != nil {
			return err
		}
		stored = string(raw)
	}

	_, err := s.db.Exec("UPDATE announcement_operations SET state=?,result=COALESCE(?,result),error_code=?,updated_at=? WHERE id=? AND payload_hash=? AND state='pending'", state, stored, code, time.Now().UTC().Format(time.RFC3339Nano), op.ID, op.Hash)
	return err
}
func announcementPermanent(code string) bool {
	switch code {
	case "INVALID_EXPIRY", "INVALID_REQUEST_ID", "INVALID_VERSION", "INVALID_GENERATION", "INVALID_ANNOUNCEMENT", "IDEMPOTENCY_CONFLICT", "ANNOUNCEMENT_FORBIDDEN", "QUALIFICATION_CHANGED", "MEMBERSHIP_GENERATION_CHANGED", "ANNOUNCEMENT_VERSION_CONFLICT", "ANNOUNCEMENT_SOURCE_CHANGED", "SOURCE_VERSION_CONFLICT", "GROUP_OWNER_UNAVAILABLE", "NOT_A_MEMBER", "HISTORY_NOT_ALLOWED", "FORBIDDEN", "FEATURE_DISABLED":
		return true
	}
	return false
}
func (c *client) newAnnouncement(conversation, source, version, expiry string, clear bool) error {
	boot, err := c.collaborationContext()
	if err != nil {
		return err
	}
	if !boot.Features.Announcements || !boot.Features.RoleAdmission {
		return fmt.Errorf("protected announcements unsupported; nothing queued")
	}
	var group struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		OwnerID  string `json:"ownerId"`
		ReadOnly bool   `json:"readOnly"`
		Member   string `json:"membershipGeneration"`
		Role     string `json:"roleGeneration"`
	}
	path := "/api/conversations/" + url.PathEscape(conversation)
	if err = c.call("GET", path, nil, &group); err != nil {
		return err
	}
	if group.ID != conversation || group.Kind != "group" || group.OwnerID != c.user || group.ReadOnly || !decimal.MatchString(group.Member) || !decimal.MatchString(group.Role) {
		return fmt.Errorf("current protected writable group owner required")
	}
	var current struct {
		Version   string `json:"version"`
		CanManage bool   `json:"canManage"`
	}
	if err = c.call("GET", path+"/announcement", nil, &current); err != nil {
		return err
	}
	if !current.CanManage || !nonnegativeDecimal(current.Version) {
		return fmt.Errorf("current announcement not manageable")
	}
	requestID := id()
	body := map[string]string{"requestId": requestID, "expectedVersion": current.Version, "membershipGeneration": group.Member, "roleGeneration": group.Role}
	method := "DELETE"
	if !clear {
		var message struct {
			ID                 string `json:"id"`
			Conversation       string `json:"conversationId"`
			ContentVersion     string `json:"contentVersion"`
			Version            string `json:"version"`
			Deleted            bool   `json:"deleted"`
			Unavailable        bool   `json:"contentUnavailable"`
			ForwardUnavailable bool   `json:"forwardUnavailable"`
		}
		if !decimal.MatchString(version) {
			return fmt.Errorf("source content version required; inspect message first")
		}
		if err = c.call("GET", "/api/messages/"+url.PathEscape(source), nil, &message); err != nil {
			return err
		}
		actual := message.ContentVersion
		if actual == "" {
			actual = message.Version
		}
		if message.ID != source || message.Conversation != conversation || message.Deleted || message.Unavailable || message.ForwardUnavailable || actual != version {
			return fmt.Errorf("source changed/unavailable; inspect and explicitly select again")
		}
		body["messageId"], body["sourceVersion"] = source, version
		method = "PUT"
		switch expiry {
		case "none", "":
		case "1d":
			body["expiresAt"] = time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
		case "7d":
			body["expiresAt"] = time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)
		default:
			return fmt.Errorf("expiry must be none, 1d or 7d")
		}
	}
	raw, _ := json.Marshal(body)
	op, err := c.store.prepareAnnouncement(announcementOperation{ID: requestID, Conversation: conversation, Epoch: c.epoch, Method: method, Payload: raw, State: "pending"})
	if err != nil {
		return err
	}
	return c.resumeAnnouncement(op.ID)
}
func nonnegativeDecimal(s string) bool { return s == "0" || decimal.MatchString(s) }
func (c *client) resumeAnnouncement(original string) error {
	op, err := c.store.announcementOperation(original)
	if err != nil {
		return err
	}
	boot, err := c.collaborationContext()
	if err != nil {
		if boot.DeploymentID != "" && (boot.DeploymentID != c.deployment || boot.RestoreEpoch != op.Epoch) {
			if freezeErr := c.store.finishAnnouncement(op, "frozen", nil, "RESTORE_EPOCH_CHANGED"); freezeErr != nil {
				return freezeErr
			}
		}
		return err
	}
	if op.Epoch != c.epoch {
		return fmt.Errorf("original announcement epoch differs; no write sent")
	}
	path := "/api/conversations/" + url.PathEscape(op.Conversation) + "/announcement"
	var fact json.RawMessage
	err = c.call("GET", path+"/operations/"+url.PathEscape(op.ID), nil, &fact)
	if err == nil {
		if err = c.store.finishAnnouncement(op, "completed", fact, ""); err != nil {
			return err
		}
		var current any
		if err = c.call("GET", path, nil, &current); err != nil {
			return err
		}
		safeFact, sanitizeErr := sanitizeAnnouncementFact(op, fact)
		if sanitizeErr != nil {
			return sanitizeErr
		}
		return printJSON(map[string]any{"operationId": op.ID, "originalFact": safeFact, "currentAnnouncement": current})
	}
	if !isAPIStatus(err, 404) {
		var failure *apiFailure
		if errors.As(err, &failure) && announcementPermanent(failure.Code) {
			if e := c.store.finishAnnouncement(op, "frozen", nil, failure.Code); e != nil {
				return e
			}
		}
		return err
	}
	if op.State != "pending" {
		return fmt.Errorf("original announcement remains %s; no write sent", op.State)
	}
	var body map[string]string
	if json.Unmarshal(op.Payload, &body) != nil {
		return fmt.Errorf("original payload invalid")
	}
	if !boot.Features.Announcements || !boot.Features.RoleAdmission || body["roleGeneration"] == "" {
		return fmt.Errorf("write protection unavailable; original preserved, only read-only lookup sent")
	}
	if err = c.call(op.Method, path, op.Payload, &fact); err != nil {
		var failure *apiFailure
		if errors.As(err, &failure) && announcementPermanent(failure.Code) {
			if e := c.store.finishAnnouncement(op, "frozen", nil, failure.Code); e != nil {
				return e
			}
		}
		return fmt.Errorf("original announcement %s retained: %w", op.ID, err)
	}
	if err = c.store.finishAnnouncement(op, "completed", fact, ""); err != nil {
		return err
	}
	var current any
	if err = c.call("GET", path, nil, &current); err != nil {
		return err
	}
	safeFact, sanitizeErr := sanitizeAnnouncementFact(op, fact)
	if sanitizeErr != nil {
		return sanitizeErr
	}
	return printJSON(map[string]any{"operationId": op.ID, "originalFact": safeFact, "currentAnnouncement": current})
}
func (c *client) collaborationCommand(cmd, args string) (bool, error) {
	parts := strings.Fields(args)
	switch cmd {
	case "search":
		q, conv, _ := strings.Cut(args, " | ")
		return true, c.startLookup("search", strings.TrimSpace(q), strings.TrimSpace(conv))
	case "favorites":
		if len(parts) > 1 {
			return true, fmt.Errorf("favorites [CONVERSATION_ID]")
		}
		return true, c.startLookup("favorites", "", strings.TrimSpace(args))
	case "more":
		if args != "" {
			return true, fmt.Errorf("more uses the original query and opaque cursor")
		}
		return true, c.lookupPage(true)
	case "open-result":
		if len(parts) != 1 {
			return true, fmt.Errorf("open-result MESSAGE_ID")
		}
		return true, c.openLookup(parts[0])
	case "mention":
		head, text, ok := strings.Cut(args, " | ")
		fields := strings.Fields(head)
		if !ok || len(fields) != 2 {
			return true, fmt.Errorf("mention CONVERSATION_ID USER_ID,USER_ID|all | TEXT")
		}
		return true, c.mention(fields[0], fields[1], text)
	case "announcement":
		if len(parts) != 1 {
			return true, fmt.Errorf("announcement CONVERSATION_ID")
		}
		if _, err := c.collaborationContext(); err != nil {
			return true, err
		}
		var result any
		err := c.call("GET", "/api/conversations/"+url.PathEscape(parts[0])+"/announcement", nil, &result)
		if err != nil {
			return true, err
		}
		return true, printJSON(result)
	case "announce":
		if len(parts) != 4 {
			return true, fmt.Errorf("announce CONVERSATION_ID SOURCE_MESSAGE_ID SOURCE_CONTENT_VERSION none|1d|7d")
		}
		return true, c.newAnnouncement(parts[0], parts[1], parts[2], parts[3], false)
	case "clear-announcement":
		if len(parts) != 1 {
			return true, fmt.Errorf("clear-announcement CONVERSATION_ID")
		}
		return true, c.newAnnouncement(parts[0], "", "", "", true)
	case "announcement-operations":
		ops, err := c.store.announcementOperations()
		if err != nil {
			return true, err
		}
		return true, printJSON(ops)
	case "resume-announcement":
		if len(parts) != 1 {
			return true, fmt.Errorf("resume-announcement ORIGINAL_OPERATION_ID")
		}
		return true, c.resumeAnnouncement(parts[0])
	}
	return false, nil
}
