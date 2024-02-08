# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

run "test-failure" {
  variables {
    # This healthcheck endpoint won't exist:
    health_endpoint = "/nonexistent"
  }

  expect_failures = [
    # We expect this to fail:
    check.health
  ]
}