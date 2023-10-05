output "test" {
  value = var.test != null ? var.test : ""
}

variable "test" {
  type      = string
  default   = null
  sensitive = true
}