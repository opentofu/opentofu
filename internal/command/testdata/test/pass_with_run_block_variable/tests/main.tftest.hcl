# tests/main.tftest.hcl

provider "docker" {
  host = run.setup.aws_access_key_id
}

run "setup" {
  module {
    source = "./tests/setup"
  }
}

run "test" {
  // ... a normal testing block ...
  command = plan
}