terraform {
  encryption {
    # Methods and key providers here.

    state {
      method = method.some_method.new_method_name
      fallback {
        method = method.some_method.old_method_name
      }
    }

    plan {
      method = method.some_method.new_method_name
      fallback {
        method = method.some_method.old_method_name
      }
    }
  }
}