terraform {
  encryption {
    # Key provider configuration here

    method "aes_gcm" "yourname" {
      keys = key_provider.yourkeyprovider.yourname
    }
  }
}