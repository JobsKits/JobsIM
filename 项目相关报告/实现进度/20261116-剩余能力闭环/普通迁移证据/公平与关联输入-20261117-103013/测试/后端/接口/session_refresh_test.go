package jobsim

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestRefreshTransactionSingleConsumeWithoutOuterProcessFence(t *testing.T) {
	path := os.Getenv("JOBS_IM_REFRESH_TEST_ENV")
	if path == "" {
		t.Skip("explicit isolated refresh environment required")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("private environment unreadable")
	}
	var info struct {
		RefreshTestRealm bool   `json:"refreshTestRealm"`
		StateDir         string `json:"stateDir"`
		BusinessURL      string `json:"businessURL"`
		JournalURL       string `json:"journalURL"`
		ServerURL        string `json:"serverURL"`
		CAPath           string `json:"caPath"`
		PGPassFile       string `json:"pgPassFile"`
		InternalBasePort int    `json:"internalBasePort"`
	}
	if json.Unmarshal(data, &info) != nil || !info.RefreshTestRealm {
		t.Fatal("explicit isolated refresh realm required")
	}
	if info.PGPassFile != "" {
		t.Setenv("PGPASSFILE", info.PGPassFile)
	}
	roots := x509.NewCertPool()
	ca, e := os.ReadFile(info.CAPath)
	if e != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatal("explicit trust root unavailable")
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	defer client.CloseIdleConnections()
	username, password := "rt_race_"+newID()[:8], "SQL refresh race "+randomToken(20)
	authenticate := func(path string) authResult {
		payload, _ := json.Marshal(registerRequest{Username: username, Password: password, RequestID: newID(), DeviceName: "transaction-race"})
		response, e := client.Post(info.ServerURL+path, "application/json", bytes.NewReader(payload))
		if e != nil {
			t.Fatal("real TLS authentication transport failed")
		}
		defer response.Body.Close()
		var auth authResult
		if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&auth) != nil || auth.RefreshToken == "" {
			t.Fatal("real isolated authentication failed; response redacted")
		}
		return auth
	}
	original := authenticate("/api/auth/register")
	control := authenticate("/api/auth/login")
	defer func() {
		q, _ := http.NewRequest("POST", info.ServerURL+"/api/auth/logout", bytes.NewReader([]byte(`{}`)))
		q.Header.Set("Authorization", "Bearer "+control.Token)
		q.Header.Set("Content-Type", "application/json")
		if response, e := client.Do(q); e == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}()
	roles := map[string]string{}
	for i, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance"} {
		roles[role] = fmt.Sprintf("http://127.0.0.1:%d", info.InternalBasePort+i)
	}
	configuration, e := PrepareConfig(Config{StateDir: info.StateDir, DatabaseURL: info.BusinessURL, JournalDatabaseURL: info.JournalURL, RoleURLs: roles, PublicURL: info.ServerURL})
	if e != nil {
		t.Fatal("isolated configuration failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	identity, e := NewApp(ctx, configuration, "identity")
	if e != nil {
		t.Fatal("isolated identity initialization failed")
	}
	defer identity.Close()
	payload, _ := json.Marshal(refreshRequest{RefreshToken: original.RefreshToken, RequestID: newID(), FamilyID: original.FamilyID, RefreshGeneration: original.RefreshGeneration})
	responses := make(chan *httptest.ResponseRecorder, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			request := httptest.NewRequest("POST", info.ServerURL+"/api/auth/refresh", bytes.NewReader(payload)).WithContext(ctx)
			request.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			// Deliberately bypass Handler/withAdmissionFence: prove the PG user /
			// session locks enforce single consume independently of the OS fence.
			if e := identity.refreshSession(writer, request); e != nil {
				writeError(writer, request, e)
			}
			responses <- writer
		}()
	}
	close(start)
	workers.Wait()
	close(responses)
	succeeded, reused := 0, 0
	for response := range responses {
		if response.Code == 200 {
			succeeded++
			continue
		}
		var rejected struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(response.Body.Bytes(), &rejected) == nil && response.Code == 401 && rejected.Error.Code == "REFRESH_REUSE_DETECTED" {
			reused++
		}
	}
	if succeeded != 1 || reused != 1 {
		t.Fatal("concurrent SQL consumption was not exactly one success and one family reuse")
	}
	var count int
	var revoked bool
	if identity.DB.QueryRow(ctx, `SELECT count(*) FROM auth.refresh_token_history WHERE family_id=$1`, original.FamilyID).Scan(&count) != nil || count != 2 {
		t.Fatal("rotation created an extra successor")
	}
	if identity.DB.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM auth.sessions WHERE family_id=$1`, original.FamilyID).Scan(&revoked) != nil || !revoked {
		t.Fatal("reuse did not revoke its original family")
	}
	request, _ := http.NewRequest("GET", info.ServerURL+"/api/me", nil)
	request.Header.Set("Authorization", "Bearer "+control.Token)
	response, e := client.Do(request)
	if e != nil {
		t.Fatal("independent device check failed")
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("reuse incorrectly revoked an independent family")
	}
	if destination := os.Getenv("JOBS_IM_REFRESH_SQL_REPORT"); destination != "" {
		raw, _ := json.MarshalIndent(map[string]any{"status": "passed", "outerOSFenceBypassedOnlyInTest": true, "realPostgreSQLConcurrentConsume": true, "oneSuccessOneReuse": true, "onlyTwoHistoryGenerations": true, "onlyOriginalFamilyRevoked": true, "independentDeviceStillAuthorized": true, "scope": "Current identity handler/SQL transactions with real isolated PostgreSQL and TLS; private test only, not a production lock bypass"}, "", "  ")
		if os.WriteFile(destination, append(raw, '\n'), 0600) != nil {
			t.Fatal("structured report write failed")
		}
	}
}
