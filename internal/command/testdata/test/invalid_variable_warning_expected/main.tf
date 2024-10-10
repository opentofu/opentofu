
variable "variable1" {
  type        = string

  validation {
    condition     = var.variable1 == "foobar"
    error_message = "The variable1 value must be: foobar"
  }
}

variable "variable2" {
  type        = string

  validation {
    condition     = var.variable2 == "barfoo"
    error_message = "The variable2 value must be: barfoo"
  }
}

