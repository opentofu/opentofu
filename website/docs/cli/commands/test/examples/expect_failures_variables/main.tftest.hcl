# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

run "main" {
  command = plan

  variables {
    instances = -1
  }

  expect_failures = [
    var.instances,
  ]
}