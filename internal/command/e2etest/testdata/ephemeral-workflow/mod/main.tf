variable "in" {
  type = string
  description = "Variable that is marked as ephemeral and doesn't matter what value is given in, ephemeral or not, the value evaluated for this variable will be marked as ephemeral"
  ephemeral = true
}

output "out1" {
  value = var.in
  ephemeral = true // NOTE: because
}
