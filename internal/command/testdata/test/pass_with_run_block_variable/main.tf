# main.tf

variable "aws_access_key_id" {
  type    = string
  default = "unix:///Users/siddharthasonker/.docker/run/docker.sock"
}

output "aws_access_key_id" {
  sensitive = false
  value = var.aws_access_key_id
}

terraform {
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
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
  image = docker_image.ubuntu.latest
  name  = "foo"
}