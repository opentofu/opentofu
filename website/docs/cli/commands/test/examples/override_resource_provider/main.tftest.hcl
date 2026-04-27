mock_provider "local" {
  override_data {
    target = data.local_file.greeting["file_a"]
    values = {
      content = "Howdy"
    }
  }
  override_data {
    target = data.local_file.greeting["file_b"]
    values = {
      content = "Salutations"
    }
  }
  override_data {
    target = data.local_file.greeting[*]
    values = {
      content = "Hey"
    }
  }
}

override_data {
  target = data.local_file.greeting["file_a"]
  values = {
    content = "Aloha"
  }
}
override_data {
  target = data.local_file.greeting[*]
  values = {
    content = "Hello"
  }
}

run "test" {
  assert {
    condition     = output.greeting_a == "Aloha World"
    error_message = "Incorrect greeting: ${output.greeting_a}"
  }
  assert {
    condition     = output.greeting_b == "Hello World"
    error_message = "Incorrect greeting: ${output.greeting_a}"
  }
  assert {
    condition     = output.greeting_c == "Hello World"
    error_message = "Incorrect greeting: ${output.greeting_a}"
  }
}
