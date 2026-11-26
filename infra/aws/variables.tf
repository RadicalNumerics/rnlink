variable "account_id" { type = string }
variable "region" {
  type    = string
  default = "us-west-2"
}
variable "vpc_id" { type = string }
variable "subnet_id" { type = string }
variable "bucket_name" { type = string }
variable "instance_type" {
  type    = string
  default = "t4g.small"
  validation {
    condition     = contains(["t4g.micro", "t4g.small", "t4g.medium"], var.instance_type)
    error_message = "Use an ARM t4g instance to match the release image."
  }
}
variable "image_tag" {
  type = string
  validation {
    condition     = can(regex("^[a-f0-9]{7,40}$", var.image_tag))
    error_message = "Use the immutable Git commit tag published to ECR."
  }
}

variable "ami_id" {
  type        = string
  description = "Pinned Amazon Linux 2023 ARM64 AMI; update deliberately."
}

variable "disk_snapshots_enabled" {
  type        = bool
  default     = true
  description = "Disable only after validating a compact backup restore."
}
