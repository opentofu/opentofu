terraform {
  required_providers {
    test = {
      source = "terraform.io/builtin/test"
    }
  }
}

variable "prevent_destroy" {
  type = bool
}

resource "test" "test" {
  count = 0

  lifecycle {
    prevent_destroy = var.prevent_destroy
  }
}
