// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuthCoordinatorActualHTTPSAndUnknownClosure(t *testing.T) {
	path := os.Getenv("JOBS_IM_CLI_AUTH_TEST_ENV")
	if path == "" {
		t.Skip("explicit isolated refresh realm required")
	}
	var info struct {
		RefreshTestRealm bool   `json:"refreshTestRealm"`
		ServerURL        string `json:"serverURL"`
		CAPath           string `json:"caPath"`
		BusinessURL      string `json:"businessURL"`
		PGPassFile       string `json:"pgPassFile"`
		PGBin            string `json:"pgBin"`
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &info) != nil || !info.RefreshTestRealm {
		t.Fatal("explicit private refresh realm unavailable")
	}
	endpoint, err := url.Parse(info.ServerURL)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() != "127.0.0.1" {
		t.Fatal("isolated strict HTTPS required")
	}
	ca, err := os.ReadFile(info.CAPath)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatal("explicit test CA unavailable")
	}
	trust := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	c := &client{base: info.ServerURL, http: &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: trust}}}
	var boot struct {
		DeploymentID string          `json:"deploymentId"`
		RestoreEpoch string          `json:"restoreEpoch"`
		Features     map[string]bool `json:"features"`
	}
	if c.call("GET", "/api/bootstrap", nil, &boot) != nil || !boot.Features["auth_refresh"] || !boot.Features["auth_retirement_chain"] {
		t.Fatal("real refresh/closure capability missing")
	}
	c.deployment, c.epoch = boot.DeploymentID, boot.RestoreEpoch
	username, password := "cli_rt_"+id()[:8], "CLI real refresh proof "+id()
	initialID := id()
	var first, control authPair
	if c.call("POST", "/api/auth/register", map[string]string{"username": username, "password": password, "requestId": initialID, "deviceName": "JobsIM CLI"}, &first) != nil {
		t.Fatal("real fixture registration failed")
	}
	if c.call("POST", "/api/auth/login", map[string]string{"username": username, "password": password, "requestId": id(), "deviceName": "Independent CLI control"}, &control) != nil {
		t.Fatal("real independent device failed")
	}
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "auth.sqlite")+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("CREATE TABLE authentication_state(singleton INTEGER PRIMARY KEY,payload TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	m := &authManager{c: c, db: db, vault: &testCredentialVault{}, username: username, refresh: true, chain: true}
	c.auth = m
	if err = m.accept(first, initialID); err != nil {
		t.Fatal("real pair not accepted by coordinator")
	}
	defer func() {
		m.logout()
		c.callWithToken("POST", "/api/auth/logout", map[string]string{}, nil, control.Token)
	}()
	sqlQuery := func(statement string) string {
		command := exec.Command(filepath.Join(info.PGBin, "psql"), info.BusinessURL, "-XqAt", "-v", "ON_ERROR_STOP=1", "-c", statement)
		command.Env = append(os.Environ(), "PGPASSFILE="+info.PGPassFile)
		output, err := command.Output()
		if err != nil {
			t.Fatal("owned-family PostgreSQL clock fixture failed")
		}
		return strings.TrimSpace(string(output))
	}
	defer sqlQuery("UPDATE auth.sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id='" + first.User.ID + "'")
	changed := sqlQuery("UPDATE auth.sessions s SET access_expires_at=now()-interval '1 second' FROM auth.instance i WHERE i.singleton AND s.family_id='" + first.FamilyID + "' AND s.user_id='" + first.User.ID + "' AND s.restore_epoch=i.restore_epoch RETURNING s.family_id::text")
	if changed != first.FamilyID {
		t.Fatal("clock fixture did not match current owned family")
	}
	var workers sync.WaitGroup
	failures := make(chan error, 6)
	for range 6 {
		workers.Add(1)
		go func() { defer workers.Done(); var me any; failures <- c.call("GET", "/api/me", nil, &me) }()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("real access expiry/single-flight failed; private response redacted")
		}
	}
	if m.pair.RefreshGeneration != "2" || sqlQuery("SELECT count(*) FROM auth.refresh_token_history WHERE family_id='"+first.FamilyID+"'") != "2" {
		t.Fatal("real coordinator issued extra successors")
	}
	active := m.pair
	var me any
	if c.callWithToken("GET", "/api/me", nil, &me, control.Token) != nil {
		t.Fatal("refresh revoked independent control family")
	}
	r0 := authIntent{Mode: "login", Username: username, RequestID: id(), DeviceName: "JobsIM CLI"}
	body, _ := json.Marshal(map[string]string{"username": username, "password": password, "requestId": r0.RequestID, "deviceName": r0.DeviceName})
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", endpoint.Host, trust)
	if err != nil {
		t.Fatal("real initial response-loss transport unavailable")
	}
	request := []byte("POST /api/auth/login HTTP/1.1\r\nHost: " + endpoint.Host + "\r\nContent-Type: application/json\r\nContent-Length: " + jsonNumber(len(body)) + "\r\nConnection: close\r\n\r\n")
	if _, err = connection.Write(append(request, body...)); err != nil {
		connection.Close()
		t.Fatal("real response-loss request failed")
	}
	unknownFamily := ""
	for range 100 {
		unknownFamily = sqlQuery("SELECT family_id::text FROM auth.local_auth_requests WHERE request_id='" + r0.RequestID + "'")
		if unknownFamily != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	connection.Close()
	if unknownFamily == "" {
		t.Fatal("real unknown initial authentication did not commit")
	}
	m.state = authState{Status: "initial_pending", Pending: []authIntent{r0}}
	m.pair = authPair{}
	c.token = ""
	if m.persist() != nil {
		t.Fatal("real unknown nonce not saved")
	}
	r1, err := m.prepareInitial("reauthenticate")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := m.prepareInitial("reauthenticate")
	if err != nil || len(r2.PreviousRequestIDs) != 2 {
		t.Fatal("unposted successor chain unavailable")
	}
	if err = m.authenticate("login", bufio.NewReader(strings.NewReader(password+"\n"))); err != nil {
		t.Fatal("real complete-closure recovery failed; proof and response redacted")
	}
	if sqlQuery("SELECT (revoked_at IS NOT NULL)::text FROM auth.sessions WHERE family_id='"+unknownFamily+"'") != "true" {
		t.Fatal("real complete closure left unknown ancestor family alive")
	}
	late := map[string]any{"username": username, "password": password, "requestId": r1.RequestID, "deviceName": r1.DeviceName, "previousRequestId": r1.PreviousRequestID, "previousRequestIds": r1.PreviousRequestIDs}
	err = c.callWithToken("POST", "/api/auth/reauthenticate", late, &me, "")
	var rejected *apiFailure
	if !errors.As(err, &rejected) || rejected.Code != "AUTHENTICATION_RETIRED" {
		t.Fatal("late unposted successor created an orphan")
	}
	if c.callWithToken("GET", "/api/me", nil, &me, control.Token) != nil || c.callWithToken("GET", "/api/me", nil, &me, active.Token) != nil {
		t.Fatal("closure revoked unrelated known families")
	}
	c.callWithToken("POST", "/api/auth/logout", map[string]string{}, nil, active.Token)
	var payload string
	db.QueryRow("SELECT payload FROM authentication_state WHERE singleton=1").Scan(&payload)
	if strings.Contains(payload, password) || strings.Contains(payload, m.pair.Token) || strings.Contains(payload, m.pair.RefreshToken) {
		t.Fatal("real credentials reached ordinary SQLite")
	}
	if destination := os.Getenv("JOBS_IM_CLI_AUTH_REPORT"); destination != "" {
		report := map[string]any{"status": "passed", "strictHTTPS": true, "realCoordinatorAgainstPostgreSQL": true, "accessExpiryFixture": "only current owned-family SQL access deadline, not fifteen minutes wall-clock waiting", "sixConcurrentCallsOneRefresh": true, "secureStoreForThisCoordinatorTest": "bounded in-memory test double; native system helper has separate evidence", "R0CommittedResponseUnreadR1NeverPostedR2CompleteClosure": true, "lateR1Rejected": true, "independentFamiliesRemainAuthorized": true, "noSecretsInOrdinarySQLite": true, "scope": "real HTTPS coordinator plus explicit SQL clock fixture; not full I/three-system secure storage acceptance"}
		encoded, _ := json.MarshalIndent(report, "", "  ")
		if os.WriteFile(destination, append(encoded, '\n'), 0600) != nil {
			t.Fatal("public auth report write failed")
		}
	}
}

