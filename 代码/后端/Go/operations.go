package jobsim

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type BackupOptions struct{ BusinessURL, JournalURL, StateDir, TLSDir, PGBin, Output, KeyFile string }
type RestoreOptions struct {
	BackupOptions
	TargetURL, TargetStateDir, OperationID string
	Offline                                bool
}
type backupFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"sha256"`
}
type backupManifest struct {
	FormatVersion  int          `json:"formatVersion"`
	DeploymentID   string       `json:"deploymentId"`
	BackupID       string       `json:"backupId"`
	CreatedAt      time.Time    `json:"createdAt"`
	RestoreEpoch   int64        `json:"restoreEpoch"`
	JournalThrough int64        `json:"journalThrough"`
	JournalHead    string       `json:"journalHead,omitempty"`
	SnapshotLSN    string       `json:"snapshotLSN,omitempty"`
	OwnerCoverage  []string     `json:"ownerCoverage,omitempty"`
	Files          []backupFile `json:"files"`
	Limitations    []string     `json:"limitations"`
}

func operatorConnect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	c, e := pgx.ParseConfig(dsn)
	if e != nil {
		return nil, fmt.Errorf("invalid operator database configuration")
	}
	c.RuntimeParams["application_name"] = "jobsim-ops"
	c.RuntimeParams["synchronous_commit"] = "on"
	return pgx.ConnectConfig(ctx, c)
}
func pgCommand(ctx context.Context, bin, dsn, command string, args ...string) error {
	u, e := url.Parse(dsn)
	if e != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("operator runner requires a PostgreSQL URL")
	}
	env := append([]string{}, os.Environ()...)
	name := u.User.Username()
	password, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	env = append(env, "PGHOST="+u.Hostname(), "PGPORT="+port, "PGUSER="+name, "PGPASSWORD="+password, "PGDATABASE="+strings.TrimPrefix(u.Path, "/"), "PGAPPNAME=jobsim-ops")
	for key, value := range u.Query() {
		switch key {
		case "sslmode", "sslrootcert", "sslcert", "sslkey":
			env = append(env, "PG"+strings.ToUpper(key)+"="+value[0])
		}
	}
	cmd := exec.CommandContext(ctx, filepath.Join(bin, command), args...)
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stderr = &output
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("%s failed; isolated target retained; PostgreSQL detail is suppressed because it can contain private row data", command)
	}
	return nil
}
func readBackupKey(path string, create bool) ([]byte, error) {
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) && create {
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return nil, e
		}
		b = make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			return nil, e
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, e = f.Write(b)
		if e == nil {
			e = f.Sync()
		}
		f.Close()
		if e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("backup key must contain exactly 32 bytes")
	}
	info, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("backup key must not be group/world readable")
	}
	return b, nil
}

const archiveMagic = "JIMBACK1"

type chunkWriter struct {
	out    io.Writer
	gcm    cipher.AEAD
	prefix []byte
	index  uint32
	buffer []byte
}

func newChunkWriter(out io.Writer, key []byte) (*chunkWriter, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	prefix := make([]byte, 8)
	if _, e = rand.Read(prefix); e != nil {
		return nil, e
	}
	if _, e = out.Write(append([]byte(archiveMagic), prefix...)); e != nil {
		return nil, e
	}
	return &chunkWriter{out: out, gcm: g, prefix: prefix}, nil
}
func (c *chunkWriter) frame(data []byte, final bool) error {
	if c.index == ^uint32(0) {
		return fmt.Errorf("backup chunk limit")
	}
	header := make([]byte, 5)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	if final {
		header[4] = 1
	}
	nonce := make([]byte, 12)
	copy(nonce, c.prefix)
	binary.BigEndian.PutUint32(nonce[8:], c.index)
	aad := append(append([]byte(archiveMagic), header...), nonce[8:]...)
	sealed := c.gcm.Seal(nil, nonce, data, aad)
	if _, e := c.out.Write(header); e != nil {
		return e
	}
	if _, e := c.out.Write(sealed); e != nil {
		return e
	}
	c.index++
	return nil
}
func (c *chunkWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		take := 65536 - len(c.buffer)
		if take > len(p) {
			take = len(p)
		}
		c.buffer = append(c.buffer, p[:take]...)
		p = p[take:]
		if len(c.buffer) == 65536 {
			if e := c.frame(c.buffer, false); e != nil {
				return n - len(p), e
			}
			c.buffer = c.buffer[:0]
		}
	}
	return n, nil
}
func (c *chunkWriter) Close() error {
	if len(c.buffer) > 0 {
		if e := c.frame(c.buffer, false); e != nil {
			return e
		}
	}
	return c.frame(nil, true)
}
func decryptArchive(src io.Reader, dst io.Writer, key []byte) error {
	header := make([]byte, 16)
	if _, e := io.ReadFull(src, header); e != nil {
		return e
	}
	if string(header[:8]) != archiveMagic {
		return fmt.Errorf("unknown encrypted backup format")
	}
	b, _ := aes.NewCipher(key)
	g, _ := cipher.NewGCM(b)
	for index := uint32(0); index < ^uint32(0); index++ {
		frame := make([]byte, 5)
		if _, e := io.ReadFull(src, frame); e != nil {
			return fmt.Errorf("backup truncated before authenticated final frame")
		}
		size := binary.BigEndian.Uint32(frame[:4])
		if size > 65536 || frame[4] > 1 || frame[4] == 1 && size != 0 {
			return fmt.Errorf("invalid backup frame")
		}
		sealed := make([]byte, int(size)+g.Overhead())
		if _, e := io.ReadFull(src, sealed); e != nil {
			return e
		}
		nonce := make([]byte, 12)
		copy(nonce, header[8:])
		binary.BigEndian.PutUint32(nonce[8:], index)
		aad := append(append([]byte(archiveMagic), frame...), nonce[8:]...)
		plain, e := g.Open(nil, nonce, sealed, aad)
		if e != nil {
			return fmt.Errorf("backup authentication failed")
		}
		if frame[4] == 1 {
			extra := make([]byte, 1)
			n, e := src.Read(extra)
			if n != 0 || e != io.EOF {
				return fmt.Errorf("unexpected data after authenticated final frame")
			}
			return nil
		}
		if _, e = dst.Write(plain); e != nil {
			return e
		}
	}
	return fmt.Errorf("backup chunk limit")
}
func fileDigest(path string) (backupFile, error) {
	f, e := os.Open(path)
	if e != nil {
		return backupFile{}, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	return backupFile{Size: n, Digest: hex.EncodeToString(h.Sum(nil))}, e
}
func Backup(ctx context.Context, o BackupOptions) error {
	admissionLock, lockError := maintenanceAdmissionGuard(ctx, o.StateDir)
	if lockError != nil {
		return lockError
	}
	defer closeExportFence(admissionLock)
	if e := VerifyIndependentJournal(ctx, o.BusinessURL, o.JournalURL); e != nil {
		return e
	}
	if o.Output == "" || o.KeyFile == "" || o.TLSDir == "" {
		return fmt.Errorf("output, separate key file and TLS directory are required")
	}
	keyAbs, e := filepath.Abs(o.KeyFile)
	if e != nil {
		return e
	}
	outDir, e := filepath.Abs(filepath.Dir(o.Output))
	if e != nil {
		return e
	}
	if rel, e := filepath.Rel(outDir, keyAbs); e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("recovery key must be outside the backup directory")
	}
	key, e := readBackupKey(o.KeyFile, true)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(o.Output), 0700); e != nil {
		return e
	}
	if _, e = os.Stat(o.Output); !os.IsNotExist(e) {
		return fmt.Errorf("backup output already exists; never overwrite a valid set")
	}
	work, e := os.MkdirTemp(filepath.Dir(o.Output), ".jobsim-backup-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(work)
	business, e := operatorConnect(ctx, o.BusinessURL)
	if e != nil {
		return e
	}
	defer business.Close(ctx)
	journal, e := operatorConnect(ctx, o.JournalURL)
	if e != nil {
		return e
	}
	defer journal.Close(ctx)
	tx, e := business.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var snapshot, dep, snapshotLSN string
	var sourceEpoch, currentEpoch, through int64
	if e = tx.QueryRow(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); e != nil {
		return e
	}
	if e = tx.QueryRow(ctx, `SELECT deployment_id,restore_epoch FROM auth.instance WHERE singleton`).Scan(&dep, &sourceEpoch); e != nil {
		return e
	}
	if e = tx.QueryRow(ctx, `SELECT pg_current_wal_lsn()::text`).Scan(&snapshotLSN); e != nil {
		return e
	}
	owners := []string{}
	for role, schema := range schemaFor {
		if role == "journal" {
			continue
		}
		var ownerDeployment string
		var ownerEpoch int64
		if e = tx.QueryRow(ctx, fmt.Sprintf(`SELECT deployment_id,restore_epoch FROM %s.instance WHERE singleton`, schema)).Scan(&ownerDeployment, &ownerEpoch); e != nil {
			return fmt.Errorf("backup owner schema is missing: %s", role)
		}
		if ownerDeployment != dep || ownerEpoch != sourceEpoch {
			return fmt.Errorf("backup owner deployment/epoch does not match: %s", role)
		}
		owners = append(owners, role)
	}
	sort.Strings(owners)
	jtx, e := journal.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return e
	}
	defer jtx.Rollback(ctx)
	if e = jtx.QueryRow(ctx, `SELECT restore_epoch FROM journal.restore_epochs WHERE singleton`).Scan(&currentEpoch); e != nil {
		return e
	}
	if sourceEpoch != currentEpoch {
		return fmt.Errorf("backup source restore gate is stale")
	}
	if e = jtx.QueryRow(ctx, `SELECT COALESCE(max(seq),0) FROM journal.entries`).Scan(&through); e != nil {
		return e
	}
	var journalHead []byte
	if e = jtx.QueryRow(ctx, `SELECT digest FROM journal.chain_head WHERE singleton`).Scan(&journalHead); e != nil {
		return e
	}
	if e = jtx.Commit(ctx); e != nil {
		return e
	}
	dump := filepath.Join(work, "database.dump")
	if e = pgCommand(ctx, o.PGBin, o.BusinessURL, "pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--snapshot", snapshot, "--exclude-schema=jobsim_restore", "--file", dump); e != nil {
		return e
	}
	_ = os.Chmod(dump, 0600)
	sources := map[string]string{"database.dump": dump, "instance.json": filepath.Join(o.StateDir, "instance.json")}
	for _, name := range []string{"server.pem", "server.key"} {
		sources["TLS/"+name] = filepath.Join(o.TLSDir, name)
	}
	for _, name := range []string{"ca.pem", "ca.key"} {
		path := filepath.Join(o.TLSDir, name)
		if _, e = os.Stat(path); e == nil {
			sources["TLS/"+name] = path
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	rows, e := tx.Query(ctx, `SELECT storage_key,size_bytes,sha256 FROM media.objects`)
	if e != nil {
		return e
	}
	for rows.Next() {
		var name string
		var size int64
		var digest []byte
		if e = rows.Scan(&name, &size, &digest); e != nil {
			rows.Close()
			return e
		}
		if filepath.Base(name) != name {
			rows.Close()
			return fmt.Errorf("unsafe media storage key")
		}
		source := filepath.Join(o.StateDir, "Media", name)
		actual, e := fileDigest(source)
		if e != nil || actual.Size != size || actual.Digest != hex.EncodeToString(digest) {
			rows.Close()
			return fmt.Errorf("referenced media bytes missing or corrupt")
		}
		sources["Media/"+name] = source
	}
	rows.Close()
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	manifest := backupManifest{FormatVersion: 1, DeploymentID: dep, BackupID: newID(), CreatedAt: time.Now().UTC(), RestoreEpoch: sourceEpoch, JournalThrough: through, JournalHead: hex.EncodeToString(journalHead), SnapshotLSN: snapshotLSN, OwnerCoverage: owners, Files: []backupFile{}, Limitations: []string{"basic self-hosted snapshot; not cross-domain RPO0", "journal cluster and separate backup key are not rolled back or embedded", "runtime/DSN credentials and deployment receipts must be supplied by operator", "short-lived derived export bytes are excluded; restored tasks and wrapped keys are cancelled before activation"}}
	for name, source := range sources {
		f, e := fileDigest(source)
		if e != nil {
			return e
		}
		f.Name = name
		manifest.Files = append(manifest.Files, f)
	}
	out, e := os.OpenFile(o.Output+".partial", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer out.Close()
	defer os.Remove(o.Output + ".partial")
	encrypted, e := newChunkWriter(out, key)
	if e != nil {
		return e
	}
	tw := tar.NewWriter(encrypted)
	meta, _ := json.Marshal(manifest)
	if len(meta) > 1<<20 {
		return fmt.Errorf("backup manifest exceeds the supported restore limit; no new backup set is published")
	}
	if e = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(meta))}); e != nil {
		return e
	}
	if _, e = tw.Write(meta); e != nil {
		return e
	}
	for _, f := range manifest.Files {
		if e = tw.WriteHeader(&tar.Header{Name: f.Name, Mode: 0600, Size: f.Size}); e != nil {
			return e
		}
		src, e := os.Open(sources[f.Name])
		if e != nil {
			return e
		}
		h := sha256.New()
		n, e := io.Copy(tw, io.TeeReader(src, h))
		src.Close()
		if e != nil || n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.Digest {
			return fmt.Errorf("backup source changed; partial set not published")
		}
	}
	if e = tw.Close(); e != nil {
		return e
	}
	if e = encrypted.Close(); e != nil {
		return e
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	if e = os.Rename(o.Output+".partial", o.Output); e != nil {
		return e
	}
	directory, e := os.Open(filepath.Dir(o.Output))
	if e != nil {
		return e
	}
	e = directory.Sync()
	directory.Close()
	return e
}
func extractBackup(ctx context.Context, o RestoreOptions, work string) (backupManifest, error) {
	var manifest backupManifest
	key, e := readBackupKey(o.KeyFile, false)
	if e != nil {
		return manifest, e
	}
	src, e := os.Open(o.Output)
	if e != nil {
		return manifest, e
	}
	defer src.Close()
	plain, e := os.OpenFile(filepath.Join(work, "verified.tar"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return manifest, e
	}
	defer plain.Close()
	if e = decryptArchive(src, plain, key); e != nil {
		return manifest, e
	}
	if _, e = plain.Seek(0, 0); e != nil {
		return manifest, e
	}
	tr := tar.NewReader(plain)
	first, e := tr.Next()
	if e != nil || first.Name != "manifest.json" || first.Size > 1<<20 {
		return manifest, fmt.Errorf("invalid backup manifest")
	}
	if e = json.NewDecoder(io.LimitReader(tr, 1<<20)).Decode(&manifest); e != nil {
		return manifest, e
	}
	if manifest.FormatVersion != 1 || len(manifest.Files) > 1000000 {
		return manifest, fmt.Errorf("unsupported backup manifest")
	}
	expected := map[string]backupFile{}
	for _, file := range manifest.Files {
		if file.Size < 0 || file.Size > 1<<40 {
			return manifest, fmt.Errorf("backup file size limit")
		}
		allowed := file.Name == "database.dump" || file.Name == "instance.json" || file.Name == "TLS/ca.pem" || file.Name == "TLS/ca.key" || file.Name == "TLS/server.pem" || file.Name == "TLS/server.key" || strings.HasPrefix(file.Name, "Media/") && validObjectID(strings.TrimPrefix(file.Name, "Media/"))
		if !allowed {
			return manifest, fmt.Errorf("unrecognized backup asset path")
		}
		if _, duplicate := expected[file.Name]; duplicate {
			return manifest, fmt.Errorf("duplicate backup manifest entry")
		}
		digest, e := hex.DecodeString(file.Digest)
		if e != nil || len(digest) != 32 {
			return manifest, fmt.Errorf("invalid backup asset digest")
		}
		expected[file.Name] = file
	}
	for _, name := range []string{"database.dump", "instance.json", "TLS/server.pem", "TLS/server.key"} {
		if _, ok := expected[name]; !ok {
			return manifest, fmt.Errorf("required backup asset is missing")
		}
	}
	seen := map[string]bool{}
	for {
		header, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return manifest, e
		}
		f, ok := expected[header.Name]
		clean := filepath.Clean(header.Name)
		if !ok || seen[header.Name] || header.Typeflag != tar.TypeReg || clean != header.Name || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || header.Size != f.Size {
			return manifest, fmt.Errorf("unsafe or unexpected archive entry")
		}
		seen[header.Name] = true
		dest := filepath.Join(work, clean)
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return manifest, e
		}
		out, e := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return manifest, e
		}
		h := sha256.New()
		n, e := io.Copy(out, io.TeeReader(tr, h))
		syncErr := out.Sync()
		out.Close()
		if e != nil || syncErr != nil || n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.Digest {
			return manifest, fmt.Errorf("backup file integrity mismatch")
		}
	}
	if len(seen) != len(expected) {
		return manifest, fmt.Errorf("incomplete backup set")
	}
	return manifest, nil
}

// ReplaySafety only reconstructs minimum denial facts. Missing latest passwords
// become recovery_required; no old password, grant or body is guessed valid.
func ReplaySafety(ctx context.Context, business, journal *pgx.Conn) error {
	_, e := replaySafetyCovered(ctx, business, journal, backupManifest{})
	return e
}

type safetyCoverage struct {
	From                   string `json:"from"`
	Through                string `json:"through"`
	ChainHead              string `json:"chainHead"`
	SnapshotThrough        string `json:"snapshotThrough"`
	SnapshotAnchorVerified bool   `json:"snapshotAnchorVerified"`
}

func replaySafetyCovered(ctx context.Context, business, journal *pgx.Conn, baseline backupManifest) (safetyCoverage, error) {
	coverage := safetyCoverage{From: "0", Through: "0", SnapshotThrough: str(baseline.JournalThrough)}
	jtx, e := journal.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return coverage, e
	}
	defer jtx.Rollback(ctx)
	var through int64
	var expectedHead []byte
	if e = jtx.QueryRow(ctx, `SELECT COALESCE(max(seq),0) FROM journal.entries`).Scan(&through); e != nil {
		return coverage, e
	}
	if e = jtx.QueryRow(ctx, `SELECT digest FROM journal.chain_head WHERE singleton`).Scan(&expectedHead); e != nil {
		return coverage, e
	}
	if through < baseline.JournalThrough || len(expectedHead) != 32 {
		return coverage, fmt.Errorf("journal cannot cover the backup snapshot watermark")
	}
	anchor, e := hex.DecodeString(baseline.JournalHead)
	if e != nil || baseline.JournalHead != "" && len(anchor) != 32 {
		return coverage, fmt.Errorf("backup journal anchor is invalid")
	}
	if baseline.JournalThrough == 0 && len(anchor) > 0 {
		if !bytes.Equal(anchor, make([]byte, 32)) {
			return coverage, fmt.Errorf("empty journal anchor differs")
		}
		coverage.SnapshotAnchorVerified = true
	}
	rows, e := jtx.Query(ctx, `SELECT seq,owner,operation_key,phase,kind,payload,metadata_digest,previous_digest,entry_digest FROM journal.entries ORDER BY seq`)
	if e != nil {
		return coverage, e
	}
	defer rows.Close()
	previous := make([]byte, 32)
	for rows.Next() {
		var q journalRecord
		var raw, digest, prev, chain []byte
		var sequence int64
		if e = rows.Scan(&sequence, &q.Owner, &q.OperationKey, &q.Phase, &q.Kind, &raw, &digest, &prev, &chain); e != nil {
			return coverage, e
		}
		if e = json.Unmarshal(raw, &q.Payload); e != nil {
			return coverage, e
		}
		encoded, _ := json.Marshal(q)
		actual := tokenDigest(string(encoded))
		expected := tokenDigest(string(append(append([]byte{}, previous...), actual...)))
		if !bytes.Equal(actual, digest) || !bytes.Equal(previous, prev) || !bytes.Equal(expected, chain) {
			return coverage, fmt.Errorf("journal chain integrity mismatch")
		}
		if coverage.From == "0" {
			coverage.From = str(sequence)
		}
		coverage.Through = str(sequence)
		if sequence == baseline.JournalThrough && len(anchor) > 0 {
			if !bytes.Equal(anchor, chain) {
				return coverage, fmt.Errorf("journal differs from the backup snapshot anchor")
			}
			coverage.SnapshotAnchorVerified = true
		}
		previous = chain
		if q.Phase != "intent" && !(q.Kind == "auth.account_state" && q.Payload["state"] == "active") {
			continue
		}
		if q.Kind == "auth.account_state" && q.Payload["state"] == "active" && q.Phase != "result" {
			continue
		}
		text := func(name string) string { v, _ := q.Payload[name].(string); return v }
		switch q.Kind {
		case "auth.reserve_username":
			_, e = business.Exec(ctx, `INSERT INTO auth.identity_reservations(username,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, text("username"), text("userId"))
		case "auth.invalidate_user":
			var generation int64
			generation, e = strconv.ParseInt(text("generation"), 10, 64)
			if e == nil {
				_, e = business.Exec(ctx, `UPDATE auth.users SET recovery_state=CASE WHEN credential_generation<$2 THEN 'recovery_required' ELSE recovery_state END,credential_generation=GREATEST(credential_generation,$2),security_version=GREATEST(security_version,$3::bigint) WHERE id=$1`, text("userId"), generation, legacySecurity(text("securityVersion")))
			}
		case "auth.account_state":
			_, e = business.Exec(ctx, `UPDATE auth.users SET state=$2,state_before_tombstone=NULLIF($3,''),ban_until=NULLIF($4,'')::timestamptz,security_version=GREATEST(security_version,$5::bigint),row_version=GREATEST(row_version,$6::bigint),pending_operation_id=NULL WHERE id=$1 AND security_version<=$5::bigint`, text("userId"), text("state"), text("stateBefore"), text("banUntil"), text("securityVersion"), text("version"))
		case "auth.mfa_step":
			step, parseErr := strconv.ParseInt(text("step"), 10, 64)
			if parseErr != nil {
				return coverage, parseErr
			}
			_, e = business.Exec(ctx, `UPDATE auth.mfa_factors SET last_accepted_step=GREATEST(last_accepted_step,$2) WHERE id=$1 AND user_id=$3`, text("factorId"), step, text("userId"))
		case "auth.revoke_device":
			_, e = business.Exec(ctx, `UPDATE auth.sessions SET revoked_at=now() WHERE user_id=$1 AND device_id=$2`, text("userId"), text("deviceId"))
		case "auth.retire_authentication":
			_, e = business.Exec(ctx, `INSERT INTO auth.authentication_retirements(user_id,request_id,retired_by_request_id) SELECT id,$2,$3 FROM auth.users WHERE id=$1 ON CONFLICT DO NOTHING`, text("userId"), text("requestKey"), text("replacementKey"))
		case "auth.recovery_batch":
			var generation int64
			generation, e = strconv.ParseInt(text("generation"), 10, 64)
			if e == nil {
				_, e = business.Exec(ctx, `UPDATE auth.recovery_code_batches SET active=false WHERE user_id=$1 AND (id<>$2 OR generation<>$3)`, text("userId"), text("batchId"), generation)
				if e == nil {
					_, e = business.Exec(ctx, `UPDATE auth.users SET recovery_generation=GREATEST(recovery_generation,$2) WHERE id=$1`, text("userId"), generation)
				}
			}
		case "directory.block":
			_, e = business.Exec(ctx, `INSERT INTO directory.blocks(owner_id,peer_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, text("userId"), text("peerId"))
			if e == nil {
				_, e = business.Exec(ctx, `UPDATE directory.contacts SET state='removed',request_gate='denied',request_generation=request_generation+1 WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1)`, text("userId"), text("peerId"))
			}
		case "directory.unblock":
			_, e = business.Exec(ctx, `DELETE FROM directory.blocks WHERE owner_id=$1 AND peer_id=$2`, text("userId"), text("peerId"))
		case "conversation.leave":
			generation := text("membershipGeneration")
			if generation == "" {
				generation = "9223372036854775807"
			}
			_, e = business.Exec(ctx, `UPDATE conversation.members SET state='left',export_pending_operation_id=NULL WHERE conversation_id=$1 AND user_id=$2 AND membership_generation<=$3::bigint`, text("conversationId"), text("userId"), generation)
			if e == nil {
				_, e = business.Exec(ctx, `UPDATE conversation.membership_periods SET left_at=COALESCE(left_at,now()) WHERE conversation_id=$1 AND user_id=$2 AND generation<=$3::bigint`, text("conversationId"), text("userId"), generation)
			}
		case "conversation.owner_unavailable":
			_, e = business.Exec(ctx, `UPDATE conversation.conversations SET state='owner_unavailable',version=version+1 WHERE id=$1 AND owner_id=$2 AND kind='group' AND state='active'`, text("conversationId"), text("userId"))
		case "message.retract":
			_, e = business.Exec(ctx, `UPDATE message.messages SET deleted=true,text='',content_version=content_version+1 WHERE id=$1 AND NOT deleted`, text("messageId"))
		case "message.content_floor":
			_, e = business.Exec(ctx, `UPDATE message.messages SET text='',content_unavailable=true,content_version=content_version+1,version=$2 WHERE id=$1 AND version<$2::bigint`, text("messageId"), text("version"))
		default:
			handled, replayError := ReplayPersonalExportSafety(ctx, business, q)
			if replayError == nil && !handled {
				handled, replayError = ReplayDraftSafety(ctx, business, q)
			}
			if replayError == nil && !handled {
				handled, replayError = ReplayExportSafety(ctx, business, q)
			}
			if replayError != nil {
				return coverage, replayError
			}
			if !handled {
				return coverage, fmt.Errorf("unknown protected fact; restore remains closed")
			}
		}
		if e != nil {
			return coverage, e
		}
	}
	if e = rows.Err(); e != nil {
		return coverage, e
	}
	if coverage.Through != str(through) || !bytes.Equal(previous, expectedHead) {
		return coverage, fmt.Errorf("journal tail is missing or differs from its durable head")
	}
	if len(anchor) > 0 && !coverage.SnapshotAnchorVerified {
		return coverage, fmt.Errorf("backup snapshot journal watermark is missing")
	}
	coverage.ChainHead = hex.EncodeToString(previous)
	if e = jtx.Commit(ctx); e != nil {
		return coverage, e
	}
	return coverage, nil
}

func legacySecurity(value string) string {
	if value == "" {
		return "1"
	}
	return value
}
