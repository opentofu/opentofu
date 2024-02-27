// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

// ConfigRemovable is an interface implemented by address types that represents
// the destination of a "removed" statement in configuration.
//
// Note that ConfigRemovable might represent:
//  1. An absolute address relative to the root of the configuration.
//  2. A direct representation of these in configuration where the author gives an
//     address relative to the current module where the address is defined.
type ConfigRemovable interface {
	Targetable
	configRemovableSigil()

	String() string
}

// The following are all the possible ConfigRemovable address types:
var (
	_ ConfigRemovable = ConfigResource{}
	_ ConfigRemovable = Module(nil)
)
