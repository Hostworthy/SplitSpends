---
name: architecture
description: Layer boundaries and dependency direction for the SplitSpends backend. Use when adding packages, wiring imports across layers, or unsure where code belongs.
---

# Architecture

Dependencies point inward to `backend/internal/core`. See [ADR-001](../../../docs/adr/001-domain-driven-design.md). `internal/archtest` enforces the rules; read its `rules` table when unsure.

| Package | Contains | May import |
| ------- | -------- | ---------- |
| `internal/core` | Entities, services, repository ports, `errs`, `money`, `clock` | Standard library (no `net/http`), `uuid` |
| `internal/database` | pgx repositories, migrations | core |
| `internal/notifications`, `internal/storage`, `internal/ai` | Adapters for ports | core ports and shared |
| `internal/platform` | Wire contract | Standard library only |
| `internal/middleware` | HTTP cross-cutting | core (auth, errs), platform |
| `internal/bootstrap` | Config and wiring | core and adapters |
| `internal/api` | Handlers, serializers, routes | bootstrap services, core types, platform, middleware |
| `internal/mcp` | MCP tools as an API client | platform |
| `cmd/*` | Process entry points | anything |

Questions to ask:

- Is it a business rule? Core, with a unit test.
- Is it SQL? A repository in `internal/database/repositories`, implementing a core port.
- Is it a request or response shape? `internal/platform`, plus `docs/API.md` and the Flutter model.
- Does an assistant need it? A tool in `internal/mcp` that calls the API.

Verify with `cd backend && go test -count=1 ./internal/archtest/`.
