cat <<EOT
$0 requires:
* AWS Credentials to be configured
  - https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html
  - https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html
* IAM Permissions in us-west-2
  - S3 CRUD operations on buckets which will follow the pattern tofu-test-*
  - DynamoDB CRUD operations on a Table named dynamoTable
EOT

TF_ACC=1 go test ./internal/backend/remote-state/s3/...

exit $?
