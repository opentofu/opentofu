terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
    }
  }
}

locals {
  files = {
    0 = {
      filename = "/tmp/0"
      content = "who dis?"
    }
    test = {
      filename = "/tmp/test"
      content = "testing 1 2 3"
    }
    prod = {
      filename = "/tmp/prod"
      content = "this is a serious string, because it's production"
    }
  }
}

provider "foo-test" {
  for_each = local.files
}