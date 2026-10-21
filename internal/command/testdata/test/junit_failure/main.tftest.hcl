run "validate_test_resource" {
  assert {
    condition = test_resource.foo.value == "hello world"
    error_message = "invalid value"
  }
}

run "validate_test_resource" {
  assert {
    condition = test_resource.foo.value == "not the value!"
    error_message = "invalid value"
  }
}
