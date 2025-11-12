locals {
  items = toset(["a", "b"])
}

module "child" {
  source   = "./child"
  for_each = local.items

  id = each.key
}
