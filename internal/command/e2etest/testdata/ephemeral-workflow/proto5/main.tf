// the provider-plugin tests uses the -plugin-cache flag so terraform pulls the
// test binaries instead of reaching out to the registry.
terraform {
  required_providers {
    simple = {
      source = "registry.opentofu.org/hashicorp/simple"
    }
  }
}

provider "simple" {
  alias = "s1"
}

data "simple_resource" "test_data1" {
  provider = simple.s1
  value = "initial data value"
}

ephemeral "simple_resource" "test_ephemeral" {
  count = 2
  provider = simple.s1
  value = "${data.simple_resource.test_data1.value}-with-renew"
}

resource "simple_resource" "test_res" {
  provider = simple.s1
  // NOTE this is wrongly configured on purpose to force a revisit of the test once ephemeral marks are implemented.
  // Once write only arguments are also implemented, adjust the implementation of the provider to support that too
  // and use that new field instead.
  value = ephemeral.simple_resource.test_ephemeral[0].value
}

data "simple_resource" "test_data2" {
  provider = simple.s1
  // NOTE this is wrongly configured on purpose to force a revisit of the test once ephemeral marks are implemented
  value = ephemeral.simple_resource.test_ephemeral[0].value
}

locals{
  simple_provider_cfg = ephemeral.simple_resource.test_ephemeral[0].value
}

provider "simple" {
  alias = "s2"
  // NOTE: Ensure that ephemeral values can be used to configure a provider.
  // This is needed in two cases: during plan/apply and also during destroy.
  // This test has been updated when DestroyEdgeTransformer was updated to
  // not create dependencies between ephemeral resources and the destroy nodes.
  // The "i_depend_on" field is just a simple configuration attribute of the provider
  // to allow creation of dependencies between a resources from a previously
  // initialized provider and the provider that is configured here.
  // The "i_depend_on" field is having no functionality behind, in the provider context,
  // but it's just a way for the "provider" block to create depedencies
  // to other blocks.
  i_depend_on = local.simple_provider_cfg
}

resource "simple_resource" "test_res_second_provider" {
  provider = simple.s2
  value = "just a simple resource to ensure that the second provider it's working fine"
}

output "final_output" {
  value = simple_resource.test_res_second_provider.value
}
