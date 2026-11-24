# SplitSpends

Self-hosted Splitwise alternative: Go API and MCP server on PostgreSQL, Flutter client for iOS, Android and web. Clean/DDD layering.

## Commands

| Command | Checks |
| ------- | ------ |
| `cd backend && go build ./... && go vet ./...` | Compile and vet |
| `cd backend && go test -count=1 ./...` | Unit tests plus `internal/archtest` layer boundaries |
| `make test-integration` | Database tests (wipes `TEST_DATABASE_URL`) |
| `cd app && flutter analyze && flutter test` | Client lint and tests |
| `make check` | Everything above except integration |

Run the relevant checks before considering work done. If a check needs infrastructure that is not running, say so instead of skipping it silently.

## Documentation Map

| Need | Read |
| ---- | ---- |
| Setup, layout, adding a feature | [docs/DEV.md](docs/DEV.md) |
| HTTP contract (source of truth for app and MCP) | [docs/API.md](docs/API.md) |
| Environment variables | [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md) |
| Architecture decisions | [docs/adr/README.md](docs/adr/README.md) |
| MCP | [docs/MCP.md](docs/MCP.md) |

## Non-Negotiables

- Domain rules live in `backend/internal/core`; it imports no infrastructure. Repositories in `internal/database` hold no business rules.
- Money is `int64` minor units. Never use floats for amounts, in Go or Dart. Split inputs are decimal strings parsed exactly.
- Services return `errs.Error` with human-readable messages; `internal/middleware` maps kinds to HTTP status.
- Wire services only in `internal/bootstrap`. Handlers call services; they never touch repositories.
- `internal/mcp` calls the HTTP API, never core or the database.
- `internal/platform` is the wire contract, standard library only. Change it together with `docs/API.md` and the Flutter models.
- Success envelopes are `{data}` or `{items,total}`; errors are `{error,code,field?,requestId?}`; JSON is camelCase.
- Never edit an applied migration; add a new numbered file in `internal/database/migrations/`.
- Log messages are dotted keys (`area.thing.outcome`) with details as attributes.
- Short names; the package carries context (`group.Service.Create`). No `From`/`Of` in function names.

## Project Rules and Skills

- Rules: [`.claude/rules/`](.claude/rules/)
- Skills: `/architecture`, `/add-domain` in [`.claude/skills/`](.claude/skills/)
