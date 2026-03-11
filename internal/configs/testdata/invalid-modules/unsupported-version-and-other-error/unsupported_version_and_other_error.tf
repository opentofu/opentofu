language {
  compatible_with {
    opentofu = "999.9" # ERROR: Incompatible module
  }
}

# There should not be any error about the following block, even though its
# block type is not recognized by the current version of OpenTofu, because
# we assume it was added in some future version OpenTofu v999.9 based on
# the version constraint above.
unrecognized {}
