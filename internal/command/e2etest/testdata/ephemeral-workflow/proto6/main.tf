// the provider-plugin tests uses the -plugin-cache flag so terraform pulls the
// test binaries instead of reaching out to the registry.
terraform {
  required_providers {
    simple = {
      source = "registry.opentofu.org/hashicorp/simple6"
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
  value = "test value"
  // NOTE write-only arguments can reference ephemeral values.
  value_wo = ephemeral.simple_resource.test_ephemeral[0].value
  provisioner "local-exec" {
    command = "echo \"visible ${self.value}\""
  }
  provisioner "local-exec" {
    command = "echo \"not visible ${self.value_wo}\""
  }
}

data "simple_resource" "test_data2" {
  provider = simple.s1
  value = "test"
  lifecycle {
    precondition {
      // NOTE: precondition blocks can reference ephemeral values
      condition = ephemeral.simple_resource.test_ephemeral[0].value != null
      error_message = "test message"
    }
  }
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

module "call" {
  source = "./mod"
  in = ephemeral.simple_resource.test_ephemeral[0].value // NOTE: because variable "in" is marked as ephemeral, this should work as expected.
}

output "out_ephemeral" {
  value = module.call.out2 // TODO: Because the output ephemeral marking is not done yet entirely, this is working now but remove this output once the marking of outputs are done completely.
}

output "final_output" {
  value = simple_resource.test_res_second_provider.value
}