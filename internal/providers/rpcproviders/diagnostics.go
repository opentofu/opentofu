// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerops"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type withClientDiags interface {
	Diagnostics() providerops.Diagnostics
}

type withClientFunctionError interface {
	Error() providerops.FunctionError
}

// appendDiags attempts to extract and transform whatever diagnostics it can
// from the given response and error.
//
// This is a generic, reusable helper just to reduce boilerplate across
// all of the [provider.Interface] method implementations that delegate to
// the underlying client.
//
// It can handle:
//   - Response types that follow the convention for returning arbitrary
//     diagnostics, using a method named Diagnostics.
//   - Response types that include a function call error
//     (i.e. CallFunctionResponse).
//   - Errors that can be returned by calls to the underlying client, which
//     should typically describe RPC-level problems like the connection being
//     interrupted or the provider not implementing some operation at all.
func appendDiags(diags tfdiags.Diagnostics, response any, err error) tfdiags.Diagnostics {
	if wd, ok := response.(withClientDiags); ok {
		diags = appendClientDiags(diags, wd)
	}
	if wd, ok := response.(withClientFunctionError); ok {
		diags = appendClientFunctionErrorDiags(diags, wd)
	}
	diags = appendClientErrorDiags(diags, err)
	return diags
}

func appendClientDiags(diags tfdiags.Diagnostics, wd withClientDiags) tfdiags.Diagnostics {
	// TODO: implement
	return diags
}

func appendClientFunctionErrorDiags(diags tfdiags.Diagnostics, we withClientFunctionError) tfdiags.Diagnostics {
	// TODO: implement
	return diags
}

func appendClientErrorDiags(diags tfdiags.Diagnostics, err error) tfdiags.Diagnostics {
	// TODO: implement
	return diags
}
