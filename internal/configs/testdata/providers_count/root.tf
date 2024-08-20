terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
    }
  }
}

provider "foo-test" {
  count = 3
}