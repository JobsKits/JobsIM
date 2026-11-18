// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
)

type receiptRetentionPolicy struct {
	Mode            string `json:"mode"`
	Delay           *int64 `json:"delaySeconds"`
	Scope           string `json:"receiptScope"`
	MaxAge          int64  `json:"undeliveredMaxAgeSeconds"`
	History         *int64 `json:"historyRetentionSeconds"`
	AttachmentMode  string `json:"attachmentMode"`
	AttachmentAge   int64  `json:"attachmentRetentionSeconds"`
	AttachmentDelay *int64 `json:"attachmentDelaySeconds"`
}

type receiptPolicyOperation struct {
	ID           string      `json:"id"`
	Binding      syncBinding `json:"binding"`
	Conversation string      `json:"conversationId"`
	Body         string      `json:"body"`
	Hash         string      `json:"bodyHash"`
}

type receiptPolicyRequest struct {
	ID       string                 `json:"requestId"`
	Expected string                 `json:"expectedVersion"`
	Policy   receiptRetentionPolicy `json:"policy"`
}

func receiptPolicyValid(p receiptRetentionPolicy) bool {
	if !oneOf(p.Mode, "cloud_history", "after_receipt") || !oneOf(p.Scope, "all_target_devices", "per_user_any_device") || p.MaxAge < 3600 || p.MaxAge > 31536000 || p.AttachmentAge < 3600 || p.AttachmentAge > 31536000 || !oneOf(p.AttachmentMode, "fixed_retention", "after_object_receipt") {
		return false
	}
	if p.Mode == "after_receipt" && (p.Delay == nil || *p.Delay < 0 || *p.Delay > 604800) || p.Mode == "cloud_history" && p.Delay != nil || p.History != nil && (*p.History < 3600 || *p.History > 31536000) {
		return false
	}
	return !(p.AttachmentMode == "after_object_receipt" && (p.AttachmentDelay == nil || *p.AttachmentDelay < 0 || *p.AttachmentDelay > 604800) || p.AttachmentMode == "fixed_retention" && p.AttachmentDelay != nil)
}

func receiptPolicyHash(o receiptPolicyOperation) string {
	o.Hash = ""
	raw, _ := json.Marshal(o)
	return fingerprint(string(raw))
}

func receiptPolicyCheck(o receiptPolicyOperation, binding syncBinding) (receiptPolicyRequest, error) {
	var q receiptPolicyRequest
	if !syncUUIDPattern.MatchString(o.ID) || !syncUUIDPattern.MatchString(o.Conversation) || o.Binding.check(binding, false) != nil || o.Hash != receiptPolicyHash(o) || syncDecode([]byte(o.Body), &q) != nil || q.ID != o.ID || !receiptPolicyValid(q.Policy) {
		return q, syncRefuse("RETENTION_INTENT_CHANGED")
	}
	if n, err := syncCounter(q.Expected); err != nil || n == 0 || n == math.MaxInt64 {
		return q, syncRefuse("RETENTION_INTENT_CHANGED")
	}
	return q, nil
}

