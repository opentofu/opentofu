variable "on" {
  type = bool
}

module "mod1" {
  source = "./mod1"

  lifecycle {
    enabled = var.on
  }
}

output "result" {
  // This is in a 1-tuple just because OpenTofu treats a fully-null
  // root module output value as if it wasn't declared at all,
  // but we want to make sure we're actually testing the result
  // of this resource directly.
  value = [try(module.mod1.result, "")]
}
