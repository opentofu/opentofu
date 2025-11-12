terraform {
  required_providers {
    null = {
      # This intentionally refers to our predecessor project's registry directly
      # because we use this to test the situation where that hostname is
      # specified explicitly.
      #
      # This registry's terms of service does not allow use from OpenTofu, so
      # it's only acceptable to use a configuration like this with OpenTofu
      # when using custom installation methods to remap this hostname to a
      # separate mirror source. DO NOT USE THIS TEST FIXTURE IN ANY TEST THAT
      # USES THE "direct" INSTALLATION METHOD FOR THIS HOSTNAME!
      source = "registry.terraform.io/hashicorp/null"
    }
  }
}