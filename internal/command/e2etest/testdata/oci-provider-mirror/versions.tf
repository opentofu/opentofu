terraform {
  required_providers {
    foo = {
      source = "example.com/test/foo"
    }
    bar = {
      source  = "example.com/test/bar"
      version = "~> 1.0.0" # Intentionally excludes 2.0.0 so we can test that version constraints are being handled properly
    }
  }
}