// host.go
// JobsIM Native Demo Host
// Created by Jobs on 2026年11月18日，星期三.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const gatewayPort = 19543
const controllerPort = 19544

type publicDescriptor struct {
	ControllerURL    string `json:"controllerURL"`
	CASHA256         string `json:"caSha256"`
	CACertificatePEM string `json:"caCertificatePEM"`
	Demo             bool   `json:"demo"`
}

type demoHost struct {
	project, root, control, instance, ip string
	descriptor                           publicDescriptor
}

func regular(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("需要普通本机文件：%s", path)
	}
	return nil
}

func privateDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("拒绝非本机私有目录：%s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(path, 0700)
}

func writeNew(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func localPrivateIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	sort.SliceStable(interfaces, func(i, j int) bool { return interfaces[i].Name == "en0" && interfaces[j].Name != "en0" })
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && ip.IsPrivate() {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("没有本机私有LAN IPv4；请接入与iPhone相同的可信局域网")
}

func resolveHost() (*demoHost, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	project := filepath.Dir(executable)
	for {
		if _, err := os.Stat(filepath.Join(project, "代码", "后端", "Go", "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(project)
		if next == project {
			return nil, fmt.Errorf("成品须保留在JobsIM完整相对源码交付目录中")
		}
		project = next
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, "Library", "Application Support", "JobsIM", "Demo", "NativeDemo-v1")
	host := &demoHost{project: project, root: root, control: filepath.Join(root, "Control"), instance: filepath.Join(root, "Instance")}
	path := filepath.Join(host.control, "demo-host.json")
	if regular(path) == nil {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &host.descriptor); err != nil {
			return nil, err
		}
		host.ip = strings.TrimSuffix(strings.TrimPrefix(host.descriptor.ControllerURL, "https://"), fmt.Sprintf(":%d", controllerPort))
	}
	return host, nil
}

func (h *demoHost) script() string {
	return filepath.Join(h.project, "部署", "MacOS", "Deployment", "【MacOS】部署JobsIM.command", "【MacOS】部署JobsIM.command")
}

func (h *demoHost) env() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	tools := filepath.Join(home, "Library", "Application Support", "JobsIM", "Toolchains")
	goCandidates, _ := filepath.Glob(filepath.Join(tools, "go1.27.*", "go", "bin", "go"))
	nodeCandidates, _ := filepath.Glob(filepath.Join(tools, "node-v24*-darwin-*", "bin", "node"))
	if len(goCandidates) == 0 || len(nodeCandidates) == 0 {
		return nil, fmt.Errorf("请先用正式部署入口准备已有Go1.27/Node24工具；demo不联网下载")
	}
	sort.Strings(goCandidates)
	sort.Strings(nodeCandidates)
	goTool, nodeTool := goCandidates[len(goCandidates)-1], nodeCandidates[len(nodeCandidates)-1]
	values := map[string]string{"JOBS_IM_DEMO_ADAPTER": "1", "JOBS_IM_DEPLOY_STATE": h.instance, "JOBS_IM_DEMO_BIND_IP": h.ip, "JOBS_IM_PG_PORT": "19545", "JOBS_IM_JOURNAL_PORT": "19546", "JOBS_IM_HTTPS_PORT": "19543", "JOBS_IM_INTERNAL_PORT": "19560", "JOBS_IM_NATS_PORT": "19547", "JOBS_IM_NATS_MAX_FILE_BYTES": "536870912", "JOBS_IM_NATS_MAX_MEMORY_BYTES": "33554432", "JOBS_IM_PUBLIC_HOST": h.ip, "JOBS_IM_PUBLIC_URL": fmt.Sprintf("https://%s:%d", h.ip, gatewayPort), "JOBS_IM_TLS_CERT": filepath.Join(h.control, "server.crt"), "JOBS_IM_TLS_KEY": filepath.Join(h.control, "server.key"), "JOBS_IM_TLS_CA": filepath.Join(h.control, "ca.crt"), "JOBS_IM_GO": goTool, "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOMAXPROCS": "2", "PATH": filepath.Dir(nodeTool) + string(os.PathListSeparator) + os.Getenv("PATH")}
	// Do not inherit a user's default-instance ports, role URLs, private admin
	// credentials or connection strings into this isolated fixed profile.
	environment := []string{}
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if strings.HasPrefix(key, "JOBS_IM_") || values[key] != "" {
			continue
		}
		environment = append(environment, entry)
	}
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment, nil
}

