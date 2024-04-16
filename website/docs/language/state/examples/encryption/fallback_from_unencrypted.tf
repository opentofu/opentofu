terraform {
  encryption {
    # Methods and key providers here.
    method "unencrypted" "migrate" {}

    state {
      method = method.some_method.new_method
      fallback {
        # The unencrypted method in a fallback block allows reading unencrypted state files.
        method = method.unencrypted.migrate
      }
    }
  }
}
