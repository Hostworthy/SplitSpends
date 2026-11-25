// Command api runs the SplitSpends HTTP API, the MCP endpoint, background
// jobs and (optionally) the web app.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Hostworthy/SplitSpends/apps/api/routes"
	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
	"github.com/Hostworthy/SplitSpends/packages/logger"
	"github.com/Hostworthy/SplitSpends/packages/mcp"
	"github.com/Hostworthy/SplitSpends/packages/middleware"
	"github.com/Hostworthy/SplitSpends/packages/spa"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "health" {
		if err := health(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}
	log := logger.New(cfg.LogLevel, cfg.LogFormat)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, err := bootstrap.Load(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer rt.Close()
	if !cfg.Serverless {
		go rt.RunJobs(ctx)
	}

	mux := http.NewServeMux()
	api := routes.New(routes.Deps{Services: rt.Services, Config: cfg, Log: log, Ping: rt.Ping, Jobs: rt.RunJobsOnce}).Routes()
	mux.Handle("/api/", api)
	mux.Handle("/health", api)
	mux.Handle("/internal/", api)
	if cfg.MCPEnabled {
		mux.Handle("/mcp", mcp.HTTPHandler(loopback(cfg.Addr), version, nil, log))
	}
	if cfg.WebDir != "" {
		mux.Handle("/", spa.Handler(cfg.WebDir))
	}

	handler := middleware.Observe(log, middleware.Security(middleware.CORS(cfg.CORSOrigins, mux)))
	srv := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	errc := make(chan error, 1)
	go func() {
		log.Info("api.server.listening", "addr", cfg.Addr, "baseUrl", cfg.BaseURL, "web", cfg.WebDir != "", "mcp", cfg.MCPEnabled, "serverless", cfg.Serverless)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	log.Info("api.server.stopping")
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// health checks the server running in this container, for the image's
// HEALTHCHECK: the distroless image has no shell or curl to do it.
func health() error {
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}
	return probe(loopback(cfg.Addr))
}

// probe succeeds when base/health answers 200 within three seconds.
func probe(base string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("api.health.unreachable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("api.health.unhealthy: %s", res.Status)
	}
	return nil
}

// loopback is the address the in-process MCP endpoint uses to reach the API.
func loopback(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port
}
