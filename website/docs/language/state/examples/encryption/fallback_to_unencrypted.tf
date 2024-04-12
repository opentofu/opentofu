terraform {
  encryption {
    # Methods and key providers here.
    method "unencrypted" "migrate" {}

    state {
      # The unencrypted method allows writing unencrypted state files.
      # unless the enforced setting is set to true.
      method = method.unencrypted.migrate
      fallback {
        method = method.some_method.old_method
      }
    }
  }
}
