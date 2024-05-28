# This is the default "docker" provider for this file:
provider "docker" {
  host = "tcp://0.0.0.0:2376"
}

# This will be the override:
provider "docker" {
  alias = "unixsocket"
  host = "unix:///var/run/docker.sock"
}

run "sockettest" {
  # Replace the "docker" provider for this test case only:
  providers = {
    docker = docker.unixsocket
  }

  assert {
    condition     = docker_image.build.name == "myapp"
    error_message = "Missing build resource"
  }
}

// Add other tests with the original provider here.