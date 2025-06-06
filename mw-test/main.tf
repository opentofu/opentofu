# Random suffix for unique bucket names
resource "random_id" "bucket_suffix" {
  byte_length = 8
}

# Create a properly versioned bucket
resource "aws_s3_bucket" "versioned" {
  bucket = "versioned-bucket-${random_id.bucket_suffix.hex}"

  tags = {
    Name        = "Versioned Bucket"
    Environment = "dev"
    CostCenter  = "engineering"
    Purpose     = "Testing versioned bucket"
    # Cost estimation hints for middleware
    EstimatedStorageGB          = "50"
    EstimatedMonthlyGETRequests = "1000000"
    EstimatedMonthlyPUTRequests = "100000"
    StorageClass                = "STANDARD"
  }
}

# Enable versioning on the this bucket only
resource "aws_s3_bucket_versioning" "versioned" {
  bucket = aws_s3_bucket.versioned.id
  versioning_configuration {
    status = "Enabled"
  }
}

# Create the "yolo" bucket without versioning
resource "aws_s3_bucket" "yolo" {
  bucket = "yolo-${random_id.bucket_suffix.hex}"

  tags = {
    Name        = "YOLO Bucket (Unversioned)"
    Environment = "dev"
    CostCenter  = "engineering"
    Purpose     = "Testing unversioned bucket detection"
  }
}