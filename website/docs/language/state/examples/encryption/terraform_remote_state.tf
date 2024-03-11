terraform {
  encryption {
    # Key provider and method configuration here

    remote {
      default {
        method = method.my_method.my_name
      }
      terraform_remote_state "my_state" {
        method = method.my_method.my_other_name
      }
    }
  }
}

data "terraform_remote_state" "my_state" {
  # ...
}