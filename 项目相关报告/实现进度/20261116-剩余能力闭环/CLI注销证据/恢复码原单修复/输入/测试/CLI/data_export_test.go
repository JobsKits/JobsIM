// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type cliDataExportFixture struct {
	*cliSyncFixture
	view           dataExportView
	feature        bool
	recorded       bool
	unknown        bool
	cancelUnknown  bool
	zip            []byte
	media          string
	ticket         string
	beforeCreate   func(string)
	beforeDownload func()
	beforeStatus   func()
}

func cliDataExportSetup(t *testing.T) *cliDataExportFixture {
	f := newCLISyncFixture(t)
	p := authPair{Token: f.client.token, FamilyID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceID: f.identity.DeviceID, RestoreEpoch: "1", SourceContextID: "personal"}
	p.User.ID = f.identity.UserID
	f.client.auth = &authManager{c: f.client, pair: p, state: authState{Status: "active"}}
	r := &cliDataExportFixture{cliSyncFixture: f, feature: true, media: "application/zip", ticket: "unit-only-ticket-in-memory-123456"}
	r.view = dataExportView{Job: dataExportJob{ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Artifact: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Epoch: "1", Build: "9007199254740993", State: "ready", Expires: time.Now().Add(time.Hour).Format(time.RFC3339Nano), Cleanup: "published"}, Format: "zip", Scope: "personal_current", Coverage: []string{"account_public_fields"}, Exclusions: []string{"access_and_refresh_tokens"}}
	var content bytes.Buffer
	z := zip.NewWriter(&content)
	for _, name := range []string{"manifest.json", "account-export.json"} {
		entry, _ := z.Create(name)
		entry.Write([]byte(`{"metadata":"owned export fixture"}`))
	}
	z.Close()
	r.zip = content.Bytes()
	f.setHook(r.serve)
	return r
}
func (r *cliDataExportFixture) serve(w http.ResponseWriter, q *http.Request) bool {
	path := "/api/me/data-exports/" + r.view.Job.ID
	switch q.URL.Path {
	case "/api/bootstrap":
		cliSyncJSON(w, 200, map[string]any{"deploymentId": r.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{"chat_export": r.feature}})
	case "/api/me/data-export-results":
		if r.recorded {
			cliSyncJSON(w, 200, r.view)
		} else {
			cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "EXPORT_OPERATION_NOT_FOUND"}})
		}
	case "/api/me/data-exports":
		raw, _ := io.ReadAll(q.Body)
		var body map[string]string
		json.Unmarshal(raw, &body)
		if r.beforeCreate != nil {
			r.beforeCreate(body["requestId"])
		}
		if r.unknown {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
		} else {
			r.recorded = true
			cliSyncJSON(w, 202, r.view)
		}
	case path:
		if r.beforeStatus != nil {
			r.beforeStatus()
		}
		cliSyncJSON(w, 200, r.view)
	case path + "/cancel":
		r.view.Job.State = "cancelled"
		r.view.Job.Cleanup = "cleared"
		if r.cancelUnknown {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
		} else {
			cliSyncJSON(w, 200, r.view)
		}
	case path + "/download-ticket":
		cliSyncJSON(w, 200, map[string]string{"downloadToken": r.ticket, "expiresAt": time.Now().Add(50 * time.Second).Format(time.RFC3339Nano), "format": "zip"})
	case path + "/download":
		if q.URL.RawQuery != "" || q.Header.Get("X-JobsIM-Download-Token") != r.ticket || q.Header.Get("Authorization") == "" {
			r.t.Error("ticket must only use authenticated header")
		}
		if r.beforeDownload != nil {
			r.beforeDownload()
		}
		w.Header().Set("Content-Type", r.media)
		w.Header().Set("Cache-Control", "no-store")
		w.Write(r.zip)
	default:
		return false
	}
	return true
}
func (r *cliDataExportFixture) create() dataExportIntent {
	o, v, err := r.client.createDataExport()
	if err != nil {
		r.t.Fatal(err)
	}
	if v.Job.ID != r.view.Job.ID {
		r.t.Fatal("wrong task")
	}
	return o
}
func (r *cliDataExportFixture) original() dataExportIntent {
	var id string
	if err := r.store.db.QueryRow("SELECT id FROM data_exports ORDER BY rowid DESC LIMIT 1").Scan(&id); err != nil {
		r.t.Fatal(err)
	}
	o, err := loadDataExport(r.store.db, id)
	if err != nil {
		r.t.Fatal(err)
	}
	return o
}
func TestDataExportOriginalIntentFullCommitBeforeHTTP(t *testing.T) {
	r := cliDataExportSetup(t)
	r.beforeCreate = func(id string) {
		o, err := loadDataExport(r.store.db, id)
		if err != nil {
			t.Error(err)
		}
		if validateDataExportIntent(o) != nil || o.Binding.Family != r.client.auth.pair.FamilyID {
			t.Error("missing durable original family")
		}
		if err = receiptDurability(r.store.db); err != nil {
			t.Error(err)
		}
	}
	o := r.create()
	row := r.original()
	if row.Job != r.view.Job.ID || row.Body != o.Body || row.Hash != o.Hash {
		t.Fatal("original job bind changed")
	}
	if _, _, err := r.client.createDataExport(); err != nil {
		t.Fatal("completed task must not block explicit next creation", err)
	}
}
func TestDataExportUnknownReopenPureQueryAndExplicitOriginalRetry(t *testing.T) {
	r := cliDataExportSetup(t)
	r.unknown = true
	if _, _, err := r.client.createDataExport(); err == nil {
		t.Fatal("unknown accepted")
	}
	o := r.original()
	if _, _, err := r.client.createDataExport(); err == nil {
		t.Fatal("unknown replaced")
	}
	path := r.store.path
	r.store.close()
	var err error
	r.store, err = openStore(path, r.identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer r.store.close()
	r.client.store = r.store
	before := len(r.calls("/api/me/data-exports"))
	if _, err = r.client.performDataExport(o.ID, false, false); err == nil {
		t.Fatal("404 accepted")
	}
	if len(r.calls("/api/me/data-exports")) != before {
		t.Fatal("automatic creation")
	}
	r.unknown = false
	if _, err = r.client.performDataExport(o.ID, true, false); err != nil {
		t.Fatal(err)
	}
	latest := r.original()
	if latest.Body != o.Body || latest.Hash != o.Hash {
		t.Fatal("rebound original")
	}
	r.recorded = false
	before = len(r.calls("/api/me/data-exports"))
	if _, err = r.client.performDataExport(o.ID, true, false); err == nil {
		t.Fatal("completed404 accepted")
	}
	if len(r.calls("/api/me/data-exports")) != before {
		t.Fatal("completed was recreated")
	}
}
func TestDataExportCapabilityWithdrawalStillChecksAndCancelsOriginal(t *testing.T) {
	r := cliDataExportSetup(t)
	o := r.create()
	r.feature = false
	if _, err := r.client.statusDataExport(o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.performDataExport(o.ID, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.downloadDataExport(o.ID, filepath.Join(t.TempDir(), "no.zip")); err == nil {
		t.Fatal("missing cap downloaded")
	}
	r.cancelUnknown = true
	if _, err := r.client.cancelDataExport(o.ID); err == nil {
		t.Fatal("unknown cancellation accepted")
	}
	row := r.original()
	if row.Cancel == "" || row.CancelHash != fingerprint(row.Cancel) {
		t.Fatal("cancel not durable")
	}
	before := len(r.calls("/api/me/data-exports/" + row.Job + "/cancel"))
	if _, err := r.client.cancelDataExport(o.ID); err != nil {
		t.Fatal(err)
	}
	if r.original().Cancel != row.Cancel || len(r.calls("/api/me/data-exports/"+row.Job+"/cancel")) != before {
		t.Fatal("terminal current state need not POST new cancel")
	}
}
func TestDataExportWrongFamilyPhysicalEpochAndOriginalHashRefuse(t *testing.T) {
	for _, kind := range []string{"family", "physical", "epoch", "body", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			r := cliDataExportSetup(t)
			o := r.create()
			before := len(r.calls("/api/me/data-export-results"))
			switch kind {
			case "family":
				r.client.auth.mu.Lock()
				r.client.auth.pair.FamilyID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
				r.client.auth.mu.Unlock()
			case "physical":
				r.store.db.Exec("UPDATE metadata SET value='dddddddd-dddd-4ddd-8ddd-dddddddddddd' WHERE key='sync-physical-generation'")
			case "epoch":
				r.client.epoch = "2"
			case "body":
				r.store.db.Exec("UPDATE data_exports SET payload_hash='bad'")
			case "cancel":
				r.store.db.Exec("UPDATE data_exports SET payload=json_set(payload,'$.cancelBody','{}','$.cancelHash','bad')")
			}
			if _, err := r.client.performDataExport(o.ID, false, false); err == nil {
				t.Fatal("wrong binding accepted")
			}
			if len(r.calls("/api/me/data-export-results")) != before {
				t.Fatal("invalid original contacted owner")
			}
		})
	}
}
func TestDataExportZIPDownloadAtomicNoOverwriteAndTicketOnlyRAM(t *testing.T) {
	r := cliDataExportSetup(t)
	o := r.create()
	destination := filepath.Join(t.TempDir(), "mine.zip")
	result, err := r.client.downloadDataExport(o.ID, destination)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(raw, r.zip) || result.SHA != fingerprint(string(raw)) {
		t.Fatal("wrong complete file", err)
	}
	var payload string
	r.store.db.QueryRow("SELECT payload FROM data_exports WHERE id=?", o.ID).Scan(&payload)
	if strings.Contains(payload, r.ticket) || strings.Contains(payload, "downloadToken") || strings.Contains(payload, "owned export fixture") {
		t.Fatal("secret/file bytes persisted")
	}
	if _, err = r.client.downloadDataExport(o.ID, destination); err == nil {
		t.Fatal("destination replaced")
	}
	if after, _ := os.ReadFile(destination); !bytes.Equal(after, raw) {
		t.Fatal("old file lost")
	}
	partials, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".jobsim-personal-*.part"))
	if len(partials) != 0 {
		t.Fatal("temporary name not removed")
	}
}
func TestDataExportBadStreamRevocationAndLateContextNeverPublish(t *testing.T) {
	for _, kind := range []string{"truncated", "mime", "family", "source", "target-race"} {
		t.Run(kind, func(t *testing.T) {
			r := cliDataExportSetup(t)
			o := r.create()
			destination := filepath.Join(t.TempDir(), "mine.zip")
			switch kind {
			case "truncated":
				r.zip = r.zip[:len(r.zip)-8]
			case "mime":
				r.media = "application/json"
			case "family":
				r.beforeDownload = func() {
					r.client.auth.mu.Lock()
					r.client.auth.pair.FamilyID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
					r.client.auth.mu.Unlock()
				}
			case "source":
				r.beforeDownload = func() { r.view.Job.State = "cancelled"; r.view.Job.Cleanup = "cleared" }
			case "target-race":
				r.beforeDownload = func() { os.WriteFile(destination, []byte("original target"), 0600) }
			}
			if _, err := r.client.downloadDataExport(o.ID, destination); err == nil {
				t.Fatal("bad download published")
			}
			if kind == "target-race" {
				b, _ := os.ReadFile(destination)
				if string(b) != "original target" {
					t.Fatal("racing target overwritten")
				}
			} else if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("destination exists", err)
			}
			partial, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".jobsim-personal-*.part"))
			if len(partial) != 0 {
				t.Fatal("failed partial remains")
			}
		})
	}
}
func TestDataExportStrictProtocolOffsetLargeCountersAndZIP(t *testing.T) {
	r := cliDataExportSetup(t)
	_, b, _, err := r.client.dataExportContext()
	if err != nil {
		t.Fatal(err)
	}
	v := r.view
	v.Job.Expires = "2026-11-17T10:00:00.123456789+08:00"
	if err = validateDataExportView(v, b, ""); err != nil {
		t.Fatal("valid RFC3339 offset", err)
	}
	v.Job.Build = "09007199254740993"
	if validateDataExportView(v, b, "") == nil {
		t.Fatal("noncanonical generation")
	}
	v = r.view
	v.Job.Epoch = "2"
	if validateDataExportView(v, b, "") == nil {
		t.Fatal("wrong epoch")
	}
	file, err := os.CreateTemp(t.TempDir(), "zip")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	file.Write(r.zip)
	if err = validateDataExportZIP(file, int64(len(r.zip))); err != nil {
		t.Fatal(err)
	}
}
func TestDataExportSQLFaultCannotCreateRemoteTask(t *testing.T) {
	r := cliDataExportSetup(t)
	if _, err := r.store.db.Exec("CREATE TRIGGER fail_data_export BEFORE INSERT ON data_exports BEGIN SELECT RAISE(FAIL,'injected persistence failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.client.createDataExport(); err == nil {
		t.Fatal("SQL fault accepted")
	}
	if len(r.calls("/api/me/data-exports")) != 0 || len(r.calls("/api/me/data-export-results")) != 0 {
		t.Fatal("HTTP before original commit")
	}
}
