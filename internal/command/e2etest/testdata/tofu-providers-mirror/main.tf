terraform {
  required_providers {
    template  = { version = "2.1.1" }
    null      = { source = "opentofu/null", version = "2.1.0" }
    terraform = { source = "terraform.io/builtin/terraform" }
  }
}
