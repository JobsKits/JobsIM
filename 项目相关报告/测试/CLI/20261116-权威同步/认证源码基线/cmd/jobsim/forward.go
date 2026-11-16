// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

type forwardSource struct {
	ID                 string `json:"id"`
	ContentVersion     string `json:"contentVersion"`
	Admission          string `json:"forwardAdmission"`
	Deleted            bool   `json:"deleted"`
	Unavailable        bool   `json:"contentUnavailable"`
	ForwardUnavailable bool   `json:"forwardUnavailable"`
	AttachmentURL      string `json:"attachmentURL"`
}

func (c *client) forwardMessage(destination, source, version string) error {
	if !decimal.MatchString(version) {
		return fmt.Errorf("original source content version required")
	}
	var current forwardSource
	if err := c.call("GET", "/api/messages/"+url.PathEscape(source), nil, &current); err != nil {
		return err
	}
	if current.ID != source || current.Deleted || current.Unavailable || current.ForwardUnavailable || current.Admission == "" {
		return fmt.Errorf("source currently unavailable; nothing queued")
	}
	if current.ContentVersion != version {
		return fmt.Errorf("source content changed; inspect and explicitly select its new version")
	}
	return c.queueAndSend(pending{ConversationID: destination, Type: "forward", ForwardOfID: source, ForwardSourceVersion: version, ForwardAdmission: current.Admission})
}

func maskForward(item map[string]any) {
	item["text"] = ""
	item["forwardUnavailable"] = true
	for _, key := range []string{"attachmentId", "attachmentURL", "attachmentName", "forwardAdmission"} {
		delete(item, key)
	}
}

func (s *localStore) forwardAttempted(id string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM metadata WHERE key=?", "forward-attempted:"+id).Scan(&count)
	return count > 0, err
}

func (c *client) transmitForward(p pending, enabled bool) error {
	attempted, err := c.store.forwardAttempted(p.ClientMsgID)
	if err != nil {
		return err
	}
	var result json.RawMessage
	if attempted || p.State == "frozen" {
		err = c.call("POST", "/api/conversations/"+url.PathEscape(p.ConversationID)+"/message-results", p.payload(), &result)
		if err == nil {
			if err = c.store.confirm(p, result); err != nil {
				return err
			}
			return printJSON(result)
		}
		if !isAPIStatus(err, 404) {
			return err
		}
		if p.State == "frozen" {
			return fmt.Errorf("original result not found; intent remains frozen; no message POST sent")
		}
	}
	if !enabled {
		return fmt.Errorf("server does not confirm forwarding support; original intent retained")
	}
	if _, err = c.store.db.Exec("INSERT OR IGNORE INTO metadata(key,value) VALUES(?,'1')", "forward-attempted:"+p.ClientMsgID); err != nil {
		return err
	}
	err = c.call("POST", "/api/conversations/"+url.PathEscape(p.ConversationID)+"/messages", p.payload(), &result)
	if err != nil {
		var failure *apiFailure
		if errors.As(err, &failure) && permanentForwardCode(failure.Code) {
			if _, freezeErr := c.store.db.Exec("UPDATE outbox SET state='frozen',freeze_reason=? WHERE client_msg_id=? AND state!='confirmed'", failure.Code, p.ClientMsgID); freezeErr != nil {
				return freezeErr
			}
		}
		return fmt.Errorf("original forward %s retained; explicitly retry or reconcile this ID: %w", p.ClientMsgID, err)
	}
	if err = c.store.confirm(p, result); err != nil {
		return err
	}
	return printJSON(result)
}

func permanentForwardCode(code string) bool {
	switch code {
	case "NOT_A_MEMBER", "HISTORY_NOT_ALLOWED", "INVALID_FORWARD", "FORWARD_PROTOCOL_REQUIRED", "FORWARD_DEPTH_LIMIT", "FORWARD_ADMISSION_CHANGED", "FORWARD_SOURCE_CHANGED", "FORWARD_SOURCE_UNAVAILABLE", "SEND_ADMISSION_CHANGED":
		return true
	}
	return false
}

func (c *client) downloadForward(id, destination string) error {
	var current forwardSource
	if err := c.call("GET", "/api/messages/"+url.PathEscape(id), nil, &current); err != nil {
		return err
	}
	path := "/api/messages/" + url.PathEscape(id) + "/attachment"
	if current.ID != id || current.Deleted || current.Unavailable || current.ForwardUnavailable || current.AttachmentURL != path {
		return fmt.Errorf("current forwarded attachment unavailable")
	}
	return c.downloadPath(path, destination)
}
