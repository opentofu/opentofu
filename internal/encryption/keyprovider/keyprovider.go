// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

type Config interface {
	Build() (KeyProvider, error)
}

type Descriptor interface {
	// ID returns the unique identifier used when parsing HCL or JSON configs.
	ID() ID

	// ConfigStruct creates a new configuration struct pointer annotated with hcl tags. The Build() receiver on
	// this struct must be able to build a KeyProvider from the configuration:
	//
	// Common errors:
	// - Returning a struct without a pointer
	// - Returning a non-struct
	ConfigStruct() Config
}

type KeyProvider interface {
	// Provide provides an encryption key. If the process fails, it returns an error.
	Provide(metadata []byte) ([]byte, []byte, error)
}
