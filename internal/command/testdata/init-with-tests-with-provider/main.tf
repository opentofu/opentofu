terraform {
  required_providers {
    test = {
      source = "opentofu/test"
      version = "1.0.2"
    }
  }
}

resource "test_instance" "foo" {
  ami = "bar"
}
