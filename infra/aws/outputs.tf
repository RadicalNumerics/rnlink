output "instance_id" { value = aws_instance.rnlink.id }
output "data_volume_id" { value = aws_ebs_volume.data.id }
output "repository_url" { value = aws_ecr_repository.rnlink.repository_url }
output "backup_bucket" { value = aws_s3_bucket.state.id }
output "ssm_command" { value = "aws ssm start-session --target ${aws_instance.rnlink.id} --region ${var.region}" }
