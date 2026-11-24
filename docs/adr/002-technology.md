# ADR-002: Go, PostgreSQL and Flutter

## Status

Accepted

## Context

The service is self-hosted, often on a small VPS or home server, so the backend should be light. The client must feel native on iOS, Android and the web, and one team should be able to maintain it.

## Decision

- **Backend: Go.** A single static binary (about 25 MB), around 20 MB of memory at idle, fast startup, a strong standard library (`net/http` routing, `log/slog`), and quick compile times. Rust was the alternative; Go wins on iteration speed for a CRUD-heavy domain while staying in the same footprint class.
- **Database: PostgreSQL** through `pgx`, with SQL migrations embedded in the binary and applied on start (`goose`). Money is stored as `bigint` minor units.
- **Client: Flutter** for iOS, Android and web from one codebase with Material 3, adaptive layouts and Skia/Impeller rendering for smooth animation. The Go server serves the compiled web app, so one container is the whole product.
- **Email: SMTP** through `go-mail`, so any provider works (Gmail, Microsoft 365, SES, Postmark, a local relay).

## Consequences

### Positive

- One container plus PostgreSQL is a complete deployment.
- The same API powers web, mobile and MCP.

### Negative

- Flutter web downloads a larger first bundle than a hand-written web app; caching headers mitigate repeat visits.

### Neutral

- A desktop app is out of scope; the web app works on desktop browsers.
