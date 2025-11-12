run "example" {
  command = plan

  assert {
    condition = output.example == "example"
    error_message = "ruh-roh"
  }
}
