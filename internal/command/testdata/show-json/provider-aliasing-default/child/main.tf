terraform {
  required_providers {
    test = {
      source = "opentofu/test"
    }
  }
}

resource "test_instance" "test" {
  ami = "bar"
}

module "with_requirement" {
  source     = "./nested"
  depends_on = [module.no_requirements]
}

module "no_requirements" {
  source = "./nested-no-requirements"
}
