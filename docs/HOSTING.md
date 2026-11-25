# Hosting Your Own Server

## Do You Need a Server?

No. The Split Spends app works on its own: choose **Use on this device** and everything stays on your phone or in your browser. Nothing is sent anywhere.

A server is only needed to:

- use the same data on several devices (phone and laptop),
- share groups and balances with other people, so they see the same numbers,
- send email notifications and invites,
- use AI receipt scanning,
- connect an AI assistant through MCP.

When you connect a server later, the app offers to upload what you already recorded on the device.

## What a Server Is

Split Spends is self-hosted: there is no central Split Spends service. You (or a friend, a family member, a club) run one server, and everyone who shares expenses connects to it. Signing in needs three things:

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

This runs PostgreSQL, Split Spends, [Caddy](https://caddyserver.com) for automatic HTTPS, and a nightly backup into `deploy/docker/backups/`: a `pg_dump` of the database (`splitspends-<date>-<time>.dump`) and an archive of the receipts kept in the `data` volume (`receipts-<date>-<time>.tar.gz`). Backups older than `BACKUP_KEEP_DAYS` (14 by default) are deleted, dumps after each successful dump and receipt archives after each successful archive, so a run that fails never removes the last good copy. A failed run leaves no file and logs `backup.dump.failed` or `backup.receipts.failed` (`docker compose logs backup`). Your `.env` and Caddy's certificates are not in the backup; keep a copy of `.env` somewhere safe.

The backups hold every account's email address and all expenses, readable only by root on the host. Copy them off the machine regularly (with `sudo`), and encrypt the copies.

#### Restore

Run these in `deploy/docker`, with the file names of the backup you want:

```bash
docker compose stop app backup
# Recreate the database from the dump, in one transaction.
docker compose run --rm --no-deps backup 'set -e
  dropdb -h db -U splitspends --force splitspends
  createdb -h db -U splitspends splitspends
  pg_restore -h db -U splitspends -d splitspends --single-transaction /backups/splitspends-2026-01-31-0300.dump'
# Replace the receipts. The volume is named <folder>_data; `docker volume ls` shows it.
docker run --rm -v docker_data:/data -v "$PWD/backups:/backups:ro" postgres:16-alpine \
  sh -c 'find /data -mindepth 1 -delete && tar -xzf /backups/receipts-2026-01-31-0300.tar.gz -C /data'
docker compose up -d
```

Keep the app stopped until the restore has finished without errors: it runs migrations when it starts, and they must not run against a half-restored database. If `pg_restore` fails, the database is left empty; fix the cause and run the restore again before starting the app. On a new machine, start only the database first (`docker compose up -d db`) and use the same `.env` as before.

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
- Keep `SIGNUP_MODE=invite` unless you really want strangers to sign up. `approval` lets anyone ask while you decide who gets in, and `closed` stops new accounts. You can change this later in the app (Account, Administration).
- Behind a proxy or CDN, set `TRUST_PROXY` to the number of proxies in front of the server (`1` for a single reverse proxy; the shipped Docker, Cloud Run, Render and AWS setups already set it) so rate limits see real client addresses; leave it `false` when the server is directly exposed. See [Client Addresses](ENVIRONMENT.md#client-addresses).
- Rate limits on sign-in and invite links are counted in each server instance's memory. Cloud Run and Lambda run several instances and start empty after idling, so there the limits apply per instance and are looser. Each sign-in code still stops working after a few wrong attempts.
- Back up the database, and the receipts if they are not in it: they live in `DATA_DIR`, S3 or the database, depending on `BLOB_STORE`. The homelab stack backs up both (see [Restore](#restore)). The other setups back up nothing themselves: use your database provider's backups, and back up `DATA_DIR` or the S3 bucket when receipts live there.
- Update regularly: pull the latest code and rebuild. Migrations run automatically on start.
- If you enable AI, receipt images and expense text are sent to that provider. Choose one you trust, or run a local model through an OpenAI-compatible server.
- The web app makes no third-party requests. Its engine, its Latin font and the Noto fallback fonts for emoji and other scripts are all in the repository and served by your own server (see [apps/web/README.md](../apps/web/README.md)).
- Terraform state contains your secrets. Keep it in an encrypted, private backend.
