// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package method

// Config describes a configuration struct for setting up an encryption Method. You should always implement this
// interface with a struct, and you should tag the fields with HCL tags so the encryption implementation can read
// the .tf code into it. For example:
//
//	type MyConfig struct {
//	    Key string `hcl:"key"`
//	}
//
//	func (m MyConfig) Build() (Method, error) { ... }
type Config interface {
	// Build takes the configuration and builds an encryption method.
	// TODO this may be better changed to return hcl.Diagnostics so warnings can be issued?
	Build() (Method, error)
}

type Descriptor interface {
	// ID returns the unique identifier used when parsing HCL or JSON configs.
	ID() ID

	// ConfigStruct creates a new configuration struct annotated with hcl tags. The Build() receiver on
	// this struct must be able to build a Method from the configuration.
	//
	// Common errors:
	// - Returning a struct without a pointer
	// - Returning a non-struct
	ConfigStruct() Config
}

// Method is a low-level encryption method interface that is responsible for encrypting a binary blob of data. It should
// not try to interpret what kind of data it is encrypting.
type Method interface {
	// Encrypt encrypts the specified data with the set configuration. This method should treat any data passed as
	// opaque and should not try to interpret its contents. The interpretation is the job of the encryption.Encryption
	// interface.
	Encrypt(data []byte) ([]byte, error)
	// Decrypt decrypts the specified data with the set configuration. This method should treat any data passed as
	// opaque and should not try to interpret its contents. The interpretation is the job of the encryption.Encryption
	// interface.
	Decrypt(data []byte) ([]byte, error)
}
