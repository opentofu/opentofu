terraform {
  required_providers {
    corefunc = {
      source = "northwood-labs/corefunc"
      version = "2.3.0"
    }
  }
}

output "lp_basic" {
  value = provider::corefunc::str_leftpad("foobar", 10)
}

output "lp_char" {
  value = provider::corefunc::str_leftpad("foobar", 10, "_")
}
