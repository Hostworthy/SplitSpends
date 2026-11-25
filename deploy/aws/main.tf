locals {
  base_url      = var.base_url != "" ? trimsuffix(var.base_url, "/") : "https://${aws_cloudfront_distribution.app.domain_name}"
  adapter_layer = var.lambda_web_adapter_layer != "" ? var.lambda_web_adapter_layer : "arn:aws:lambda:${var.region}:753240598075:layer:LambdaAdapterLayerArm64:30"
  api_paths     = ["/api/*", "/mcp", "/health", "/internal/*"]
}

resource "random_password" "secret_key" {
  length  = 64
  special = false
}

resource "random_password" "jobs_secret" {
  length  = 48
  special = false
}

# ---------------------------------------------------------------- storage

resource "aws_s3_bucket" "receipts" {
  bucket_prefix = "${var.name}-receipts-"
}

resource "aws_s3_bucket" "web" {
  bucket_prefix = "${var.name}-web-"
}

resource "aws_s3_bucket_public_access_block" "private" {
  for_each                = { receipts = aws_s3_bucket.receipts.id, web = aws_s3_bucket.web.id }
  bucket                  = each.value
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "private" {
  for_each = { receipts = aws_s3_bucket.receipts.id, web = aws_s3_bucket.web.id }
  bucket   = each.value
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_ownership_controls" "private" {
  for_each = { receipts = aws_s3_bucket.receipts.id, web = aws_s3_bucket.web.id }
  bucket   = each.value
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "receipts" {
  bucket = aws_s3_bucket.receipts.id
  versioning_configuration {
    status = "Enabled"
  }
}

data "aws_iam_policy_document" "receipts" {
  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.receipts.arn, "${aws_s3_bucket.receipts.arn}/*"]
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "receipts" {
  bucket     = aws_s3_bucket.receipts.id
  policy     = data.aws_iam_policy_document.receipts.json
  depends_on = [aws_s3_bucket_public_access_block.private]
}

data "aws_iam_policy_document" "web" {
  statement {
    sid       = "CloudFrontReadOnly"
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.web.arn}/*"]
    principals {
      type        = "Service"
      identifiers = ["cloudfront.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "AWS:SourceArn"
      values   = [aws_cloudfront_distribution.app.arn]
    }
  }
}

resource "aws_s3_bucket_policy" "web" {
  bucket     = aws_s3_bucket.web.id
  policy     = data.aws_iam_policy_document.web.json
  depends_on = [aws_s3_bucket_public_access_block.private]
}

locals {
  content_types = {
    html = "text/html; charset=utf-8", js = "text/javascript", mjs = "text/javascript", css = "text/css",
    json = "application/json", wasm = "application/wasm", png = "image/png", jpg = "image/jpeg",
    svg  = "image/svg+xml", ico = "image/x-icon", ttf = "font/ttf", otf = "font/otf", woff2 = "font/woff2",
    txt  = "text/plain", map = "application/json", bin = "application/octet-stream", frag = "text/plain",
  }
  web_files = fileset(var.web_dir, "**")
}

resource "aws_s3_object" "web" {
  for_each     = local.web_files
  bucket       = aws_s3_bucket.web.id
  key          = each.value
  source       = "${var.web_dir}/${each.value}"
  etag         = filemd5("${var.web_dir}/${each.value}")
  content_type = lookup(local.content_types, lower(element(split(".", each.value), length(split(".", each.value)) - 1)), "application/octet-stream")
  cache_control = (
    can(regex("(^|/)(index\\.html|flutter_service_worker\\.js|version\\.json|manifest\\.json|flutter_bootstrap\\.js)$", each.value))
    ? "no-cache" : "public, max-age=604800"
  )
}

# ---------------------------------------------------------------- lambda

data "archive_file" "lambda" {
  type        = "zip"
  source_dir  = var.lambda_dir
  output_path = "${path.module}/build/lambda.zip"
}

data "aws_iam_policy_document" "assume_lambda" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name_prefix        = "${var.name}-"
  assume_role_policy = data.aws_iam_policy_document.assume_lambda.json
}

resource "aws_iam_role_policy_attachment" "logs" {
  role       = aws_iam_role.lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

data "aws_iam_policy_document" "receipts_access" {
  statement {
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["${aws_s3_bucket.receipts.arn}/*"]
  }
}

resource "aws_iam_role_policy" "receipts" {
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.receipts_access.json
}

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${var.name}"
  retention_in_days = 30
}

resource "aws_lambda_function" "app" {
  function_name    = var.name
  role             = aws_iam_role.lambda.arn
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  handler          = "run.sh"
  filename         = data.archive_file.lambda.output_path
  source_code_hash = data.archive_file.lambda.output_base64sha256
  memory_size      = 512
  timeout          = 30
  layers           = [local.adapter_layer]

  environment {
    variables = {
      AWS_LAMBDA_EXEC_WRAPPER      = "/opt/bootstrap"
      AWS_LWA_PORT                 = "8080"
      AWS_LWA_READINESS_CHECK_PATH = "/health"
      ADDR                         = "127.0.0.1:8080"
      SERVERLESS                   = "true"
      DATABASE_URL                 = var.database_url
      DATABASE_MAX_CONNS           = "3"
      SECRET_KEY                   = random_password.secret_key.result
      JOBS_SECRET                  = random_password.jobs_secret.result
      BASE_URL                     = local.base_url
      TRUST_PROXY                  = "2"
      SIGNUP_MODE                  = var.signup_mode
      SIGNUP_ALLOWED_DOMAINS       = var.signup_allowed_domains
      ADMIN_EMAIL                  = var.admin_email
      BLOB_STORE                   = "s3"
      S3_BUCKET                    = aws_s3_bucket.receipts.id
      S3_REGION                    = var.region
      SMTP_HOST                    = var.smtp.host
      SMTP_PORT                    = tostring(var.smtp.port)
      SMTP_SECURITY                = var.smtp.security
      SMTP_USERNAME                = var.smtp.username
      SMTP_PASSWORD                = var.smtp.password
      SMTP_FROM                    = var.smtp.from
      AI_PROVIDER                  = var.ai.provider
      AI_API_KEY                   = var.ai.api_key
      AI_MODEL                     = var.ai.model
      AI_BASE_URL                  = var.ai.base_url
      AI_EFFORT                    = var.ai.effort
    }
  }

  depends_on = [aws_cloudwatch_log_group.lambda, aws_iam_role_policy_attachment.logs]
}

# API Gateway is CloudFront's origin for the server. Auth is enforced by the
# application with bearer tokens. (A Lambda function URL would create a
# dependency cycle: the function needs the CloudFront domain for BASE_URL.)
resource "aws_apigatewayv2_api" "app" {
  name          = var.name
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_integration" "app" {
  api_id                 = aws_apigatewayv2_api.app.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.app.invoke_arn
  payload_format_version = "2.0"
  timeout_milliseconds   = 30000
}

resource "aws_apigatewayv2_route" "app" {
  api_id    = aws_apigatewayv2_api.app.id
  route_key = "$default"
  target    = "integrations/${aws_apigatewayv2_integration.app.id}"
}

resource "aws_apigatewayv2_stage" "app" {
  api_id      = aws_apigatewayv2_api.app.id
  name        = "$default"
  auto_deploy = true
  default_route_settings {
    throttling_burst_limit = 100
    throttling_rate_limit  = 50
  }
}

resource "aws_lambda_permission" "apigw" {
  statement_id  = "AllowAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.app.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.app.execution_arn}/*/*"
}

# ---------------------------------------------------------------- cdn

resource "aws_cloudfront_origin_access_control" "web" {
  name                              = "${var.name}-web"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

# Serve index.html for app routes such as /groups/123 without touching API 404s.
resource "aws_cloudfront_function" "spa" {
  name    = "${var.name}-spa"
  runtime = "cloudfront-js-2.0"
  publish = true
  code    = <<-JS
    function handler(event) {
      var req = event.request;
      var last = req.uri.split('/').pop();
      if (req.uri === '/' || last.indexOf('.') === -1) { req.uri = '/index.html'; }
      return req;
    }
  JS
}

locals {
  managed = {
    caching_disabled  = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"
    caching_optimized = "658327ea-f89d-4fab-a63d-7e88639e58f6"
    all_except_host   = "b689b0a8-53d0-40ab-baf2-68738e2966ac"
    security_headers  = "67f7725c-6f97-4210-82d7-5512b31e9d03"
  }
}

resource "aws_cloudfront_distribution" "app" {
  enabled             = true
  comment             = var.name
  default_root_object = "index.html"
  price_class         = "PriceClass_100"
  http_version        = "http2and3"

  origin {
    origin_id                = "web"
    domain_name              = aws_s3_bucket.web.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.web.id
  }

  origin {
    origin_id   = "api"
    domain_name = replace(aws_apigatewayv2_api.app.api_endpoint, "https://", "")
    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
      origin_read_timeout    = 30
    }
  }

  default_cache_behavior {
    target_origin_id           = "web"
    viewer_protocol_policy     = "redirect-to-https"
    allowed_methods            = ["GET", "HEAD", "OPTIONS"]
    cached_methods             = ["GET", "HEAD"]
    compress                   = true
    cache_policy_id            = local.managed.caching_optimized
    response_headers_policy_id = local.managed.security_headers
    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.spa.arn
    }
  }

  dynamic "ordered_cache_behavior" {
    for_each = local.api_paths
    content {
      path_pattern               = ordered_cache_behavior.value
      target_origin_id           = "api"
      viewer_protocol_policy     = "redirect-to-https"
      allowed_methods            = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
      cached_methods             = ["GET", "HEAD"]
      compress                   = true
      cache_policy_id            = local.managed.caching_disabled
      origin_request_policy_id   = local.managed.all_except_host
      response_headers_policy_id = local.managed.security_headers
    }
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
    minimum_protocol_version       = "TLSv1.2_2021"
  }
}

# ---------------------------------------------------------------- jobs

resource "aws_cloudwatch_event_connection" "jobs" {
  name               = "${var.name}-jobs"
  authorization_type = "API_KEY"
  auth_parameters {
    api_key {
      key   = "Authorization"
      value = "Bearer ${random_password.jobs_secret.result}"
    }
  }
}

resource "aws_cloudwatch_event_api_destination" "jobs" {
  name                             = "${var.name}-jobs"
  connection_arn                   = aws_cloudwatch_event_connection.jobs.arn
  invocation_endpoint              = "${aws_apigatewayv2_api.app.api_endpoint}/internal/jobs"
  http_method                      = "POST"
  invocation_rate_limit_per_second = 1
}

resource "aws_cloudwatch_event_rule" "jobs" {
  name                = "${var.name}-jobs"
  schedule_expression = var.jobs_schedule
}

data "aws_iam_policy_document" "assume_events" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "events" {
  name_prefix        = "${var.name}-events-"
  assume_role_policy = data.aws_iam_policy_document.assume_events.json
}

resource "aws_iam_role_policy" "events" {
  role = aws_iam_role.events.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = "events:InvokeApiDestination", Resource = aws_cloudwatch_event_api_destination.jobs.arn }]
  })
}

resource "aws_cloudwatch_event_target" "jobs" {
  rule     = aws_cloudwatch_event_rule.jobs.name
  arn      = aws_cloudwatch_event_api_destination.jobs.arn
  role_arn = aws_iam_role.events.arn
}
