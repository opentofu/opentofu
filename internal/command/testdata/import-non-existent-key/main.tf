locals {
  items = toset(["a", "b"])
}

resource "test_instance" "this" {
  for_each = local.items
}

import {
  id = "ff"
  to = test_instance.this["f"]
}
