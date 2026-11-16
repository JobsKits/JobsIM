// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"
)

type localConversation struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	MemberIDs            []string `json:"memberIds"`
	MembershipGeneration string   `json:"membershipGeneration"`
}

type governanceOperation struct {
	ID           string          `json:"operationId"`
	Kind         string          `json:"kind"`
	PeerID       string          `json:"peerId"`
	RestoreEpoch string          `json:"restoreEpoch"`
	Payload      json.RawMessage `json:"payload"`
	PayloadHash  string          `json:"payloadHash"`
	State        string          `json:"state"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
}

func (c *client) conversationRecords() ([]json.RawMessage, error) {
	var directory struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := c.call("GET", "/api/conversations", nil, &directory); err != nil {
		return nil, err
	}
	return directory.Items, nil
}

func (s *localStore) rememberConversationsTx(tx *sql.Tx, items []json.RawMessage) error {
	for _, raw := range items {
		var conversation localConversation
		if err := json.Unmarshal(raw, &conversation); err != nil {
			return err
		}
		if conversation.ID == "" || (conversation.Kind != "direct" && conversation.Kind != "group" && conversation.Kind != "self") {
			return fmt.Errorf("invalid authoritative conversation directory")
		}
		if _, err := tx.Exec("INSERT INTO conversations(epoch,id,payload) VALUES(?,?,?) ON CONFLICT(epoch,id) DO UPDATE SET payload=excluded.payload", s.epoch, conversation.ID, string(raw)); err != nil {
			return err
		}
	}
	return nil
}
func (s *localStore) rememberConversations(items []json.RawMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.rememberConversationsTx(tx, items); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *localStore) prepareRelationshipRevocation(peer, reason string, fresh []json.RawMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.rememberConversationsTx(tx, fresh); err != nil {
		return err
	}
	rows, err := tx.Query("SELECT epoch,payload FROM conversations")
	if err != nil {
		return err
	}
	type target struct{ epoch, conversation string }
	targets := map[target]bool{}
	for rows.Next() {
		var raw, epoch string
		if err = rows.Scan(&epoch, &raw); err != nil {
			rows.Close()
			return err
		}
		var conversation localConversation
		if err = json.Unmarshal([]byte(raw), &conversation); err != nil {
			rows.Close()
			return err
		}
		if conversation.Kind != "direct" {
			continue
		}
		mine, theirs := false, false
		for _, user := range conversation.MemberIDs {
			mine = mine || user == s.identity.UserID
			theirs = theirs || user == peer
		}
		if mine && theirs {
			targets[target{epoch, conversation.ID}] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for conversation := range targets {
		if _, err = tx.Exec("UPDATE outbox SET state='frozen',freeze_reason=?,updated_at=? WHERE restore_epoch=? AND conversation_id=? AND state='pending'", reason, time.Now().UTC().Format(time.RFC3339Nano), conversation.epoch, conversation.conversation); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *localStore) prepareLeave(conversation, originalGeneration string, fresh []json.RawMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.rememberConversationsTx(tx, fresh); err != nil {
		return err
	}
	var raw string
	if err = tx.QueryRow("SELECT payload FROM conversations WHERE epoch=? AND id=?", s.epoch, conversation).Scan(&raw); err != nil {
		return fmt.Errorf("current group snapshot unavailable: %w", err)
	}
	var snapshot localConversation
	if err = json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return err
	}
	if snapshot.Kind != "group" {
		return fmt.Errorf("leave requires a group conversation")
	}
	if !decimal.MatchString(originalGeneration) || snapshot.MembershipGeneration != originalGeneration {
		return fmt.Errorf("original membership generation is no longer current; no local history or Outbox changed")
	}
	if _, err = tx.Exec("UPDATE outbox SET state='frozen',freeze_reason='CONVERSATION_LEFT',updated_at=? WHERE conversation_id=? AND state!='confirmed'", time.Now().UTC().Format(time.RFC3339Nano), conversation); err != nil {
		return err
	}
	for _, table := range []string{"messages", "cursors", "conversations"} {
		key := "conversation_id"
		if table == "conversations" {
			key = "id"
		}
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE "+key+"=?", conversation); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func operationHash(op governanceOperation) string {
	raw, _ := json.Marshal(map[string]any{"kind": op.Kind, "peerId": op.PeerID, "restoreEpoch": op.RestoreEpoch, "payload": op.Payload})
	return fingerprint(string(raw))
}

type rowScanner interface{ Scan(...any) error }

func scanOperation(row rowScanner) (governanceOperation, error) {
	var op governanceOperation
	var raw string
	var result sql.NullString
	if err := row.Scan(&op.ID, &op.Kind, &op.PeerID, &op.RestoreEpoch, &raw, &op.PayloadHash, &op.State, &result, &op.ErrorCode); err != nil {
		return op, err
	}
	op.Payload = json.RawMessage(raw)
	if result.Valid {
		op.Result = json.RawMessage(result.String)
	}
	var payload map[string]string
	if err := json.Unmarshal(op.Payload, &payload); err != nil {
		return op, fmt.Errorf("local governance payload invalid")
	}
	valid := payload["requestId"] == op.ID && op.PeerID != "" && decimal.MatchString(op.RestoreEpoch)
	valid = valid && ((op.Kind == "request" && len(payload) == 2 && payload["userId"] == op.PeerID) || (op.Kind == "reopen" && len(payload) == 1))
	if !valid || operationHash(op) != op.PayloadHash {
		return op, fmt.Errorf("local governance integrity mismatch; nothing sent")
	}
	return op, nil
}

const operationColumns = "id,kind,peer_id,restore_epoch,payload,payload_hash,state,result,error_code"

func (s *localStore) operation(original string) (governanceOperation, error) {
	return scanOperation(s.db.QueryRow("SELECT "+operationColumns+" FROM governance_operations WHERE id=?", original))
}
func (s *localStore) operations() ([]governanceOperation, error) {
	rows, err := s.db.Query("SELECT " + operationColumns + " FROM governance_operations ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []governanceOperation{}
	for rows.Next() {
		op, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, op)
	}
	return result, rows.Err()
}
func (s *localStore) prepareOperation(kind, peer string) (governanceOperation, error) {
	if (kind != "request" && kind != "reopen") || peer == "" {
		return governanceOperation{}, fmt.Errorf("request or reopen requires a peer identity")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return governanceOperation{}, err
	}
	defer tx.Rollback()
	original, err := scanOperation(tx.QueryRow("SELECT "+operationColumns+" FROM governance_operations WHERE kind=? AND peer_id=? AND state='pending'", kind, peer))
	if err == nil {
		return original, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return original, err
	}
	op := governanceOperation{ID: id(), Kind: kind, PeerID: peer, RestoreEpoch: s.epoch, State: "pending"}
	payload := map[string]string{"requestId": op.ID}
	if kind == "request" {
		payload["userId"] = peer
	}
	op.Payload, _ = json.Marshal(payload)
	op.PayloadHash = operationHash(op)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.Exec("INSERT INTO governance_operations(id,kind,peer_id,restore_epoch,payload,payload_hash,state,created_at,updated_at) VALUES(?,?,?,?,?,?,'pending',?,?)", op.ID, kind, peer, op.RestoreEpoch, string(op.Payload), op.PayloadHash, now, now); err != nil {
		return op, err
	}
	return op, tx.Commit()
}
func (s *localStore) finishOperation(op governanceOperation, state string, result json.RawMessage, code string) error {
	var saved any
	if result != nil {
		saved = string(result)
	}
	_, err := s.db.Exec("UPDATE governance_operations SET state=?,result=?,error_code=?,updated_at=? WHERE id=? AND payload_hash=? AND state='pending'", state, saved, code, time.Now().UTC().Format(time.RFC3339Nano), op.ID, op.PayloadHash)
	return err
}
func (c *client) governance(kind, peer string) error {
	if c.store == nil {
		return fmt.Errorf("durable local store unavailable; nothing sent")
	}
	op, err := c.store.prepareOperation(kind, peer)
	if err != nil {
		return err
	}
	return c.resumeGovernance(op.ID)
}
func (c *client) resumeGovernance(original string) error {
	op, err := c.store.operation(original)
	if err != nil {
		return err
	}
	if op.State == "frozen" || op.State == "failed" {
		return fmt.Errorf("original operation is %s (%s); inspect current relationship before creating a new intent", op.State, op.ErrorCode)
	}
	if op.State == "pending" {
		var boot struct {
			DeploymentID string `json:"deploymentId"`
			RestoreEpoch string `json:"restoreEpoch"`
		}
		if err = c.call("GET", "/api/bootstrap", nil, &boot); err != nil {
			return err
		}
		if boot.DeploymentID != c.deployment || boot.RestoreEpoch != op.RestoreEpoch || op.RestoreEpoch != c.epoch {
			if e := c.store.finishOperation(op, "frozen", nil, "RESTORE_EPOCH_CHANGED"); e != nil {
				return e
			}
			return fmt.Errorf("original operation frozen after instance/epoch mismatch; nothing sent")
		}
		path := "/api/contacts/requests"
		if op.Kind == "reopen" {
			path = "/api/contacts/" + url.PathEscape(op.PeerID) + "/reopen"
		}
		var result json.RawMessage
		if err = c.call("POST", path, op.Payload, &result); err != nil {
			var failure *apiFailure
			if errors.As(err, &failure) && failure.Status >= 400 && failure.Status < 500 && failure.Status != 429 {
				if e := c.store.finishOperation(op, "failed", nil, failure.Code); e != nil {
					return e
				}
			}
			return fmt.Errorf("original %s operation %s retained; operations / resume ORIGINAL_ID: %w", op.Kind, op.ID, err)
		}
		if err = c.store.finishOperation(op, "completed", result, ""); err != nil {
			return err
		}
		op.Result = result
		op.State = "completed"
	}
	var relationship any
	status := "current"
	if err = c.call("GET", "/api/contacts/"+url.PathEscape(op.PeerID)+"/relationship", nil, &relationship); err != nil {
		status = "unavailable"
	}
	return printJSON(map[string]any{"operationId": op.ID, "state": op.State, "result": op.Result, "relationshipStatus": status, "relationship": relationship})
}
