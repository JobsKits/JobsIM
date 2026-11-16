// Created by Jobs on 2026年11月14日，星期六.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func installationIdentity(directory string) (string, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "installation-id")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return "", fmt.Errorf("installation identity must be a private regular file")
	}
	read, err := os.ReadFile(path)
	if err == nil {
		value := strings.TrimSpace(string(read))
		if len(value) != 32 || strings.Trim(value, "0123456789abcdef") != "" {
			return "", fmt.Errorf("invalid installation identity; original retained")
		}
		return value, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	value := id()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return installationIdentity(directory)
	}
	if err != nil {
		return "", err
	}
	if _, err = file.WriteString(value + "\n"); err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil {
		return "", err
	}
	if closed != nil {
		return "", closed
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return "", err
	}
	err = directoryFile.Sync()
	directoryFile.Close()
	if err != nil {
		return "", err
	}
	return value, nil
}
func (c *client) send(conversation, text, messageID, kind string) error {
	return c.queueAndSend(pending{ConversationID: conversation, Text: text, ClientMsgID: messageID, Type: kind})
}
func (c *client) queueAndSend(p pending) error {
	if c.store == nil {
		return fmt.Errorf("local durable store unavailable; nothing sent")
	}
	if p.ClientMsgID == "" {
		p.ClientMsgID = id()
	}
	p.DeploymentID = c.deployment
	p.UserID = c.user
	p.RestoreEpoch = c.epoch
	p.State = "pending"
	previous, err := c.store.outbox()
	if err != nil {
		return err
	}
	if original, exists := previous[p.ClientMsgID]; exists {
		p.SendAdmission = original.SendAdmission
	} else {
		if p.Type != "forward" {
			if err = c.store.draft(p.ConversationID, p.Text); err != nil {
				return err
			}
		}
		var conversations []json.RawMessage
		var directoryErr error
		if p.hasMentions() {
			if err = c.prepareMentions(&p); err != nil {
				return err
			}
			conversations = nil
		} else if p.Type == "forward" {
			var current json.RawMessage
			directoryErr = c.call("GET", "/api/conversations/"+url.PathEscape(p.ConversationID), nil, &current)
			conversations = []json.RawMessage{current}
		} else {
			conversations, directoryErr = c.conversationRecords()
		}
		if directoryErr != nil {
			return directoryErr
		}
		if err = c.store.rememberConversations(conversations); err != nil {
			return err
		}
		for _, raw := range conversations {
			var conversation struct {
				ID            string `json:"id"`
				SendAdmission string `json:"sendAdmission"`
			}
			if err = json.Unmarshal(raw, &conversation); err != nil {
				return err
			}
			if conversation.ID == p.ConversationID {
				p.SendAdmission = conversation.SendAdmission
				break
			}
		}
		if p.SendAdmission == "" {
			return fmt.Errorf("current sending admission unavailable; draft retained, nothing queued")
		}
	}
	if err := c.store.enqueue(p); err != nil {
		return err
	}
	return c.transmit(p)
}
func (c *client) transmit(p pending) error {
	if c.store == nil || c.store.identity.UserID != c.user || c.store.identity.DeploymentID != c.deployment || c.store.epoch != c.epoch {
		return fmt.Errorf("local account/deployment/epoch context mismatch; nothing sent")
	}
	if c.auth != nil && c.store.identity.DeviceID != c.serverDevice {
		return fmt.Errorf("old device namespace cannot submit using a new authentication")
	}
	var bootstrap struct {
		DeploymentID string `json:"deploymentId"`
		RestoreEpoch string `json:"restoreEpoch"`
		Features     struct {
			SendAdmission bool `json:"send_admission"`
			Forwarding    bool `json:"forwarding"`
		} `json:"features"`
	}
	if err := c.call("GET", "/api/bootstrap", nil, &bootstrap); err != nil {
		return err
	}
	if bootstrap.DeploymentID != c.deployment || bootstrap.RestoreEpoch != c.epoch {
		err := c.store.retireSyncIdentity("RESTORE_EPOCH_CHANGED")
		c.token = ""
		if err != nil {
			return err
		}
		return syncRefuse("RESTORE_EPOCH_CHANGED")
	}
	if bootstrap.DeploymentID != c.deployment || (p.Type != "forward" && !bootstrap.Features.SendAdmission) {
		return fmt.Errorf("server does not confirm send admission support; original outbox retained without POST")
	}
	current, err := c.store.outbox()
	if err != nil {
		return err
	}
	original, exists := current[p.ClientMsgID]
	if !exists || original.State != p.State || operationDigest(original) != operationDigest(p) {
		return fmt.Errorf("original Outbox changed; nothing sent")
	}
	p = original
	if p.Type == "forward" {
		if bootstrap.RestoreEpoch != c.epoch || p.RestoreEpoch != c.epoch {
			return fmt.Errorf("restore epoch changed; original forward retained without POST")
		}
		return c.transmitForward(p, bootstrap.Features.SendAdmission && bootstrap.Features.Forwarding)
	}
	if p.State == "frozen" && (p.FreezeReason == "CONTACT_REMOVED" || p.FreezeReason == "PEER_BLOCKED" || p.FreezeReason == "CONVERSATION_LEFT") {
		if bootstrap.RestoreEpoch != c.epoch || p.RestoreEpoch != c.epoch {
			return fmt.Errorf("restore epoch changed; governance result remains unknown and frozen")
		}
		return c.lookupGovernanceResult(p)
	}
	if p.hasMentions() {
		return c.transmitMentions(p)
	}
	var result json.RawMessage
	if err := c.call("POST", "/api/conversations/"+url.PathEscape(p.ConversationID)+"/messages", p.payload(), &result); err != nil {
		var failure *apiFailure
		if errors.As(err, &failure) && failure.Code == "SEND_ADMISSION_CHANGED" {
			if freezeErr := c.store.freezeAdmission(p); freezeErr != nil {
				return fmt.Errorf("sending admission rejected; freezing original operation failed: %w", freezeErr)
			}
			return fmt.Errorf("sending admission changed; original message frozen; compose a new intent: %w", err)
		}
		fmt.Fprintln(os.Stderr, "Pending original ID:", p.ClientMsgID)
		return err
	}
	return c.reconcileReadOnly(p)
}

