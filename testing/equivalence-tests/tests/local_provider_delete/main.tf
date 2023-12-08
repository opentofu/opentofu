terraform {
  required_providers {
    local = {
      source  = "opentofu/local"
      version = "2.2.3"
    }
  }
}

locals {
  contents = jsonencode({
    "goodbye" = "world"
  })
}

provider "local" {}
