# SplitSpends MCP Server (stdio)

Runs the SplitSpends MCP tools over stdio for local assistants, talking to any SplitSpends server with a personal API token. The same tools are served over HTTP at `/mcp` by [`apps/api`](../api).

```bash
SPLITSPENDS_URL=https://split.example.com SPLITSPENDS_TOKEN=ss_... go run ./apps/mcp
```

Setup for Claude and other clients: [docs/MCP.md](../../docs/MCP.md).
