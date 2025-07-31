variable "state_filename" {
  type    = string
  default = "local-state.tfstate"
}

terraform {
  backend "_test_local" {
    path = var.state_filename
  }
}
