terraform {
  encryption {
    # Key provider and method configuration here

    remote_state_data_sources {
      default {
        method = method.method_type.my_method_name
      }
      remote_state_data_source "my_state" {
        method = method.method_type.my_other_method_name
      }
    }
  }
}

data "terraform_remote_state" "my_state" {
  # ...
}