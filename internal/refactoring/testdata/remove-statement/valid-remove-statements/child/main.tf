removed {
  from = foo.removed_resource_from_child_module
}

module "grandchild" {
  source = "./grandchild"
}

removed {
  from = module.removed_module_from_child_module
}