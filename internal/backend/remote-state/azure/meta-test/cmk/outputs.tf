output "storage_account_name" {
  value = azurerm_storage_account.cmk.name
}

output "resource_group_name" {
  value = azurerm_resource_group.cmk.name
}

output "container_name" {
  value = azurerm_storage_container.cmk.name
}

output "encryption_scope_name" {
  value = azurerm_storage_encryption_scope.cmk.name
}
