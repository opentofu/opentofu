output "storage_account_name" {
  value = local.storage_account_name
}

output "resource_group_name" {
  value = local.resource_group_name
}

output "container_name" {
  value = local.container_name
}

output "ssh_username" {
  value = local.vm_username
}

output "ssh_ip" {
  value = azurerm_public_ip.pip.ip_address
}
