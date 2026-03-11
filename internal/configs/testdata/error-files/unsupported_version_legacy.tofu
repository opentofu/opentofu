terraform {
  # This is the old way to declare OpenTofu version constraints for a module,
  # from before OpenTofu v1.12. This form is accepted only in .tofu-suffixed files.
  required_version = "999.9" # ERROR: Incompatible module
}
