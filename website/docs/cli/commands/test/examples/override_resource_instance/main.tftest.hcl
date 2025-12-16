
override_data {
  target = data.local_file.greeting["file_a"]
  values = {
    content = "Howdy"
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
    condition     = output.greeting_a == "Howdy World"
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
