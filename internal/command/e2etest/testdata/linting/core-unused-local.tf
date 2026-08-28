////// core:unused-local

// used in a provider => not reported
locals {
  local_for_provider = "local_for_provider value"
}

provider "simple" {
  alias = "for_unused_local"
  i_depend_on = local.local_for_provider
}

// not used at all => reported
locals{
 local_not_used_at_all = "local_not_used_at_all value"
}

// used in output => not reported
locals {
  local_used_in_output = "local_used_in_output value"
}

output "output_for_local_used_in_output" {
  value = local.local_used_in_output
}

// used in a postcondition => not reported
locals {
  local_for_postcondition = "local_for_postcondition value"
}
resource "simple_resource" "unused_local" {
  provider = simple.for_unused_local
  lifecycle {
    postcondition {
      condition = local.local_for_postcondition != ""
      error_message = "local cannot be empty"
    }
  }
}

// used in module call => not reported
locals {
  local_for_module_call = "local_for_module_call value"
}
module "call2" {
  source = "./simple-module"
  used_mod_in = local.local_for_module_call
}