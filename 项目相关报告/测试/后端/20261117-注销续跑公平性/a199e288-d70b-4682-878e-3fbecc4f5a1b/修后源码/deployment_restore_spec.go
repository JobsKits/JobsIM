package jobsim

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var deploymentPhysicalDomains = []string{"identity", "directory", "conversation", "message", "media", "control", "sync", "journal"}

type DeploymentRestoreSpec struct {
	Version     int               `json:"version"`
	Operation   string            `json:"operationId"`
	Backup      string            `json:"backup"`
	BackupSHA   string            `json:"backupSha256"`
	RecoveryKey string            `json:"recoveryKeyFile"`
	SourceState string            `json:"sourceState"`
	TargetState string            `json:"targetState"`
	Repository  string            `json:"repository"`
	PGBin       string            `json:"postgresBin"`
	OpenSSL     string            `json:"openssl"`
	Sources     map[string]string `json:"sourceDatabaseUrls"`
	Targets     map[string]string `json:"targetDatabaseUrls,omitempty"`
	MainPort    int               `json:"mainPort"`
	JournalPort int               `json:"journalPort"`
	HTTPSPort   int               `json:"httpsPort"`
	RolePort    int               `json:"internalBasePort"`
	NATSPort    int               `json:"natsPort"`
	PublicHost  string            `json:"publicHost"`
}

