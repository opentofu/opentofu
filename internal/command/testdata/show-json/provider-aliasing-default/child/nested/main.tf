terraform {
  required_providers {
    test = {
      source = "opentofu/test"
    }
  }
}

resource "test_instance" "test" {
  ami = "baz"
}
