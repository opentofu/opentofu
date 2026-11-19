run "validate_test_resource" {
  assert {
    condition = test_resource.foo.value == "hello world"
    error_message = "invalid value"
  }
}

run "expected_error" {
  command = plan
  variables {
    instances = 10
  }
  expect_failures = [
    var.instances,
  ]
}

run "validate_test_resource" {
  assert {
    condition = test_resource.foo.value == "hello world!"
    error_message = "invalid value"
  }
}
