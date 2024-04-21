run "setup" {
  module {
    source = "./tests/setup"
  }
}

provider "test" {
  value = run.setup.sample_test_value
}

run "test" {
  // ... a normal testing block ...
  command = plan
}