func (h *demoHost) prepare() error {
	ip, err := localPrivateIP()
	if err != nil {
		return err
	}
	if h.ip != "" && h.ip != ip {
		return fmt.Errorf("demo LAN地址已变化；现有CA/资料保留，须在Mac明确重新配对，不能静默改应用信任")
	}
	h.ip = ip
	home, _ := os.UserHomeDir()
	for _, path := range []string{filepath.Join(home, "Library", "Application Support", "JobsIM"), filepath.Join(home, "Library", "Application Support", "JobsIM", "Demo"), h.root, h.control, filepath.Join(h.control, "Logs")} {
		if err = privateDirectory(path); err != nil {
			return err
		}
	}
	if err = h.prepareTLS(); err != nil {
		return err
	}
	environment, err := h.env()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(h.control, "Logs", "prepare.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	command := exec.Command("/bin/zsh", h.script(), "demo-prepare", "--yes", "--mode", "server", "--state", h.instance)
	command.Env = environment
	command.Stdout = log
	command.Stderr = log
	fmt.Println("准备本机demo当前Go/Web产品；不启动业务PG/NATS。")
	if err = command.Run(); err != nil {
		return fmt.Errorf("demo产品准备失败，见本机Control/Logs/prepare.log：%w", err)
	}
	return h.exportDescriptor()
}

func (h *demoHost) prepareTLS() error {
	caPath := filepath.Join(h.control, "ca.crt")
	if _, err := os.Lstat(caPath); os.IsNotExist(err) {
		for _, name := range []string{"ca.key", "server.crt", "server.key", "demo-host.json"} {
			if _, e := os.Lstat(filepath.Join(h.control, name)); e == nil {
				return fmt.Errorf("TLS材料不完整；保留并拒绝重置信任")
			}
		}
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return err
		}
		serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
		now := time.Now()
		ca := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "JobsIM Native Demo CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(3, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
		der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
		if err != nil {
			return err
		}
		ca, err = x509.ParseCertificate(der)
		if err != nil {
			return err
		}
		if err = writeNew(filepath.Join(h.control, "ca.key"), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
			return err
		}
		if err = writeNew(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
			return err
		}
		leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return err
		}
		serial, _ = rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
		leaf := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "JobsIM Native Demo"}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(0, 0, 90), IPAddresses: []net.IP{net.ParseIP(h.ip), net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"}, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
		if err != nil {
			return err
		}
		if err = writeNew(filepath.Join(h.control, "server.key"), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}), 0600); err != nil {
			return err
		}
		if err = writeNew(filepath.Join(h.control, "server.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for _, name := range []string{"ca.crt", "ca.key", "server.crt", "server.key"} {
		if err := regular(filepath.Join(h.control, name)); err != nil {
			return err
		}
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return fmt.Errorf("现有demo CA无效")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !ca.IsCA {
		return fmt.Errorf("现有demo CA无效")
	}
	leaf, err := tls.LoadX509KeyPair(filepath.Join(h.control, "server.crt"), filepath.Join(h.control, "server.key"))
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, err = cert.Verify(x509.VerifyOptions{Roots: pool, DNSName: h.ip}); err != nil {
		return fmt.Errorf("demo现有证书/地址须在Mac重新配对：%w", err)
	}
	digest := sha256.Sum256(ca.Raw)
	h.descriptor = publicDescriptor{ControllerURL: fmt.Sprintf("https://%s:%d", h.ip, controllerPort), CASHA256: hex.EncodeToString(digest[:]), CACertificatePEM: string(caPEM), Demo: true}
	raw, _ := json.MarshalIndent(h.descriptor, "", "  ")
	raw = append(raw, '\n')
	path := filepath.Join(h.control, "demo-host.json")
	if previous, e := os.ReadFile(path); e == nil {
		if string(previous) != string(raw) {
			return fmt.Errorf("公开描述与原CA不符，保留并拒绝覆盖")
		}
		return nil
	}
	return writeNew(path, raw, 0600)
}

func (h *demoHost) exportDescriptor() error {
	data, _ := json.MarshalIndent(h.descriptor, "", "  ")
	data = append(data, '\n')
	directory := filepath.Join(h.project, "代码", "前端", "iOS", "JobsIM", "Resource", "DemoHost")
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	path := filepath.Join(directory, "demo-host.json")
	if _, err := os.Lstat(path); err == nil {
		if err = regular(path); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	fmt.Println("公开demo描述：", path)
	fmt.Println("控制面：", h.descriptor.ControllerURL)
	fmt.Println("描述仅含控制URL和CA公钥证书/指纹；不包含密码、私钥或数据目录。")
	return nil
}

func (h *demoHost) check() error {
	if h.ip == "" {
		return fmt.Errorf("尚未在Mac运行prepare；check不初始化任何资源")
	}
	raw, _ := json.Marshal(h.descriptor)
	fmt.Println(string(raw))
	return nil
}
