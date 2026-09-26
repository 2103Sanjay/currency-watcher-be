output "api_url" {
  description = "Public URL of the Go API."
  value       = "http://${aws_eip.api.public_ip}${var.host_port == 80 ? "" : ":${var.host_port}"}"
}

output "health_check_url" {
  description = "Health endpoint; returns the deployed commit SHA."
  value       = "http://${aws_eip.api.public_ip}${var.host_port == 80 ? "" : ":${var.host_port}"}/api/health"
}

output "ec2_instance_id" {
  description = "ID of the EC2 instance running the API."
  value       = aws_instance.api.id
}

output "ecr_repository_url" {
  description = "Full URL of the ECR repository."
  value       = aws_ecr_repository.api.repository_url
}

output "github_deploy_role_arn" {
  description = "IAM role assumed by GitHub Actions through OIDC."
  value       = aws_iam_role.github_deploy.arn
}

output "github_actions_variables" {
  description = "Values to add under GitHub → Settings → Secrets and variables → Actions → Variables."
  value = {
    AWS_REGION      = var.aws_region
    AWS_ROLE_ARN    = aws_iam_role.github_deploy.arn
    ECR_REPOSITORY  = aws_ecr_repository.api.name
    EC2_INSTANCE_ID = aws_instance.api.id
    API_URL         = "http://${aws_eip.api.public_ip}${var.host_port == 80 ? "" : ":${var.host_port}"}"
  }
}
