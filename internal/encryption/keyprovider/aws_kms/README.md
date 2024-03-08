# AWS KMS Key Provider

> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

This folder contains the code for the AWS KMS Key Provider. The user will be able to provide a reference to an AWS KMS key which can be used to encrypt and decrypt the data.

## Configuration

You can configure this key provider by specifying the following options:

```hcl2
terraform {
    encryption {
        key_provider "aws_kms" "myprovider" {
           kms_key_id = "1234abcd-12ab-34cd-56ef-1234567890ab"
        }
    }
}
```
## Key Provider Options - kms_key_id

The kms_key_id can refer to one of the following:

- Key ID: 1234abcd-12ab-34cd-56ef-1234567890ab
- Key ARN: arn:aws:kms:us-east-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab
- Alias name: alias/ExampleAlias
- Alias ARN: arn:aws:kms:us-east-2:111122223333:alias/ExampleAlias

For more information see https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/kms#GenerateDataKeyInput

## State Snapshotting and Key Usage

### Overview

OpenTofu generates a new encryption key for every time we store encrypted data, ensuring high security by minimizing key reuse.
This has some minor cost implications that should be communicated to the end users, There may be more keys generated than expected as OpenTofu uses a new key for each state snapshot.
It is important to generate a new key for each state snapshot to ensure that the state snapshot is encrypted with a unique key instead of reusing the same key for all state snapshots and thus reducing the security of the system.