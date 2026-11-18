// controller.go
// JobsIM Native Demo Host
// Created by Jobs on 2026年11月18日，星期三.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type demoConnection struct {
	ServerURL        string `json:"serverURL"`
	CASHA256         string `json:"caSha256"`
	CACertificatePEM string `json:"caCertificatePEM"`
	DeploymentID     string `json:"deploymentId"`
}

type demoAccount struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	RegistrationID string `json:"registrationId,omitempty"`
}

type demoProof struct {
	Token, UserID, Username string
	LastLoginAttempt        time.Time
}

type demoChild struct {
	command *exec.Cmd
	done    chan struct{}
	err     error
}

type demoController struct {
	host             *demoHost
	client           *http.Client
	ctx              context.Context
	mu               sync.Mutex
	validationMu     sync.Mutex
	proofs           []demoProof
	state, errorCode string
	connection       demoConnection
	accounts         []demoAccount
	child            *demoChild
	closed           bool
	window           time.Time
	starts           int
}

func (h *demoHost) serve(ctx context.Context) error {
	if h.ip == "" {
		return fmt.Errorf("先运行本机prepare；serve不生成替代实例")
	}
	if err := h.prepareTLS(); err != nil {
		return err
	}
	if ip, err := localPrivateIP(); err != nil || ip != h.ip {
		return fmt.Errorf("demo LAN地址不再匹配；在Mac明确重新配对")
	}
	lockPath := filepath.Join(h.control, "controller.lock")
	if _, err := os.Lstat(lockPath); err == nil {
		if err = regular(lockPath); err != nil {
			return err
		}
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("本机固定demo controller已运行；未启动重复进程")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(h.descriptor.CACertificatePEM)) {
		return fmt.Errorf("demo CA无效")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, Proxy: nil}
	defer transport.CloseIdleConnections()
	c := &demoController{host: h, client: &http.Client{Transport: transport, Timeout: 8 * time.Second}, ctx: ctx, state: "idle"}
	defer c.close()
	server := &http.Server{Addr: net.JoinHostPort(h.ip, fmt.Sprint(controllerPort)), Handler: c.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	done := make(chan error, 1)
	go func() {
		done <- server.ListenAndServeTLS(filepath.Join(h.control, "server.crt"), filepath.Join(h.control, "server.key"))
	}()
	fmt.Println("JobsIM固定demo控制面：", h.descriptor.ControllerURL)
	fmt.Println("Mac窗口保持前台；iPhone开关只确保此demo启动，无远程停止/删库入口。Ctrl+C仅停止此demo并保留其资料。")
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (c *demoController) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/demo/v1/ensure-start", c.ensure)
	mux.HandleFunc("GET /api/demo/v1/status", func(w http.ResponseWriter, r *http.Request) { c.respond(w, r, false) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private,no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); origin != "" && origin != c.host.descriptor.ControllerURL {
			writeDemo(w, 403, map[string]string{"state": "failed", "error": "CONTROLLER_ORIGIN_REJECTED"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func writeDemo(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private,no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (c *demoController) ensure(w http.ResponseWriter, r *http.Request) {
	// Native JobsNetworking omits an empty JSON dictionary. Empty bytes and {}
	// both express the same fixed action; any parameter or extra document fails.
	content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		writeDemo(w, 400, map[string]string{"state": "failed", "error": "EMPTY_FIXED_REQUEST_REQUIRED"})
		return
	}
	if len(bytes.TrimSpace(content)) > 0 {
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			writeDemo(w, 415, map[string]string{"state": "failed", "error": "JSON_REQUIRED"})
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(content))
		var body map[string]json.RawMessage
		if err := decoder.Decode(&body); err != nil || body == nil || len(body) != 0 {
			writeDemo(w, 400, map[string]string{"state": "failed", "error": "EMPTY_FIXED_REQUEST_REQUIRED"})
			return
		}
		var tail any
		if decoder.Decode(&tail) != io.EOF {
			writeDemo(w, 400, map[string]string{"state": "failed", "error": "ONE_REQUEST_REQUIRED"})
			return
		}
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		writeDemo(w, 503, map[string]string{"state": "failed", "error": "CONTROLLER_CLOSING"})
		return
	}
	if c.state == "ready" || c.child != nil && c.state != "starting" {
		c.mu.Unlock()
		c.revalidateReady(w, r)
		return
	}
	if c.state != "ready" && c.state != "starting" {
		now := time.Now()
		if c.window.IsZero() || now.Sub(c.window) >= time.Minute {
			c.window = now
			c.starts = 0
		}
		if c.starts >= 6 {
			c.mu.Unlock()
			writeDemo(w, 429, map[string]string{"state": "failed", "error": "DEMO_START_RATE_LIMITED"})
			return
		}
		c.starts++
		c.state = "starting"
		c.errorCode = ""
		go c.start()
	}
	c.mu.Unlock()
	c.respond(w, r, true)
}

func (c *demoController) respond(w http.ResponseWriter, r *http.Request, includeAccounts bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		writeDemo(w, 503, map[string]string{"state": "failed", "error": "CONTROLLER_CLOSING"})
		return
	}
	status := 200
	body := map[string]any{"state": c.state}
	if c.state == "starting" {
		status = 202
	}
	if c.state == "failed" {
		status = 503
		body["error"] = c.errorCode
	}
	if c.state == "ready" {
		body["connection"] = c.connection
		body["webURL"] = c.connection.ServerURL + "/"
		if includeAccounts {
			accounts := []demoAccount{}
			for _, a := range c.accounts {
				accounts = append(accounts, demoAccount{Username: a.Username, Password: a.Password})
			}
			body["accounts"] = accounts
		}
	}
	writeDemo(w, status, body)
}

func (c *demoController) failed(code string) {
	c.mu.Lock()
	if !c.closed {
		c.state = "failed"
		c.errorCode = code
	}
	c.mu.Unlock()
}

func (c *demoController) start() {
	if err := c.startFixed(); err != nil {
		c.stopChild()
		c.failed("DEMO_START_FAILED")
		fmt.Fprintln(os.Stderr, "固定demo启动失败（不回传私密日志）；请检查Mac Control/Logs。")
	}
}

func (c *demoController) startFixed() error {
	environment, err := c.host.env()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(c.host.control, "Logs", "runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	command := exec.Command("/bin/zsh", c.host.script(), "demo-run", "--yes", "--mode", "server", "--state", c.host.instance)
	command.Env = environment
	command.Stdout = log
	command.Stderr = log
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child := &demoChild{command: command, done: make(chan struct{})}
	c.mu.Lock()
	if c.closed || c.child != nil {
		c.mu.Unlock()
		log.Close()
		return fmt.Errorf("owned demo child already active")
	}
	if err = command.Start(); err != nil {
		c.mu.Unlock()
		log.Close()
		return err
	}
	c.child = child
	c.mu.Unlock()
	go func() {
		child.err = command.Wait()
		_ = log.Close()
		close(child.done)
		c.failed("DEMO_PROCESS_EXITED")
	}()
	ctx, cancel := context.WithTimeout(c.ctx, 100*time.Second)
	defer cancel()
	var bootstrap map[string]any
	for {
		if err = c.api(ctx, "GET", "/api/bootstrap", nil, "", &bootstrap); err == nil {
			break
		}
		select {
		case <-child.done:
			return fmt.Errorf("owned demo process exited")
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	deployment, ok := bootstrap["deploymentId"].(string)
	if !ok || deployment == "" {
		return fmt.Errorf("missing actual deployment identity")
	}
	var health map[string]any
	if err = c.api(ctx, "GET", "/api/health", nil, "", &health); err != nil {
		return err
	}
	if err = c.verifyWeb(ctx); err != nil {
		return err
	}
	accounts, err := c.seed(ctx, deployment)
	if err != nil {
		return err
	}
	select {
	case <-child.done:
		return fmt.Errorf("owned demo process exited")
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("controller closing")
	}
	c.connection = demoConnection{ServerURL: fmt.Sprintf("https://%s:%d", c.host.ip, gatewayPort), CASHA256: c.host.descriptor.CASHA256, CACertificatePEM: c.host.descriptor.CACertificatePEM, DeploymentID: deployment}
	c.accounts = accounts
	c.state = "ready"
	c.errorCode = ""
	fmt.Println("固定demo已通过严格CA健康、Web资源和非管理员种子普通登录验真。")
	return nil
}

func (c *demoController) stopChild() {
	c.mu.Lock()
	child := c.child
	c.mu.Unlock()
	if child == nil {
		return
	}
	select {
	case <-child.done:
	default:
		_ = child.command.Process.Signal(syscall.SIGTERM)
		select {
		case <-child.done:
		case <-time.After(45 * time.Second):
			fmt.Fprintln(os.Stderr, "demo子进程未完成精确收尾；保留资源，不杀未知进程。")
		}
	}
	c.mu.Lock()
	if c.child == child {
		select {
		case <-child.done:
			c.child = nil
		default:
		}
	}
	c.mu.Unlock()
}

func (c *demoController) close() {
	c.mu.Lock()
	c.closed = true
	proofs := append([]demoProof(nil), c.proofs...)
	c.proofs = nil
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, proof := range proofs {
		_ = c.api(ctx, "POST", "/api/auth/logout", map[string]any{}, proof.Token, nil)
	}
	c.stopChild()
}
