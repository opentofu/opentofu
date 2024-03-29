module "child" {
  source = "./child"
}

removed {
  from = module.child
}