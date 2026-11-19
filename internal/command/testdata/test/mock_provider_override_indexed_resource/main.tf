terraform {
  required_providers {
    test = {
      source = "hashicorp/test"
    }
  }
}

variable "ids" {
  type    = list(string)
  default = ["id1", "id2"]
}


import {
  to = test_resource.example[tonumber(each.key)]
  id = each.value
  for_each = {
    for idx, item in var.ids : idx => item
  }
}

resource "test_resource" "example" {
  count = 2
}

output "computed_values" {
  value = test_resource.example[*].computed_value
}
