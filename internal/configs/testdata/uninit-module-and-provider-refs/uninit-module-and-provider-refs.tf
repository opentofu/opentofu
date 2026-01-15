
# This fixture is based on an example given in
# https://github.com/hashicorp/terraform/issues/38067

terraform {
  required_providers {
    example = {
      source = "example/example"
    }
  }
}

provider "example" {
  alias = "a"
}

provider "example" {
  alias = "b"
}

module "child" {
  source = "./child"

  providers = {
    example.a = example.a
    example.b = example.b
  }
}

module "uninit" {
  source = "./uninit"
}