func loadDeploymentRestoreSpec(path string) (DeploymentRestoreSpec, error) {
	var s DeploymentRestoreSpec
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return s, fmt.Errorf("restore spec must be a private regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&s) != nil || decoder.Decode(new(any)) != io.EOF {
		return s, fmt.Errorf("one complete private restore spec required")
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return s, err
	}
	resolve := func(value string) string {
		if !filepath.IsAbs(value) {
			value = filepath.Join(base, value)
		}
		return filepath.Clean(value)
	}
	s.Backup, s.RecoveryKey, s.SourceState, s.TargetState, s.Repository, s.PGBin = resolve(s.Backup), resolve(s.RecoveryKey), resolve(s.SourceState), resolve(s.TargetState), resolve(s.Repository), resolve(s.PGBin)
	for _, field := range []*string{&s.SourceState, &s.Repository} {
		canonical, e := filepath.EvalSymlinks(*field)
		if e != nil {
			return s, fmt.Errorf("existing source state and repository are required")
		}
		*field = canonical
	}
	if info, e := os.Lstat(s.TargetState); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return s, fmt.Errorf("restore target cannot be a symbolic link")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(s.TargetState))
	if e != nil {
		return s, fmt.Errorf("target parent must already exist")
	}
	s.TargetState = filepath.Join(parent, filepath.Base(s.TargetState))
	if s.OpenSSL == "" {
		s.OpenSSL = "openssl"
	} else if strings.ContainsAny(s.OpenSSL, "/\\") {
		s.OpenSSL = resolve(s.OpenSSL)
	}
	if s.Version != 1 || !validReceiptNonce(s.Operation) || !syncValidationDigest(s.BackupSHA) || s.TargetState == s.SourceState || s.TargetState == s.Repository || strings.HasPrefix(s.TargetState, s.Repository+string(os.PathSeparator)) || strings.ContainsAny(s.TargetState, "'\r\n") {
		return s, fmt.Errorf("unsupported or unsafe isolated restore identity")
	}
	if rel, _ := filepath.Rel(s.TargetState, s.RecoveryKey); rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return s, fmt.Errorf("recovery key must remain outside target runtime state")
	}
	ports := map[int]bool{}
	for _, port := range []int{s.MainPort, s.JournalPort, s.HTTPSPort, s.NATSPort} {
		if port < 1025 || port > 65535 || ports[port] {
			return s, fmt.Errorf("distinct unprivileged target ports required")
		}
		ports[port] = true
	}
	for i := 0; i < 9; i++ {
		port := s.RolePort + i
		if port < 1025 || port > 65535 || ports[port] {
			return s, fmt.Errorf("internal target ports overlap or overflow")
		}
		ports[port] = true
	}
	if s.PublicHost == "" {
		s.PublicHost = "127.0.0.1"
	}
	if strings.ContainsAny(s.PublicHost, "/:'\r\n ") || s.PublicHost == "" {
		return s, fmt.Errorf("plain IPv4 or DNS target host required")
	}
	allowedTargets := map[string]bool{}
	for _, role := range deploymentPhysicalDomains {
		allowedTargets[role] = true
	}
	for role := range s.Targets {
		if !allowedTargets[role] {
			return s, fmt.Errorf("unknown target domain in restore spec")
		}
	}
	for _, role := range deploymentPhysicalDomains {
		address := s.Sources[role]
		u, e := url.Parse(address)
		if e != nil || u.User == nil || (u.Scheme != "postgresql" && u.Scheme != "postgres") || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || u.Port() == "" || u.Path == "" || u.Path == "/postgres" {
			return s, fmt.Errorf("each source requires an explicit private loopback domain URL")
		}
		actual, e := pgx.ParseConfig(address)
		if e != nil || net.ParseIP(actual.Host) == nil || !net.ParseIP(actual.Host).IsLoopback() {
			return s, fmt.Errorf("source connection cannot override its loopback host")
		}
		for _, fallback := range actual.Fallbacks {
			if net.ParseIP(fallback.Host) == nil || !net.ParseIP(fallback.Host).IsLoopback() {
				return s, fmt.Errorf("source fallback cannot leave loopback")
			}
		}
		if ports[int(actual.Port)] {
			return s, fmt.Errorf("target ports must not control the source cluster")
		}
	}
	if len(s.Sources) != len(deploymentPhysicalDomains) {
		return s, fmt.Errorf("exactly eight physical source domain URLs required; governance shares identity")
	}
	return s, nil
}
func mustPort(value string) int { n, _ := strconv.Atoi(value); return n }
func deploymentTool(ctx context.Context, path string, args []string, environment []string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	if environment != nil {
		cmd.Env = environment
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return fmt.Errorf("restore tool %s failed; private target retained", filepath.Base(path))
	}
	return nil
}
func extractDeploymentCMS(ctx context.Context, s DeploymentRestoreSpec, work string) (string, error) {
	original, err := fileDigest(s.Backup)
	if err != nil || original.Digest != s.BackupSHA {
		return "", fmt.Errorf("encrypted backup digest differs")
	}
	if err = deploymentRequireSpace(ctx, work, original.Size); err != nil {
		return "", err
	}
	verified := filepath.Join(work, "verified.tar.gz")
	if err = deploymentTool(ctx, s.OpenSSL, []string{"cms", "-decrypt", "-binary", "-inform", "DER", "-in", s.Backup, "-inkey", s.RecoveryKey, "-out", verified}, nil); err != nil {
		return "", err
	}
	file, err := os.Open(verified)
	if err != nil {
		return "", err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("authenticated deployment archive is not gzip")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	root := filepath.Join(work, "verified")
	if err = os.Mkdir(root, 0700); err != nil {
		return "", err
	}
	count := 0
	seen := map[string]bool{}
	for {
		header, e := reader.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", fmt.Errorf("deployment archive is truncated")
		}
		count++
		if count > 1000000 {
			return "", fmt.Errorf("deployment archive exceeds entry count bound")
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00\r\n") {
			return "", fmt.Errorf("unsafe deployment archive path")
		}
		metadata := strings.HasPrefix(filepath.Base(name), "._")
		classified := name
		if metadata {
			classified = filepath.ToSlash(filepath.Join(filepath.Dir(name), strings.TrimPrefix(filepath.Base(name), "._")))
		}
		top := strings.Split(classified, "/")[0]
		allowed := top == "owner" || top == "Application" || top == "TLS" || top == "Secrets" || top == "broker-manifest.txt"
		for _, role := range deploymentPhysicalDomains {
			allowed = allowed || classified == role+".dump" || classified == role+".list"
		}
		if !allowed {
			return "", fmt.Errorf("unexpected deployment archive component")
		}
		if metadata {
			if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > 1<<20 {
				return "", fmt.Errorf("invalid optional AppleDouble metadata")
			}
			if _, e = io.CopyN(io.Discard, reader, header.Size); e != nil {
				return "", e
			}
			continue
		}
		if seen[name] && header.Typeflag != tar.TypeDir {
			return "", fmt.Errorf("duplicate deployment archive entry")
		}
		seen[name] = true
		if count > 1000000 || header.Size < 0 || header.Size > 16<<30 {
			return "", fmt.Errorf("deployment archive exceeds resource bounds")
		}
		dest := filepath.Join(root, filepath.FromSlash(name))
		if err = deploymentRequireSpace(ctx, work, header.Size); err != nil {
			return "", err
		}
		if header.Typeflag == tar.TypeDir {
			if e = os.MkdirAll(dest, 0700); e != nil {
				return "", e
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return "", fmt.Errorf("deployment archive links or special files are forbidden")
		}
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return "", e
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return "", e
		}
		written, e := io.CopyN(out, reader, header.Size)
		if e == nil {
			e = out.Sync()
		}
		closeErr := out.Close()
		if e != nil || closeErr != nil || written != header.Size {
			return "", fmt.Errorf("deployment archive private file incomplete")
		}
	}
	if _, err = io.Copy(io.Discard, gzipReader); err != nil {
		return "", fmt.Errorf("deployment archive checksum differs")
	}
	for _, role := range deploymentPhysicalDomains {
		if err = deploymentTool(ctx, filepath.Join(s.PGBin, "pg_restore"), []string{"--list", filepath.Join(root, role+".dump")}, nil); err != nil {
			return "", err
		}
	}
	var instance struct {
		Deployment string `json:"deploymentId"`
		Key        string `json:"key"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "Application", "instance.json"))
	if err != nil || json.Unmarshal(raw, &instance) != nil || !syncValidationID(instance.Deployment) || instance.Key == "" {
		return "", fmt.Errorf("archive has no original deployment identity")
	}
	broker, err := os.ReadFile(filepath.Join(root, "broker-manifest.txt"))
	if err != nil || !strings.Contains(string(broker), "component=NATS JetStream") || !strings.Contains(string(broker), "state=stopped-before-archive") {
		return "", fmt.Errorf("complete stopped broker scope is required; unknown archives remain closed")
	}
	for _, name := range []string{"operator", "identity", "directory", "conversation", "message", "media", "control", "sync", "journal"} {
		if _, err = deploymentPassword(root, name); err != nil {
			return "", err
		}
	}
	return root, nil
}
func deploymentPassword(root, role string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "Secrets", role+".password"))
	value := strings.TrimSpace(string(raw))
	if err != nil || len(value) != 64 {
		return "", fmt.Errorf("original private database role material is incomplete")
	}
	if _, err = hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("original database role material is not canonical")
	}
	return value, nil
}
func deploymentURL(port int, user, password, database string) string {
	return (&url.URL{Scheme: "postgresql", User: url.UserPassword(user, password), Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Path: "/" + database, RawQuery: "sslmode=disable"}).String()
}
func deploymentBinding(s DeploymentRestoreSpec) string {
	raw, _ := json.Marshal(s)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
func deploymentAvailablePorts(s DeploymentRestoreSpec) error {
	ports := []int{s.MainPort, s.JournalPort, s.HTTPSPort, s.NATSPort}
	for i := 0; i < 9; i++ {
		ports = append(ports, s.RolePort+i)
	}
	for _, port := range ports {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 150*time.Millisecond)
		if err != nil {
			continue
		}
		connection.Close()
		label := ""
		if port == s.MainPort {
			label = "Main"
		}
		if port == s.JournalPort {
			label = "Journal"
		}
		if label != "" {
			var checkpoint deploymentRestoreCheckpoint
			raw, e := os.ReadFile(filepath.Join(s.TargetState, "deployment-restore-operation.json"))
			if e == nil && json.Unmarshal(raw, &checkpoint) == nil && checkpoint.Binding == deploymentBinding(s) {
				data := filepath.Join(s.TargetState, label+"Database")
				identity, e := deploymentClusterIdentity(context.Background(), s.PGBin, data)
				receipt, re := os.ReadFile(filepath.Join(s.TargetState, "Processes", label+".cluster"))
				if e == nil && re == nil && strings.TrimSpace(string(receipt)) == identity {
					status := exec.Command(filepath.Join(s.PGBin, "pg_ctl"), "-D", data, "status")
					status.Stdout, status.Stderr = io.Discard, io.Discard
					if status.Run() == nil && deploymentClusterPort(data, port) {
						continue
					}
				}
			}
		}
		return fmt.Errorf("a target port is occupied without its exact owned restore cluster; no process was stopped")
	}
	return nil
}

func deploymentClusterPort(data string, port int) bool {
	raw, err := os.ReadFile(filepath.Join(data, "postmaster.pid"))
	if err != nil {
		return false
	}
	lines := strings.Split(string(raw), "\n")
	return len(lines) > 3 && strings.TrimSpace(lines[3]) == strconv.Itoa(port)
}

func deploymentTLSHostArgs(host, certificate string) []string {
	if net.ParseIP(host) != nil {
		return []string{"-verify_ip", host, certificate}
	}
	return []string{"-verify_hostname", host, certificate}
}
