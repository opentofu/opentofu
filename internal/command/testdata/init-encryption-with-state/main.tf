terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.8"
    }
  }

  encryption {
    key_provider "pbkdf2" "main" {
      passphrase = var.passphrase
    }
    method "aes_gcm" "main" {
      keys = key_provider.pbkdf2.main
    }
    state {
      method = method.aes_gcm.main
    }
  }
}

variable "passphrase" {
  type      = string
  sensitive = true
  default   = ""
}
