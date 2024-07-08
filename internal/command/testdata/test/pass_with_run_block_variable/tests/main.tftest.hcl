variables {
  sample_test_value = "data"
}

provider "test" {
  data_prefix = var.sample_test_value
}

run "test" {
  // ... a normal testing block ...
  command = plan
}