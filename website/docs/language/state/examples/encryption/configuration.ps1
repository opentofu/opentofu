$Env:TF_ENCRYPTION = @"
terraform {
  encryption {
    key_provider "some_key_provider" "some_name" {
      # Key provider options here
    }

    method "some_method" "some_method_name" {
      # Method options here
      keys = key_provider.some_key_provider.some_name
    }

    statefile {
      # Encryption/decryption for local state files
      method = method.some_method.some_method_name
    }

    planfile {
      # Encryption/decryption for local plan files
      method = method.some_method.some_method_name
    }

    backend {
      # Encryption/decryption method for backends
      method = method.some_method.some_method_name
    }

    remote {
      # See below
    }
  }
}
"@