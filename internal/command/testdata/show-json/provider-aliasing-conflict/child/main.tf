terraform {
  required_providers {
    test = {
      source = "opentofu2/test"
    }
  }
}

resource "test_instance" "test" {
  ami = "bar"
}
