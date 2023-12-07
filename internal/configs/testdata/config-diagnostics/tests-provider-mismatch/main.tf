terraform {
  required_providers {
    foo = {
      source = "opentofu/foo"
      configuration_aliases = [foo.bar]
    }
  }
}

provider "bar" {}

resource "foo_resource" "resource" {}

resource "bar_resource" "resource" {}
