terraform {
  required_providers {
    tfcoremock = {
      source  = "opentofu/tfcoremock"
      version = "0.1.1"
    }
  }
}

provider "tfcoremock" {}

resource "tfcoremock_object" "object" {}
