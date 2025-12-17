resource "time_static" "rg_timestamp" {}

resource "random_string" "resource_suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  storage_account_name = "acctestsa${random_string.resource_suffix.result}"
  resource_group_name  = "acctestRG-backend-${time_static.rg_timestamp.unix}-${random_string.resource_suffix.result}"
  container_name       = "acctestcont"
  vm_username          = "azureadmin"
  cluster_name         = "azClusterTest${random_string.resource_suffix.result}"
  dns_prefix           = "cluster-${random_string.resource_suffix.result}"

  k8s_sa_name = "workload-identity-${random_string.resource_suffix.result}"
}

resource "azurerm_resource_group" "storage_test" {
  name     = local.resource_group_name
  location = var.location
}

resource "azurerm_user_assigned_identity" "example" {
  location            = azurerm_resource_group.storage_test.location
  name                = "open-tofu-test-identity"
  resource_group_name = azurerm_resource_group.storage_test.name
}

resource "azurerm_storage_account" "test_account" {
  name                     = local.storage_account_name
  resource_group_name      = azurerm_resource_group.storage_test.name
  location                 = azurerm_resource_group.storage_test.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_storage_container" "test_container" {
  name                  = local.container_name
  storage_account_id    = azurerm_storage_account.test_account.id
  container_access_type = "private"
}

resource "azurerm_role_assignment" "example" {
  scope                = azurerm_storage_account.test_account.id
  role_definition_name = "Storage Account Contributor"
  principal_id         = azurerm_user_assigned_identity.example.principal_id
}

resource "azurerm_role_assignment" "blob_contributor" {
  scope                = azurerm_storage_container.test_container.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.example.principal_id
}

resource "azurerm_kubernetes_cluster" "main" {
  name                = local.cluster_name
  resource_group_name = azurerm_resource_group.storage_test.name
  location            = azurerm_resource_group.storage_test.location
  dns_prefix          = local.dns_prefix

  identity {
    type = "SystemAssigned"
  }

  default_node_pool {
    name       = "agentpool"
    vm_size    = "standard_d8_v3" // if it doesn't work, try "Standard_D2_v2". This is dependent on the availability in the Azure account.
    node_count = 1
    upgrade_settings {
      max_surge = "10%"
    }
  }
  linux_profile {
    admin_username = local.vm_username

    ssh_key {
      key_data = file(var.ssh_pub_key_path)
    }
  }
  network_profile {
    network_plugin    = "kubenet"
    load_balancer_sku = "standard"
  }

  oidc_issuer_enabled       = true
  workload_identity_enabled = true
}

resource "azurerm_federated_identity_credential" "ksa-wif" {
  name                = "k8sworkloadid"
  resource_group_name = azurerm_resource_group.storage_test.name
  audience            = ["api://AzureADTokenExchange"]
  issuer              = azurerm_kubernetes_cluster.main.oidc_issuer_url
  parent_id           = azurerm_user_assigned_identity.example.id
  subject             = "system:serviceaccount:default:${local.k8s_sa_name}"
}
