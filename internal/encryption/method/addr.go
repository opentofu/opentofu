// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package method

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/hcl/v2"
)

// TODO is there a generalized way to regexp-check names?
var addrRe = regexp.MustCompile(`^method\.([a-zA-Z_0-9-]+)\.([a-zA-Z_0-9-]+)$`)
var nameRe = regexp.MustCompile("^([a-zA-Z_0-9-]+)$")
var idRe = regexp.MustCompile("^([a-zA-Z_0-9-]+)$")

// Addr is a type-alias for method address strings that identify a specific encryption method configuration.
// The Addr is an opaque value. Do not perform string manipulation on it outside the functions supplied by the
// method package.
type Addr string

// Validate validates the Addr for formal naming conformance, but does not check if the referenced method actually
// exists in the configuration.
func (a Addr) Validate() hcl.Diagnostics {
	if !addrRe.MatchString(string(a)) {
		return hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid encryption method address",
				Detail: fmt.Sprintf(
					"The supplied encryption method address does not match the required form of %s",
					addrRe.String(),
				),
			},
		}
	}
	return nil
}

// NewAddr creates a new Addr type from the provider and name supplied. The Addr is a type-alias for encryption method
// address strings that identify a specific encryption method configuration. You should treat the value as opaque and
// not perform string manipulation on it outside the functions supplied by the method package.
func NewAddr(method string, name string) (addr Addr, err hcl.Diagnostics) {
	if !nameRe.MatchString(method) {
		err = err.Append(
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "The provided encryption method type is invalid",
				Detail: fmt.Sprintf(
					"The supplied encryption method type (%s) does not match the required form of %s.",
					method,
					nameRe.String(),
				),
			},
		)
	}
	if !nameRe.MatchString(name) {
		err = err.Append(
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "The provided encryption method name is invalid",
				Detail: fmt.Sprintf(
					"The supplied encryption method name (%s) does not match the required form of %s.",
					name,
					nameRe.String(),
				),
			},
		)
	}
	return Addr(fmt.Sprintf("method.%s.%s", method, name)), err
}
