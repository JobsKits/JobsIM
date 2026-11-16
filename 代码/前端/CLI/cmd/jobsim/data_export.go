// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type dataExportBinding struct {
	Identity localIdentity `json:"identity"`
	Physical string        `json:"physicalStore"`
	Epoch    string        `json:"restoreEpoch"`
	Family   string        `json:"familyId"`
}
type dataExportJob struct {
	ID       string `json:"id"`
	Artifact string `json:"artifactId"`
	Epoch    string `json:"restoreEpoch"`
	Build    string `json:"buildGeneration"`
	State    string `json:"state"`
	Expires  string `json:"expiresAt"`
	Cleanup  string `json:"cleanupState"`
	Failure  string `json:"failureCode,omitempty"`
}
type dataExportView struct {
	Job        dataExportJob `json:"job"`
	Format     string        `json:"format"`
	Scope      string        `json:"scope"`
	Captured   string        `json:"capturedAt,omitempty"`
	Coverage   []string      `json:"coverage"`
	Exclusions []string      `json:"exclusions"`
}
type dataExportIntent struct {
	ID         string            `json:"requestId"`
	Binding    dataExportBinding `json:"binding"`
	Body       string            `json:"originalBody"`
	Hash       string            `json:"originalHash"`
	Created    string            `json:"createdAt"`
	Job        string            `json:"jobId,omitempty"`
	Cancel     string            `json:"cancelBody,omitempty"`
	CancelHash string            `json:"cancelHash,omitempty"`
	View       *dataExportView   `json:"view,omitempty"`
}

var dataExportWord = regexp.MustCompile(`^[a-z_]{1,100}$`)
var dataExportFailure = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)

