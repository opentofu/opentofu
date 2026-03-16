// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

resource "time_static" "rg_timestamp" {}

resource "random_string" "resource_suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  storage_account_name  = "acctestsa${random_string.resource_suffix.result}"
  resource_group_name   = "acctestRG-backend-cmk-${time_static.rg_timestamp.unix}-${random_string.resource_suffix.result}"
  container_name        = "acctestcont"
  key_vault_name        = "acctestkv${random_string.resource_suffix.result}"
  key_name              = "acctestkvkey${random_string.resource_suffix.result}"
  encryption_scope_name = "acctestencscope"
}

data "azurerm_client_config" "current" {}

resource "azurerm_resource_group" "cmk" {
  name     = local.resource_group_name
  location = var.location
}

resource "azurerm_user_assigned_identity" "cmk" {
  location            = azurerm_resource_group.cmk.location
  name                = "open-tofu-cmk-test-identity"
  resource_group_name = azurerm_resource_group.cmk.name
}

resource "azurerm_storage_account" "cmk" {
  name                     = local.storage_account_name
  resource_group_name      = azurerm_resource_group.cmk.name
  location                 = azurerm_resource_group.cmk.location
  account_tier             = "Standard"
  account_replication_type = "LRS"

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.cmk.id]
  }

  lifecycle {
    ignore_changes = [customer_managed_key]
  }
}

resource "azurerm_storage_container" "cmk" {
  name                  = local.container_name
  storage_account_id    = azurerm_storage_account.cmk.id
  container_access_type = "private"
}

resource "azurerm_role_assignment" "storage_contributor" {
  scope                = azurerm_storage_account.cmk.id
  role_definition_name = "Storage Account Contributor"
  principal_id         = azurerm_user_assigned_identity.cmk.principal_id
}

resource "azurerm_role_assignment" "blob_contributor" {
  scope                = azurerm_storage_container.cmk.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.cmk.principal_id
}

resource "azurerm_key_vault" "cmk" {
  name                       = local.key_vault_name
  location                   = azurerm_resource_group.cmk.location
  resource_group_name        = azurerm_resource_group.cmk.name
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  sku_name                   = "standard"
  purge_protection_enabled   = true
  soft_delete_retention_days = 7
}

# Allow the user-assigned identity to use the key for storage encryption
resource "azurerm_key_vault_access_policy" "storage_identity" {
  key_vault_id = azurerm_key_vault.cmk.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = azurerm_user_assigned_identity.cmk.principal_id

  key_permissions = [
    "Get", "WrapKey", "UnwrapKey",
  ]
}

# Allow the current user/SP running OpenTofu to manage keys
resource "azurerm_key_vault_access_policy" "current_user" {
  key_vault_id = azurerm_key_vault.cmk.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = data.azurerm_client_config.current.object_id

  key_permissions = [
    "Get", "Create", "Delete", "List", "Purge", "Recover", "Update", "GetRotationPolicy",
  ]
}

resource "azurerm_key_vault_key" "cmk" {
  name         = local.key_name
  key_vault_id = azurerm_key_vault.cmk.id
  key_type     = "RSA"
  key_size     = 2048
  key_opts     = ["decrypt", "encrypt", "sign", "unwrapKey", "verify", "wrapKey"]

  depends_on = [azurerm_key_vault_access_policy.current_user]
}

# Link the Key Vault key to the storage account via the user-assigned identity
resource "azurerm_storage_account_customer_managed_key" "cmk" {
  storage_account_id        = azurerm_storage_account.cmk.id
  key_vault_id              = azurerm_key_vault.cmk.id
  key_name                  = azurerm_key_vault_key.cmk.name
  user_assigned_identity_id = azurerm_user_assigned_identity.cmk.id

  depends_on = [azurerm_key_vault_access_policy.storage_identity]
}

# Encryption scope backed by the Key Vault key
resource "azurerm_storage_encryption_scope" "cmk" {
  name               = local.encryption_scope_name
  storage_account_id = azurerm_storage_account.cmk.id
  source             = "Microsoft.KeyVault"
  key_vault_key_id   = azurerm_key_vault_key.cmk.id

  depends_on = [azurerm_storage_account_customer_managed_key.cmk]
}
