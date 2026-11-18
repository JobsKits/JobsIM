// seed.go
// JobsIM Native Demo Host
// Created by Jobs on 2026年11月18日，星期三.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type seedState struct {
	DeploymentID string        `json:"deploymentId"`
	Epoch        string        `json:"restoreEpoch"`
	Accounts     []demoAccount `json:"accounts"`
	MessageIDs   []string      `json:"messageIds"`
	Applied      bool          `json:"applied"`
}

func demoNonce() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	value[6] = (value[6] & 15) | 64
	value[8] = (value[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func (c *demoController) api(ctx context.Context, method, path string, body any, token string, out any) error {
	if len(path) < 5 || path[:5] != "/api/" {
		return fmt.Errorf("fixed API path required")
	}
	var content []byte
	if body != nil {
		var err error
		content, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("https://%s:%d%s", c.host.ip, gatewayPort, path), bytes.NewReader(content))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return demoHTTPError{Status: response.StatusCode}
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
}

func (c *demoController) verifyWeb(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://%s:%d/", c.host.ip, gatewayPort), nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 || !bytes.Contains(bytes.ToLower(data), []byte("<html")) {
		return fmt.Errorf("current Web frontend unavailable")
	}
	return nil
}

func (c *demoController) seed(ctx context.Context, deployment string) ([]demoAccount, error) {
	var bootstrap struct {
		Epoch string `json:"restoreEpoch"`
	}
	if err := c.api(ctx, "GET", "/api/bootstrap", nil, "", &bootstrap); err != nil {
		return nil, err
	}
	path := filepath.Join(c.host.control, "seed-accounts.json")
	var state seedState
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		state = seedState{DeploymentID: deployment, Epoch: bootstrap.Epoch, MessageIDs: []string{demoNonce(), demoNonce(), demoNonce()}}
		for _, name := range []string{"demo_jobs", "demo_lin", "demo_xia"} {
			var password [24]byte
			if _, err = rand.Read(password[:]); err != nil {
				return nil, err
			}
			state.Accounts = append(state.Accounts, demoAccount{Username: name, Password: "Demo " + hex.EncodeToString(password[:]), RegistrationID: demoNonce()})
		}
		raw, _ := json.MarshalIndent(state, "", "  ")
		if err = writeNew(path, append(raw, '\n'), 0600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		if err = regular(path); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &state); err != nil {
			return nil, err
		}
	}
	if state.DeploymentID != deployment || state.Epoch != bootstrap.Epoch || len(state.Accounts) != 3 || len(state.MessageIDs) != 3 {
		return nil, fmt.Errorf("original demo seed identity/epoch differs; no automatic reset")
	}
	tokens, ids := []string{}, []string{}
	proofs := []demoProof{}
	preserveProofs := false
	defer func() {
		if preserveProofs {
			return
		}
		for _, token := range tokens {
			_ = c.api(context.Background(), "POST", "/api/auth/logout", map[string]any{}, token, nil)
		}
	}()
	for index, account := range state.Accounts {
		if !state.Applied {
			// Exact original registration may already have committed before a lost response.
			_ = c.api(ctx, "POST", "/api/auth/register", map[string]any{"username": account.Username, "password": account.Password, "requestId": account.RegistrationID, "deviceName": "native-demo-seed"}, "", nil)
		}
		var login struct {
			Token string `json:"token"`
			User  struct {
				ID    string `json:"id"`
				Admin bool   `json:"admin"`
			} `json:"user"`
		}
		if err := c.api(ctx, "POST", "/api/auth/login", map[string]any{"username": account.Username, "password": account.Password, "requestId": demoNonce(), "deviceName": "native-demo-seed-check"}, "", &login); err != nil {
			return nil, err
		}
		if login.Token == "" || login.User.ID == "" || login.User.Admin {
			return nil, fmt.Errorf("actual non-admin demo login required")
		}
		proofs = append(proofs, demoProof{Token: login.Token, UserID: login.User.ID, Username: account.Username, LastLoginAttempt: time.Now()})
		tokens = append(tokens, login.Token)
		ids = append(ids, login.User.ID)
		if !state.Applied {
			names := []string{"Jobs", "小林", "小夏"}
			if err := c.api(ctx, "PATCH", "/api/me", map[string]string{"displayName": names[index], "signature": "让消息回到人与人之间。"}, login.Token, nil); err != nil {
				return nil, err
			}
		}
	}
	if !state.Applied {
		for index := 1; index < len(ids); index++ {
			var request struct {
				ID    string `json:"id"`
				State string `json:"state"`
			}
			if err := c.api(ctx, "POST", "/api/contacts/requests", map[string]string{"userId": ids[index], "requestId": "demo-friend-" + state.Accounts[index].RegistrationID}, tokens[0], &request); err != nil {
				return nil, err
			}
			if request.State != "accepted" {
				if request.ID == "" {
					return nil, fmt.Errorf("actual contact request required")
				}
				if err := c.api(ctx, "POST", "/api/contacts/requests/"+request.ID+"/accept", map[string]any{}, tokens[index], nil); err != nil {
					return nil, err
				}
			}
		}
		var direct struct {
			ID        string `json:"id"`
			Admission string `json:"sendAdmission"`
		}
		if err := c.api(ctx, "POST", "/api/conversations", map[string]any{"kind": "direct", "memberIds": []string{ids[0]}}, tokens[1], &direct); err != nil {
			return nil, err
		}
		if direct.ID == "" || direct.Admission == "" {
			return nil, fmt.Errorf("actual direct send admission required")
		}
		if err := c.api(ctx, "POST", "/api/conversations/"+direct.ID+"/messages", map[string]string{"clientMsgId": state.MessageIDs[0], "type": "text", "text": "你好，周末一起聊聊吧。", "restoreEpoch": state.Epoch, "sendAdmission": direct.Admission}, tokens[1], nil); err != nil {
			return nil, err
		}
		var group struct {
			ID        string `json:"id"`
			Admission string `json:"sendAdmission"`
		}
		if err := c.api(ctx, "POST", "/api/conversations", map[string]any{"kind": "group", "title": "周末小组", "memberIds": []string{ids[1], ids[2]}}, tokens[0], &group); err != nil {
			return nil, err
		}
		if group.ID == "" || group.Admission == "" {
			return nil, fmt.Errorf("actual group send admission required")
		}
		if err := c.api(ctx, "POST", "/api/conversations/"+group.ID+"/messages", map[string]string{"clientMsgId": state.MessageIDs[1], "type": "text", "text": "欢迎大家加入周末小组，今天有什么新鲜事？", "restoreEpoch": state.Epoch, "sendAdmission": group.Admission}, tokens[0], nil); err != nil {
			return nil, err
		}
		state.Applied = true
		raw, _ := json.MarshalIndent(state, "", "  ")
		temp, err := os.CreateTemp(c.host.control, ".seed-result-")
		if err != nil {
			return nil, err
		}
		name := temp.Name()
		defer os.Remove(name)
		if _, err = temp.Write(append(raw, '\n')); err == nil {
			err = temp.Sync()
		}
		closeErr := temp.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if err = os.Rename(name, path); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("controller closing")
	}
	c.proofs = proofs
	c.mu.Unlock()
	preserveProofs = true
	return state.Accounts, nil
}
