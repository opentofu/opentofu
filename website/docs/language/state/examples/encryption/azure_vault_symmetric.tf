terraform {
  encryption {
    key_provider "azure_vault" "symmetric" {
      // symmetric keys are available in HSM only
      vault_uri = "https://hardware-example.managedhsm.azure.net/"
      vault_key_name = "my-aes-key"
      // We only select this if this is an AES key
      symmetric = true
      // Keep note of how large the key is when you create it.
      // This encryption key provider will only work if the size
      // specified here matches the size of the AES key generated.
      symmetric_key_size = 256

      // This key length relates to the data-encryption key,
      // NOT the key-encryption key specified above.
      key_length = 32
    }
  }
}
