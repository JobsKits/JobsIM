// credential.go
// JobsIM Native Demo Host
// Created by Jobs on 2026年11月18日，星期三.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type demoHTTPError struct {
	Status int
}

func (e demoHTTPError) Error() string {
	return fmt.Sprintf("fixed demo API returned%d; response omitted", e.Status)
}

type demoLoginProof struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Admin    bool   `json:"admin"`
	} `json:"user"`
}

// Ready is rechecked without burning a password login on every switch toggle.
// A changed credential invalidates these ordinary independent proof families.
func (c *demoController) revalidateReady(w http.ResponseWriter, r *http.Request) {
	c.validationMu.Lock()
	defer c.validationMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	err := c.validateCandidates(ctx)
	c.mu.Lock()
	if !c.closed {
		if err != nil {
			c.state = "failed"
			c.errorCode = "DEMO_CANDIDATE_UNAVAILABLE"
		} else {
			c.state = "ready"
			c.errorCode = ""
		}
	}
	c.mu.Unlock()
	c.respond(w, r, true)
}

func (c *demoController) validateCandidates(ctx context.Context) error {
	c.mu.Lock()
	proofs := append([]demoProof(nil), c.proofs...)
	accounts := append([]demoAccount(nil), c.accounts...)
	closed, child := c.closed, c.child
	c.mu.Unlock()
	if closed || len(proofs) == 0 || len(proofs) != len(accounts) {
		return fmt.Errorf("current ordinary demo candidate proofs unavailable")
	}
	if child != nil {
		select {
		case <-child.done:
			return fmt.Errorf("owned demo process exited")
		default:
		}
	}
	for index, proof := range proofs {
		var me struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Admin    bool   `json:"admin"`
		}
		err := c.api(ctx, "GET", "/api/me", nil, proof.Token, &me)
		if err == nil {
			if me.ID != proof.UserID || me.Username != accounts[index].Username || me.Admin {
				return fmt.Errorf("original ordinary demo identity changed")
			}
			continue
		}
		var status demoHTTPError
		if !errors.As(err, &status) || status.Status != 401 || time.Since(proof.LastLoginAttempt) < time.Minute {
			return fmt.Errorf("current demo candidate cannot be verified")
		}
		// Expired access proof may authenticate again, at most once per account
		// per minute. Status never enters this path and no password is reset.
		proof.LastLoginAttempt = time.Now()
		c.mu.Lock()
		if c.closed || index >= len(c.proofs) {
			c.mu.Unlock()
			return fmt.Errorf("controller closing")
		}
		c.proofs[index].LastLoginAttempt = proof.LastLoginAttempt
		c.mu.Unlock()
		var login demoLoginProof
		err = c.api(ctx, "POST", "/api/auth/login", map[string]any{
			"username": accounts[index].Username, "password": accounts[index].Password,
			"requestId": demoNonce(), "deviceName": "native-demo-candidate-check",
		}, "", &login)
		if err != nil || login.Token == "" || login.User.ID != proof.UserID || login.User.Username != accounts[index].Username || login.User.Admin {
			return fmt.Errorf("original demo credential no longer authenticates")
		}
		proof.Token = login.Token
		c.mu.Lock()
		if c.closed || index >= len(c.proofs) {
			c.mu.Unlock()
			_ = c.api(ctx, "POST", "/api/auth/logout", map[string]any{}, proof.Token, nil)
			return fmt.Errorf("controller closing")
		}
		c.proofs[index] = proof
		c.mu.Unlock()
	}
	return nil
}
