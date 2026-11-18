// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var dataExportTicket = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

type dataExportDownload struct {
	Path    string `json:"path"`
	Size    string `json:"sizeBytes"`
	SHA     string `json:"sha256"`
	Request string `json:"requestId"`
	Job     string `json:"jobId"`
	Format  string `json:"format"`
}
type dataExportGuardedReader struct {
	source  io.Reader
	current func() bool
}

func (r dataExportGuardedReader) Read(p []byte) (int, error) {
	if !r.current() {
		return 0, syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	return r.source.Read(p)
}
func dataExportReady(v dataExportView) bool {
	expires, err := time.Parse(time.RFC3339Nano, v.Job.Expires)
	return err == nil && expires.After(time.Now()) && v.Job.State == "ready" && v.Job.Cleanup == "published"
}
func validateDataExportZIP(file *os.File, size int64) error {
	if size < 22 || size > 1<<30 {
		return syncRefuse("DATA_EXPORT_ZIP_INVALID")
	}
	tail := make([]byte, min(size, 65557))
	if _, err := file.ReadAt(tail, size-int64(len(tail))); err != nil {
		return syncRefuse("DATA_EXPORT_ZIP_INVALID")
	}
	end := bytes.LastIndex(tail, []byte{'P', 'K', 5, 6})
	if end < 0 || end+22 > len(tail) || end+22+int(binary.LittleEndian.Uint16(tail[end+20:end+22])) != len(tail) {
		return syncRefuse("DATA_EXPORT_ZIP_INVALID")
	}
	z, err := zip.NewReader(file, size)
	if err != nil || len(z.File) > 40005 {
		return syncRefuse("DATA_EXPORT_ZIP_INVALID")
	}
	var expanded uint64
	seen := map[string]bool{}
	for _, entry := range z.File {
		name := entry.Name
		if name == "" || seen[name] || strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") || strings.Contains(name, "../") || entry.Mode()&os.ModeSymlink != 0 {
			return syncRefuse("DATA_EXPORT_ZIP_INVALID")
		}
		for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
			if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
				return syncRefuse("DATA_EXPORT_ZIP_INVALID")
			}
		}
		seen[name] = true
		if entry.UncompressedSize64 > 1<<30 || expanded > 1<<30-entry.UncompressedSize64 {
			return syncRefuse("DATA_EXPORT_ZIP_CAPACITY")
		}
		expanded += entry.UncompressedSize64
		source, e := entry.Open()
		if e != nil {
			return syncRefuse("DATA_EXPORT_ZIP_INVALID")
		}
		n, e := io.Copy(io.Discard, io.LimitReader(source, int64(entry.UncompressedSize64)+1))
		source.Close()
		if e != nil || uint64(n) != entry.UncompressedSize64 {
			return syncRefuse("DATA_EXPORT_ZIP_INVALID")
		}
	}
	if !seen["manifest.json"] || !seen["account-export.json"] {
		return syncRefuse("DATA_EXPORT_ZIP_INVALID")
	}
	return nil
}
func dataExportDestination(destination string) (string, error) {
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", syncRefuse("DATA_EXPORT_DESTINATION_INVALID")
	}
	path := filepath.Join(parent, filepath.Base(absolute))
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		return "", syncRefuse("DATA_EXPORT_DESTINATION_EXISTS")
	}
	return path, nil
}
func (c *client) publishDataExport(s *localStore, b dataExportBinding, o dataExportIntent, temp, destination string, info os.FileInfo) error {
	// No HTTP under this lock: it only serializes local publication with logout
	// and family replacement; refresh within the same family remains compatible.
	c.auth.mu.Lock()
	defer c.auth.mu.Unlock()
	current := func() bool {
		return c.syncCurrent(s, nil) && c.auth.state.Status == "active" && c.auth.pair.FamilyID == b.Family && c.auth.pair.DeviceID == b.Identity.DeviceID && c.auth.pair.User.ID == b.Identity.UserID
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.dataExportControl(tx, b, current); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE data_exports SET payload_hash=payload_hash WHERE id=?", o.ID); err != nil {
		return err
	}
	row, err := loadDataExport(tx, o.ID)
	if err != nil {
		return err
	}
	if row.Binding != b || row.Body != o.Body || row.Hash != o.Hash || row.Job != o.Job || row.Cancel != "" {
		return syncRefuse("DATA_EXPORT_CONTEXT_CHANGED")
	}
	latest, err := os.Lstat(temp)
	if err != nil || !latest.Mode().IsRegular() || !os.SameFile(info, latest) {
		return syncRefuse("DATA_EXPORT_TEMP_CHANGED")
	}
	// Hard-link publication is atomic and refuses an existing destination on
	// every supported filesystem. Unsupported filesystems fail without rename.
	if err = os.Link(temp, destination); err != nil {
		return err
	}
	if err = receiptSyncDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	return tx.Commit()
}
func (c *client) downloadDataExport(id, destination string) (dataExportDownload, error) {
	var result dataExportDownload
	s, b, current, err := c.dataExportContext()
	if err != nil {
		return result, err
	}
	o, err := loadDataExport(s.db, id)
	if err != nil {
		return result, err
	}
	if o.Binding != b || o.Job == "" || o.Cancel != "" {
		return result, syncRefuse("DATA_EXPORT_UNAVAILABLE")
	}
	if err = c.dataExportCapability(b, current, true); err != nil {
		return result, err
	}
	live, err := c.statusDataExport(id)
	if err != nil {
		return result, err
	}
	if !dataExportReady(live) {
		return result, syncRefuse("DATA_EXPORT_UNAVAILABLE")
	}
	path, err := dataExportDestination(destination)
	if err != nil {
		return result, err
	}
	var ticket struct {
		Token   string `json:"downloadToken"`
		Expires string `json:"expiresAt"`
		Format  string `json:"format"`
	}
	route := "/api/me/data-exports/" + o.Job
	if err = c.call("POST", route+"/download-ticket", map[string]any{}, &ticket); err != nil {
		return result, err
	}
	deadline, e := time.Parse(time.RFC3339Nano, ticket.Expires)
	jobDeadline, _ := time.Parse(time.RFC3339Nano, live.Job.Expires)
	if e != nil || !deadline.After(time.Now()) || deadline.After(time.Now().Add(65*time.Second)) || deadline.After(jobDeadline) || ticket.Format != "zip" || !dataExportTicket.MatchString(ticket.Token) || !current() {
		return result, syncRefuse("DATA_EXPORT_TICKET_INVALID")
	}
	response, err := c.authenticatedDownloadResponse("GET", route+"/download", "", nil, 120*time.Second, ticket.Token)
	ticket.Token = ""
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	media, _, e := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != 200 || e != nil || media != "application/zip" || !strings.Contains(strings.ToLower(response.Header.Get("Cache-Control")), "no-store") {
		return result, syncRefuse("DATA_EXPORT_STREAM_REJECTED")
	}
	if response.ContentLength > 1<<30 {
		return result, syncRefuse("DATA_EXPORT_ZIP_CAPACITY")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".jobsim-personal-"+o.ID+"-*.part")
	if err != nil {
		return result, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return result, err
	}
	temp := file.Name()
	defer func() {
		file.Close()
		if latest, e := os.Lstat(temp); e == nil && os.SameFile(info, latest) {
			_ = os.Remove(temp)
			_ = receiptSyncDirectory(filepath.Dir(temp))
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(dataExportGuardedReader{response.Body, current}, (1<<30)+1))
	if err != nil {
		return result, err
	}
	if n > 1<<30 || response.ContentLength >= 0 && response.ContentLength != n {
		return result, syncRefuse("DATA_EXPORT_STREAM_TRUNCATED")
	}
	if err = validateDataExportZIP(file, n); err != nil {
		return result, err
	}
	if err = file.Sync(); err != nil {
		return result, err
	}
	if err = receiptSyncDirectory(filepath.Dir(temp)); err != nil {
		return result, err
	}
	if err = c.dataExportCapability(b, current, true); err != nil {
		return result, err
	}
	fresh, err := c.statusDataExport(id)
	if err != nil {
		return result, err
	}
	if !dataExportReady(fresh) || fresh.Job.Artifact != live.Job.Artifact || fresh.Job.Build != live.Job.Build {
		return result, syncRefuse("DATA_EXPORT_SOURCE_CHANGED")
	}
	if err = c.publishDataExport(s, b, o, temp, path, info); err != nil {
		return result, err
	}
	return dataExportDownload{Path: path, Size: strconv.FormatInt(n, 10), SHA: hex.EncodeToString(hash.Sum(nil)), Request: o.ID, Job: o.Job, Format: "zip"}, nil
}
