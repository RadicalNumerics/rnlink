resource "aws_iam_role_policy" "metrics" {
  role = aws_iam_role.rnlink.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect    = "Allow", Action = "cloudwatch:PutMetricData", Resource = "*",
    Condition = { StringEquals = { "cloudwatch:namespace" = "RNLink" } }
  }] })
}
resource "aws_ssm_document" "runtime" {
  name          = "rnlink-configure-runtime"
  document_type = "Command"
  content = jsonencode({
    schemaVersion = "2.2"
    description   = "Deploy RN with loopback readiness, compact backups, and metrics"
    mainSteps = [{ action = "aws:runShellScript", name = "configure", inputs = {
      timeoutSeconds = "900"
      runCommand = [templatefile("${path.module}/runtime.sh.tftpl", {
        region   = var.region
        registry = split("/", aws_ecr_repository.rnlink.repository_url)[0]
        image    = "${aws_ecr_repository.rnlink.repository_url}:${var.image_tag}"
        backup   = filebase64("${path.module}/scripts/backup.py")
        monitor  = filebase64("${path.module}/scripts/monitor.py")
        config = base64encode(jsonencode({
          region = var.region, bucket = var.bucket_name,
          data   = "/srv/rnlink/data", last_backup = "/srv/rnlink/last-backup-success.json"
        }))
      })]
    } }]
  })
}
resource "aws_ssm_association" "runtime" {
  name                             = aws_ssm_document.runtime.name
  document_version                 = aws_ssm_document.runtime.latest_version
  wait_for_success_timeout_seconds = 900
  targets {
    key    = "InstanceIds"
    values = [aws_instance.rnlink.id]
  }
  depends_on = [aws_volume_attachment.data, aws_iam_role_policy.metrics]
}
locals {
  app_alarms = {
    readiness = { metric = "ServiceHealthy", operator = "LessThanThreshold", threshold = 1, periods = 3 }
    disk      = { metric = "DiskUsedPercent", operator = "GreaterThanThreshold", threshold = 85, periods = 3 }
    backup    = { metric = "BackupAgeSeconds", operator = "GreaterThanThreshold", threshold = 1800, periods = 2 }
  }
}
resource "aws_cloudwatch_metric_alarm" "application" {
  for_each            = local.app_alarms
  alarm_name          = "rnlink-${each.key}"
  namespace           = "RNLink"
  metric_name         = each.value.metric
  statistic           = "Average"
  period              = 60
  evaluation_periods  = each.value.periods
  comparison_operator = each.value.operator
  threshold           = each.value.threshold
  treat_missing_data  = "breaching"
  dimensions          = { Service = "rnlink" }
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
}
