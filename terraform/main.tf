provider "aws" {
  region = var.aws_region

  # Applied to every resource, so costs and ownership are easy to trace.
  default_tags {
    tags = {
      Project     = var.project_name
      Environment = var.environment
      ManagedBy   = "terraform"
      Repository  = var.github_repository
    }
  }
}

locals {
  name = "${var.project_name}-${var.environment}"
}

data "aws_caller_identity" "current" {}

data "aws_availability_zones" "available" {
  state = "available"
}

# Latest Amazon Linux 2023 (x86_64) image, published by AWS in SSM Parameter
# Store. It comes with the SSM agent, AWS CLI and curl preinstalled.
data "aws_ssm_parameter" "al2023_ami" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
}
