terraform {
  encryption {
    key_provider "external" "foo" {
      encrypt_command = ["./some_program", "--encrypt"]
      decrypt_command = ["./some_program", "--decrypt"]
      # Optional:
      keys = key_provider.some.provider
    }
  }
}