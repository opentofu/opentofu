output "storage_account_name" {
  value = local.storage_account_name
}

output "resource_group_name" {
  value = local.resource_group_name
}

output "container_name" {
  value = local.container_name
}

output "repository_information" {
  value = azuredevops_git_repository.this
}
