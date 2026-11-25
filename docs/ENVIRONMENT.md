# Environment Variables

The server reads everything from the environment. Invalid or missing required values stop startup with a dotted error key such as `bootstrap.config.env.required.not-found.DATABASE_URL`.

## Core

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `DATABASE_URL` | required | PostgreSQL URL, for example `postgres://user:pass@host:5432/splitspends?sslmode=disable`. Use a direct or session-mode connection, not a transaction-mode pooler (PgBouncer in transaction mode, Neon's `-pooler` host, Supabase's port 6543): migrations at start hold a session lock so that instances starting together apply them one at a time |
| `SECRET_KEY` | required | At least 32 characters; hex is decoded. Signs login codes. Generate with `openssl rand -hex 32` |
| `BASE_URL` | `http://localhost:8080` | Public URL, used in email links |
| `ADDR` | `:8080` | Listen address |
| `APP_NAME` | `Split Spends` | Name shown in emails and the app |
| `WEB_DIR` | empty | Directory with the Flutter web build; the Docker image sets `/srv/web` |
| `DATA_DIR` | `./data` | Where receipt images are stored |
| `DATABASE_MAX_CONNS` | `10` | Connection pool size, from 1 to 2147483647. Migrations at start open one more connection for their lock and close it when done |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `CORS_ORIGINS` | empty | Comma-separated browser origins allowed to call the API from elsewhere, or `*` |
| `TRUST_PROXY` | `false` | How many reverse proxies sit in front of the server, each appending the address it was connected from to `X-Forwarded-For`. Rate limits use the entry that many places from the right; entries further left are written by the client and ignored. `true` means `1`, `false` means `0` (use the connecting address). See [Client Addresses](#client-addresses) |
| `MCP_ENABLED` | `true` | Serve the MCP endpoint at `/mcp` |
| `MCP_ADMIN_TOOLS` | `false` | Offer the `admin_` tools on `/mcp`. Leave off unless you administer the instance through an assistant; see [MCP](MCP.md#tools) |
| `GRAVATAR` | `on` | `off` hides the "Use my Gravatar picture" option and stops sending picture links. People opt in one by one; their app then loads the picture from gravatar.com, which sees a hash of their email and who views it |

### Client Addresses

Sign-in codes and invite previews are rate limited per client address. IPv6 clients are counted per /64 network, the block one home or mobile connection is usually given. Behind a reverse proxy the connecting address is the proxy's, so the server reads `X-Forwarded-For` instead. Each proxy appends the address it was connected from to the right of whatever the client sent, so only the right-most entries can be trusted: set `TRUST_PROXY` to the number of proxies in front of the server, and the server uses the entry that many places from the right. If the header has fewer entries, the connecting address is used. `X-Real-Ip` is never read.

| Setup | `TRUST_PROXY` |
| ----- | ------------- |
| Server reached directly | `false` |
| Docker Compose with the bundled Caddy | `1` |
| One nginx, Traefik or similar proxy that appends to `X-Forwarded-For` | `1` |
| Google Cloud Run (`deploy/gcp`) | `1` |
| Render (`render.yaml`) | `1` |
| AWS: CloudFront then API Gateway (`deploy/aws`) | `2` |
| A CDN in front of any of the above | add one per extra proxy |

A value that is too high lets clients choose the address that is rate limited; one that is too low puts everyone behind the same proxy into one shared limit. To check a setup, log the `X-Forwarded-For` header of one request from a known address and count the entries to the right of it, plus one.

## Serverless and Storage

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `SERVERLESS` | `false` | Set on Lambda, Cloud Run and similar: no in-process background jobs, emails are sent before the response returns |
| `JOBS_SECRET` | empty | At least 32 characters. Enables `POST /internal/jobs` for an external scheduler. Required when `SERVERLESS=true` (the server refuses to start without it); call it every 15 to 60 minutes |
| `SECRETS_ARN` | empty | ARN of an AWS Secrets Manager secret holding a JSON object of variables, such as `{"DATABASE_URL": "...", "SECRET_KEY": "..."}`. Read once at start with the function or instance role's credentials (`secretsmanager:GetSecretValue`); each key fills the variable of that name unless it is already set. `deploy/aws` uses it to keep secrets out of the Lambda configuration |
| `AWS_ENDPOINT_URL_SECRETS_MANAGER` | AWS | Secrets Manager endpoint for `SECRETS_ARN`, for example a local emulator; defaults to the regional endpoint in the ARN |
| `BLOB_STORE` | `local` | Where receipt images go: `local` (`DATA_DIR`), `s3` or `postgres` (in the database; good for small free-tier hosts) |
| `S3_BUCKET` | empty | Bucket for `BLOB_STORE=s3` |
| `S3_REGION` | `AWS_REGION` | Bucket region |
| `S3_ENDPOINT` | AWS | Other S3-compatible services: `<account>.r2.cloudflarestorage.com`, `storage.googleapis.com`, `minio:9000` |
| `S3_ACCESS_KEY_ID` / `S3_SECRET_ACCESS_KEY` | empty | Static keys; without them the standard `AWS_*` variables and instance or function roles are used |
| `S3_PREFIX` | empty | Optional key prefix |

Buckets stay private: the API checks access and streams receipts itself.

## Sign-in

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `SIGNUP_MODE` | `invite` | Starting sign-up mode until an administrator changes it in the app (Account, Administration). `open`: anyone. `invite`: the allowed list, people a member added by email, group invite links, and people an administrator or manager approved. `approval`: anyone can ask and an administrator or manager approves them. `closed`: no new accounts. The very first account can always sign up and becomes the administrator (see `ADMIN_EMAIL`). `domain` is still read, as `invite` with the domains below |
| `ADMIN_EMAIL` | empty | The email address of the person who runs the server. While no account is active, only this address can sign in, and it becomes the administrator; nobody else can claim a new server first. Empty: whoever signs in first becomes the administrator, whatever `SIGNUP_MODE` says, and the server logs `auth.bootstrap.open` at start until someone has |
| `SIGNUP_ALLOWED_DOMAINS` | empty | Comma-separated domains (`example.com` or `@example.com`) that start out on the allowed list. The server refuses to start when an entry is not a domain |
| `SESSION_TTL_DAYS` | `90` | How long app sessions last. API tokens do not expire until revoked |

Login codes are 6 digits, valid for 10 minutes, allow 5 attempts, and at most 5 codes per email per hour from one network and 10 per email per hour in total. Requests for unknown emails in `invite` mode get the same response as real ones and send nothing.

## Email

Leave `SMTP_HOST` empty in development: emails, including login codes, are written to the server log.

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `SMTP_HOST` | empty | SMTP server |
| `SMTP_PORT` | `587` | Port |
| `SMTP_SECURITY` | `starttls` | `starttls` (port 587), `tls` (implicit TLS, port 465) or `none` (local relays only) |
| `SMTP_USERNAME` | empty | Login, if the server needs one |
| `SMTP_PASSWORD` | empty | Password or app password |
| `SMTP_FROM` | `SMTP_USERNAME` | From address, for example `Split Spends <split@example.com>` |

Common providers:

| Provider | Host | Port | Security | Notes |
| -------- | ---- | ---- | -------- | ----- |
| Gmail / Google Workspace | `smtp.gmail.com` | 587 | `starttls` | Use an app password |
| Outlook / Microsoft 365 | `smtp.office365.com` | 587 | `starttls` | SMTP AUTH must be enabled for the mailbox |
| Fastmail | `smtp.fastmail.com` | 465 | `tls` | App password |
| Amazon SES | `email-smtp.<region>.amazonaws.com` | 587 | `starttls` | SMTP credentials, verified sender |
| Postmark | `smtp.postmarkapp.com` | 587 | `starttls` | Server token as username and password |
| Mailgun | `smtp.mailgun.org` | 587 | `starttls` | |
| SendGrid | `smtp.sendgrid.net` | 587 | `starttls` | Username `apikey` |

## Push Notifications

Push works without any setup: the server speaks standard Web Push, which reaches browsers through their own push service and Android phones through a [UnifiedPush](https://unifiedpush.org) distributor such as ntfy. No Firebase or Google account is needed (see [ADR-009](adr/009-push-notifications.md)).

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` | generated | The server's VAPID key pair (RFC 8292), base64url without padding. Set both or neither |
| `VAPID_SUBJECT` | see below | Contact for push service operators: `mailto:you@example.com` or an `https://` URL |
| `PUSH_ALLOW_PRIVATE` | `false` | Allow push endpoints on private or loopback addresses, for example a self-hosted ntfy on your LAN |

Keys: when `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` are empty, the first server to start generates a pair and stores it in the database (`vapid_keys` table). Restarts and other instances read the same pair, so nothing needs configuring, and the database backup covers it. Set the variables only to manage the key yourself, for example to keep it across a fresh database. Browsers and distributors bind each subscription to the key, so changing it later silently breaks existing devices: they stop receiving notifications, are removed after repeated failures, and people tap Enable push notifications again. To generate a pair yourself, use any Web Push tool, for example `npx web-push generate-vapid-keys`.

The private key in the database is stored like the rest of your data: it only lets someone send notifications to the devices in the same database, so protect the database as usual.

`VAPID_SUBJECT` defaults to `BASE_URL` when that is `https`, otherwise to `mailto:` plus the `SMTP_FROM` address, otherwise to `mailto:admin@<BASE_URL host>`.

Push is on whenever a key pair is available; `GET /api/v1/config` reports `pushEnabled` and `vapidPublicKey`. With `SERVERLESS=true`, notifications are pushed before the response returns, like emails.

Outgoing requests: push endpoints come from devices, so the server refuses endpoints that resolve to loopback, private, carrier-grade NAT, link-local, multicast or other reserved addresses, or to a cloud metadata service, and does not follow redirects; IPv6 addresses that carry an IPv4 address (mapped, NAT64, 6to4) are judged by that address. An endpoint whose host is such an address is refused when the device registers. `PUSH_ALLOW_PRIVATE=true` lifts the private, carrier-grade NAT and loopback part; reserved ranges and the metadata services (including AWS's `fd00:ec2::254` and Alibaba Cloud's `100.100.100.200`, which sit inside private ranges) stay blocked. When the server reaches the internet through an `HTTPS_PROXY`, the proxy connects to endpoints itself: the server still refuses an endpoint whose host is such an address or resolves to one with the server's own DNS, but a host it cannot resolve is handed to the proxy, so restrict what the proxy may reach as well.

## AI (Optional)

Split Spends does not run a model. When a provider is configured, the server sends the user's instructions (and a receipt photo, when there is one) together with a system prompt describing their groups, people and the rules to that provider, and turns the structured answer into a draft the user reviews. AI is off unless a provider is configured, and the app hides AI buttons when it is off.

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `AI_PROVIDER` | empty | `anthropic`, `openai` (any OpenAI-compatible server) or `none` |
| `AI_API_KEY` | empty | Provider key. Required for `anthropic` |
| `AI_MODEL` | `claude-opus-5-5` for `anthropic` | Model id. Required for `openai` |
| `AI_BASE_URL` | empty | For `openai`: the API root before `/chat/completions` (examples below) |
| `AI_EFFORT` | model default | Anthropic only: `low`, `medium`, `high`, `xhigh`, `max` |
| `AI_RATE_LIMIT` | `30` | AI requests (receipt scans and expense drafts together) each person may make per hour. Each server instance counts on its own, so with several instances (serverless) a person can make this many on each |

Examples:

| Provider | `AI_PROVIDER` | `AI_BASE_URL` | `AI_MODEL` (example) |
| --- | --- | --- | --- |
| Anthropic | `anthropic` | empty | empty (defaults to `claude-opus-5-5`) |
| OpenAI | `openai` | `https://api.openai.com/v1` | a vision model of your choice |
| Google Gemini | `openai` | `https://generativelanguage.googleapis.com/v1beta/openai` | a Gemini model of your choice |
| OpenRouter | `openai` | `https://openrouter.ai/api/v1` | any listed vision model |
| Ollama (local) | `openai` | `http://localhost:11434/v1` | a local vision model |
| Your own server | `openai` | its OpenAI-compatible root | its model id |

### Exchange Rates for Receipts

| Variable | Default | Purpose |
| --- | --- | --- |
| `RATES_URL` | `https://open.er-api.com/v6/latest/USD` | Exchange rates for converting a receipt in another currency into the group's currency. Fetched only when AI is on, at most about twice a day, and cached in the database. Any URL returning the [open ExchangeRate-API](https://www.exchangerate-api.com/docs/free) or [Frankfurter](https://frankfurter.dev) format works, including a self-hosted Frankfurter (`https://<host>/v1/latest?base=USD`). `none` turns conversion off. Only the rate list is downloaded; no user data is sent |

The OpenAI-compatible provider must support JSON-schema response formats and, for receipts, image input. Receipt photos and instructions are sent to the configured provider, so pick one whose data handling you are comfortable with, or run a local model.

## Homelab Backups

Read by the backup service in `deploy/docker/docker-compose.yml`, not by the server. See [Homelab with Docker](HOSTING.md#homelab-with-docker).

| Variable | Default | Purpose |
| --- | --- | --- |
| `BACKUP_KEEP_DAYS` | `14` | Days to keep the nightly database dumps and receipt archives in `deploy/docker/backups/`. Older dumps are deleted after each successful dump, older archives after each successful archive |
