module "second" {
  source = "../second"
}

output "id" {
  value = module.second.id
}