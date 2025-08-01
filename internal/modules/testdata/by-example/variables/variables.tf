# This example is a good place to add relatively-simple valid examples of
# input variables where it seems like overkill to add an entirely new example
# directory, but if you're adding a collection of example blocks related to
# a specific feature then probably better to start a new directory to collect
# those together thematically.

variable "unconstrained" {
}

variable "string" {
  type = string
}

variable "list_of_string" {
  type = list(string)
}

variable "explicitly_unconstrained" {
  type = any
}

variable "string_optional" {
  type   = string
  default = "hello"
}

variable "unconstrained_optional" {
  type   = string
  default = "hello"
}
