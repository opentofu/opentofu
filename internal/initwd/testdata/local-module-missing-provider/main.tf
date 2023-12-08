terraform {
  required_providers {
    foo = {
      source = "opentofu/foo"
      // since this module declares an alias with no config, it is not valid as
      // a root module.
      configuration_aliases = [ foo.alternate ]
    }
  }
}

resource "foo_instance" "bam" {
  provider = foo.alternate
}
