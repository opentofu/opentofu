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
  type      = bool
  default   = true
  ephemeral = true
}

data "simple_resource" "res" {
  lifecycle {
    enabled = var.in
  }
  value = "test value"
}

resource "simple_resource" "res" {
  lifecycle {
    enabled = var.in
  }
  value = "test value"
}

ephemeral "simple_resource" "res" {
  lifecycle {
    enabled = var.in
  }
  value = "test value"
}

