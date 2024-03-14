terraform {
  encryption {
    # Methods and key providers here.

    state {
      method = method.some_method.new_method
      fallback {
        method = method.some_method.old_method
      }
    }

    plan {
      method = method.some_method.new_method
      fallback {
        method = method.some_method.old_method
      }
    }
  }
}