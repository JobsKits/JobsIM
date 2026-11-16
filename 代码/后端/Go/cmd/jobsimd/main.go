package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	jobsim "jobsim/backend"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		slog.Error("JobsIM stopped", "error", e)
		os.Exit(1)
	}
}
func run() error {
	exports := flag.Bool("experimental-exports", false, "Controlled export protocol validation only; not full I data export")
	receiptPolicy := flag.String("receipt-policy-file", os.Getenv("JOBS_IM_RECEIPT_POLICY_FILE"), "Deployment-approved exact native builds and storage verification digests; empty keeps every store transport-only")
	webDir := flag.String("web-dir", os.Getenv("JOBS_IM_WEB_DIR"), "Same-origin built Web distribution directory")
	role := flag.String("service", "all", "gateway|identity|directory|conversation|message|media|control|journal|governance|sync|all")
	addr := flag.String("addr", "127.0.0.1:8443", "Gateway listen address")
	base := flag.Int("internal-base-port", 8101, "First internal role port")
	state := flag.String("state-dir", os.Getenv("JOBS_IM_STATE_DIR"), "External private instance data directory")
	cert := flag.String("tls-cert", os.Getenv("JOBS_IM_TLS_CERT"), "TLS certificate")
	key := flag.String("tls-key", os.Getenv("JOBS_IM_TLS_KEY"), "TLS private key")
	insecure := flag.Bool("insecure-loopback", false, "Development HTTP, loopback only")
	flag.Parse()
	if *insecure {
		host, _, e := net.SplitHostPort(*addr)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("development HTTP requires explicit loopback IP")
		}
	} else if (*role == "gateway" || *role == "all") && (*cert == "" || *key == "") {
		return fmt.Errorf("TLS certificate/key required; local development may explicitly use --insecure-loopback")
	}
	public := os.Getenv("JOBS_IM_PUBLIC_URL")
	if public == "" {
		scheme := "https"
		if *insecure {
			scheme = "http"
		}
		public = scheme + "://" + *addr
	}
	urls := map[string]string{}
	roles := []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance", "sync"}
	for i, r := range roles {
		urls[r] = "http://127.0.0.1:" + strconv.Itoa(*base+i)
	}
	profiles, e := jobsim.LoadReceiptProfiles(*receiptPolicy)
	if e != nil {
		return e
	}
	cfg, e := jobsim.PrepareConfig(jobsim.Config{ReceiptProfiles: profiles, ExperimentalExports: *exports, JournalDatabaseURL: os.Getenv("JOBS_IM_JOURNAL_DATABASE_URL"), WebDir: *webDir, DatabaseURL: os.Getenv("JOBS_IM_DATABASE_URL"), StateDir: *state, Name: os.Getenv("JOBS_IM_NAME"), PublicURL: public, InsecureLoopback: *insecure, RoleURLs: urls})
	if e != nil {
		return e
	}
	cfg.NATSURL = os.Getenv("JOBS_IM_NATS_URL")
	cfg.NATSTokenFile = os.Getenv("JOBS_IM_NATS_TOKEN_FILE")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	selected := []string{*role}
	if *role == "all" {
		selected = append(roles, "gateway")
	}
	apps := []*jobsim.App{}
	servers := []*http.Server{}
	failures := make(chan error, len(selected))
	defer func() {
		for _, a := range apps {
			a.Close()
		}
	}()
	for _, r := range selected {
		if r != "gateway" {
			valid := false
			for _, v := range roles {
				if r == v {
					valid = true
				}
			}
			if !valid {
				return fmt.Errorf("unknown service role")
			}
		}
		roleConfig := cfg
		if d := os.Getenv("JOBS_IM_" + strings.ToUpper(r) + "_DATABASE_URL"); d != "" && r != "journal" {
			roleConfig.DatabaseURL = d
		}
		app, e := jobsim.NewApp(ctx, roleConfig, r)
		if e != nil {
			return e
		}
		apps = append(apps, app)
		listen := *addr
		if r != "gateway" {
			for i, v := range roles {
				if r == v {
					listen = "127.0.0.1:" + strconv.Itoa(*base+i)
				}
			}
		}
		srv := &http.Server{Addr: listen, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
		servers = append(servers, srv)
		listener, e := net.Listen("tcp", listen)
		if e != nil {
			return e
		}
		go func(s *http.Server, l net.Listener, gateway bool) {
			var err error
			if gateway && !*insecure {
				err = s.ServeTLS(l, *cert, *key)
			} else {
				err = s.Serve(l)
			}
			if !errors.Is(err, http.ErrServerClosed) {
				failures <- err
			}
		}(srv, listener, r == "gateway")
	}
	for _, app := range apps {
		app.StartSyncWorkers(ctx)
		app.StartPersonalExportOwnerWorkers(ctx)
		if app.Role == "sync" {
			app.StartSelfDraftWorkers(ctx)
		}
		if app.Role == "message" {
			app.StartForwardWorkers(ctx)
			app.StartDraftReplyWorkers(ctx)
		}
		if app.Role == "identity" {
			app.StartSessionWorkers(ctx)
		}
		if app.Role == "governance" {
			if *exports {
				app.StartExportWorkers(ctx)
			} else {
				app.StartExportCleanup(ctx)
			}
		}
	}
	slog.Info("JobsIM development slice running; close this terminal to stop", "deploymentId", cfg.DeploymentID, "url", public, "roles", selected)
	select {
	case <-ctx.Done():
	case e = <-failures:
		cancel()
	}
	shutdown, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	for _, s := range servers {
		_ = s.Shutdown(shutdown)
	}
	return e
}
