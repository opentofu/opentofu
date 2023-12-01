variable "image_name" {
  default = "app"
}

terraform {
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

resource "docker_image" "build" {
  name = var.image_name
  build {
    context = "."
  }
}