# Hosting Your Own Server

## Do You Need a Server?

No. The SplitSpends app works on its own: choose **Use on this device** and everything stays on your phone or in your browser. Nothing is sent anywhere.

A server is only needed to:

- use the same data on several devices (phone and laptop),
- share groups and balances with other people, so they see the same numbers,
- send email notifications and invites,
- use AI receipt scanning,
- connect an AI assistant through MCP.

When you connect a server later, the app offers to upload what you already recorded on the device.

## What a Server Is

SplitSpends is self-hosted: there is no central SplitSpends service. You (or a friend, a family member, a club) run one server, and everyone who shares expenses connects to it. Signing in needs three things:

1. **The server URL**, for example `https://split.example.com`. The person who runs the server gives it to you.
2. **Your email address.** You get a 6-digit sign-in code by email. There is no password to remember.
3. **An invite, usually.** Most servers only let in people an existing member has added as a friend or to a group. If the server says nothing was sent, ask whoever runs it to add you.

## Choosing Where to Run It

| Option | Monthly cost | Effort | Good for |
| ------ | ------------ | ------ | -------- |
| [Homelab with Docker](#homelab-with-docker) | Free (your hardware) | Low | You already run a home server or a NAS |
| [Any VPS with Docker](#any-vps-with-docker) | About 4 to 6 USD | Low | Simple, always on, full control |
| [Render (one click)](#render-one-click) | Free, sleeps when idle | Lowest | Trying it out |
| [Google Cloud Run](#google-cloud-run) | Free tier, plus database | Medium | Low traffic, scales to zero |
| [AWS Lambda](#aws-lambda) | Free tier, plus database | Medium | You already use AWS |

Free PostgreSQL databases that work with every option: [Neon](https://neon.tech) (free tier, does not expire) and [Supabase](https://supabase.com) (free tier, pauses after a week without traffic). Use the connection string with `sslmode=require`.

Every option needs an email account that can send mail over SMTP (for example Gmail with an app password). See [ENVIRONMENT.md](ENVIRONMENT.md#email).

### Homelab with Docker

Requires Docker, a domain name pointing at your home IP, and ports 80 and 443 forwarded to the machine.

```bash
git clone https://github.com/Hostworthy/SplitSpends && cd SplitSpends/deploy/docker
cp ../.env.example .env          # set DOMAIN, BASE_URL=https://DOMAIN, POSTGRES_PASSWORD, SECRET_KEY, SMTP
docker compose up -d --build
```

This runs PostgreSQL, SplitSpends, [Caddy](https://caddyserver.com) for automatic HTTPS, and a nightly `pg_dump` into `deploy/docker/backups/`. Copy those backups off the machine now and then, and delete old ones when you no longer need them.

LAN or VPN only (no public domain): skip Caddy and use `deploy/docker-compose.yml`, then reach the server at `http://<machine-ip>:8080` over your LAN or a VPN such as Tailscale or WireGuard. Browsers and phones treat plain HTTP as insecure, so prefer a VPN with HTTPS (for example Tailscale's built-in certificates) over exposing port 8080.

### Any VPS with Docker

The same as the homelab, on a small Linux VPS from any provider (1 vCPU and 1 GB RAM is plenty). Point your domain at the VPS and follow the homelab steps.

### Render (One Click)

[Deploy to Render](https://render.com/deploy?repo=https://github.com/Hostworthy/SplitSpends) uses [`render.yaml`](../render.yaml): a free web service and a free database. After it deploys, set `BASE_URL` to the service URL and fill in the SMTP variables in the Render dashboard.

Limits: free services sleep after 15 minutes without traffic (the first request takes about a minute), and Render's free database is deleted after 30 days. For anything beyond a trial, replace it with a Neon database: remove the database from the blueprint and set `DATABASE_URL`.

### Google Cloud Run

Terraform in [`deploy/gcp`](../deploy/gcp/README.md). Cloud Run's free tier covers personal use; the container scales to zero when idle. Pair it with a free Neon database, or set `create_database = true` for Cloud SQL (about 10 USD a month). Secrets live in Secret Manager; receipts are stored in PostgreSQL.

### AWS Lambda

Terraform in [`deploy/aws`](../deploy/aws/README.md): Lambda (arm64) running the same server through the AWS Lambda Web Adapter, API Gateway, CloudFront for HTTPS and the web app, private S3 buckets for the web app and receipts, and EventBridge for scheduled jobs. Use a Neon or Supabase database (Lambda outside a VPC cannot reach a private RDS instance without a NAT gateway, which costs about 32 USD a month).

## Push Notifications

Push works out of the box, with nothing to configure and no Google or Apple account:

- **Web app:** open Account and tap **Enable push notifications**, then allow notifications when the browser asks. The app must be served over HTTPS (or `localhost`). On iPhone and iPad, add the web app to the Home Screen first and enable push from there.
- **Android app:** install a UnifiedPush distributor, for example [ntfy](https://ntfy.sh) from F-Droid or Google Play, and open it once. Then tap **Enable push notifications** in Account and pick the distributor. [Other distributors](https://unifiedpush.org/users/distributors/) work too, including one that relays through Google's push service if you prefer.
- **iPhone app:** not available yet.

The server creates its push key on first start and keeps it in the database. If you run your own ntfy server on the local network, set `PUSH_ALLOW_PRIVATE=true`. Details are in [ENVIRONMENT.md](ENVIRONMENT.md#push-notifications).

## Security Checklist

- Always serve over HTTPS. Every option above does, except plain LAN setups.
- Set a long random `SECRET_KEY` (`openssl rand -hex 32`) and keep it secret.
- Keep `SIGNUP_MODE=invite` unless you really want strangers to sign up.
- Behind a proxy or CDN, set `TRUST_PROXY=true` so rate limits see real client IPs; never set it when the server is directly exposed.
- Back up the database. Receipts live in `DATA_DIR`, S3 or the database, depending on `BLOB_STORE`.
- Update regularly: pull the latest code and rebuild. Migrations run automatically on start.
- If you enable AI, receipt images and expense text are sent to that provider. Choose one you trust, or run a local model through an OpenAI-compatible server.
- The web app makes no third-party requests. Its engine, its Latin font and the Noto fallback fonts for emoji and other scripts are all in the repository and served by your own server (see [apps/web/README.md](../apps/web/README.md)).
- Terraform state contains your secrets. Keep it in an encrypted, private backend.
