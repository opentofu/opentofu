variable "ephemeral_var" {
  type      = string
  ephemeral = true
}

variable "regular_var" {
  type = string
}

output "ephemeral_out_from_ephemeral_in" {
  value     = var.ephemeral_var
  ephemeral = true
}

// NOTE: This output is used to test that it can receive a non-ephemeral value but it results in an
// ephemeral value when used.
output "ephemeral_out_from_regular_var" {
  value     = var.regular_var
  ephemeral = true
}

// NOTE: This output is used to test that a hardcoded raw value will be marked as ephemeral when
// referenced in an expression.
output "ephemeral_out_hardcoded_with_non_ephemeral_value" {
  value     = "raw value"
  ephemeral = true
}
