terraform {
  required_version = ">= 1.10, < 2.0"
  required_providers {
    archive = { source = "hashicorp/archive", version = "~> 2.7" }
    aws     = { source = "hashicorp/aws", version = "~> 6.0" }
  }
  backend "s3" {}
}
provider "aws" {
  region              = var.region
  allowed_account_ids = [var.account_id]
  default_tags { tags = { Project = "rnlink", ManagedBy = "Terraform" } }
}