func (c *client) lookupGovernanceResult(p pending) error {
	return c.reconcileReadOnly(p)
}
func (c *client) readOutbox() (map[string]pending, error) {
	if c.store == nil {
		return nil, fmt.Errorf("local store unavailable")
	}
	return c.store.outbox()
}
func (c *client) showOutbox() error {
	items, err := c.readOutbox()
	if err != nil {
		return err
	}
	return printJSON(items)
}
func (c *client) retry(messageID string, reconcile bool) error {
	items, err := c.readOutbox()
	if err != nil {
		return err
	}
	p, ok := items[messageID]
	if !ok {
		return fmt.Errorf("unknown original message identity")
	}
	if p.State == "confirmed" {
		return fmt.Errorf("already confirmed")
	}
	if reconcile {
		return c.reconcileReadOnly(p)
	}
	if p.State == "frozen" && !reconcile {
		return fmt.Errorf("original queue is frozen (%s); reconcile ORIGINAL_ID checks its result without changing the original payload", p.FreezeReason)
	}
	if p.RestoreEpoch != c.epoch && !reconcile {
		return fmt.Errorf("restore epoch changed; automatic retry prohibited")
	}
	// This command is the user's explicit retry. Unknown/409 never reaches creation.
	if err = c.reconcileReadOnly(p); err == nil {
		return nil
	} else if !isAPIStatus(err, 404) {
		return err
	}
	if p.SendAdmission == "" {
		if err = c.store.freezeAdmission(p); err != nil {
			return err
		}
		return fmt.Errorf("original sending admission absent; only read-only reconciliation is permitted")
	}
	if err = c.store.syncReady(); err != nil {
		return err
	}
	return c.transmit(p)
}
func (c *client) syncHistory(conversation, cursor string) error {
	var page struct {
		Items  []json.RawMessage `json:"items"`
		Cursor string            `json:"cursor"`
	}
	if err := c.call("GET", "/api/conversations/"+url.PathEscape(conversation)+"/messages?after="+url.QueryEscape(cursor)+"&limit=200", nil, &page); err != nil {
		if isAPIStatus(err, 401, 403, 409) {
			return err
		}
		cached, localErr := c.store.history(conversation)
		if localErr != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Offline; displaying only this account's local cache.")
		if localErr = printJSON(map[string]any{"source": "local_cache", "items": cached, "restoreEpoch": c.epoch}); localErr != nil {
			return localErr
		}
		return err
	}
	if err := c.store.savePage(conversation, page.Cursor, page.Items); err != nil {
		return err
	}
	return printJSON(page)
}
func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds allowed size")
	}
	return data, nil
}

type apiFailure struct {
	Status  int
	Code    string
	Message string
}

func (e *apiFailure) Error() string {
	return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, e.Message)
}
func isAPIStatus(err error, statuses ...int) bool {
	var api *apiFailure
	if !errors.As(err, &api) {
		return false
	}
	for _, status := range statuses {
		if api.Status == status {
			return true
		}
	}
	return false
}

func (c *client) download(object, destination string) error {
	if object == "" {
		return fmt.Errorf("object required")
	}
	return c.downloadPath("/api/media/"+url.PathEscape(object), destination)
}
func (c *client) downloadPath(path, destination string) error {
	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("object and destination required")
	}
	response, err := c.authenticatedResponse("GET", path, "", nil, 120*time.Second)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("download HTTP %d; original destination retained", response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	info, _ := file.Stat()
	complete := false
	defer func() {
		file.Close()
		if !complete {
			if current, statErr := os.Lstat(destination); statErr == nil && info != nil && os.SameFile(info, current) {
				os.Remove(destination)
			}
		}
	}()
	size, err := io.Copy(file, io.LimitReader(response.Body, (64<<20)+1))
	if err != nil {
		return err
	}
	if size > 64<<20 {
		return fmt.Errorf("download exceeds current 64 MiB limit")
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	complete = true
	return printJSON(map[string]any{"path": destination, "size": size, "contentType": response.Header.Get("Content-Type")})
}
