terraform {
  required_providers {
    foo = {
      source = "opentofu/foo"
      version = "1.0.0"
    }
  }
}

module "mod" {
  source = "./mod"
  providers = {
    foo = foo
  }
}
