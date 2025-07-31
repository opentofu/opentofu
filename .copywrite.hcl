schema_version = 1

project {
  license        = "MPL-2.0"
  copyright_year = 2014

  # (OPTIONAL) Represents the copyright holder used in all statements
  # Default: HashiCorp, Inc.
    copyright_holder = "The OpenTofu Authors\nSPDX-License-Identifier: MPL-2.0\nCopyright (c) 2023 HashiCorp, Inc."

  # (OPTIONAL) A list of globs that should not have copyright/license headers.
  # Supports doublestar glob patterns for more flexibility in defining which
  # files or folders should be ignored
  header_ignore = [
    "**/*.tf",
    "**/*.tftest.hcl",
    "**/*.terraform.lock.hcl",
    "website/docs/**/examples/**",
    "**/testdata/**",
    "**/*.pb.go",
    "**/*_string.go",
    "**/mock*.go",
  ]
}
