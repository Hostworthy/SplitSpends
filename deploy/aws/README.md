# AWS Deployment (Lambda)

```text
Browser / app ──HTTPS──> CloudFront ──/api, /mcp, /health──> API Gateway ──> Lambda (Go server + Web Adapter)
                              └──────── everything else ──> S3 (web app, private, OAC)
Lambda ──> S3 receipts bucket (private, encrypted, versioned)
Lambda ──> PostgreSQL (Neon, Supabase, or your own)
EventBridge (every 30 min) ──> /internal/jobs (recurring expenses, cleanup)
```

## Prerequisites

- Terraform 1.6+, AWS credentials with permission to create the resources above
- Go and Flutter (to build), or CI that builds for you
- A PostgreSQL URL with `sslmode=require` (a free [Neon](https://neon.tech) database works well)
- SMTP credentials

## Deploy

```bash
make lambda web-build                  # from the repository root
cd deploy/aws
cp terraform.tfvars.example terraform.tfvars   # fill in
terraform init
terraform apply
```

Open the `url` output. That is also the server URL for the mobile app. Redeploy after changes with `make lambda web-build` and `terraform apply`.

## Notes

- **Cost:** Lambda, API Gateway and CloudFront free tiers cover personal use. EventBridge API destinations store the jobs secret in Secrets Manager (about 0.40 USD a month).
- **Timeouts:** API Gateway allows 30 seconds per request, so AI receipt scans run with `AI_EFFORT=low` here (`effort` in the `ai` variable).
- **Rate limits** on sign-in are per Lambda instance; per-email code limits are enforced in the database regardless.
- **Database connections:** every concurrent request runs in its own Lambda execution environment with its own pool of up to 3 connections (`DATABASE_MAX_CONNS`), limited only by the API Gateway throttle (50 requests a second, bursts of 100) and your account's concurrency quota. If your database allows few connections, as free tiers often do, set `max_concurrency` so that `max_concurrency` x 3 stays below that limit; requests beyond it are turned away instead of exhausting the database for everyone. AWS keeps at least 10 executions unreserved per account, so an account whose concurrency quota is 10 (common on new accounts) cannot reserve any: leave `max_concurrency` unset there, or ask AWS to raise the quota.
- **Secrets** (database URL, SMTP and AI keys, generated `SECRET_KEY`) are Lambda environment variables, encrypted at rest by AWS. They are also in Terraform state: use an encrypted remote backend such as S3 with a DynamoDB lock table.
- **Custom domain:** add an ACM certificate and an alias to the CloudFront distribution, then set `base_url`.
- **Layer:** `lambda_web_adapter_layer` defaults to the public AWS Lambda Web Adapter arm64 layer; override it if your region needs a different ARN.
