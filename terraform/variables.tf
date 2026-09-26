# ---- General -----------------------------------------------------------------

variable "aws_region" {
  description = "AWS region to deploy into."
  type        = string
  default     = "ap-south-1"
}

variable "project_name" {
  description = "Name prefix for every resource."
  type        = string
  default     = "currency-watcher"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,30}$", var.project_name))
    error_message = "project_name must be lowercase letters, digits and hyphens (2-31 chars)."
  }
}

variable "environment" {
  description = "Deployment environment name, used in tags and resource names."
  type        = string
  default     = "prod"
}

# ---- Networking ----------------------------------------------------------------

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "public_subnet_cidr" {
  description = "CIDR block for the public subnet that hosts the instance."
  type        = string
  default     = "10.20.1.0/24"
}

variable "allowed_ingress_cidrs" {
  description = "IPv4 CIDRs allowed to reach the API on host_port."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

# ---- Compute -------------------------------------------------------------------

variable "instance_type" {
  description = "EC2 instance type. Must be x86_64 to match the image built by CI (linux/amd64)."
  type        = string
  default     = "t3.micro"
}

variable "root_volume_size_gb" {
  description = "Size of the instance's root volume in GiB (the Amazon Linux 2023 image needs at least 8)."
  type        = number
  default     = 8

  validation {
    condition     = var.root_volume_size_gb >= 8
    error_message = "root_volume_size_gb must be at least 8."
  }
}

# ---- Application ---------------------------------------------------------------

variable "app_port" {
  description = "Port the Go API listens on inside the container."
  type        = number
  default     = 8080
}

variable "host_port" {
  description = "Public port on the instance that forwards to app_port."
  type        = number
  default     = 80
}

variable "allowed_origins" {
  description = "Origins allowed by the API's CORS policy (the React app's URL)."
  type        = list(string)
  default     = ["http://localhost:5173"]
}

variable "cache_ttl" {
  description = "How long the API caches exchange rates, as a Go duration (e.g. 1h, 30m)."
  type        = string
  default     = "1h"
}

variable "ecr_images_to_keep" {
  description = "Number of most recent images kept in ECR; older ones are expired."
  type        = number
  default     = 10
}

# ---- CI/CD (GitHub Actions) -------------------------------------------------------

variable "github_repository" {
  description = "GitHub repository allowed to deploy, as owner/name."
  type        = string
  default     = "2103Sanjay/currency-watcher-be"

  validation {
    condition     = can(regex("^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$", var.github_repository))
    error_message = "github_repository must look like owner/name."
  }
}

# GitHub puts these immutable numeric IDs into the OIDC token subject, e.g.
# repo:owner@<owner_id>/name@<repository_id>:environment:production, so the
# role can't be taken over by someone re-registering a renamed repo's name.
# Look them up with:
#   curl https://api.github.com/repos/<owner>/<name>   (fields "owner.id" and "id")
variable "github_owner_id" {
  description = "Numeric ID of the GitHub repository owner (user or organisation)."
  type        = number
  default     = 133321022
}

variable "github_repository_id" {
  description = "Numeric ID of the GitHub repository."
  type        = number
  default     = 1388870848
}

variable "github_environment" {
  description = "GitHub Actions environment whose jobs may assume the deploy role."
  type        = string
  default     = "production"
}

variable "github_oidc_provider_arn" {
  description = "ARN of an existing GitHub OIDC provider in the account. Leave null to create one (an account can only have one)."
  type        = string
  default     = null
}
