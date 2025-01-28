terraform {
  required_providers {
    tfcoremock = {
      source  = "tfcoremock"
      version = "0.3.0"
    }
  }
}

resource "tfcoremock_simple_resource" "foo" {}