func jsonNumber(value int) string { raw, _ := json.Marshal(value); return string(raw) }

func TestAuthNativeSystemVaultSeparateDiagnostic(t *testing.T) {
	if os.Getenv("JOBS_IM_CLI_NATIVE_VAULT") != "1" {
		t.Skip("explicit native system credential diagnostic required")
	}
	v, err := openCredentialVault(fingerprint("independent-vault-diagnostic-" + id()))
	if err != nil {
		t.Fatal("native secure facility unavailable; no plaintext fallback")
	}
	defer v.Delete()
	if _, err = v.Read(); !errors.Is(err, errVaultNotFound) {
		t.Fatal("independent diagnostic entry did not start empty")
	}
	first := []byte("JobsIM nonsecret diagnostic one")
	if v.Write(first) != nil {
		t.Fatal("native secure write failed")
	}
	actual, err := v.Read()
	if err != nil || !bytes.Equal(first, actual) {
		t.Fatal("native secure roundtrip failed")
	}
	erase(actual)
	second := []byte("JobsIM nonsecret diagnostic atomic replacement")
	if v.Write(second) != nil {
		t.Fatal("native secure replacement failed")
	}
	actual, err = v.Read()
	if err != nil || !bytes.Equal(second, actual) {
		t.Fatal("native secure replacement roundtrip failed")
	}
	erase(actual)
	if v.Delete() != nil {
		t.Fatal("native secure deletion failed")
	}
	if _, err = v.Read(); !errors.Is(err, errVaultNotFound) {
		t.Fatal("native secure deletion retained old data")
	}
}
