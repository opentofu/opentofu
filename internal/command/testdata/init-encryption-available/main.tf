variable "encryption_kms_key_id" {
  type = string
  default = "1234abcd-12ab-34cd-56ef-1234567890ab"
}

terraform {

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.8"
    }
  }

  encryption {
    key_provider "aws_kms" "key" {
      kms_key_id = var.encryption_kms_key_id
      region     = "eu-central-1"
      key_spec   = "AES_256"
    }
    method "aes_gcm" "encrypt-aes" {
      keys = key_provider.aws_kms.key
    }
    state {
      enforced = true
      method   = method.aes_gcm.encrypt-aes
    }
    plan {
      enforced = true
      method   = method.aes_gcm.encrypt-aes
    }
  }
}