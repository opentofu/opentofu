terraform {
  required_providers {
    null = {
      source  = "opentofu/null"
      version = "3.1.1"
    }
  }
}

provider "null" {}
