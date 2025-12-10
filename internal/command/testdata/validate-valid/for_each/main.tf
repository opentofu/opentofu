variable "server_ids" {
  type    = list(string)
  default = ["one", "two"]
}

resource "test_instance" "main" {
  count = 2
}

import {
  to = test_instance.main[tonumber(each.key)]
  id = each.value
  for_each = {
    for idx, item in var.server_ids : idx => item
  }
}
