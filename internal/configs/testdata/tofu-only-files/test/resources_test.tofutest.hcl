# test_run_one runs a partial plan
run "test_run_one" {
  command = plan

  plan_options {
    target = [
      test_object.b
    ]
  }

  assert {
    condition = test_object.b.test_string == "world"
    error_message = "invalid value"
  }
}

# test_run_two does a complete apply operation
run "test_run_two" {
  variables {
    input = "custom"
  }

  assert {
    condition = test_object.a.test_string == "hello"
    error_message = "invalid value"
  }
}
