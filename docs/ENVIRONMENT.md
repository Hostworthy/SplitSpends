# Environment Variables

The server reads everything from the environment. Invalid or missing required values stop startup with a dotted error key such as `bootstrap.config.env.required.not-found.DATABASE_URL`.

## Core

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `DATABASE_URL` | required | PostgreSQL URL, for example `postgres://user:pass@host:5432/splitspends?sslmode=disable` |
| `SECRET_KEY` | required | At least 32 characters; hex is decoded. Signs login codes. Generate with `openssl rand -hex 32` |
| `BASE_URL` | `http://localhost:8080` | Public URL, used in email links |
| `ADDR` | `:8080` | Listen address |
| `APP_NAME` | `SplitSpends` | Name shown in emails and the app |
| `WEB_DIR` | empty | Directory with the Flutter web build; the Docker image sets `/srv/web` |
| `DATA_DIR` | `./data` | Where receipt images are stored |
| `DATABASE_MAX_CONNS` | `10` | Connection pool size |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `CORS_ORIGINS` | empty | Comma-separated browser origins allowed to call the API from elsewhere, or `*` |
| `TRUST_PROXY` | `false` | Use `X-Forwarded-For` for client IPs (rate limits) behind a reverse proxy |
| `MCP_ENABLED` | `true` | Serve the MCP endpoint at `/mcp` |

## Serverless and Storage

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `SERVERLESS` | `false` | Set on Lambda, Cloud Run and similar: no in-process background jobs, emails are sent before the response returns |
| `JOBS_SECRET` | empty | At least 32 characters. Enables `POST /internal/jobs` for an external scheduler (needed when `SERVERLESS=true`, every 15 to 60 minutes) |
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
| `SIGNUP_MODE` | `invite` | `open`: anyone. `invite`: the first account, plus anyone an existing user added as a friend or group member. `domain`: invited users plus emails from `SIGNUP_ALLOWED_DOMAINS` |
| `SIGNUP_ALLOWED_DOMAINS` | empty | Comma-separated domains for `SIGNUP_MODE=domain` |
| `SESSION_TTL_DAYS` | `90` | How long app sessions last. API tokens do not expire until revoked |

Login codes are 6 digits, valid for 10 minutes, allow 5 attempts, and at most 5 codes per email per hour. Requests for unknown emails in `invite` mode get the same response as real ones and send nothing.

## Email

Leave `SMTP_HOST` empty in development: emails, including login codes, are written to the server log.

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `SMTP_HOST` | empty | SMTP server |
| `SMTP_PORT` | `587` | Port |
| `SMTP_SECURITY` | `starttls` | `starttls` (port 587), `tls` (implicit TLS, port 465) or `none` (local relays only) |
| `SMTP_USERNAME` | empty | Login, if the server needs one |
| `SMTP_PASSWORD` | empty | Password or app password |
| `SMTP_FROM` | `SMTP_USERNAME` | From address, for example `SplitSpends <split@example.com>` |

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

## AI (Optional)

Receipt scanning and natural-language drafts are off unless a provider is configured. The app hides AI buttons when it is off.

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `AI_PROVIDER` | empty | `anthropic`, `openai` (any OpenAI-compatible server) or `none` |
| `AI_API_KEY` | empty | Provider key. Required for `anthropic` |
| `AI_MODEL` | `claude-opus-5-5` for `anthropic` | Model id. Required for `openai` |
| `AI_BASE_URL` | empty | For `openai`: the API root before `/chat/completions`, such as `http://localhost:11434/v1` (Ollama) or `https://openrouter.ai/api/v1` |
| `AI_EFFORT` | model default | Anthropic only: `low`, `medium`, `high`, `xhigh`, `max` |

The OpenAI-compatible provider must support JSON-schema response formats and, for receipts, image input (for example a vision model in Ollama). Receipt images are sent to the configured provider, so pick one whose data handling you are comfortable with.
