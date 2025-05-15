terraform {
  encryption {
    # Key provider configuration here

    method "aes_gcm" "yourname" {
      keys = key_provider.your_key_provider_type.your_key_provider_name
    }
  }
}