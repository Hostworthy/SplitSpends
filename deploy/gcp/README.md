# Google Cloud Deployment (Cloud Run)

```text
Browser / app ──HTTPS──> Cloud Run (Go server + web app, scales to zero)
Cloud Run ──> PostgreSQL (Neon free tier, or Cloud SQL via the IAM connector)
Cloud Scheduler (every 30 min) ──> /internal/jobs
Secret Manager ──> database URL, SECRET_KEY, SMTP and AI keys
```

## Deploy

```bash
cd deploy/gcp
cp terraform.tfvars.example terraform.tfvars   # fill in project_id, database_url, smtp
cp backend.tf.example backend.tf               # fill in your state bucket (see below)
terraform init
terraform apply                                # first run: APIs, registry, secrets (no service yet)

# Build and push the image from the repository root
REGISTRY=$(terraform output -raw registry)
cd ../.. && gcloud builds submit --tag "$REGISTRY/app:v1" .

# Set image = "<registry>/app:v1" in terraform.tfvars, then
cd deploy/gcp && terraform apply
```

Terraform state holds every secret of the stack in plain text, so keep it in a private Cloud Storage bucket rather than in `terraform.tfstate` on your machine: create a bucket with uniform access and object versioning (`gcloud storage buckets create gs://NAME --uniform-bucket-level-access` and `gcloud storage buckets update gs://NAME --versioning`), and name it in `backend.tf` before the first `terraform init`. Terraform locks the state while it runs. If you already applied with local state, add `backend.tf` and run `terraform init -migrate-state` to move it, then delete the local `terraform.tfstate` files.

Open the `url` output. That is also the server URL for the mobile app. To update, build a new tag, change `image` and apply.

## Notes

- **Free:** Cloud Run's free tier covers personal use in `us-central1`, `us-east1` or `us-west1`. With `create_database = true`, Cloud SQL costs about 10 USD a month; it accepts connections only through the IAM-authenticated Cloud SQL connector.
- **Receipts** are stored in PostgreSQL (`BLOB_STORE=postgres`), so no bucket is needed.
- **Secrets** are read from Secret Manager by the service account at runtime. They are also in Terraform state, which is why the Deploy steps keep it in Cloud Storage.
- **Custom domain:** map it with Cloud Run domain mappings or a load balancer, then set `base_url`.
