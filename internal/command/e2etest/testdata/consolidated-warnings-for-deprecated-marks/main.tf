module "call" {
  source = "./mod1"
  input  = "test"
  input2 = "test2"
}

module "second_call" {
  source = "./mod1"
  input  = "test"
  input2 = "test2"
}

locals {
  i1 = module.call.modout1
  i2 = module.call.modout2
  i3 = module.second_call.modout1
  i4 = module.second_call.modout2
  # Duplicating the output from "call" module to consolidate
  i5 = module.call.modout1
  i6 = module.call.modout2
}
