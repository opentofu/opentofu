terraform {
  required_providers {
    foo = {
      source = "opentofu/foo"
      version = "1.0.0"
      configuration_aliases = [ foo.bar ]
    }
  }
}

resource "foo_resource" "a" {
  provider = foo.bar
}
