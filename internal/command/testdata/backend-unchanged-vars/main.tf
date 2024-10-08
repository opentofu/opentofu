variable "state_filename" {
  type    = string
  default = "local-state.tfstate"
}

terraform {
  backend "local" {
    path = var.state_filename
  }
}
