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

variable "in" {
  ephemeral = true
  type = string
}

variable "in2" {
  ephemeral = true
  type = string
  default = "in2 default value just to test the validations"
  validation {
    condition = var.in2 != "fail_on_this_value"
    error_message = "variable 'in2' value '${var.in2}' cannot be 'fail_on_this_value'"
  }
}
