# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

run "test" {
  module {
    source="../"
  }
  variables {
    name = "OpenTofu"
  }
  assert {
    condition = output.greeting == "Hello OpenTofu!"
    error_message = "Incorrect greeting: ${output.greeting}"
  }
}