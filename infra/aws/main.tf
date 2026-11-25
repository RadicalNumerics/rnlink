data "aws_subnet" "selected" { id = var.subnet_id }
# Bootstrap this bucket once with the documented CLI commands, then import it.
resource "aws_s3_bucket" "state" {
  bucket = var.bucket_name
  lifecycle { prevent_destroy = true }
}
resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id
  versioning_configuration { status = "Enabled" }
}
resource "aws_s3_bucket_public_access_block" "state" {
  bucket                  = aws_s3_bucket.state.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id
  rule {
    apply_server_side_encryption_by_default { sse_algorithm = "AES256" }
  }
}
resource "aws_s3_bucket_policy" "state" {
  depends_on = [aws_s3_bucket_public_access_block.state]
  bucket     = aws_s3_bucket.state.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Sid       = "RequireTLS", Effect = "Deny", Principal = "*", Action = "s3:*",
    Resource  = [aws_s3_bucket.state.arn, "${aws_s3_bucket.state.arn}/*"],
    Condition = { Bool = { "aws:SecureTransport" = "false" } }
  }] })
}
resource "aws_s3_bucket_lifecycle_configuration" "transfer" {
  bucket = aws_s3_bucket.state.id
  rule {
    id     = "RetainThirtyDaysOfDatabaseBackups"
    status = "Enabled"
    filter { prefix = "backups/" }
    expiration { days = 30 }
    noncurrent_version_expiration { noncurrent_days = 30 }
  }
  rule {
    id     = "ExpireMigrationArchives"
    status = "Enabled"
    filter { prefix = "transfer/" }
    expiration { days = 1 }
    noncurrent_version_expiration { noncurrent_days = 1 }
    abort_incomplete_multipart_upload { days_after_initiation = 1 }
  }
}
resource "aws_ecr_repository" "rnlink" {
  name                 = "rnlink"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration { encryption_type = "AES256" }
}
resource "aws_security_group" "rnlink" {
  name_prefix = "rnlink-"
  description = "RN link service: outbound only; tailnet and SSM access"
  vpc_id      = var.vpc_id
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_iam_role" "rnlink" {
  name = "rnlink-instance"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole"
  }] })
}
resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.rnlink.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}
resource "aws_iam_role_policy" "artifacts" {
  role = aws_iam_role.rnlink.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*" },
    { Effect = "Allow", Action = ["ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage"], Resource = aws_ecr_repository.rnlink.arn },
    { Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.state.arn}/transfer/*" },
    { Effect = "Allow", Action = ["s3:PutObject"], Resource = "${aws_s3_bucket.state.arn}/backups/*" }
  ] })
}
resource "aws_iam_instance_profile" "rnlink" { role = aws_iam_role.rnlink.name }
resource "aws_ebs_volume" "data" {
  availability_zone = data.aws_subnet.selected.availability_zone
  size              = 8
  type              = "gp3"
  encrypted         = true
  tags              = { Name = "rnlink-data" }
  lifecycle { prevent_destroy = true }
}
resource "aws_instance" "rnlink" {
  ami                         = var.ami_id
  instance_type               = var.instance_type
  subnet_id                   = var.subnet_id
  associate_public_ip_address = true
  vpc_security_group_ids      = [aws_security_group.rnlink.id]
  iam_instance_profile        = aws_iam_instance_profile.rnlink.name
  user_data_replace_on_change = true
  user_data = templatefile("${path.module}/user-data.sh.tftpl", {
    volume_id = replace(aws_ebs_volume.data.id, "-", "")
    region    = var.region
    image     = "${aws_ecr_repository.rnlink.repository_url}:${var.image_tag}"
    registry  = split("/", aws_ecr_repository.rnlink.repository_url)[0]
    bucket    = var.bucket_name
  })
  root_block_device {
    volume_size = 12
    volume_type = "gp3"
    encrypted   = true
  }
  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }
  credit_specification { cpu_credits = "standard" }
  tags = { Name = "rnlink" }
  lifecycle {
    precondition {
      condition     = data.aws_subnet.selected.vpc_id == var.vpc_id
      error_message = "Subnet must belong to the selected VPC."
    }
  }
  depends_on = [aws_iam_role_policy_attachment.ssm, aws_iam_role_policy.artifacts]
}
resource "aws_volume_attachment" "data" {
  stop_instance_before_detaching = true
  device_name                    = "/dev/sdf"
  volume_id                      = aws_ebs_volume.data.id
  instance_id                    = aws_instance.rnlink.id
}

resource "aws_iam_role" "snapshots" {
  name = "rnlink-snapshots"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect = "Allow", Principal = { Service = "dlm.amazonaws.com" }, Action = "sts:AssumeRole"
  }] })
}
resource "aws_iam_role_policy_attachment" "snapshots" {
  role       = aws_iam_role.snapshots.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSDataLifecycleManagerServiceRole"
}
resource "aws_dlm_lifecycle_policy" "data" {
  description        = "Daily rnlink data and Tailscale identity snapshots"
  execution_role_arn = aws_iam_role.snapshots.arn
  state              = "ENABLED"
  policy_details {
    resource_types = ["VOLUME"]
    target_tags    = { Name = "rnlink-data", Project = "rnlink" }
    schedule {
      name      = "Daily retained for one week"
      copy_tags = true
      create_rule {
        interval      = 24
        interval_unit = "HOURS"
        times         = ["09:00"]
      }
      retain_rule { count = 7 }
    }
  }
  depends_on = [aws_iam_role_policy_attachment.snapshots]
}
