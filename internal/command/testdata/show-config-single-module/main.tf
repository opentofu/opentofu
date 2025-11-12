terraform {
  required_providers {
    test = {
      source = "example.com/bar/test"
      version = "~> 2.0.0"
    }
  }
}

variable "foo" {
  type = string

  sensitive = true
}

provider "test" {
  foo = var.foo
}

resource "test_instance" "foo" {
  count = 5

  foo = var.foo

  provisioner "local-exec" {
    command = "echo 'not actually executed'"
  }
}

module "child" {
  # The single-module mode of "tofu show" is supposed to work without
  # installing any dependencies, so it's okay that this refers to
  # a fake location.
  source  = "example.com/not/actually/used"
  version = "~> 1.0.0"
}

output "foo" {
  value = test_instance.foo[0].foo

  sensitive = true
}
