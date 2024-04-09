variable "sample_test_value" {
  type    = string
  default = "nowhere"
}

output "sample_test_value" {
  sensitive = false
  value = var.sample_test_value
}

provider "test" {
  region = "somewhere"
}