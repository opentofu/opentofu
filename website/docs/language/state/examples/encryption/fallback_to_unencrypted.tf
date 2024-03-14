terraform {
  encryption {
    # Methods and key providers here.

    statefile {
      # The empty block allows writing unencrypted state files
      # unless the enforced setting is set to true.
      fallback {
        method = method.some_method.old_method
      }
    }
  }
}