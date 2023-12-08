terraform {
  required_providers {
    simple = {
      source = "opentofu/test"
    }
  }
}

resource "simple_resource" "test" {
}
