terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # State is kept locally (terraform.tfstate, git-ignored) for simplicity.
  # For a team, switch to a remote backend such as S3 with state locking.
}
