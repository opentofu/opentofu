variable "input" {
  type = string
}

variable "joined" {
  type = string
}

output "input_output" {
  value = var.input
}

output "joined_output" {
  value = var.joined
}

resource "test_resource" "dummy" {
  value = "placeholder"
}