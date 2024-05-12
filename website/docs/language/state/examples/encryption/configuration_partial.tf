terraform {
  encryption {
    #key_provider block will be set via environment variable

    method "aes_gcm" "main" {
      keys = key_provider.pbkdf2.main
    }

    state {
      enforced = true
      method   = method.aes_gcm.main
    }

    plan {
      enforced = true
      method   = method.aes_gcm.main
    }
  }
}