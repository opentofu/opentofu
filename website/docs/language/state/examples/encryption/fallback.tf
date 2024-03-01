terraform {
  encryption {
    # Methods and key providers here.

    statefile {
      method = method.some_method.new_method
      fallback {
        method = method.some_method.old_method
      }
    }

    planfile {
      method = method.some_method.new_method
      fallback {
        method = method.some_method.old_method
      }
    }

    backend {
      method = method.some_method.new_method
      fallback {
        method = method.some_method.old_method
      }
    }
  }
}