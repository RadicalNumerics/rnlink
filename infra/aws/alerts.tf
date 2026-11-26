resource "aws_sns_topic" "alerts" { name = "rnlink-alerts" }
resource "aws_sns_topic_subscription" "email" {
  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = "max.bates@radicalnumerics.ai"
}
resource "aws_secretsmanager_secret" "slack" {
  name        = "rnlink/slack-webhook"
  description = "Incoming webhook for reflex-ops-services; value managed outside Terraform"
}
resource "aws_iam_role" "slack" {
  name = "rnlink-slack-alerts"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect = "Allow", Principal = { Service = "lambda.amazonaws.com" }, Action = "sts:AssumeRole"
  }] })
}
resource "aws_cloudwatch_log_group" "slack" {
  name              = "/aws/lambda/rnlink-slack-alerts"
  retention_in_days = 30
}
resource "aws_sqs_queue" "slack_failures" {
  name                      = "rnlink-slack-alert-failures"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
}
resource "aws_iam_role_policy" "slack" {
  role = aws_iam_role.slack.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = aws_secretsmanager_secret.slack.arn },
    { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "${aws_cloudwatch_log_group.slack.arn}:*" },
    { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.slack_failures.arn }
  ] })
}
data "archive_file" "slack" {
  type        = "zip"
  source_file = "${path.module}/scripts/slack_alert.py"
  output_path = "${path.module}/slack-alert.zip"
}
resource "aws_lambda_function" "slack" {
  function_name    = "rnlink-slack-alerts"
  role             = aws_iam_role.slack.arn
  runtime          = "python3.13"
  handler          = "slack_alert.handler"
  filename         = data.archive_file.slack.output_path
  source_code_hash = data.archive_file.slack.output_base64sha256
  timeout          = 30
  memory_size      = 128
  environment { variables = { WEBHOOK_SECRET_ARN = aws_secretsmanager_secret.slack.arn } }
  dead_letter_config { target_arn = aws_sqs_queue.slack_failures.arn }
  depends_on = [aws_iam_role_policy.slack, aws_cloudwatch_log_group.slack]
}
resource "aws_lambda_permission" "sns" {
  statement_id  = "AllowRNAlerts"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.slack.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.alerts.arn
}
resource "aws_sns_topic_subscription" "slack" {
  topic_arn  = aws_sns_topic.alerts.arn
  protocol   = "lambda"
  endpoint   = aws_lambda_function.slack.arn
  depends_on = [aws_lambda_permission.sns]
}
resource "aws_cloudwatch_metric_alarm" "instance" {
  alarm_name          = "rnlink-instance-health"
  namespace           = "AWS/EC2"
  metric_name         = "StatusCheckFailed"
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 2
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  treat_missing_data  = "breaching"
  dimensions          = { InstanceId = aws_instance.rnlink.id }
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
}
resource "aws_sns_topic" "delivery_failures" { name = "rnlink-alert-delivery-failures" }
resource "aws_sns_topic_subscription" "delivery_failures_email" {
  topic_arn = aws_sns_topic.delivery_failures.arn
  protocol  = "email"
  endpoint  = "max.bates@radicalnumerics.ai"
}
resource "aws_cloudwatch_metric_alarm" "slack_errors" {
  alarm_name          = "rnlink-slack-delivery-errors"
  namespace           = "AWS/Lambda"
  metric_name         = "Errors"
  statistic           = "Sum"
  period              = 60
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  treat_missing_data  = "notBreaching"
  dimensions          = { FunctionName = aws_lambda_function.slack.function_name }
  alarm_actions       = [aws_sns_topic.delivery_failures.arn]
  ok_actions          = [aws_sns_topic.delivery_failures.arn]
}
