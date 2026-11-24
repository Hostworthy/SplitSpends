output "url" {
  description = "Open this in a browser and enter it as the server URL in the app."
  value       = local.base_url
}

output "mcp_url" {
  value = "${local.base_url}/mcp"
}

output "receipts_bucket" {
  value = aws_s3_bucket.receipts.id
}
