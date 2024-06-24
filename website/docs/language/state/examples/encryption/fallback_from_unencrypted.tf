variable "passphrase" {
  # Change passphrase to be at least 16 characters long:
  default = "changeme!"
}

terraform {
  encryption {
    ## Step 1: Add the unencrypted method:
    method "unencrypted" "migrate" {}

    ## Step 2: Add the desired key provider:
    key_provider "pbkdf2" "mykey" {
      passphrase = var.passphrase
    }

    ## Step 3: Add the desired encryption method:
    method "aes_gcm" "new_method" {
      keys = key_provider.pbkdf2.mykey
    }

    state {
      ## Step 4: Link the desired encryption method:
      method = method.aes_gcm.new_method

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
