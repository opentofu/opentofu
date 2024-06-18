locals {
  # Change passphrase to be at least 16 characters long:
  passphrase = "changeme!"
}

terraform {
  encryption {
    ## Step 1: Add the desired key provider:
    key_provider "pbkdf2" "mykey" {
      ## You can also use variables here!
      passphrase = local.passphrase
    }
    ## Step 2: Set up your encryption method:
    method "aes_gcm" "new_method" {
      keys = key_provider.pbkdf2.mykey
    }

    state {
      ## Step 3: Link the desired encryption method:
      method = method.aes_gcm.new_method

      ## Step 4: Run "tofu apply".

      ## Step 5: Consider adding the "enforced" option:
      # enforced = true
    }

    ## Step 6: Repeat steps 3-5 for plan{} if needed.
  }
}
