removed {
  from = foo.basic_resource
}

removed {
  from = module.basic_module
}

removed {
  from = module.child.foo.removed_resource_from_root_module
}

module "child" {
  source = "./child"
}
