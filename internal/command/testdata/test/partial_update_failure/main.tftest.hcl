run "partial" {
  plan_options {
    target = [test_resource.foo]
  }

  assert {
    condition = test_resource.bar.value == "bar"
    error_message = "should fail"
  }
}
