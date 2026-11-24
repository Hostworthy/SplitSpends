# SplitSpends

Self-hosted Splitwise alternative: Go API and MCP server on PostgreSQL, Flutter client for iOS, Android and web. Clean/DDD layering. `apps/` holds only runnable apps (`api`, `mcp`, `apk` for Android, `ios`, `web`); shared code lives in `packages/`.

## Commands

| Command | Checks |
| ------- | ------ |
| `go build ./... && go vet ./...` | Compile and vet (repository root is the Go module) |
| `go test -count=1 ./...` | Unit tests plus `packages/archtest` layer boundaries |
| `make test-integration` | Database tests (wipes `TEST_DATABASE_URL`) |
| `make lint` | gofmt, go vet, golangci-lint (`.golangci.yml`), dart format, flutter analyze on every Dart member (root `analysis_options.yaml` is shared) |
| `make fmt` | gofmt, dart fix and dart format |
| `make test` | Go tests, `packages/ledger` (dart test), `packages/client` (flutter test) |
| `make vuln` | govulncheck on the Go code and standard library |
| `make check` | `lint`, `vuln` and `test` |

Run the relevant checks before considering work done. If a check needs infrastructure that is not running, say so instead of skipping it silently.

## Documentation Map

| Need | Read |
| ---- | ---- |
| Setup, layout, adding a feature | [docs/DEV.md](docs/DEV.md) |
| HTTP contract (source of truth for app and MCP) | [docs/API.md](docs/API.md) |
| Environment variables | [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md) |
| Architecture decisions | [docs/adr/README.md](docs/adr/README.md) |
| MCP | [docs/MCP.md](docs/MCP.md) |
| Releases | [docs/RELEASING.md](docs/RELEASING.md) |

## Non-Negotiables

- Domain rules live in `packages/core`; it imports no infrastructure. Repositories in `packages/database` hold no business rules.
- `apps/` contains only runnable apps; never put shared code there. Packages never import apps (`packages/archtest` enforces it).
- Split and balance rules exist twice, in `packages/core` (Go) and `packages/ledger` (Dart); change both and extend `spec/vectors.json`.
- Money is `int64` minor units. Never use floats for amounts, in Go or Dart. Split inputs are decimal strings parsed exactly.
- Services return `errs.Error` with human-readable messages; `packages/middleware` maps kinds to HTTP status.
- Wire services only in `packages/bootstrap`. Handlers call services; they never touch repositories.
- `packages/mcp` calls the HTTP API, never core or the database.
- `packages/platform` is the wire contract, standard library only. Change it together with `docs/API.md` and the Flutter models.
- Success envelopes are `{data}` or `{items,total}`; errors are `{error,code,field?,requestId?}`; JSON is camelCase.
- Never edit an applied migration; add a new numbered file in `packages/database/migrations/`.
- Log messages are dotted keys (`area.thing.outcome`) with details as attributes.
- Short names; the package carries context (`group.Service.Create`). No `From`/`Of` in function names.

## Project Rules and Skills

- Rules: [`.claude/rules/`](.claude/rules/)
- Skills: `/architecture`, `/add-domain` in [`.claude/skills/`](.claude/skills/)
