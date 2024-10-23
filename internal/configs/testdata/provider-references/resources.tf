resource "null_resource" "a" {
  provider = null
}

resource "null_resource" "b" {
  provider = null["alias"]
}

resource "null_resource" "c" {
  provider = null.alias
}

resource "null_resource" "d" {
  provider = null["${local.alias}"]
}

resource "null_resource" "e" {
  for_each = local.aliases
  provider = null[each.key]
}

resource "null_resource" "f" {
  for_each = local.aliases
  provider = null[each.value]
}

resource "null_resource" "g" {
  count = 2
  provider = null["a-${count.index}"]
}

resource "null_resource" "h" {
  count = 2
  provider = null["${local.alias}"]
}
