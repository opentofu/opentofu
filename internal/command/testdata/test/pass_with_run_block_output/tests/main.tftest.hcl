provider "test" {
  data_prefix = run.setup.sample_test_value
}

run "setup" {
  module {
    source = "./tests/setup"
  }
}

run "test" {
}