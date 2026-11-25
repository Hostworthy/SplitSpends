# ADR-004: MCP Server as an API Client

## Status

Accepted

## Context

Assistants should be able to read balances and add expenses with the same rules as the app. A second path into the domain would duplicate authorization and validation.

## Decision

- `packages/mcp` builds an MCP server whose tools call the public HTTP API with the caller's bearer token. It imports only `packages/platform`, never core or the database.
- The main server mounts it at `/mcp` as stateless Streamable HTTP. One MCP server, with its tool schemas, is built at startup; each tool call reads the bearer token of its own request and calls the API over loopback with it. A request without a bearer token is answered 401 before any MCP work.
- The SDK runs tool calls on goroutines of its own, outside the HTTP recovery middleware, so every tool is registered through one helper that recovers a panic into a tool error.
- `apps/mcp` runs the same server over stdio for local assistants, configured with `SPLITSPENDS_URL` and `SPLITSPENDS_TOKEN`.
- Tools accept decimal amounts ("12.50") and return readable text with names and ids, because language models handle those better than minor units.
- DNS-rebinding protection is disabled for `/mcp` because every call needs an explicit bearer token, which a rebinding page cannot supply; this lets the server sit behind a local reverse proxy.

## Consequences

### Positive

- One set of rules for app, scripts and assistants.

### Negative

- Remote MCP uses static bearer tokens, not OAuth; clients that only support OAuth cannot connect yet.
