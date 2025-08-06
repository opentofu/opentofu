// Module that is used strictly to test passing in and out ephemeral variables.
variable "in" {
  type = string
}

output "out" {
  value = var.in
}