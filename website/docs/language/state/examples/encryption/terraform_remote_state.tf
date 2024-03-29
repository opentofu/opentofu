terraform {
  encryption {
    # Key provider and method configuration here

    remote_state_data_sources {
      default {
        method = method.my_method.my_name
      }
      remote_state_data_source "my_state" {
        method = method.my_method.my_other_name
      }
    }
  }
}

data "terraform_remote_state" "my_state" {
  # ...
}