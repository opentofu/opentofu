terraform {
  encryption {
    # Methods and key providers here.

    statefile {
      method = method.some_method.new_method
      fallback {
        # The empty fallback block allows reading unencrypted state files.
      }
    }
  }
}