////// core:unused-variable

// used in a provider => not reported
variable "var_for_provider" {
  type = string
  default = "default var_for_provider"
}

provider "simple" {
  alias = "for_unused_variable"
  i_depend_on = var.var_for_provider
}

// not used at all => reported
variable "var_not_used_at_all" {
  type    = string
  default = "var_not_used_at_all"
}

// used in its own validation block => not reported
variable "var_used_only_in_validation" {
  type    = string
  default = "default used_only_in_validation"
  validation {
    condition     = var.var_used_only_in_validation != ""
    error_message = "variable cannot be empty"
  }
}

// used in output => not reported
variable "var_used_in_output" {
  type = string
  default = "default var_used_in_output"
}

output "output_for_var_used_in_output" {
  value = var.var_used_in_output
}

// used in a postcondition => not reported
variable "var_for_postcondition" {
  type = string
  default = "default var_for_postcondition"
}

resource "simple_resource" "unused_variable" {
  provider = simple.for_unused_variable
  lifecycle {
    postcondition {
      condition = var.var_for_postcondition != ""
      error_message = "variable cannot be empty"
    }
  }
}

// used in module call => not reported
variable "var_for_module_call" {
  type = string
  default = "default var_for_module_call"
}
module "call1" {
  source = "./simple-module"
  used_mod_in = var.var_for_module_call
}