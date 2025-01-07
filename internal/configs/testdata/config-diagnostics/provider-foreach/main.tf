locals {
        stuff = toset([])
	bval = true
	i = 1
}

provider "null" { # Constant, no warning
        for_each = {"foo": "bar"}
        alias = "plain"
}
resource "null_resource" "plain" {
        for_each = {"foo": "bar"}
        provider = null.plain[each.key]
}

provider "null" { # Constant, no warning
        for_each = toset(["foo", "bar"])
        alias = "toset"
}
resource "null_resource" "toset" {
        for_each = toset(["foo", "bar"])
        provider = null.toset[each.key]
}

provider "null" {
        for_each = local.stuff
        alias = "value"
}
resource "null_resource" "value" {
        for_each = local.stuff
        provider = null.value[each.key]
}

provider "null" {
        for_each = local.stuff 
        alias = "parens"
}
resource "null_resource" "parens" {
        for_each = (local.stuff)
        provider = null.parens[each.key]
}

provider "null" {
        for_each = local.bval ? {"foo": "bar"} : {}
        alias = "cond"
}
resource "null_resource" "cond" {
        for_each = local.bval ? {"foo": "bar"} : {}
        provider = null.cond[each.key]
}

provider "null" {
        for_each = {"foo": local.i + -2}
        alias = "op"
}
resource "null_resource" "op" {
        for_each = {"foo": local.i + -2}
        provider = null.op[each.key]
}

provider "null" {
        for_each = {for s in local.stuff : s => upper(s)}
        alias = "for"
}
resource "null_resource" "for" {
        for_each = {for s in local.stuff : s => upper(s)}
        provider = null.for[each.key]
}
