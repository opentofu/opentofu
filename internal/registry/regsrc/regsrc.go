// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package regsrc provides helpers for working with source strings that identify
// resources within a OpenTofu registry.
package regsrc

var (
	// PublicRegistryHost is a FriendlyHost that represents the public registry.
	PublicRegistryHost = NewFriendlyHost("registry.opentofu.org")
)
