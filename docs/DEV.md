# Development

## Prerequisites

- Go 1.26+
- Flutter 3.50+ (Dart 3.14+)
- Docker (for the local PostgreSQL), or any PostgreSQL 14+
- Xcode for iOS builds, Android Studio for Android builds

## First Run

```bash
make setup   # backend/.env, Postgres in Docker, Go and Flutter dependencies
make dev     # API on :8080 and the web app on :5173
```

With `SMTP_HOST` empty, login codes appear in the API log (`notifications.email.logged`). The development `.env` uses `SIGNUP_MODE=open`.

Migrations are embedded in the binary and applied on start.

## Make Targets

| Target | Purpose |
| ------ | ------- |
| `make setup` | Env file, Postgres, dependencies |
| `make dev` | API plus Flutter web |
| `make api` / `make web` | One side only |
| `make check` | gofmt, go vet, Go tests (including the architecture test), `dart format`, `flutter analyze`, `flutter test` |
| `make test-integration` | Database-backed tests against `TEST_DATABASE_URL` (the database is wiped) |
| `make build` | `bin/splitspends` and `bin/splitspends-mcp` |
| `make docker` | Production image |

## Repository Layout

```text
backend/                 Go module (API, MCP, jobs)
  cmd/server             HTTP server: API, /mcp, web app, background jobs
  cmd/mcp                stdio MCP server
  internal/core          Domain center: entities, services, ports (no infrastructure)
    domains/<context>    user, auth, friend, group, expense, balance, activity, receipt, assist, stats
    ports                Interfaces adapters implement (Tx, Notifier, BlobStore, ReceiptReader)
    shared               errs, money, clock
  internal/database      PostgreSQL adapters (pgx) and embedded migrations
  internal/notifications SMTP email adapter
  internal/storage       Local filesystem blob store
  internal/ai            Claude and OpenAI-compatible receipt readers
  internal/platform      HTTP contract (request and response shapes), standard library only
  internal/middleware    Request ids, logging, CORS, auth, rate limits, error mapping
  internal/api           HTTP handlers and serializers
  internal/mcp           MCP tools, implemented as a client of the HTTP API
  internal/bootstrap     Configuration and wiring (composition root)
  internal/archtest      Dependency rules
app/                     Flutter client for iOS, Android and web
deploy/                  Production docker compose and env template
docs/                    API contract, environment, MCP, ADRs
```

## Adding a Feature

Follow [ADR-001](adr/001-domain-driven-design.md):

1. Entity and rules in `internal/core/domains/<context>/`, with colocated tests.
2. Repository interface (port) next to the entity; services depend on the interface.
3. PostgreSQL implementation in `internal/database/repositories/`. New tables get a new numbered migration in `internal/database/migrations/` (never edit an applied one).
4. Wire it in `internal/bootstrap/runtime.go`.
5. Request and response shapes in `internal/platform/contract.go`; document them in [API.md](API.md).
6. Handler and serializer in `internal/api/`, route in `router.go`.
7. Client models and screens in `app/lib/features/<feature>/`.
8. MCP tool in `internal/mcp/server.go` if assistants should use it.
9. `make check`.

## Testing

- Pure domain logic (splits, balances, receipt reconciliation) has table-style unit tests in `internal/core`.
- `internal/archtest` fails when a package imports across a forbidden boundary.
- `internal/bootstrap/integration_test.go` runs services against a real database.
- The Flutter app has unit and widget tests under `app/test`.
