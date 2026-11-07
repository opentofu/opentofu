
locals {
  public_modules = "../public" # github.com/user/terraform-module-thing
}

module "producer" {
  source = "${local.public_modules}/producer"
}

module "consumer" {
  source = "${local.public_modules}/consumer"

  produced = module.producer
}
