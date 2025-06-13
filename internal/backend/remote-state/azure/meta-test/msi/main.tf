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
}

resource "azurerm_resource_group" "storage_test" {
  name     = local.resource_group_name
  location = var.location
}

resource "azurerm_virtual_network" "example" {
  name                = "example-network"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.storage_test.location
  resource_group_name = azurerm_resource_group.storage_test.name
}

resource "azurerm_subnet" "example" {
  name                 = "internal"
  resource_group_name  = azurerm_resource_group.storage_test.name
  virtual_network_name = azurerm_virtual_network.example.name
  address_prefixes     = ["10.0.2.0/24"]
}

resource "azurerm_public_ip" "pip" {
  name                = "storage-test-pip"
  resource_group_name = azurerm_resource_group.storage_test.name
  location            = azurerm_resource_group.storage_test.location
  allocation_method   = "Static"
}

resource "azurerm_network_security_group" "ssh" {
  name                = "ssh_2_machine"
  location            = azurerm_resource_group.storage_test.location
  resource_group_name = azurerm_resource_group.storage_test.name
  security_rule {
    access                     = "Allow"
    direction                  = "Inbound"
    name                       = "SSH"
    priority                   = 1001
    protocol                   = "Tcp"
    source_port_range          = "*"
    source_address_prefix      = "*"
    destination_port_range     = "22"
    destination_address_prefix = "*"
  }
}

resource "azurerm_network_interface" "example" {
  name                = "example-nic"
  location            = azurerm_resource_group.storage_test.location
  resource_group_name = azurerm_resource_group.storage_test.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.example.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.pip.id
  }
}


resource "azurerm_network_interface_security_group_association" "main" {
  network_interface_id      = azurerm_network_interface.example.id
  network_security_group_id = azurerm_network_security_group.ssh.id
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

resource "azurerm_linux_virtual_machine" "example" {
  name                = "msi-test-machine"
  resource_group_name = azurerm_resource_group.storage_test.name
  location            = azurerm_resource_group.storage_test.location
  size                = "Standard_D2s_v3"
  computer_name       = "hostname"
  admin_username      = local.vm_username
  network_interface_ids = [
    azurerm_network_interface.example.id,
  ]

  admin_ssh_key {
    username   = local.vm_username
    public_key = file(var.ssh_pub_key_path)
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.example.id]
  }

  depends_on = [azurerm_role_assignment.example]
}
