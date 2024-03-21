variable "sample_test_value" {
  type    = string
  default = "test_value"
}

output "sample_test_value" {
  sensitive = false
  value = var.sample_test_value
}

terraform {
  required_providers {
    docker = {
      source  = "test/docker"
      version = ">= 2.0.0"
    }
  }
}

# Pulls the image
resource "docker_image" "ubuntu" {
  name = "ubuntu:latest"
}

# Create a container
resource "docker_container" "foo" {
  image = docker_image.ubuntu.image_id
  name  = "foo"
}