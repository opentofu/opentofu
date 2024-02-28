removed {
  from = module.child.foo.basic_resource
}

module "child" {
  source = "./child"
}
