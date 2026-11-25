# Development

## Prerequisites

- Go 1.26+
- Flutter 3.50+ (Dart 3.14+)
- Docker (for the local PostgreSQL), or any PostgreSQL 14+
- [ShellCheck](https://www.shellcheck.net) for `make lint`
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
| `make check` | `make lint`, `make vuln` and `make test` |
| `make lint` | gofmt, go vet, golangci-lint, `dart format`, `flutter analyze` on every Dart member, web fallback fonts, shellcheck |
| `make vuln` | [govulncheck](https://go.dev/doc/security/vuln/) (pinned) on the Go code: fails when code calls a function with a known vulnerability, in a module or the standard library |
| `make test` | Go tests (including the architecture test), `dart test` for the ledger, `flutter test` for the client |
| `make fmt` | gofmt, `dart fix --apply` and `dart format` |
| `make hooks` | Quick gofmt and dart format checks before every commit (`scripts/hooks/pre-commit`) |
| `make brand` | Re-render icons, splash images and banners from `scripts/brand/render.py` (see [packages/brand](../packages/brand/README.md)) |
| `make test-integration` | Database-backed tests against `TEST_DATABASE_URL` (the database is wiped) |
| `make build` | `bin/splitspends` and `bin/splitspends-mcp` |
| `make web-build` / `make apk` | Web build (`apps/web/build/web`) / Android APK (`apps/apk/build`) |
| `make docker` | Production image |

## Linting and Formatting

- Go: `gofmt`, `go vet` and [golangci-lint](https://golangci-lint.run) with `.golangci.yml` (the standard set plus checks for closed bodies and rows, error wrapping, contexts on requests, misspellings, wasted assignments and gosec's security checks outside tests). `make lint` runs a pinned release through `go run`, so it is built with this repository's Go version; the first run takes a minute.
- Dart: every workspace member includes `analysis_options.yaml` at the root on top of its base set: strict casts and raw types, unawaited futures and unclosed streams as errors, final locals, single quotes and more. `dart format` uses the default style.
- Shell scripts (`scripts/*/*.sh` and the pre-commit hook): [ShellCheck](https://www.shellcheck.net).
- `.editorconfig` sets UTF-8, LF, final newlines and indentation for every editor.
- CI runs the same checks: the `backend` job runs gofmt, vet, golangci-lint and shellcheck; the `app` job runs dart format and flutter analyze; the `vuln` job runs govulncheck, also weekly so new advisories show up without a push.
- CI, the release workflow and the Docker image run `flutter pub get --enforce-lockfile`, which fails when `pubspec.lock` does not match the pubspec files or a package archive does not match its recorded hash. Commit the updated `pubspec.lock` with every dependency change.
- Dependabot (`.github/dependabot.yml`) proposes weekly update pull requests for Go modules, pub packages, workflow actions and the Dockerfile base images.
- The Dockerfile pins its base images by digest and checks the Flutter archive against `FLUTTER_SHA256`. When you change `.flutter-version`, set `FLUTTER_SHA256` to that release's `sha256` from [releases_linux.json](https://storage.googleapis.com/flutter_infra_release/releases/releases_linux.json); the image build fails until the two match.

## Repository Layout

```text
apps/                     Runnable apps only
  api/                    Go HTTP server: API, /mcp, web app, background jobs (main.go)
    routes/               HTTP handlers and serializers
  mcp/                    Go stdio MCP server for local assistants (main.go)
  apk/                    Android app (Flutter shell): android/ runner + lib/main.dart
  ios/                    Flutter shell: ios/ runner + lib/main.dart
  web/                    Flutter shell: web/ runner + lib/main.dart
packages/                 Shared code the apps are built from
  core/                   Go domain centre: entities, services, ports (no infrastructure)
    domains/<context>     user, auth, admin, friend, group, expense, category, balance, activity, receipt, assist, stats, transfer, push, reminder, invite, rates
    ports                 Interfaces adapters implement (Tx, Notifier, BlobStore, ReceiptReader)
    shared                errs, money, clock
  database/               PostgreSQL adapters (pgx) and embedded migrations
  notifications/          SMTP email adapter
  webpush/                Web Push adapter (browsers, and Android through UnifiedPush)
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
deploy/                   docker/ (Compose with HTTPS and backups, for a homelab or VPS), aws/ and gcp/ (Terraform)
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
- Database tests skip unless `TEST_DATABASE_URL` is set. Each one calls `bootstraptest.Open(t, bootstraptest.Options{})` (`packages/bootstrap/bootstraptest`), which wipes and migrates the schema, wires every service with a recorded notifier (`env.Notifier.Code(email)`, `Sent()`) and a clock that moves only on `env.Clock.Add`, and holds a database lock so packages never run database tests at the same time. `env.SignIn(t, email, name)` returns a session. `packages/bootstrap` tests services this way.
- `apps/api/routes` tests the HTTP API: `s := newServer(t, bootstraptest.Options{})` serves the wired routes behind the middleware chain of `apps/api/main.go`; `s.signIn(t, email, name).Token` signs in over HTTP, and `s.call(t, method, path, token, body).want(t, status)` sends JSON, with `data[T]`, `items[T]` and `failure` decoding the envelopes. The test server trusts one proxy hop and `signIn` gives each person a client address of their own, so a test can sign in as many people as it needs; calls without `X-Forwarded-For` share one address, as the rate limits expect. For who-may-not tests, `s.group(t, token, name, emails...)` and `s.expense(t, token, groupID, total, payer, split...)` set up the data and `s.refused(t, token, status, code, []request{...})` checks that every call in the table is refused with that status and error code. `TestRoutesRequireAuth` reads `router.go` and expects 401 without a token from every route that is not in its `anonymous` list; add a route there only when it must work signed out.
- `packages/middleware` has plain `httptest` unit tests.
- `packages/ledger/test` runs `spec/vectors.json` against the Dart rules; `packages/client/test` has unit and widget tests.
- `apps/apk/integration_test` runs the real app on an Android device or emulator: on-device mode, then signing in to a server and uploading. CI runs it on an Android 13 emulator against a development server when started by hand (Actions, CI, Run workflow), checks the upload through the API, and publishes the release APK as the `splitspends-apk` artifact. To run it yourself, start the API with `SIGNUP_MODE=open` and no SMTP, then:

  ```bash
  python3 scripts/ci/codes.py server.log 8099 &   # serves the logged sign-in codes
  cd apps/apk
  flutter test integration_test \
    --dart-define=SPLITSPENDS_TEST_SERVER=http://10.0.2.2:8080 \
    --dart-define=SPLITSPENDS_TEST_CODES=http://10.0.2.2:8099
  ```

  Without the defines only the on-device part runs.
