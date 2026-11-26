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

The systemd backup timer runs every 15 minutes (up to 30 seconds jitter).
It uses SQLite's online backup API, checks database integrity, and uploads a
compressed archive containing only `golink.db` and the atomic Tailscale identity
file `.config/tsnet-golink/tailscaled.state`. S3 confirms its SHA-256 checksum
before the successful-backup timestamp advances. Logs and certificates are
excluded; certificates regenerate automatically. Archives contain credentials:
keep them private. The host can write backups and read migration archives but
cannot read Terraform state or historic backups.

Current backup objects expire after 30 days; older versions expire 30 days after
becoming noncurrent. Backups and state are encrypted, versioned, and blocked
from public access. Keep `disk_snapshots_enabled = true` until a compact backup
restore has passed, then set it to false to stop creating daily disk snapshots.
Existing snapshots are not deleted by disabling the policy.

Application updates use immutable ECR commit tags. Update `image_tag` and apply
a reviewed full plan. SSM pulls the image, verifies a pre-deployment backup,
restarts RN, checks readiness, and installs timers. A failed deployment restores
the last known-good service unit. This is application rollback, not automatic
database rollback: schema-changing releases require a migration-specific plan.
The pre-deployment backup key is retained in `/opt/rnlink/pre-deploy-backup.json`.
If a failed association does not rerun on apply, explicitly plan with
`-replace=aws_ssm_association.runtime` after fixing its cause.

Bootstrap/user-data changes do not replace the running instance. For an AMI
upgrade, explicitly plan instance replacement, preserving the separate data
volume. SSM requires the mounted database before starting a copied identity.
For first-time migration to an empty volume, provision the host and volume first
with targeted plans, restore the data as described above, then apply the full
plan to configure the runtime. Do not start the old host again.

### Restore drill and recovery

Download a chosen `backups/*.tar.gz` using an authorized operator's AWS SSO
credentials, into a private temporary directory (`umask 077`). Extract only the
two expected paths listed above, using safe tar extraction. Open the extracted
SQLite database and run `PRAGMA integrity_check`; inspect link counts and
metadata. Validate that the identity file parses as JSON. A drill must never
start another copy of the Tailscale identity. Remove the scratch copy afterward.

For actual recovery, stop `rnlink.service` and confirm the old container is
stopped. Retain a private copy of the current data first. Restore the validated
archive into an empty `/srv/rnlink/data` on the mounted data volume (do not leave
old SQLite WAL/SHM files beside a restored database), set ownership to
`65532:65532`, and apply the SSM runtime configuration. Verify `http://rn/` and
`rn/gpus/` from a tailnet client. Never run two copies of the identity.

### Monitoring

A timer publishes `RNLink` metrics every minute: database/Tailscale readiness,
maximum root/data disk use, and seconds since a confirmed backup. CloudWatch
alerts after three unhealthy minutes, three minutes above 85% disk use, or two
minutes with a backup older than 30 minutes. Missing metrics count as failure.
EC2 status checks and Slack Lambda errors have separate alarms. Alarm and
recovery notifications go to Slack and the confirmed email subscriptions.
Readiness listens only on host loopback; it is not publicly exposed. It checks
local Tailscale status, not independent client-to-server reachability.

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

### Tailscale expiry warning

Every minute, the private readiness endpoint reads the current Tailscale node
key expiry from the local Tailscale API. At seven days or less remaining, the
monitor publishes one warning per UTC calendar day to the RN alert topic
(Slack and email), without mentioning anyone. The timestamp is read fresh on
every check: renewing the node key beyond seven days automatically stops the
warnings. A persistent marker advances only after SNS accepts the message;
failed publications retry on the next check. Delivery is at-least-once, so a
crash after publication but before recording the marker can cause a duplicate.

Explicitly disabled key expiry suppresses expiry warnings. Missing or malformed
expiry telemetry triggers `rnlink-expiry-telemetry` instead of silently treating
the key as non-expiring. HTTPS certificate renewal is separate and does not
renew Tailscale node authorization.
