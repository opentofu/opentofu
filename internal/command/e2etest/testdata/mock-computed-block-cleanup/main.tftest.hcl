# Minimal reproducer for https://github.com/opentofu/opentofu/issues/3644

mock_provider "vsphere" {}

run "create" {}
