terraform {
  encryption {
    key_provider "azure_vault" "asymmetric" {
      vault_uri = "https://example-keys.vault.azure.net"
      key_name = "my-rsa-key"
      key_length = 32
    }
  }
}
