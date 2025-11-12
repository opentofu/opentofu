variable "passphrase" {
  # Change passphrase to be at least 16 characters long:
  default   = "changeme!"
  sensitive = true
}

terraform {
  encryption {
    ## Step 1: Add the desired key provider:
    key_provider "pbkdf2" "my_key_provider_name" {
      passphrase = var.passphrase
    }
    ## Step 2: Set up your encryption method:
    method "aes_gcm" "my_method_name" {
      keys = key_provider.pbkdf2.my_key_provider_name
    }

    state {
      ## Step 3: Link the desired encryption method:
      method = method.aes_gcm.my_method_name

      ## Step 4: Run "tofu apply".

      ## Step 5: Consider adding the "enforced" option:
      # enforced = true
    }

    ## Step 6: Repeat steps 3-5 for plan{} if needed.
  }
}
