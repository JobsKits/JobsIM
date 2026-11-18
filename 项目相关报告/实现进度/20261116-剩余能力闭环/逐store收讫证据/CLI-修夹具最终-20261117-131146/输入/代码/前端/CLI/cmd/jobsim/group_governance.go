// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type groupCLIIntent struct {
	ID           string          `json:"id"`
	Conversation string          `json:"conversationId"`
	Route        string          `json:"route"`
	Body         json.RawMessage `json:"body"`
	State        string          `json:"state"`
	Family       string          `json:"familyId"`
}

func (c *client) groupContext() error {
	if c.store == nil || c.store.epoch != c.epoch || c.store.identity.DeploymentID != c.deployment || c.store.identity.UserID != c.user || c.store.identity.Origin != c.base || c.store.identity.DeviceID != c.serverDevice {
		return syncRefuse("GROUP_LOCAL_CONTEXT_CHANGED")
	}
	return c.store.checkSyncFile()
}

func (c *client) groupTable() error {
	if err := c.groupContext(); err != nil {
		return err
	}
	_, err := c.store.db.Exec(`CREATE TABLE IF NOT EXISTS group_intents(epoch TEXT NOT NULL,id TEXT NOT NULL,conversation TEXT NOT NULL,route TEXT NOT NULL,body TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','complete','rejected')),PRIMARY KEY(epoch,id));CREATE UNIQUE INDEX IF NOT EXISTS group_one_pending ON group_intents(epoch,conversation) WHERE state='pending';`)
	if err != nil {
		return err
	}
	var exists bool
	if err = c.store.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pragma_table_info('group_intents') WHERE name='family')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		_, err = c.store.db.Exec(`ALTER TABLE group_intents ADD COLUMN family TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

func (c *client) groupOriginal(id string) (groupCLIIntent, error) {
	var row groupCLIIntent
	if err := c.groupTable(); err != nil {
		return row, err
	}
	var raw string
	err := c.store.db.QueryRow(`SELECT id,conversation,route,body,state,family FROM group_intents WHERE epoch=? AND id=?`, c.epoch, id).Scan(&row.ID, &row.Conversation, &row.Route, &raw, &row.State, &row.Family)
	row.Body = json.RawMessage(raw)
	return row, err
}

func (c *client) groupCreate(conversation, route string, body map[string]any, runtime map[string]any) error {
	if !syncUUIDPattern.MatchString(conversation) || !groupCLIRouteValid(route) {
		return fmt.Errorf("canonical group identity and known operation required")
	}
	if body["token"] != nil || body["password"] != nil || body["otp"] != nil {
		return fmt.Errorf("proof must stay in memory")
	}
	if err := c.groupTable(); err != nil {
		return err
	}
	if c.auth == nil || !syncUUIDPattern.MatchString(c.auth.pair.FamilyID) || c.auth.pair.DeviceID != c.serverDevice || c.auth.pair.User.ID != c.user {
		return syncRefuse("GROUP_ORIGINAL_FAMILY_REQUIRED")
	}
	request, err := syncNewUUID()
	if err != nil {
		return err
	}
	body["requestId"] = request
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = c.store.db.Exec(`INSERT INTO group_intents(epoch,id,conversation,route,body,state,family) VALUES(?,?,?,?,?,'pending',?)`, c.epoch, request, conversation, route, string(raw), c.auth.pair.FamilyID)
	if err != nil {
		return fmt.Errorf("another original group operation is pending; use group-operations")
	}
	row := groupCLIIntent{request, conversation, route, raw, "pending", c.auth.pair.FamilyID}
	return c.groupTransmit(row, true, runtime)
}

func (c *client) groupTransmit(row groupCLIIntent, retry bool, runtime map[string]any) error {
	if err := c.groupContext(); err != nil {
		return err
	}
	prefix := "/api/conversations/" + url.PathEscape(row.Conversation)
	route, target, management := strings.Cut(row.Route, ":")
	path, lookup, method := prefix+"/"+row.Route, prefix+"/"+row.Route, "POST"
	if management {
		if !groupCLIRouteValid(row.Route) {
			return syncRefuse("GROUP_ORIGINAL_INVALID")
		}
		lookup = prefix + "/governance"
		if route == "invite_revoke" {
			path, method = prefix+"/invites/"+target, "DELETE"
		} else {
			path = prefix + "/join-requests/" + target
		}
	}
	var result map[string]any
	err := c.call("GET", lookup+"/operations/"+url.PathEscape(row.ID), nil, &result)
	if err != nil {
		var api *apiFailure
		if !retry || !errors.As(err, &api) || api.Status != 404 || api.Code != "GROUP_OPERATION_NOT_FOUND" {
			return fmt.Errorf("original group operation %s retained: %w", row.ID, err)
		}
		if c.auth == nil || row.Family == "" || c.auth.pair.FamilyID != row.Family {
			return syncRefuse("GROUP_ORIGINAL_FAMILY_REQUIRED")
		}
		var body map[string]any
		if json.Unmarshal(row.Body, &body) != nil || body["requestId"] != row.ID {
			return syncRefuse("GROUP_ORIGINAL_INVALID")
		}
		if row.Route == "join-requests" && runtime["token"] == nil {
			return fmt.Errorf("re-enter the original invitation with join-group; its capability is never saved")
		}
		if body["action"] == "recover" {
			if c.reader == nil {
				return fmt.Errorf("fresh administrator authenticator code required")
			}
			otp, readErr := secretInput(c.reader, "Fresh administrator authenticator code: ")
			if readErr != nil {
				erase(otp)
				return readErr
			}
			valid := len(otp) == 6
			for _, digit := range otp {
				valid = valid && digit >= '0' && digit <= '9'
			}
			if !valid {
				erase(otp)
				return fmt.Errorf("six-digit authenticator code required")
			}
			var proof struct {
				Authorization string `json:"authorizationId"`
			}
			proofErr := c.call("POST", "/api/auth/step-up", map[string]any{"action": "group.recover", "targetId": row.Conversation, "requestId": row.ID, "payload": body, "otp": string(otp)}, &proof)
			erase(otp)
			if proofErr != nil {
				return proofErr
			}
			if !syncUUIDPattern.MatchString(proof.Authorization) {
				return syncRefuse("GROUP_STEP_UP_INVALID")
			}
			body["stepUpId"] = proof.Authorization
		}
		for key, value := range runtime {
			if key != "token" {
				return syncRefuse("GROUP_ORIGINAL_INVALID")
			}
			body[key] = value
		}
		err = c.call(method, path, body, &result)
		if err != nil {
			var rejected *apiFailure
			if errors.As(err, &rejected) && oneOf(rejected.Code, "GROUP_PERMISSION_CHANGED", "GROUP_TARGET_CHANGED", "GROUP_TARGET_UNAVAILABLE", "GROUP_READ_ONLY", "GROUP_ROLE_BOUNDARY", "OWNER_REQUIRED", "GROUP_ADMIN_REQUIRED") {
				_, _ = c.store.db.Exec(`UPDATE group_intents SET state='rejected' WHERE epoch=? AND id=? AND body=? AND state='pending'`, c.epoch, row.ID, string(row.Body))
			}
			return fmt.Errorf("original group operation %s retained: %w", row.ID, err)
		}
	}
	if err = c.groupContext(); err != nil {
		return err
	}
	if result["requestId"] != row.ID {
		return syncRefuse("GROUP_RESULT_MISMATCH")
	}
	if management {
		expected := "revoked"
		if route == "join_decide" {
			var original map[string]any
			if json.Unmarshal(row.Body, &original) != nil {
				return syncRefuse("GROUP_ORIGINAL_INVALID")
			}
			expected = "rejected"
			if original["approve"] == true {
				expected = "approved"
			}
		}
		if result["status"] != "applied" || result["action"] != route || result["targetId"] != target || result["state"] != expected {
			return syncRefuse("GROUP_RESULT_MISMATCH")
		}
	}
	if _, err = c.store.db.Exec(`UPDATE group_intents SET state='complete' WHERE epoch=? AND id=? AND body=?`, c.epoch, row.ID, string(row.Body)); err != nil {
		return err
	}
	delete(result, "token")
	return printJSON(result)
}

func groupCLICounter(value string) bool {
	n, err := syncCounter(value)
	return err == nil && n > 0
}

type groupCLIChanges struct {
	MuteAll          *bool  `json:"muteAll,omitempty"`
	SlowSeconds      *int   `json:"slowSeconds,omitempty"`
	MaxMembers       *int   `json:"maxMembers,omitempty"`
	TargetID         string `json:"targetId,omitempty"`
	TargetMembership string `json:"targetMembershipGeneration,omitempty"`
	TargetRole       string `json:"targetRoleGeneration,omitempty"`
	Role             string `json:"role,omitempty"`
	MutedUntil       string `json:"mutedUntil,omitempty"`
}

func (c *client) groupCommand(cmd, args string) (bool, error) {
	parts := strings.Fields(args)
	switch cmd {
	case "group-policy", "group-invites", "group-applications":
		if len(parts) < 1 || len(parts) > 2 {
			return true, fmt.Errorf("%s CONVERSATION_ID [CURSOR]", cmd)
		}
		suffix := map[string]string{"group-policy": "governance", "group-invites": "invites", "group-applications": "join-requests"}[cmd]
		path := "/api/conversations/" + url.PathEscape(parts[0]) + "/" + suffix
		if len(parts) == 2 {
			path += "?after=" + url.QueryEscape(parts[1]) + "&limit=50"
		}
		var value map[string]any
		if err := c.call("GET", path, nil, &value); err != nil {
			return true, err
		}
		if items, ok := value["items"].([]any); ok {
			for _, item := range items {
				if row, yes := item.(map[string]any); yes {
					delete(row, "token")
				}
			}
		}
		return true, printJSON(value)
	case "invite-code":
		if len(parts) != 2 || !syncUUIDPattern.MatchString(parts[0]) || !syncUUIDPattern.MatchString(parts[1]) {
			return true, fmt.Errorf("invite-code CONVERSATION_ID INVITATION_ID")
		}
		after := ""
		seen := map[string]bool{}
		for page := 0; page < 100; page++ {
			var value struct {
				Items []map[string]any `json:"items"`
				Next  string           `json:"nextCursor"`
			}
			if err := c.call("GET", "/api/conversations/"+parts[0]+"/invites?limit=100&after="+url.QueryEscape(after), nil, &value); err != nil {
				return true, err
			}
			for _, invite := range value.Items {
				if invite["id"] == parts[1] {
					token, ok := invite["token"].(string)
					if !ok || token == "" {
						return true, syncRefuse("GROUP_INVITATION_UNAVAILABLE")
					}
					raw, _ := json.Marshal(map[string]string{"deploymentId": c.deployment, "conversationId": parts[0], "inviteId": parts[1], "token": token})
					fmt.Println("JobsIM-Invite-1:" + base64.StdEncoding.EncodeToString(raw))
					return true, nil
				}
			}
			if value.Next == "" {
				break
			}
			if seen[value.Next] || !syncUUIDPattern.MatchString(value.Next) {
				return true, syncRefuse("GROUP_PAGE_INVALID")
			}
			seen[value.Next] = true
			after = value.Next
		}
		return true, fmt.Errorf("invitation unavailable in the bounded current group list")
	case "group-change":
		head, changes, ok := strings.Cut(args, " | ")
		fields := strings.Fields(head)
		if !ok || len(fields) != 5 || !oneOf(fields[4], "settings", "mute", "role", "dissolve") {
			return true, fmt.Errorf("group-change CONVERSATION_ID VERSION ACTOR_MEMBERSHIP ACTOR_ROLE settings|mute|role|dissolve | JSON_CHANGES")
		}
		for _, v := range fields[1:4] {
			if !groupCLICounter(v) {
				return true, fmt.Errorf("positive exact original permission counters required")
			}
		}
		var typed groupCLIChanges
		decoder := json.NewDecoder(bytes.NewBufferString(changes))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&typed) != nil || decoder.Decode(new(any)) != io.EOF {
			return true, fmt.Errorf("one typed group change object required")
		}
		if typed.SlowSeconds != nil && (*typed.SlowSeconds < 0 || *typed.SlowSeconds > 3600) || typed.MaxMembers != nil && (*typed.MaxMembers < 2 || *typed.MaxMembers > 10000) {
			return true, fmt.Errorf("group policy outside allowed range")
		}
		raw, _ := json.Marshal(typed)
		var body map[string]any
		decoder = json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		_ = decoder.Decode(&body)
		body["action"], body["version"], body["membershipGeneration"], body["roleGeneration"] = fields[4], fields[1], fields[2], fields[3]
		return true, c.groupCreate(fields[0], "governance", body, nil)
	case "transfer":
		if len(parts) != 7 || !syncUUIDPattern.MatchString(parts[1]) {
			return true, fmt.Errorf("transfer CONVERSATION_ID NEW_OWNER_ID GROUP_VERSION ACTOR_MEMBERSHIP ACTOR_ROLE TARGET_MEMBERSHIP TARGET_ROLE; inspect group-policy and members first")
		}
		for _, v := range parts[2:] {
			if !groupCLICounter(v) {
				return true, fmt.Errorf("positive exact original permission counters required")
			}
		}
		return true, c.groupCreate(parts[0], "governance", map[string]any{"action": "transfer", "targetId": parts[1], "version": parts[2], "membershipGeneration": parts[3], "roleGeneration": parts[4], "targetMembershipGeneration": parts[5], "targetRoleGeneration": parts[6]}, nil)
	case "group-recover":
		if len(parts) != 4 || !syncUUIDPattern.MatchString(parts[0]) || !syncUUIDPattern.MatchString(parts[1]) || !groupCLICounter(parts[2]) || !groupCLICounter(parts[3]) {
			return true, fmt.Errorf("group-recover CONVERSATION_ID EXISTING_SUCCESSOR_ID GROUP_VERSION TARGET_MEMBERSHIP; independent local operator grant and fresh MFA required")
		}
		return true, c.groupCreate(parts[0], "governance", map[string]any{"action": "recover", "targetId": parts[1], "version": parts[2], "targetMembershipGeneration": parts[3]}, nil)
	case "group-operations":
		if err := c.groupTable(); err != nil {
			return true, err
		}
		rows, err := c.store.db.Query(`SELECT id,conversation,route,state FROM group_intents WHERE epoch=? ORDER BY rowid`, c.epoch)
		if err != nil {
			return true, err
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, conversation, route, state string
			if err = rows.Scan(&id, &conversation, &route, &state); err != nil {
				return true, err
			}
			items = append(items, map[string]string{"id": id, "conversationId": conversation, "route": route, "state": state})
		}
		if err = rows.Err(); err != nil {
			return true, err
		}
		return true, printJSON(map[string]any{"items": items})
	case "group-check", "group-retry":
		if len(parts) != 1 {
			return true, fmt.Errorf("%s ORIGINAL_OPERATION_ID", cmd)
		}
		row, err := c.groupOriginal(parts[0])
		if err != nil {
			return true, err
		}
		return true, c.groupTransmit(row, cmd == "group-retry", nil)
	case "create-invite":
		if len(parts) != 7 {
			return true, fmt.Errorf("create-invite CONVERSATION_ID VERSION ACTOR_MEMBERSHIP ACTOR_ROLE MAX_USES HOURS require-approval|direct")
		}
		for _, v := range parts[1:4] {
			if !groupCLICounter(v) {
				return true, fmt.Errorf("original actor permission counters required")
			}
		}
		uses, e := strconv.Atoi(parts[4])
		hours, e2 := strconv.Atoi(parts[5])
		if e != nil || e2 != nil || uses < 1 || uses > 10000 || hours < 1 || hours > 720 || !oneOf(parts[6], "require-approval", "direct") {
			return true, fmt.Errorf("bounded uses/time and explicit approval mode required")
		}
		return true, c.groupCreate(parts[0], "invites", map[string]any{"version": parts[1], "membershipGeneration": parts[2], "roleGeneration": parts[3], "expiresAt": time.Now().UTC().Add(time.Duration(hours) * time.Hour).Format(time.RFC3339Nano), "maxUses": uses, "approvalRequired": parts[6] == "require-approval"}, nil)
	case "group-approve", "group-reject", "revoke-invite":
		if len(parts) != 5 || !syncUUIDPattern.MatchString(parts[1]) {
			return true, fmt.Errorf("%s CONVERSATION_ID ORIGINAL_ITEM_ID VERSION ACTOR_MEMBERSHIP ACTOR_ROLE", cmd)
		}
		for _, v := range parts[2:] {
			if !groupCLICounter(v) {
				return true, fmt.Errorf("original actor permission counters required")
			}
		}
		body := map[string]any{"version": parts[2], "membershipGeneration": parts[3], "roleGeneration": parts[4]}
		route := "invite_revoke:" + parts[1]
		if cmd != "revoke-invite" {
			route = "join_decide:" + parts[1]
			body["approve"] = cmd == "group-approve"
		}
		return true, c.groupCreate(parts[0], route, body, nil)
	case "join-group":
		const prefix = "JobsIM-Invite-1:"
		if !strings.HasPrefix(args, prefix) || len(args) > 4096 {
			return true, fmt.Errorf("join-group ORIGINAL_INVITATION_CODE")
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(args, prefix))
		if err != nil {
			return true, fmt.Errorf("invalid invitation code")
		}
		var invite map[string]string
		if json.Unmarshal(raw, &invite) != nil || invite["deploymentId"] != c.deployment || !syncUUIDPattern.MatchString(invite["conversationId"]) || !syncUUIDPattern.MatchString(invite["inviteId"]) || invite["token"] == "" {
			return true, fmt.Errorf("valid invitation for this exact deployment required")
		}
		if err = c.groupTable(); err != nil {
			return true, err
		}
		var original string
		err = c.store.db.QueryRow(`SELECT id FROM group_intents WHERE epoch=? AND conversation=? AND state='pending'`, c.epoch, invite["conversationId"]).Scan(&original)
		proof := map[string]any{"token": invite["token"]}
		if err == nil {
			row, e := c.groupOriginal(original)
			if e != nil {
				return true, e
			}
			var body map[string]any
			if json.Unmarshal(row.Body, &body) != nil || row.Route != "join-requests" || body["inviteId"] != invite["inviteId"] {
				return true, fmt.Errorf("another original operation is pending")
			}
			return true, c.groupTransmit(row, true, proof)
		}
		if err != sql.ErrNoRows {
			return true, err
		}
		return true, c.groupCreate(invite["conversationId"], "join-requests", map[string]any{"inviteId": invite["inviteId"]}, proof)
	}
	return false, nil
}

func groupCLIRouteValid(route string) bool {
	if oneOf(route, "governance", "invites", "join-requests") {
		return true
	}
	kind, target, ok := strings.Cut(route, ":")
	return ok && oneOf(kind, "invite_revoke", "join_decide") && syncUUIDPattern.MatchString(target)
}
