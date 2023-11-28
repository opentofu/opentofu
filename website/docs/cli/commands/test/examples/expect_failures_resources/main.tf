variable "health_endpoint" {
  default = "/"
}

terraform {
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

resource "docker_container" "webserver" {
  name  = ""
  image = "nginx"
  rm    = true

  ports {
    internal = 80
    external = 8080
  }
}

check "health" {
  data "http" "www" {
    url = "http://localhost:8080${var.health_endpoint}"
    depends_on = [docker_container.webserver]
  }

  assert {
    condition = data.http.www.status_code == 200
    error_message = "Invalid status code returned: ${data.http.www.status_code}"
  }
}