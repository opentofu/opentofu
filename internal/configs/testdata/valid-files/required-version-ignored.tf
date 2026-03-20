terraform {
  # Because this isn't in a .tofu-suffixed file, we ignore it on the assumption
  # that it's describing a requirement for OpenTofu's predecessor. Therefore
  # this is considered "valid" just because we pay no attention to it.
  required_version = "999.99"
}
