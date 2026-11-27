run "validate_test_resource_other" {
  assert {
    condition = test_resource.bar.value == "Hello!"
    error_message = "invalid value"
  }
}
