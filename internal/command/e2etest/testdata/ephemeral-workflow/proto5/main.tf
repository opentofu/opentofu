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
  // NOTE this is wrongly configured on purpose to force a revisit of the test once ephemeral marks are implemented.
  // Once write only arguments are also implemented, adjust the implementation of the provider to support that too
  // and use that new field instead.
  value = ephemeral.simple_resource.test_ephemeral[0].value
}

data "simple_resource" "test_data2" {
  provider = simple5
  // NOTE this is wrongly configured on purpose to force a revisit of the test once ephemeral marks are implemented
  value = ephemeral.simple_resource.test_ephemeral[0].value
}

locals{
  tmp = data.simple_resource.test_data2.value
}