output "registry" {
  description = "Push the image here, then set var.image and apply again."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.images.repository_id}"
}

output "url" {
  value = local.base_url
}

output "mcp_url" {
  value = "${local.base_url}/mcp"
}
