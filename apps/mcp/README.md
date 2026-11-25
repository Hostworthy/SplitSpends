# Split Spends MCP Server (stdio)

Runs the Split Spends MCP tools over stdio for local assistants, talking to any Split Spends server with a personal API token. The same tools are served over HTTP at `/mcp` by [`apps/api`](../api).

```bash
SPLITSPENDS_URL=https://split.example.com SPLITSPENDS_TOKEN=ss_... go run ./apps/mcp
```

The `admin_` tools are off unless `SPLITSPENDS_ADMIN_TOOLS=true` is set.

Setup for Claude and other clients: [docs/MCP.md](../../docs/MCP.md).
