
# This intentionally refers to the registry of OpenTofu's predecessor, but
# the associated configuration refers to the shorthand "hashicorp/null"
# and so will be understood by OpenTofu as depending instead on
# "registry.opentofu.org/hashicorp/null", thereby activating our special
# fixup behavior and selecting the same version of OpenTofu's re-release
# of this provider.
provider "registry.terraform.io/hashicorp/null" {
  version = "3.2.0"
  hashes = [
    "h1:DvLRiv4Pbjq3Rh0yNWtq+9dwVXqHF+bQspfhckLyFWU=",
  ]
}
