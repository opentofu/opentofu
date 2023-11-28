terraform {
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

resource "docker_container" "webserver" {
  name  = "nginx-test"
  image = "nginx"
  ports {
    internal = 80
    external = 8080
  }
}