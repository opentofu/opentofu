variable "name" {}

output "greeting" {
  value = "Hello ${var.name}!"
}