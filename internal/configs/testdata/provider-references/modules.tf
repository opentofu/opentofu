module "submod-a" {
  count = 2

  source = "./submod"
  providers = {
    null              = null
    null.alternative  = null["a-${count.index}"]
  }
}

module "submod-b" {
  for_each = local.aliases

  source = "./submod"
  providers = {
    null              = null["alias"]
    null.alternative  = null[each.key]
  }
}

module "submod-c" {
  for_each = local.aliases

  source = "./submod"
  providers = {
    null              = null.alias
    null.alternative  = null[each.value]
  }
}

module "submod-d" {
  for_each = local.aliases

  source = "./submod"
  providers = {
    null              = null.alias
    null.alternative  = null["${local.alias}"]
  }
}

module "submod-e" {
  source = "./submod"
  providers = {
    null              = null.alias
    null.alternative  = null["${local.alias}"]
  }
}
