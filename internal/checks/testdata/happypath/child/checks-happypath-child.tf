terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.81.0"
    }
  }
}
resource "null_resource" "b" {
  lifecycle {
    precondition {
      condition     = self.id == ""
      error_message = "Impossible."
    }
  }
}

resource "null_resource" "c" {
  count = 2

  lifecycle {
    postcondition {
      condition     = self.id == ""
      error_message = "Impossible."
    }
  }
}

data "aws_s3_object" "foo" {
  lifecycle {
    precondition {
      condition     = self.id == ""
      error_message = "Impossible data."
    }
  }
  bucket = "test-bucket"
  key    = "test-key"
}

ephemeral "aws_secretsmanager_secret_version" "bar" {
  lifecycle {
    precondition {
      condition     = self.id == ""
      error_message = "Impossible ephemeral."
    }
  }
  secret_id = "secret-manager-id"
}

output "b" {
  value = null_resource.b.id

  precondition {
    condition     = null_resource.b.id != ""
    error_message = "B has no id."
  }
}
