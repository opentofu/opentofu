run "test" {
  module {
    source = "./tests/module"
  }
  assert {
	condition = output.name == "hello"
    error_message = "Incorrect name: ${output.name}"
  }
}
