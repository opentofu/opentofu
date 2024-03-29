// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

// Addr is a type-alias for key provider address strings that identify a specific key provider configuration.
// The Addr is an opaque value. Do not perform string manipulation on it outside the functions supplied by the
// keyprovider package.
type Addr string

// Validate validates the Addr for formal naming conformance, but does not check if the referenced key provider actually
// exists in the configuration.
func (a Addr) Validate() hcl.Diagnostics {
	if !addrRe.MatchString(string(a)) {
		return hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid key provider address",
				Detail: fmt.Sprintf(
					"The supplied key provider address does not match the required form of %s",
					addrRe.String(),
				),
			},
		}
	}
	return nil
}

// NewAddr creates a new Addr type from the provider and name supplied. The Addr is a type-alias for key provider
// address strings that identify a specific key provider configuration. You should treat the value as opaque and not
// perform string manipulation on it outside the functions supplied by the keyprovider package.
func NewAddr(provider string, name string) (addr Addr, err hcl.Diagnostics) {
	if !nameRe.MatchString(provider) {
		err = err.Append(
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "The provided key provider type is invalid",
				Detail: fmt.Sprintf(
					"The supplied key provider type (%s) does not match the required form of %s.",
					provider,
					nameRe.String(),
				),
			},
		)
	}
	if !nameRe.MatchString(name) {
		err = err.Append(
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "The provided key provider name is invalid",
				Detail: fmt.Sprintf(
					"The supplied key provider name (%s) does not match the required form of %s.",
					name,
					nameRe.String(),
				),
			},
		)
	}
	return Addr(fmt.Sprintf("key_provider.%s.%s", provider, name)), err
}
