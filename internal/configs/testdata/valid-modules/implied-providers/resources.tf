// These resources map to the configured "foo" provider"
resource foo_resource "a" {}
data foo_resource "b" {}
ephemeral foo_resource "c" {}

// These resources map to a default "hashicorp/bar" provider
resource bar_resource "d" {}
data bar_resource "e" {}
ephemeral bar_resource "f" {}

// These resources map to the configured "whatever" provider, which has FQN
// "acme/something".
resource whatever_resource "g" {}
data whatever_resource "h" {}
ephemeral whatever_resource "i" {}
