terraform {
  required_providers {
    dummy = {
      source = "my/dummy"
    }
  }
}

locals {
        fe = {}
} 

provider "dummy" {
  alias = "key"
  for_each = local.fe
}

resource "dummy_warning" "test" {
  for_each = local.fe
  provider = dummy.key[each.key]
}


module "mod_warning" {
  source = "./mod"
  for_each = local.fe
  providers = {
        dummy = dummy.key[each.key]
  }
}

resource "dummy_error" "test" {
  for_each = local.fe
  provider = dumme.key[each.key]
}


module "mod_error" {
  source = "./mod"
  for_each = local.fe
  providers = {
        dummy = dumme.key[each.key]
  }
}

