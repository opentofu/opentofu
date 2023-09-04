run "first" {
  variables {
    input = "first"
  }

  assert {
    condition = test_resource.foo.value == "first"
    error_message = "invalid value"
  }
}

run "second" {
  command=plan
  plan_options {
    mode=refresh-only
  }

  variables {
    input = "second"
  }

  assert {
    condition = test_resource.foo.value == "first"
    error_message = "invalid value"
  }
}

run "third" {
  command=plan
  plan_options {
    mode=normal
  }

  variables {
    input = "second"
  }

  assert {
    condition = test_resource.foo.value == "second"
    error_message = "invalid value"
  }
}
