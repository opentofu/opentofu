// the provider-plugin tests uses the -plugin-cache flag so terraform pulls the
// test binaries instead of reaching out to the registry.
terraform {
  required_providers {
    simple5 = {
      source = "registry.opentofu.org/hashicorp/simple"
    }
    simple6 = {
      source = "registry.opentofu.org/hashicorp/simple6"
    }
  }
}

resource "simple_resource" "test-proto5" {
  provider = simple5
}

resource "simple_resource" "test-proto6" {
  provider = simple6
}

output "function_output5" {
	value = provider::simple5::simple_function("foo", "bar", "baz")
}

output "function_output6" {
	value = provider::simple6::simple_function("foo", "bar", "baz")
}
