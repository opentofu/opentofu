language {
  compatible_with {
    opentofu = ">= 1.12"

    # Anything else that's valid in an HCL body is permitted here and completely
    # ignored by OpenTofu, so that other software can define its own
    # compatibility-related arguments.
    ignored = "blah"
    also_ignored {}
  }

  # The following are the only valid ways to set these arguments in today's
  # OpenTofu. We support these only enough to return specialized error messages
  # if we find something like what we're imagining we might support in future
  # versions of OpenTofu.
  edition = tofu2024
  experiments = []
}
