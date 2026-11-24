# Development

## Prerequisites

- Go 1.26+
- Flutter 3.50+ (Dart 3.14+)
- Docker (for the local PostgreSQL), or any PostgreSQL 14+
- Xcode for iOS builds, Android Studio for Android builds

## First Run

```bash
make setup   # apps/api/.env, Postgres in Docker, Go and Dart dependencies
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
| `make web-build` / `make apk` | Web build (`apps/web/build/web`) / Android APK (`apps/android/build`) |
| `make docker` | Production image |

## Repository Layout

```text
apps/                     Runnable apps only
  api/                    Go HTTP server: API, /mcp, web app, background jobs (main.go)
    routes/               HTTP handlers and serializers
  mcp/                    Go stdio MCP server for local assistants (main.go)
  android/                Flutter shell: android/ runner + lib/main.dart
  ios/                    Flutter shell: ios/ runner + lib/main.dart
  web/                    Flutter shell: web/ runner + lib/main.dart
packages/                 Shared code the apps are built from
  core/                   Go domain centre: entities, services, ports (no infrastructure)
    domains/<context>     user, auth, friend, group, expense, balance, activity, receipt, assist, stats, transfer
    ports                 Interfaces adapters implement (Tx, Notifier, BlobStore, ReceiptReader)
    shared                errs, money, clock
  database/               PostgreSQL adapters (pgx) and embedded migrations
  notifications/          SMTP email adapter
  storage/                Local disk and S3-compatible blob stores
  ai/                     Anthropic and OpenAI-compatible AI providers
  platform/               HTTP contract (request and response shapes), standard library only
  middleware/             Request ids, logging, CORS, auth, rate limits, error mapping
  mcp/                    MCP tools, implemented as a client of the HTTP API
  bootstrap/              Configuration and wiring (composition root)
  logger/, spa/           Structured logging; serving the web build
  archtest/               Dependency rules between apps and packages
  ledger/                 Dart: split, balance and recurrence rules (mirrors core)
  client/                 Flutter: every screen, the local store, sync, assets and fonts
spec/vectors.json         Shared test cases run by both the Go and the Dart rules
scripts/                  README screenshots; fonts/fallback.sh checks the committed web fallback fonts
deploy/                   Docker Compose (server, homelab), Terraform (AWS, GCP)
docs/                     API contract, environment, hosting, MCP, ADRs
go.mod, pubspec.yaml      One Go module and one Dart workspace at the root
```

`apps/` holds only things you run; everything they share lives in `packages/`. The three Flutter apps are thin shells: the UI is one package (`packages/client`) so Android, iOS and the web stay identical.

## Adding a Feature

Follow [ADR-001](adr/001-domain-driven-design.md):

1. Entity and rules in `packages/core/domains/<context>/`, with colocated tests.
2. Repository interface (port) next to the entity; services depend on the interface.
3. PostgreSQL implementation in `packages/database/repositories/`. New tables get a new numbered migration in `packages/database/migrations/` (never edit an applied one).
4. Wire it in `packages/bootstrap/runtime.go`.
5. Request and response shapes in `packages/platform/contract.go`; document them in [API.md](API.md).
6. Handler and serializer in `apps/api/routes/`, route in `router.go`.
7. Client models and screens in `packages/client/lib/features/<feature>/`.
8. MCP tool in `packages/mcp/server.go` if assistants should use it.
9. `make check`.

## Testing

- Pure domain logic (splits, balances, receipt reconciliation) has table-style unit tests in `packages/core`.
- `packages/archtest` fails when a package imports across a forbidden boundary.
- `packages/bootstrap/integration_test.go` runs services against a real database.
- `packages/ledger/test` runs `spec/vectors.json` against the Dart rules; `packages/client/test` has unit and widget tests.
- `apps/android/integration_test` runs the real app on an Android device or emulator: on-device mode, then signing in to a server and uploading. CI runs it on an Android 13 emulator against a development server, checks the upload through the API, and publishes the release APK as the `splitspends-apk` artifact. To run it yourself, start the API with `SIGNUP_MODE=open` and no SMTP, then:

  ```bash
  python3 scripts/ci/codes.py server.log 8099 &   # serves the logged sign-in codes
  cd apps/android
  flutter test integration_test \
    --dart-define=SPLITSPENDS_TEST_SERVER=http://10.0.2.2:8080 \
    --dart-define=SPLITSPENDS_TEST_CODES=http://10.0.2.2:8099
  ```

  Without the defines only the on-device part runs.
