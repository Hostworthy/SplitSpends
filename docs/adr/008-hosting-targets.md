# ADR-008: Hosting Targets

## Status

Accepted

## Context

Self-hosters range from homelabs to people who want a free cloud setup. One binary should run everywhere.

## Decision

- The same server binary runs in Docker (homelab, VPS, Render, Cloud Run) and on AWS Lambda through the Lambda Web Adapter. No platform-specific handlers.
- `SERVERLESS=true` turns off in-process background work (jobs, queued emails); a scheduler calls `POST /internal/jobs` with `JOBS_SECRET`.
- Migrations take a PostgreSQL advisory lock so concurrent cold starts are safe.
- Receipt storage is a port with three adapters: local disk, S3-compatible buckets (AWS S3, R2, MinIO, GCS interop) and PostgreSQL (no extra service on free tiers).
- Terraform for AWS and GCP, a Render blueprint, and Docker Compose files for homelabs ship in `deploy/`. Each keeps buckets private, uses HTTPS, scopes IAM to what the app needs and keeps secrets out of the repository.
- PostgreSQL is the only database (see ADR-005 for why money stays relational).

## Consequences

### Positive

- Free options exist (local-only, Render, Cloud Run or Lambda with Neon), and paid ones scale without code changes.

### Negative

- Serverless rate limits are per instance; per-email limits in the database still apply.
