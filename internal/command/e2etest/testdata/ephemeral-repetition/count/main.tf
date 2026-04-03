// the provider-plugin tests uses the -plugin-cache flag so terraform pulls the
// test binaries instead of reaching out to the registry.
terraform {
  required_providers {
    simple = {
      source = "registry.opentofu.org/hashicorp/simple"
    }
  }
}

variable "in" {
  type      = number
  default   = 1
  ephemeral = true
}

data "simple_resource" "res" {
  count = var.in
  value = "test value"
}

resource "simple_resource" "res" {
  count = var.in
  value = "test value"
}

ephemeral "simple_resource" "res" {
  count = var.in
  value = "test value"
}

