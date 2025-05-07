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
