variable "test_var" {
  type      = string
  default   = "test_var value"
  ephemeral = true
}

// NOTE: An output that wants to use ephemeral values needs to be configured with "ephemeral = true"
output "test" {
  value = var.test_var
}