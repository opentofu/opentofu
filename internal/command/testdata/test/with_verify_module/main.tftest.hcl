variables {
  id = "resource"
  value = "Hello, world!"
}

run "test" {
}

run "verify" {
  module {
    source = "./verify"
  }

  assert {
    condition = data.test_data_source.resource_data.value == "Hello, world!"
    error_message = "bad value"
  }

  assert {
    condition = test_resource.another_resource.id == "hi"
    error_message = "bad value"
  }
}