# RN on AWS

One ARM EC2 instance runs rnlink's userspace Tailscale server. The default is a
`t4g.small` (2 vCPUs, 2 GiB), Amazon Linux 2023, a 12 GiB encrypted root disk,
and a separate 8 GiB encrypted gp3 data disk. The security group allows no
inbound connections. The public IPv4 address is for outbound Internet access;
there is no public web listener or SSH port. Administration uses SSM.

The data volume and state bucket are protected from Terraform destruction.
The instance can be replaced while retaining its database and Tailscale state.
Never run two copies of the same Tailscale state simultaneously.

## Initial setup

Use AWS SSO and export `AWS_PROFILE` for the intended account. Copy
`terraform.tfvars.example` and `backend.hcl.example` without the `.example`
suffix, supplying the account, existing VPC/public subnet, unique bucket name,
pinned AL2023 ARM AMI ID, and Git commit image tag. Resolve an AMI once using
`aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 --query Parameter.Value --output text`. Actual settings, state, and saved plans are ignored.
The provider rejects credentials for a different account.

Bootstrap the state bucket once (replace the example values):

```sh
export AWS_PROFILE=your-sso-profile
aws sso login
aws s3api create-bucket --bucket YOUR_BUCKET --region us-west-2 \
  --create-bucket-configuration LocationConstraint=us-west-2
aws s3api put-public-access-block --bucket YOUR_BUCKET \
  --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
aws s3api put-bucket-versioning --bucket YOUR_BUCKET \
  --versioning-configuration Status=Enabled
aws s3api put-bucket-encryption --bucket YOUR_BUCKET \
  --server-side-encryption-configuration '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
terraform init -backend-config=backend.hcl
terraform import aws_s3_bucket.state YOUR_BUCKET
terraform plan -out=bootstrap.tfplan -target=aws_ecr_repository.rnlink
terraform apply bootstrap.tfplan
```

Build an ARM64 image from the repository root, tag it with its Git commit,
and push it to the ECR URL from `terraform output -raw repository_url`.
ECR tags are immutable. Use `aws ecr get-login-password` with Docker's
`--password-stdin`, never put a token in an argument or checked-in file.
Then run `terraform plan -out=deploy.tfplan`, review it, and apply that plan.
Subsequent changes should use a full plan; the targeted operation above only
bootstraps the image repository before the first host exists.

## Data migration and startup

Provisioning enables the service but deliberately does not start it. The
service also requires `/srv/rnlink/data/golink.db`, preventing accidental
empty initialization or simultaneous use of an existing node identity.

1. Export links for verification, stop the old container gracefully, and copy
   its entire `/home/nonroot` directory, including hidden files. This includes
   the SQLite database, click statistics, Tailscale identity, and certificates.
2. Keep an offline backup. Transfer the archive only via the private bucket's
   `transfer/` prefix. Archives contain credentials: never commit them, put
   them in Terraform variables/user data, or include their contents in logs.
3. Through SSM, confirm `/srv/rnlink` is mounted, restore the archive into
   `/srv/rnlink/data`, and set ownership to `65532:65532`.
4. Start `rnlink.service`. Verify the index, links, and metadata from a tailnet
   client while the old container remains stopped. Check
   `systemctl status rnlink` and `docker ps` through SSM.
5. Test `systemctl start rnlink-backup`. Remove the migration object **and its
   versions** from S3 after success. Transfer objects also expire after one day.
6. Remove the old container only after verifying the new host. Retain the old
   data volume and offline backup for rollback; do not run the old copy again
   while AWS is using that Tailscale identity.

For a new installation with no existing state, enroll rnlink interactively
on the host instead of copying another node's identity. Never pass a reusable
Tailscale auth key through Terraform.

## Operations

```sh
aws ssm start-session --target INSTANCE_ID --region us-west-2
sudo systemctl status rnlink
sudo journalctl -u rnlink --since '10 minutes ago'
sudo systemctl restart rnlink
```

The daily systemd timer uses SQLite's online backup API and uploads a consistent
database snapshot under `backups/`. It does not stop the app. The host can write
backups and read migration archives, but cannot access Terraform state. Database backups are retained for 30 days and
contain link metadata, not the node's credentials. The persistent EBS volume
holds the identity; DLM retains seven daily snapshots of that disk. These are
crash-consistent disk snapshots; SQLite backups provide database-level consistency.
Take an additional EBS snapshot before disk maintenance.
Backups and state are encrypted and versioned; public access is blocked.

To roll back a failed migration, first stop the AWS service, then restart the
old container using its retained data. To update the application, publish a new
immutable commit tag, update `image_tag`, and review the full Terraform plan.
The pinned AMI prevents unrelated plans from following a moving latest image.
Volume detachment first stops the old instance. The current configuration
replaces the host when its bootstrap changes; the
separate data disk survives and is attached to the replacement. Start the
service after confirming the mount and data are intact.

Do not use `terraform destroy` as a routine rollback: state and data have
`prevent_destroy` guards. Removing those guards is a deliberate data-retirement
operation, not part of application updates.

### Slack alerts

CloudWatch alarms publish to `rnlink-alerts`. Its Lambda subscription forwards
notifications to `#reflex-ops-services` using the incoming webhook stored in
Secrets Manager at `rnlink/slack-webhook`. Set or rotate the secret value outside
Terraform; never put it in tfvars, source control, or logs. The Lambda role can
read only this secret. No Slack read access or interactive bot is required.

Slack delivery failures retry through Lambda asynchronous invocation and land
in `rnlink-slack-alert-failures` after retries are exhausted (14-day retention).
The `rnlink-slack-delivery-errors` alarm sends email through a separate SNS topic
to avoid a broken Slack integration reporting only to itself. Confirm both AWS
SNS email subscriptions to enable normal and delivery-failure email alerts.

Test the complete pipeline with a clearly labeled test message published to the
`rnlink-alerts` SNS topic. Check Lambda errors and the channel for delivery.
