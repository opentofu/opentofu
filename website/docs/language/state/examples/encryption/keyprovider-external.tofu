terraform {
  encryption {
    key_provider "external" "foo" {
      command = ["./some_program", "some_parameter"]
    }
  }
}
