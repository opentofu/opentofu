variable "instances" {
  type = number

  validation {
    condition     = var.instances >= 0
    error_message = "The number of instances must be positive or zero"
  }
}