terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

# Define middleware
middleware "cost_estimator" {
  command = "opentofu-cost-estimator"
  env = {
    "COST_ESTIMATOR_LOG_FILE" = "/tmp/cost_estimator.log"
  }
}

middleware "ban_unversioned_s3" {
  command = "node"
  args = ["..//middleware/ban-unversioned-s3-buckets/dist/index.js"]
}

middleware "cost_manager" {
  command = "python3"
  args = [
    "./cost-manager.py",
    "--max-budget", "0.1"
  ]
}

# Configure the AWS Provider with middleware
provider "aws" {
  region = "us-east-1"
  middleware = [
    middleware.cost_estimator,
    middleware.ban_unversioned_s3,
    middleware.cost_manager
  ]
}