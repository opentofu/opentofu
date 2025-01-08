terraform {
  encryption {
    key_provider "externalcommand" "foo" {
      command = ["./some_program", "some_parameter"]
    }
  }
}