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
    // NOTE: When this condition will fail, the actual value of the variable will be printed.
    // TODO ephemeral: the description above applies for sensitive and ephemeral values. Left another comment in
    //  the code with a question about this. Maybe we want to do something about this confidential information
    //  exposure.
    condition = var.variable_for_output_precondition == "default value"
    // NOTE: This error_message is going to generate a warning because ephemeral values are used in the expression.
    error_message = "Variable `variable_for_output_precondition` does not have the required value: ${var.variable_for_output_precondition}"
  }
}