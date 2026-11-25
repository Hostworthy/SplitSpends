// Command api runs the SplitSpends HTTP API, the MCP endpoint, background
// jobs and (optionally) the web app.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	mux := http.NewServeMux()
	api := routes.New(routes.Deps{Services: rt.Services, Settings: cfg.Settings(), Log: log, Ping: rt.Ping, Jobs: rt.RunJobsOnce}).Routes()
	mux.Handle("/api/", api)
	mux.Handle("/health", api)
	mux.Handle("/internal/", api)
	if cfg.MCPEnabled {
		mux.Handle("/mcp", mcpHandler(cfg, log))
	}
	secure := strings.HasPrefix(cfg.BaseURL, "https://")
	if cfg.WebDir != "" {
		mux.Handle("/", middleware.Web(secure, spa.Handler(cfg.WebDir)))
	}

	handler := middleware.Observe(log, middleware.Security(secure, middleware.CORS(cfg.CORSOrigins, mux)))
	srv := newServer(cfg.Addr, handler)
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("api.server.listening", "addr", cfg.Addr, "baseUrl", cfg.BaseURL, "web", cfg.WebDir != "", "mcp", cfg.MCPEnabled, "serverless", cfg.Serverless)
		errc <- srv.Serve(ln)
	}()
	// Jobs start once the server accepts connections, and the database stays
	// open until a run in progress has stopped.
	jobs := make(chan struct{})
	if cfg.Serverless {
		close(jobs)
	} else {
		go func() {
			defer close(jobs)
			rt.RunJobs(ctx)
		}()
	}
	defer func() {
		stop()
		<-jobs
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

// mcpLimit is how many requests one client address may send to /mcp a
// minute. Each tool call is a request, and an agent at work sends far fewer.
// The API cannot tell MCP clients apart by address, because their tool calls
// reach it over loopback, so the limit sits in front of /mcp, before the
// token is checked.
const mcpLimit = 300

// mcpHandler serves the MCP endpoint, limited per client address.
func mcpHandler(cfg *bootstrap.Config, log *slog.Logger) http.Handler {
	limit := middleware.NewLimiter(mcpLimit, time.Minute)
	h := mcp.HTTPHandler(loopback(cfg.Addr), version, mcp.Options{Admin: cfg.MCPAdminTools}, nil, log)
	return middleware.Limit(limit, log, middleware.ByAddress(cfg.ProxyHops), h)
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

// Limits of one request. readTimeout bounds reading the whole request, body
// included, so a client cannot hold a connection by sending its body slowly;
// it leaves room for a receipt of receipt.MaxSize on a slow link.
// writeTimeout counts from the end of the headers and must stay above the
// longest handler: POST /internal/jobs runs for up to 5 minutes, and a
// receipt scan waits up to 3 minutes for the AI provider after its upload.
const (
	readTimeout    = 2 * time.Minute
	writeTimeout   = 6 * time.Minute
	maxHeaderBytes = 64 << 10
)

func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}
