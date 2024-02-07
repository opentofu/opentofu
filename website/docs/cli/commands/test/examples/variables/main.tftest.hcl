# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# First, set the variable here:
variables {
  name = "OpenTofu"
}

run "basic" {
  assert {
    condition     = output.greeting == "Hello OpenTofu!"
    error_message = "Incorrect greeting: ${output.greeting}"
  }
}

run "override" {
  # Override it for this test case only here:
  variables {
    name = "OpenTofu user"
  }
  assert {
    condition     = output.greeting == "Hello OpenTofu user!"
    error_message = "Incorrect greeting: ${output.greeting}"
  }
}