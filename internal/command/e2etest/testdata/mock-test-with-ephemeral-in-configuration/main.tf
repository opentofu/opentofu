# Minimal reproducer for https://github.com/opentofu/opentofu/issues/4251
# Bug requires to run `tofu test` when the configuration contains also an `ephemeral` block

terraform {
  required_version = ">= 1.11.5"
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "3.8.1"
    }
    sleep = {
      source  = "yottta/sleep"
      version = "0.0.4"
    }
  }
}

resource "random_id" "test" {
  byte_length = 2
}

ephemeral "random_password" "test" {
  length = 10

}

resource "sleep_sleeper" "test" {
  string_wo         = ephemeral.random_password.test.result
  string_wo_version = 1
}
