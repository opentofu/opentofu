module "call" {
  source = "./mod"
  input = "test"
}

locals {
  _ = module.call.output
}