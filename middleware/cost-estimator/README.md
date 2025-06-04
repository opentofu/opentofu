# OpenTofu Cost Estimator Middleware

A middleware for OpenTofu that provides real-time cost estimation for cloud resources during planning and deployment.

## Features

- **Multi-cloud support**: AWS, Google Cloud, and Azure
- **Real-time estimation**: Get cost estimates during `tofu plan`
- **Resource-aware**: Understands different resource types and their pricing models
- **Tag-based hints**: Use tags to provide storage and usage estimates
- **Confidence levels**: Know how accurate the estimates are

## Installation

```bash
npm install
npm run build
```

## Usage

### In your Terraform/OpenTofu configuration:

```hcl
middleware "cost_estimator" {
  command = "node"
  args    = ["/path/to/cost-estimator/dist/index.js"]
}

provider "aws" {
  region = "us-east-1"
  middleware = [middleware.cost_estimator]
}

resource "aws_s3_bucket" "example" {
  bucket = "my-bucket"
  
  tags = {
    # Optional: Help the estimator with usage patterns
    EstimatedStorageGB = "500"
    EstimatedMonthlyGETRequests = "1000000"
    EstimatedMonthlyPUTRequests = "100000"
  }
}
```

### Environment Variables

- `COST_ESTIMATOR_LOG_FILE`: Path to log file (optional)

## Supported Resources

### AWS
- `aws_instance` - EC2 instances
- `aws_s3_bucket` - S3 buckets (with tag-based storage hints)
- `aws_db_instance` - RDS instances
- `aws_lambda_function` - Lambda functions
- `aws_dynamodb_table` - DynamoDB tables
- `aws_ebs_volume` - EBS volumes

### Google Cloud
- `google_compute_instance` - Compute Engine instances
- `google_storage_bucket` - Cloud Storage buckets
- `google_sql_database_instance` - Cloud SQL instances

### Azure
- `azurerm_virtual_machine` - Virtual machines
- `azurerm_storage_account` - Storage accounts
- `azurerm_sql_database` - SQL databases

## Cost Estimation Tags

You can provide hints to improve cost estimation accuracy using resource tags:

- `EstimatedStorageGB` - Expected storage usage in GB
- `EstimatedMonthlyGETRequests` - Expected GET requests per month
- `EstimatedMonthlyPUTRequests` - Expected PUT requests per month

## Example Output

```
Terraform will perform the following actions:

  # aws_s3_bucket.example will be created
  + resource "aws_s3_bucket" "example" {
      ...
    }
    
  Middleware: cost-estimator
  Message: Estimated cost: $12.30/month

Plan: 1 to add, 0 to change, 0 to destroy.
```

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Type check
npm run typecheck

# Lint
npm run lint

# Watch mode for development
npm run watch
```

## Limitations

- Pricing data is simplified and may not reflect exact costs
- Does not account for free tier benefits
- Network transfer costs are not calculated
- Spot/preemptible instance pricing not supported yet

## License

MPL-2.0