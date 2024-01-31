# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

run "test" {
  assert {
    condition     = file(local_file.test.filename) == "Hello world!"
    error_message = "Incorrect content in ${local_file.test.filename}."
  }
}