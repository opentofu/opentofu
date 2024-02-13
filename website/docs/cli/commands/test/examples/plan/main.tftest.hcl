# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

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