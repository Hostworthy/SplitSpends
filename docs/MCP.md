# MCP Server

Split Spends ships a Model Context Protocol server so assistants such as Claude can read balances and add expenses for you. It is a client of the public HTTP API: every call is checked exactly like a call from the app.

## Get a Token

In the app, open **Account → API tokens**, create a token and copy it (it starts with `ss_` and is shown once). Revoke it there at any time.

## Option 1: Remote (Streamable HTTP)

A Split Spends server serves `POST /mcp` unless it is started with `MCP_ENABLED=false` (see [Environment](ENVIRONMENT.md)); that setting removes the endpoint and leaves the API as it is. Send the token as a bearer header. A request without one is answered `401 Unauthorized` with `WWW-Authenticate: Bearer` and the usual error envelope; an invalid or revoked token reaches the tools and comes back as a tool error asking for a new token. Each client address may send 300 requests a minute to `/mcp` (`429 Too Many Requests` with the `RATE_LIMITED` envelope beyond that).

Claude Code:

```bash
claude mcp add --transport http splitspends https://split.example.com/mcp \
  --header "Authorization: Bearer ss_your_token"
```

Generic client configuration:

```json
{
  "mcpServers": {
    "splitspends": {
      "type": "http",
      "url": "https://split.example.com/mcp",
      "headers": { "Authorization": "Bearer ss_your_token" }
    }
  }
}
```

## Option 2: Local (stdio)

Build or download `splitspends-mcp` (`make build` writes `bin/splitspends-mcp`; the Docker image contains it at `/usr/local/bin/splitspends-mcp`).

```json
{
  "mcpServers": {
    "splitspends": {
      "command": "/path/to/splitspends-mcp",
      "env": {
        "SPLITSPENDS_URL": "https://split.example.com",
        "SPLITSPENDS_TOKEN": "ss_your_token"
      }
    }
  }
}
```

Set `SPLITSPENDS_URL` to the exact address the server answers on, usually its `https://` origin, without `/api/v1` (a path prefix such as `https://example.com/split` is fine when the server sits under one). The binary refuses to start when the address has no `http://` or `https://` scheme, contains a user name, query or fragment, or ends in `/api/v1`. Use `https://`: the token goes with every request and never expires, so over plain `http://` anyone on the network path can read it. The binary warns at startup when `http://` points at another machine; `http://localhost` is fine. The MCP server does not follow redirects, because following one can turn a write into a read: when the server redirects, the tool fails with an error that names the address to use instead.

## Tools

| Tool | What it does |
| ---- | ------------ |
| `get_overview` | Who you are, total balances, groups and friends |
| `list_groups` | Groups with ids, members and your balance |
| `get_group` | Member balances, settle-up plan and recent expenses |
| `list_friends` | Friends with ids, emails and balances |
| `list_expenses` | Search by group, friend, text and dates |
| `get_expense` | Who paid and who owes what |
| `list_activity` | Recent activity |
| `list_categories` | Category ids for new expenses: the built-ins plus your own custom categories |
| `add_expense` | Equal, exact, percent or shares split; amounts are decimal strings. In a group with a default split, it is used when no split is given |
| `record_payment` | Settle up between two people |
| `remind_to_settle` | Email someone who owes you a reminder to settle up (one group or everything; once per 24 hours) |
| `delete_expense` | Soft delete (restorable in the app) |
| `admin_get_signup`, `admin_set_signup` | Administrators: sign-up mode and allowed list |
| `admin_list_requests`, `admin_decide_request` | Administrators and managers: people waiting to join; approve or decline |
| `admin_list_accounts`, `admin_set_role` | Administrators: accounts, and making someone a user, manager or admin |
| `admin_disable_account`, `admin_enable_account` | Administrators: disable (signs out everywhere, removes the email) or enable an account |

The admin tools are off by default. The tools read text that other people wrote, such as expense descriptions, notes and names, and an assistant can mistake such text for instructions; keeping the admin tools away from everyday sessions means an administrator's token cannot be talked into changing the instance. Turn them on only for a session you use to administer: set `MCP_ADMIN_TOOLS=true` on the server for `/mcp`, or `SPLITSPENDS_ADMIN_TOOLS=true` for the stdio server. Even then the API refuses them unless your role allows it, and `get_overview` shows your role.

Text that people wrote is shown in double quotes, and the server tells the assistant to treat it as data, not instructions. That lowers the risk but does not remove it, so keep tool confirmation on in your assistant for anything that changes data.

## Security

- An API token carries the full rights of the account that created it, through the API as well as through the MCP tools: whoever holds it can read everything that person can see and add, change or delete expenses in their name. It does not expire. Revoke it under **Account → API tokens** when an assistant no longer needs it. A token cannot create or revoke tokens; that takes the app.
- An administrator's or manager's token keeps those rights even while the admin tools are off: turning them off only keeps the tools away from the assistant. Prefer an assistant token from an account that is not an administrator.
- The token travels with every request, to `/mcp` and from the stdio server to the API, so use `https://` addresses (see [Option 2](#option-2-local-stdio)). Turn `/mcp` off with `MCP_ENABLED=false` if nobody on your instance uses it.

Example prompts: "How much do I owe in the Lisbon trip?", "Add 84.20 for groceries in Flat, split with everyone", "Ben paid me back 25 euros", "Remind Sam about the Lisbon trip".

## Design

See [ADR-004](adr/004-mcp-server.md). The MCP layer never imports the domain or the database; an architecture test enforces this.
