check "expected_to_fail" {
  assert {
    condition = test_resource.resource.value != "value"
    error_message = "something"
  }
}

run "test" {}
