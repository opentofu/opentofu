terraform {
  encryption {
    key_provider "azure_vault" "my_key" {
      vault_uri = "https://hardware-example.managedhsm.azure.net/"
      vault_key_name = "my-aes-key"
      symmetric = true

      symmetric_key_size = 192
      key_length = 32
    }
    method "aes_gcm" "crypto" {
      keys = key_provider.azure_vault.my_key
    }
    state {
      method = method.aes_gcm.crypto
    }
    plan {
      method = method.aes_gcm.crypto
    }
  }
}
