run "test" {
  command = plan
  plan_options {
    refresh = false
  }
  variables {
    image_name = "myapp"
  }
  assert {
    condition     = docker_image.build.name == "myapp"
    error_message = "Missing build resource"
  }
}