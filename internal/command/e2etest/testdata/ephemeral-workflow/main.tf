// the provider-plugin tests uses the -plugin-cache flag so terraform pulls the
// test binaries instead of reaching out to the registry.
terraform {
  required_providers {
    simple = {
      source = "registry.opentofu.org/hashicorp/simple"
    }
  }
}

variable "simple_input" {
  type = string
}

variable "ephemeral_input" {
  type      = string
  ephemeral = true
  validation {
    condition = var.ephemeral_input == "ephemeral_val"
    error_message = "this is just to ensure that error_message is not executed when condition suceeds. If the condition fails, it will fail because this message references an ephemeral value ${var.ephemeral_input}}"
  }
}

provider "simple" {
  alias = "s1"
  // NOTE we want to ensure that ephemeral variables can be used as provider configuration
  i_depend_on = var.ephemeral_input
}

data "simple_resource" "test_data1" {
  provider = simple.s1
  value    = var.simple_input
}

ephemeral "simple_resource" "test_ephemeral" {
  count    = 2
  provider = simple.s1
  // Having that "-with-renew" suffix, later when this value will be passed into "simple_resource.test_res.value_wo",
  // the plugin will delay the response on some requests to allow ephemeral Renew calls to be performed.
  value    = "${data.simple_resource.test_data1.value}-${var.ephemeral_input}-with-renew"
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
    command = "echo \"not visible ${ephemeral.simple_resource.test_ephemeral[0].value}\""
  }
  provisioner "local-exec" {
    command = "echo \"not visible ${var.ephemeral_input}\""
  }
  // NOTE: value_wo cannot be used in a provisioner because it is returned as null by the provider so the interpolation fails
}

data "simple_resource" "test_data2" {
  provider = simple.s1
  value    = "test"
  lifecycle {
    precondition {
      // NOTE: precondition blocks can reference ephemeral values
      condition     = ephemeral.simple_resource.test_ephemeral[0].value != null
      error_message = "test message"
    }
  }
}

locals {
  simple_provider_cfg = ephemeral.simple_resource.test_ephemeral[0].value
}

provider "simple" {
  alias       = "s2"
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
  value    = "just a simple resource to ensure that the second provider it's working fine"
}

module "call" {
  source = "./mod"
  in     = ephemeral.simple_resource.test_ephemeral[0].value
  // NOTE: because variable "in" is marked as ephemeral, this should work as expected.
}

output "final_output" {
  value = simple_resource.test_res_second_provider.value
}

provider "simple" {
  alias       = "s3"
  i_depend_on = tofu.applying
}
