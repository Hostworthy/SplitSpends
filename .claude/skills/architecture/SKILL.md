---
name: architecture
description: Layer boundaries and dependency direction for SplitSpends (Go and Dart). Use when adding packages, wiring imports across layers, or unsure where code belongs.
---

# Architecture

Dependencies point inward to `packages/core`. See [ADR-001](../../../docs/adr/001-domain-driven-design.md). `packages/archtest` enforces the rules; read its `rules` table when unsure.

| Package | Contains | May import |
| ------- | -------- | ---------- |
| `packages/core` | Entities, services, repository ports, `errs`, `money`, `clock` | Standard library (no `net/http`), `uuid` |
| `packages/database` | pgx repositories, migrations | core |
| `packages/notifications`, `packages/storage`, `packages/ai` | Adapters for ports | core ports and shared |
| `packages/platform` | Wire contract | Standard library only |
| `packages/middleware` | HTTP cross-cutting | core (auth, errs), platform |
| `packages/bootstrap` | Config and wiring | core and adapters |
| `apps/api/routes` | Handlers, serializers, routes | bootstrap services, core types, platform, middleware |
| `packages/mcp` | MCP tools as an API client | platform |
| `apps/api`, `apps/mcp` | Process entry points (`main.go`) | anything in `packages/` |
| `packages/ledger` (Dart) | Split and balance rules, mirrors `packages/core` | Dart SDK only |
| `packages/client` (Flutter) | Screens, local store, sync, assets | `packages/ledger`, Flutter plugins |
| `apps/apk`, `apps/ios`, `apps/web` | Platform runners calling `packages/client` | `packages/client` only |

Questions to ask:

- Is it a business rule? Core, with a unit test.
- Is it SQL? A repository in `packages/database/repositories`, implementing a core port.
- Is it a request or response shape? `packages/platform`, plus `docs/API.md` and the Flutter model.
- Does an assistant need it? A tool in `packages/mcp` that calls the API.

Packages never import apps; `apps/` only holds runnable apps.

Verify with `go test -count=1 ./packages/archtest/`.