func (c *client) performReceiptPolicy(id string, control syncControl, current func() bool) (json.RawMessage, error) {
	var raw, hash, state, result string
	if err := c.store.db.QueryRow("SELECT payload,payload_hash,state,result FROM receipt_policy_operations WHERE id=?", id).Scan(&raw, &hash, &state, &result); err != nil {
		return nil, err
	}
	var o receiptPolicyOperation
	if syncDecode([]byte(raw), &o) != nil || o.ID != id || o.Hash != hash {
		return nil, syncRefuse("RETENTION_INTENT_CHANGED")
	}
	q, err := receiptPolicyCheck(o, control.syncBinding)
	if err != nil {
		return nil, err
	}
	if state == "completed" {
		if err = receiptPolicyResult(json.RawMessage(result), q); err != nil {
			return nil, err
		}
		return json.RawMessage(result), nil
	}
	if state != "pending" {
		return nil, syncRefuse("RETENTION_INTENT_CHANGED")
	}
	if _, err = c.store.receiptControl(c.store.db, &control, current, true); err != nil {
		return nil, err
	}
	var response json.RawMessage
	// This explicit retry sends the original nonce/CAS/body once. The owner
	// returns an original committed result before checking new-policy readiness.
	if err = c.call("PUT", "/api/conversations/"+url.PathEscape(o.Conversation)+"/retention", json.RawMessage(o.Body), &response); err != nil {
		var api *apiFailure
		if errors.As(err, &api) && oneOf(api.Code, "RETENTION_VERSION_CONFLICT", "OPERATION_CONFLICT", "INVALID_RETENTION_POLICY", "RETENTION_OWNER_REQUIRED") {
			tx, beginErr := c.store.db.Begin()
			if beginErr != nil {
				return nil, beginErr
			}
			defer tx.Rollback()
			if _, checkErr := c.store.receiptControl(tx, &control, current, false); checkErr != nil {
				return nil, checkErr
			}
			if _, saveErr := tx.Exec("UPDATE receipt_policy_operations SET state='failed',result=? WHERE id=? AND payload=? AND payload_hash=? AND state='pending'", api.Code, id, raw, hash); saveErr != nil {
				return nil, saveErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return nil, commitErr
			}
		}
		return nil, err
	}
	if err = receiptPolicyResult(response, q); err != nil {
		return nil, err
	}
	tx, err := c.store.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = c.store.receiptControl(tx, &control, current, true); err != nil {
		return nil, err
	}
	if _, err = tx.Exec("UPDATE receipt_policy_operations SET state='completed',result=? WHERE id=? AND payload=? AND payload_hash=? AND state='pending'", string(response), id, raw, hash); err != nil {
		return nil, err
	}
	if !current() {
		return nil, syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	return response, tx.Commit()
}

func receiptPolicyResult(raw json.RawMessage, q receiptPolicyRequest) error {
	var v struct {
		Policy  receiptRetentionPolicy `json:"policy"`
		Version string                 `json:"version"`
		Applies string                 `json:"appliesTo"`
	}
	n, err := syncCounter(q.Expected)
	if err != nil || n == math.MaxInt64 || syncDecode(raw, &v) != nil || v.Version != strconv.FormatUint(n+1, 10) || v.Applies != "new_messages_only" {
		return syncRefuse("RETENTION_RESULT_INVALID")
	}
	want, _ := json.Marshal(q.Policy)
	got, _ := json.Marshal(v.Policy)
	if !bytes.Equal(want, got) {
		return syncRefuse("RETENTION_RESULT_INVALID")
	}
	return nil
}

func (c *client) receiptPolicyCommand(cmd, args string, control syncControl, current func() bool) error {
	if cmd == "retention-operations" {
		rows, err := c.store.db.Query("SELECT id,conversation_id,state FROM receipt_policy_operations WHERE epoch=? ORDER BY rowid", c.epoch)
		if err != nil {
			return err
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, conversation, state string
			if err = rows.Scan(&id, &conversation, &state); err != nil {
				return err
			}
			items = append(items, map[string]string{"requestId": id, "conversationId": conversation, "state": state})
		}
		if err = rows.Err(); err != nil {
			return err
		}
		return printJSON(map[string]any{"items": items})
	}
	if cmd == "retention-retry" {
		raw, err := c.performReceiptPolicy(strings.TrimSpace(args), control, current)
		if err != nil {
			return err
		}
		return printJSON(raw)
	}
	if cmd == "retention" {
		id := strings.TrimSpace(args)
		if !syncUUIDPattern.MatchString(id) {
			return syncRefuse("INVALID_CONVERSATION_ID")
		}
		var v struct {
			Policy  receiptRetentionPolicy `json:"policy"`
			Version string                 `json:"version"`
			Applies string                 `json:"appliesTo"`
		}
		if err := c.call("GET", "/api/conversations/"+url.PathEscape(id)+"/retention", nil, &v); err != nil {
			return err
		}
		if n, err := syncCounter(v.Version); err != nil || n == 0 || !receiptPolicyValid(v.Policy) || v.Applies != "new_messages_only" {
			return syncRefuse("RETENTION_RESULT_INVALID")
		}
		return printJSON(v)
	}
	header, body, present := strings.Cut(args, "|")
	parts := strings.Fields(header)
	if !present || len(parts) != 2 || !syncUUIDPattern.MatchString(parts[0]) {
		return fmt.Errorf("retention-set CONVERSATION_ID EXPECTED_VERSION | POLICY_JSON")
	}
	var policy receiptRetentionPolicy
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&policy) != nil || decoder.Decode(new(any)) != io.EOF || !receiptPolicyValid(policy) {
		return syncRefuse("INVALID_RETENTION_POLICY")
	}
	if n, err := syncCounter(parts[1]); err != nil || n == 0 || n == math.MaxInt64 {
		return syncRefuse("INVALID_RETENTION_POLICY")
	}
	v, err := c.getReceiptNegotiation(control)
	if err != nil {
		return err
	}
	if policy.Mode == "after_receipt" && !receiptBool(v.After, true) || policy.AttachmentMode == "after_object_receipt" && !receiptBool(v.AfterObject, true) {
		return syncRefuse("RETENTION_QUALIFICATION_REQUIRED")
	}
	id, err := syncNewUUID()
	if err != nil {
		return err
	}
	request, _ := json.Marshal(receiptPolicyRequest{ID: id, Expected: parts[1], Policy: policy})
	o := receiptPolicyOperation{ID: id, Binding: control.syncBinding, Conversation: parts[0], Body: string(request)}
	o.Hash = receiptPolicyHash(o)
	raw, _ := json.Marshal(o)
	tx, err := c.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = c.store.receiptControl(tx, &control, current, true); err != nil {
		return err
	}
	if err = receiptDurability(tx); err != nil {
		return err
	}
	var pending int
	if err = tx.QueryRow("SELECT count(*) FROM receipt_policy_operations WHERE epoch=? AND conversation_id=? AND state='pending'", c.epoch, o.Conversation).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return syncRefuse("RETENTION_ORIGINAL_OPERATION_PENDING")
	}
	if _, err = tx.Exec("INSERT INTO receipt_policy_operations(id,epoch,conversation_id,payload,payload_hash,state) VALUES(?,?,?,?,?,'pending')", id, c.epoch, o.Conversation, string(raw), o.Hash); err != nil {
		return err
	}
	if !current() {
		return syncRefuse("RECEIPT_CONTEXT_CHANGED")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	response, err := c.performReceiptPolicy(id, control, current)
	if err != nil {
		return fmt.Errorf("%w; original retention requestId=%s", err, id)
	}
	return printJSON(response)
}
