package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type demoProofTransport func(*http.Request) (*http.Response, error)

func (f demoProofTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func proofFixture(method func(*http.Request) (int, string)) *demoController {
	return &demoController{host: &demoHost{ip: "127.0.0.1"}, ctx: context.Background(),
		state: "ready", accounts: []demoAccount{{Username: "ordinary", Password: "fixture-only"}},
		proofs: []demoProof{{UserID: "stable-user", Username: "ordinary", Token: "original-proof", LastLoginAttempt: time.Now().Add(-time.Hour)}},
		child:  &demoChild{done: make(chan struct{})},
		client: &http.Client{Transport: demoProofTransport(func(r *http.Request) (*http.Response, error) {
			status, body := method(r)
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}}
}

func ensureProof(t *testing.T, c *demoController, expected int) map[string]any {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/demo/v1/ensure-start", nil)
	w := httptest.NewRecorder()
	c.handler().ServeHTTP(w, r)
	if w.Code != expected {
		t.Fatalf("fixed ensure status%d expected%d", w.Code, expected)
	}
	var value map[string]any
	if json.Unmarshal(w.Body.Bytes(), &value) != nil {
		t.Fatal("fixed response not JSON")
	}
	return value
}

func TestDemoReadyRepeatedEnsureUsesCurrentProofWithoutPasswordLogin(t *testing.T) {
	reads, logins := 0, 0
	c := proofFixture(func(r *http.Request) (int, string) {
		if r.URL.Path == "/api/auth/login" {
			logins++
			return 401, `{}`
		}
		reads++
		return 200, `{"id":"stable-user","username":"ordinary","admin":false}`
	})
	for i := 0; i < 8; i++ {
		if ensureProof(t, c, 200)["state"] != "ready" {
			t.Fatal("valid candidate not ready")
		}
	}
	if reads != 8 || logins != 0 {
		t.Fatal("switch toggles consumed password login rate window")
	}
	w := httptest.NewRecorder()
	c.handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/demo/v1/status", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "accounts") || reads != 8 || logins != 0 {
		t.Fatal("status exposed credentials or authenticated")
	}
}

func TestDemoChangedUsernameRejectsCachedCandidateAndPreservesService(t *testing.T) {
	c := proofFixture(func(r *http.Request) (int, string) {
		return 200, `{"id":"stable-user","username":"renamed","admin":false}`
	})
	child := c.child
	result := ensureProof(t, c, 503)
	if result["state"] != "failed" || result["accounts"] != nil || c.child != child {
		t.Fatal("changed username was returned as usable or service replaced")
	}
}

func TestDemoInvalidatedPasswordProofFailsAndBoundsLoginAttempts(t *testing.T) {
	logins := 0
	c := proofFixture(func(r *http.Request) (int, string) {
		if r.URL.Path == "/api/auth/login" {
			logins++
		}
		return 401, `{}`
	})
	child := c.child
	ensureProof(t, c, 503)
	ensureProof(t, c, 503)
	if logins != 1 || c.child != child || c.accounts[0].Password != "fixture-only" {
		t.Fatal("invalid credential retried unboundedly, reset or restarted service")
	}
}

func TestDemoExpiredProofCanReauthenticateSameOrdinaryIdentity(t *testing.T) {
	logins := 0
	c := proofFixture(func(r *http.Request) (int, string) {
		if r.URL.Path == "/api/auth/login" {
			logins++
			return 200, `{"token":"renewed-proof","user":{"id":"stable-user","username":"ordinary","admin":false}}`
		}
		if r.Header.Get("Authorization") == "Bearer renewed-proof" {
			return 200, `{"id":"stable-user","username":"ordinary","admin":false}`
		}
		return 401, `{}`
	})
	ensureProof(t, c, 200)
	ensureProof(t, c, 200)
	if logins != 1 || c.proofs[0].Token != "renewed-proof" {
		t.Fatal("expired proof did not renew within same account")
	}
}
