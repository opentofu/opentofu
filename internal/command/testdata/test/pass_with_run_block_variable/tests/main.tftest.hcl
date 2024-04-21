variables {
  sample_test_value = "us-east-1"
}

provider "test" {
  region = var.sample_test_value
}

run "test" {
  // ... a normal testing block ...
  command = plan
}