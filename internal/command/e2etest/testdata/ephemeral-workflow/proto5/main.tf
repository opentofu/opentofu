// the provider-plugin tests uses the -plugin-cache flag so terraform pulls the
// test binaries instead of reaching out to the registry.
terraform {
  required_providers {
    simple5 = {
      source = "registry.opentofu.org/hashicorp/simple"
    }
  }
}

data "simple_resource" "test_data1" {
  provider = simple5
  value = "initial data value"
}

ephemeral "simple_resource" "test_ephemeral" {
  count = 2
  provider = simple5
  value = "${data.simple_resource.test_data1.value}-with-renew"
}

resource "simple_resource" "test_res" {
  provider = simple5
  // NOTE write-only arguments can reference ephemeral values.
  value_wo = ephemeral.simple_resource.test_ephemeral[0].value
}

data "simple_resource" "test_data2" {
  provider = simple5
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
  tmp = data.simple_resource.test_data2.value
}