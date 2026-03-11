language {
  compatible_with {
    # Specifying a version constraint that accepts OpenTofu v1.11.0 produces
    # a warning because this syntax for version constraints was added in
    # v1.12.0, so it's impossible to successfully declare compatibility with
    # an earlier version using this syntax.
    opentofu = ">= 1.11" # WARNING: Ineffective version constraint
  }
}
