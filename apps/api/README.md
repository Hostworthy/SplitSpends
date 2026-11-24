# Split Spends API Server

The Go HTTP server: REST API (`/api/v1`), MCP over HTTP (`/mcp`), background jobs and, when `WEB_DIR` is set, the web app. Handlers live in `routes/`; everything else comes from [`packages/`](../../packages).

```bash
cp apps/api/.env.example apps/api/.env   # or: make setup
make api                                  # runs on :8080
make build                                # bin/splitspends and bin/splitspends-mcp
```

Settings: [docs/ENVIRONMENT.md](../../docs/ENVIRONMENT.md). API contract: [docs/API.md](../../docs/API.md).
