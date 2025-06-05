# OpenTofu Middleware: Ban Unversioned S3 Buckets

This middleware prevents the creation or update of S3 buckets without versioning enabled. It analyzes the plan to ensure all `aws_s3_bucket` resources have a corresponding `aws_s3_bucket_versioning` resource.

## Installation

```bash
npm install
npm run build
```

## Usage

Add to your OpenTofu configuration:

```hcl
middleware "ban_unversioned_s3" {
  command = "/path/to/node /path/to/ban-unversioned-s3-buckets/dist/index.js"
}

provider "aws" {
  middleware = [middleware.ban_unversioned_s3]
}
```

## How it Works

The middleware:
1. Scans the plan for all `aws_s3_bucket` resources being created or updated
2. Identifies all `aws_s3_bucket_versioning` resources 
3. Matches buckets with their versioning configuration
4. Fails the plan if any S3 bucket lacks versioning

## Example Output

When an unversioned bucket is detected:
```
[BAN-UNVERSIONED-S3] Bucket aws_s3_bucket.yolo does not have versioning enabled
Error: Middleware check failed
Found 1 S3 bucket(s) without versioning enabled
```

## Development

```bash
# Install dependencies
npm install

# Build the middleware
npm run build

# Watch for changes during development
npm run watch

# Run type checking
npm run typecheck

# Lint code
npm run lint

# Format code
npm run format
```

## Implementation Details

This middleware works with the modern AWS provider pattern where versioning is configured via a separate `aws_s3_bucket_versioning` resource rather than inline in the bucket resource. 

For example, a properly versioned bucket looks like:
```hcl
resource "aws_s3_bucket" "example" {
  bucket = "my-bucket"
}

resource "aws_s3_bucket_versioning" "example" {
  bucket = aws_s3_bucket.example.id
  versioning_configuration {
    status = "Enabled"
  }
}
```

The middleware will detect if any S3 bucket lacks a corresponding versioning resource and fail the plan accordingly.