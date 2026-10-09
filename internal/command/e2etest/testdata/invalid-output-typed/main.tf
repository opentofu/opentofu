module "child" {
  source = "./child"
}

output "result" {
  value = module.child.raw
  type = number
}