func dataExportMigrate(tx *sql.Tx) error {
	_, err := tx.Exec("CREATE TABLE IF NOT EXISTS data_exports(id TEXT PRIMARY KEY,epoch TEXT NOT NULL,payload TEXT NOT NULL,payload_hash TEXT NOT NULL)")
	return err
}
func dataExportHash(o dataExportIntent) string {
	o.Hash = ""
	o.Job = ""
	o.Cancel = ""
	o.CancelHash = ""
	o.View = nil
	raw, _ := json.Marshal(o)
	return fingerprint(string(raw))
}
func validateDataExportIntent(o dataExportIntent) error {
	var body map[string]string
	if !syncUUIDPattern.MatchString(o.ID) || syncDecode([]byte(o.Body), &body) != nil || len(body) != 2 || body["requestId"] != o.ID || body["restoreEpoch"] != o.Binding.Epoch || dataExportHash(o) != o.Hash || !syncUUIDPattern.MatchString(o.Binding.Physical) || !syncUUIDPattern.MatchString(o.Binding.Family) || o.Binding.Identity.Space != "personal" {
		return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
	}
	if n, err := syncCounter(o.Binding.Epoch); err != nil || n == 0 {
		return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
	}
	if _, err := time.Parse(time.RFC3339Nano, o.Created); err != nil {
		return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
	}
	if o.Job != "" && !syncUUIDPattern.MatchString(o.Job) {
		return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
	}
	if o.Cancel != "" {
		var cancel map[string]string
		if syncDecode([]byte(o.Cancel), &cancel) != nil || len(cancel) != 1 || !syncUUIDPattern.MatchString(cancel["requestId"]) || fingerprint(o.Cancel) != o.CancelHash || o.Job == "" {
			return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
		}
	} else if o.CancelHash != "" {
		return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
	}
	if o.View != nil && validateDataExportView(*o.View, o.Binding, o.Job) != nil {
		return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
	}
	return nil
}
func validateDataExportView(v dataExportView, b dataExportBinding, job string) error {
	j := v.Job
	if v.Format != "zip" || v.Scope != "personal_current" || !syncUUIDPattern.MatchString(j.ID) || !syncUUIDPattern.MatchString(j.Artifact) || job != "" && j.ID != job || j.Epoch != b.Epoch || !oneOf(j.State, "requested", "building", "ready", "cancelled", "expired", "failed") || !dataExportWord.MatchString(j.Cleanup) || len(j.Cleanup) > 48 || j.Failure != "" && !dataExportFailure.MatchString(j.Failure) {
		return syncRefuse("DATA_EXPORT_PROTOCOL_INVALID")
	}
	if n, err := syncCounter(j.Build); err != nil || n == 0 {
		return syncRefuse("DATA_EXPORT_PROTOCOL_INVALID")
	}
	if _, err := time.Parse(time.RFC3339Nano, j.Expires); err != nil {
		return syncRefuse("DATA_EXPORT_PROTOCOL_INVALID")
	}
	if v.Captured != "" {
		if _, err := time.Parse(time.RFC3339Nano, v.Captured); err != nil {
			return syncRefuse("DATA_EXPORT_PROTOCOL_INVALID")
		}
	}
	for _, list := range [][]string{v.Coverage, v.Exclusions} {
		if list == nil || len(list) > 64 {
			return syncRefuse("DATA_EXPORT_PROTOCOL_INVALID")
		}
		for _, name := range list {
			if !dataExportWord.MatchString(name) {
				return syncRefuse("DATA_EXPORT_PROTOCOL_INVALID")
			}
		}
	}
	return nil
}
func (c *client) dataExportContext() (*localStore, dataExportBinding, func() bool, error) {
	s, current, err := c.receiptContext()
	if err != nil {
		return nil, dataExportBinding{}, nil, err
	}
	if c.auth == nil {
		return nil, dataExportBinding{}, nil, syncRefuse("DATA_EXPORT_AUTH_REQUIRED")
	}
	pair, err := c.auth.snapshot()
	if err != nil {
		return nil, dataExportBinding{}, nil, err
	}
	if pair.User.ID != s.identity.UserID || pair.DeviceID != s.identity.DeviceID || pair.RestoreEpoch != s.epoch || pair.SourceContextID != "personal" {
		return nil, dataExportBinding{}, nil, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	var physical string
	if err = s.db.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); err != nil {
		return nil, dataExportBinding{}, nil, err
	}
	b := dataExportBinding{Identity: s.identity, Physical: physical, Epoch: s.epoch, Family: pair.FamilyID}
	if !syncUUIDPattern.MatchString(b.Family) || !syncUUIDPattern.MatchString(b.Physical) {
		return nil, b, nil, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	if err = s.dataExportControl(s.db, b, current); err != nil {
		return nil, b, nil, err
	}
	return s, b, current, nil
}
func (s *localStore) dataExportControl(q syncSQL, b dataExportBinding, current func() bool) error {
	if !current() || s.identity != b.Identity || s.epoch != b.Epoch || s.checkSyncFile() != nil {
		return syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	var physical string
	var retired int
	if err := q.QueryRow("SELECT value FROM metadata WHERE key='sync-physical-generation'").Scan(&physical); err != nil {
		return err
	}
	if err := q.QueryRow("SELECT count(*) FROM metadata WHERE key='auth_retired' AND value='1'").Scan(&retired); err != nil {
		return err
	}
	if physical != b.Physical || retired != 0 {
		return syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	return nil
}
func loadDataExport(q syncSQL, id string) (dataExportIntent, error) {
	var o dataExportIntent
	var raw, hash string
	if err := q.QueryRow("SELECT payload,payload_hash FROM data_exports WHERE id=?", id).Scan(&raw, &hash); err != nil {
		return o, err
	}
	if syncDecode([]byte(raw), &o) != nil || validateDataExportIntent(o) != nil || o.ID != id || o.Hash != hash {
		return o, syncRefuse("DATA_EXPORT_INTENT_CHANGED")
	}
	return o, nil
}
func (s *localStore) changeDataExport(b dataExportBinding, current func() bool, id string, change func(*dataExportIntent) error) (dataExportIntent, error) {
	var o dataExportIntent
	tx, err := s.db.Begin()
	if err != nil {
		return o, err
	}
	defer tx.Rollback()
	if err = s.dataExportControl(tx, b, current); err != nil {
		return o, err
	}
	if err = receiptDurability(tx); err != nil {
		return o, err
	}
	if id == "" {
		var pending int
		if err = tx.QueryRow("SELECT count(*) FROM data_exports WHERE epoch=? AND json_extract(payload,'$.jobId') IS NULL", b.Epoch).Scan(&pending); err != nil {
			return o, err
		}
		if pending != 0 {
			return o, syncRefuse("DATA_EXPORT_ORIGINAL_PENDING")
		}
		o.ID, err = syncNewUUID()
		if err != nil {
			return o, err
		}
		o.Binding = b
		o.Created = time.Now().UTC().Format(time.RFC3339Nano)
		body, _ := json.Marshal(map[string]string{"requestId": o.ID, "restoreEpoch": b.Epoch})
		o.Body = string(body)
		o.Hash = dataExportHash(o)
	} else {
		o, err = loadDataExport(tx, id)
		if err != nil {
			return o, err
		}
		if o.Binding != b {
			return o, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
		}
	}
	if err = change(&o); err != nil {
		return o, err
	}
	if err = validateDataExportIntent(o); err != nil {
		return o, err
	}
	raw, _ := json.Marshal(o)
	if _, err = tx.Exec("INSERT INTO data_exports VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", o.ID, b.Epoch, string(raw), o.Hash); err != nil {
		return o, err
	}
	if !current() {
		return o, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	return o, tx.Commit()
}
func (c *client) dataExportCapability(b dataExportBinding, current func() bool, required bool) error {
	if !current() {
		return syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	var boot struct {
		Scope    string          `json:"deploymentId"`
		Epoch    string          `json:"restoreEpoch"`
		Features map[string]bool `json:"features"`
	}
	if err := c.call("GET", "/api/bootstrap", nil, &boot); err != nil {
		return err
	}
	if !current() || boot.Scope != b.Identity.DeploymentID || boot.Epoch != b.Epoch {
		return syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	if required && !boot.Features["chat_export"] {
		return syncRefuse("DATA_EXPORT_UNAVAILABLE")
	}
	return nil
}
func (c *client) dataExportRead(s *localStore, b dataExportBinding, current func() bool, o dataExportIntent, method, path string, body any) (dataExportView, error) {
	var v dataExportView
	if err := s.dataExportControl(s.db, b, current); err != nil {
		return v, err
	}
	if err := c.call(method, path, body, &v); err != nil {
		return v, err
	}
	if err := validateDataExportView(v, b, o.Job); err != nil {
		return v, err
	}
	_, err := s.changeDataExport(b, current, o.ID, func(row *dataExportIntent) error {
		if row.Body != o.Body || row.Hash != o.Hash || row.Job != "" && row.Job != v.Job.ID {
			return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
		}
		row.Job = v.Job.ID
		row.View = &v
		return nil
	})
	return v, err
}
func (c *client) performDataExport(id string, retry, fresh bool) (dataExportView, error) {
	s, b, current, err := c.dataExportContext()
	if err != nil {
		return dataExportView{}, err
	}
	o, err := loadDataExport(s.db, id)
	if err != nil {
		return dataExportView{}, err
	}
	if o.Binding != b {
		return dataExportView{}, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	if err = c.dataExportCapability(b, current, retry || fresh); err != nil {
		return dataExportView{}, err
	}
	v, err := c.dataExportRead(s, b, current, o, "POST", "/api/me/data-export-results", json.RawMessage(o.Body))
	if err == nil {
		return v, nil
	}
	var api *apiFailure
	if !errors.As(err, &api) || api.Status != 404 || api.Code != "EXPORT_OPERATION_NOT_FOUND" || (!retry && !fresh) || o.Job != "" || o.Cancel != "" {
		return v, err
	}
	if err = c.dataExportCapability(b, current, true); err != nil {
		return v, err
	}
	return c.dataExportRead(s, b, current, o, "POST", "/api/me/data-exports", json.RawMessage(o.Body))
}
func (c *client) createDataExport() (dataExportIntent, dataExportView, error) {
	s, b, current, err := c.dataExportContext()
	if err != nil {
		return dataExportIntent{}, dataExportView{}, err
	}
	if err = c.dataExportCapability(b, current, true); err != nil {
		return dataExportIntent{}, dataExportView{}, err
	}
	o, err := s.changeDataExport(b, current, "", func(*dataExportIntent) error { return nil })
	if err != nil {
		return o, dataExportView{}, err
	}
	v, err := c.performDataExport(o.ID, false, true)
	return o, v, err
}
func (c *client) statusDataExport(id string) (dataExportView, error) {
	s, b, current, err := c.dataExportContext()
	if err != nil {
		return dataExportView{}, err
	}
	o, err := loadDataExport(s.db, id)
	if err != nil {
		return dataExportView{}, err
	}
	if o.Binding != b {
		return dataExportView{}, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	if err = c.dataExportCapability(b, current, false); err != nil {
		return dataExportView{}, err
	}
	if o.Job == "" {
		return c.performDataExport(id, false, false)
	}
	return c.dataExportRead(s, b, current, o, "GET", "/api/me/data-exports/"+o.Job, nil)
}
func (c *client) cancelDataExport(id string) (dataExportView, error) {
	v, err := c.statusDataExport(id)
	if err != nil {
		return v, err
	}
	s, b, current, err := c.dataExportContext()
	if err != nil {
		return v, err
	}
	o, err := s.changeDataExport(b, current, id, func(row *dataExportIntent) error {
		if row.Job != v.Job.ID {
			return syncRefuse("DATA_EXPORT_INTENT_CHANGED")
		}
		if row.Cancel == "" {
			nonce, e := syncNewUUID()
			if e != nil {
				return e
			}
			body, _ := json.Marshal(map[string]string{"requestId": nonce})
			row.Cancel = string(body)
			row.CancelHash = fingerprint(row.Cancel)
		}
		return nil
	})
	if err != nil {
		return v, err
	}
	if oneOf(v.Job.State, "cancelled", "expired", "failed") && v.Job.Cleanup == "cleared" {
		return v, nil
	}
	return c.dataExportRead(s, b, current, o, "POST", "/api/me/data-exports/"+o.Job+"/cancel", json.RawMessage(o.Cancel))
}
func (c *client) dataExportCommand(cmd, args string) (bool, error) {
	if !oneOf(cmd, "data-export", "data-exports", "data-export-status", "data-export-reconcile", "data-export-retry", "data-export-cancel", "data-export-download") {
		return false, nil
	}
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	s, b, current, err := c.dataExportContext()
	if err != nil {
		return true, err
	}
	if cmd == "data-exports" {
		rows, e := s.db.Query("SELECT id FROM data_exports ORDER BY rowid DESC")
		if e != nil {
			return true, e
		}
		var ids []string
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				break
			}
			ids = append(ids, id)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return true, e
		}
		items := []map[string]any{}
		for _, id := range ids {
			o, e := loadDataExport(s.db, id)
			if e != nil {
				return true, e
			}
			if o.Binding == b {
				pending := o.Cancel != "" && !(o.View != nil && oneOf(o.View.Job.State, "cancelled", "expired", "failed") && o.View.Job.Cleanup == "cleared")
				items = append(items, map[string]any{"requestId": id, "jobId": o.Job, "cancelPending": pending, "lastObserved": o.View, "restoreEpoch": b.Epoch})
			}
		}
		if !current() {
			return true, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
		}
		return true, printJSON(map[string]any{"items": items})
	}
	var v dataExportView
	if cmd == "data-export" {
		if strings.TrimSpace(args) != "" {
			return true, fmt.Errorf("data-export takes no arguments")
		}
		o, result, e := c.createDataExport()
		if e != nil {
			return true, fmt.Errorf("original export %s retained: %w", o.ID, e)
		}
		return true, printJSON(map[string]any{"requestId": o.ID, "view": result})
	}
	id := strings.TrimSpace(args)
	if cmd == "data-export-download" {
		id, destination, ok := strings.Cut(id, " ")
		if !ok || strings.TrimSpace(destination) == "" {
			return true, fmt.Errorf("data-export-download ORIGINAL_REQUEST_ID PATH")
		}
		result, e := c.downloadDataExport(id, strings.TrimSpace(destination))
		if e != nil {
			return true, e
		}
		return true, printJSON(result)
	}
	switch cmd {
	case "data-export-status":
		v, err = c.statusDataExport(id)
	case "data-export-cancel":
		v, err = c.cancelDataExport(id)
	default:
		v, err = c.performDataExport(id, cmd == "data-export-retry", false)
	}
	if err != nil {
		return true, err
	}
	return true, printJSON(v)
}
