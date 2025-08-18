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
	diags = appendClientErrorDiags(diags, err)
	return diags
}

func appendClientDiags(diags tfdiags.Diagnostics, wd withClientDiags) tfdiags.Diagnostics {
	for diag := range wd.Diagnostics().All() {
		// TODO: Support AttributePath here once the upstream library has
		// support for it. In that case we'd need to use tfdiags.AttributeValue
		// instead of tfdiags.Sourceless here.
		diags = diags.Append(tfdiags.Sourceless(
			convertDiagnosticSeverity(diag.Severity()),
			diag.Summary(),
			diag.Detail(),
		))
	}
	return diags
}

func appendClientErrorDiags(diags tfdiags.Diagnostics, err error) tfdiags.Diagnostics {
	// FIXME: Make this recognize certain common error types and transform
	// them into user-friendly diagnostics.
	if err != nil {
		diags = diags.Append(err)
	}
	return diags
}

func appendConvertSchemaDiags(diags tfdiags.Diagnostics, err error) tfdiags.Diagnostics {
	if err == nil {
		return diags
	}
	// FIXME: Make this into a real diagnostic
	return diags.Append(err)
}

func convertDiagnosticSeverity(severity providerops.DiagnosticSeverity) tfdiags.Severity {
	switch severity {
	case providerops.DiagnosticError:
		return tfdiags.Error
	case providerops.DiagnosticWarning:
		return tfdiags.Warning
	default:
		var zero tfdiags.Severity
		return zero
	}
}
