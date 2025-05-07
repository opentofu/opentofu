variable "passphrase" {
  # Change passphrase to be at least 16 characters long:
  default   = "changeme!"
  sensitive = true
}

terraform {
  encryption {
    ## Step 1: Add the unencrypted method:
    method "unencrypted" "migrate" {}

    ## Step 2: Add the desired key provider:
    key_provider "pbkdf2" "my_key_provider_name" {
      passphrase = var.passphrase
    }

    ## Step 3: Add the desired encryption method:
    method "aes_gcm" "my_method_name" {
      keys = key_provider.pbkdf2.my_key_provider_name
    }

    state {
      ## Step 4: Link the desired encryption method:
      method = method.aes_gcm.my_method_name

      ## Step 5: Add the "fallback" block referencing the
      ## "unencrypted" method.
      fallback {
        method = method.unencrypted.migrate
      }

      ## Step 6: Run "tofu apply".

      ## Step 7: Remove the "fallback" block above and
      ## consider adding the "enforced" option:
      # enforced = true
    }

    ## Step 8: Repeat steps 4-8 for plan{} if needed.
  }
}
