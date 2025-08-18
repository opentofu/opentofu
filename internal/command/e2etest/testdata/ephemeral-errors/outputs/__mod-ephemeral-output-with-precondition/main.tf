variable "variable_for_output_precondition" {
  type = string
  default = "default value"
  ephemeral = true
}

// NOTE: This is meant to test the precondition warnings and errors when ephemeral values are used.
output "output_with_precondition" {
  value = var.variable_for_output_precondition
  ephemeral = true
  precondition {
    // NOTE: When this condition will fail, for ephemeral references in the condition, the diff between the left
    // side and right side will be skipped. This is printed only for sensitive values.
    condition = var.variable_for_output_precondition == "default value"
    // NOTE: This error_message is going to generate a warning because ephemeral values are used in the expression.
    error_message = "Variable `variable_for_output_precondition` does not have the required value: ${var.variable_for_output_precondition}"
  }
}