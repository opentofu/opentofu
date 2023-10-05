output "string" {
  value = var.string != null ? var.string : ""
}

variable "string" {
  type      = string
  default   = null
  sensitive = true
}

output "list" {
  value = var.list != null ? var.list : []
}

variable "list" {
  type      = list(string)
  default   = null
  sensitive = true
}

output "bool" {
  value = var.bool != null ? var.bool : false
}

variable "bool" {
  type      = bool
  default   = null
  sensitive = true
}

output "map" {
  value = var.map != null ? var.map : { foo = "bar" }
}

variable "map" {
  type      = map(string)
  default   = null
  sensitive = true
}

output "number" {
  value = var.number != null ? var.number : 6
}

variable "number" {
  type      = number
  default   = null
  sensitive = true
}

output "object" {
  value = var.object != null ? var.object : {}
}

variable "object" {
  type      = object({})
  default   = null
  sensitive = true
}

output "set" {
  value = var.set != null ? var.set : [false]
}

variable "set" {
  type      = set(bool)
  default   = null
  sensitive = true
}

output "tuple" {
  value = var.tuple != null ? var.tuple :[]
}

variable "tuple" {
  type      = tuple([string])
  default   = null
  sensitive = true
}

output "any" {
  value = var.any != null ? var.any : ""
}

variable "any" {
  type      = any
  default   = null
  sensitive = true
}