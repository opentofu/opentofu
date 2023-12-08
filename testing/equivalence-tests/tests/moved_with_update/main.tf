terraform {
  required_providers {
    tfcoremock = {
      source  = "opentofu/tfcoremock"
      version = "0.1.1"
    }
  }
}

provider "tfcoremock" {}

resource "tfcoremock_simple_resource" "moved" {
  string = "Hello, change!"
}

moved {
  from = tfcoremock_simple_resource.base
  to = tfcoremock_simple_resource.moved
}
