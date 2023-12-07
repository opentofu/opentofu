terraform {
  required_providers {
    test = {
      source = "opentofu/test"
    }
    dupe = {
      source = "opentofu/test"
    }
    other = {
      source = "opentofu/default"
    }

    wrong-name = {
      source = "opentofu/foo"
    }
  }
}

provider "default" {
}

resource "foo_resource" {
}
