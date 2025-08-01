variable "in" {
  type = string
  description = "Variable that is marked as ephemeral and doesn't matter what value is given in, ephemeral or not, the value evaluated for this variable will be marked as ephemeral"
  ephemeral = true
}

output "out1" {
  value = var.in
  ephemeral = true // NOTE: because
}

output "out2" {
  value = "rawvalue" // TODO ephemeral - this is returning a raw value and since incomplete work, the evaluated value is not marked as ephemeral. Once this will be fixed, the test should fail
  ephemeral = true
}