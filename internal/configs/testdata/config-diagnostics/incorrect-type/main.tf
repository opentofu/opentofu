terraform {
  required_providers {
    foo = {
      source = "opentofu/foo"
    }
    baz = {
      source = "opentofu/baz"
    }
  }
}

module "mod" {
  source = "./mod"
  providers = {
    foo = foo
    baz = baz
  }
}
