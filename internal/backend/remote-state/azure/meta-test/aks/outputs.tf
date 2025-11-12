output "storage_account_name" {
  value = local.storage_account_name
}

output "resource_group_name" {
  value = local.resource_group_name
}

output "container_name" {
  value = local.container_name
}

output "cluster_name" {
  value = local.cluster_name
}

output "ksa_name" {
  value = local.k8s_sa_name
}

output "az_client_id" {
  value = azurerm_user_assigned_identity.example.client_id
}
