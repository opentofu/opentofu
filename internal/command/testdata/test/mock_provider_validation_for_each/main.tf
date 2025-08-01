provider "test" {
  alias    = "by_region"
  for_each = toset(var.regions)
}

variable "regions" {
  type    = list(string)
  default = ["us-east-1"]
}

resource "test_resource" "primary" {
  value    = "foo"
  for_each = toset(var.regions)
  provider = test.by_region[each.value]
}
