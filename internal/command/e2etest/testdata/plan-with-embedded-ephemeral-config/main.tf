// the provider-plugin tests uses the -plugin-cache flag so terraform pulls the
// test binaries instead of reaching out to the registry.
terraform {
  required_providers {
    simple = {
      source = "registry.opentofu.org/hashicorp/simple"
    }
  }
}

resource "simple_resource" "test_res" {
  value    = "test value"
}

ephemeral "simple_resource" "test_ephemeral" {
  value = "ephemeral value"
}
