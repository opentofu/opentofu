resource "test_resource" "resource" {}

check "check" {
  assert {
    condition = test_resource.resource.id == ""
    error_message = "check block: resource has no id"
  }
}