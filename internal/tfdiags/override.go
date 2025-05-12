// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"github.com/hashicorp/hcl/v2"
)

// overriddenDiagnostic implements the Diagnostic interface by wrapping another
// Diagnostic while overriding the severity of the original Diagnostic.
type overriddenDiagnostic struct {
	original Diagnostic
	severity Severity
	extra    interface{}
}

var _ Diagnostic = overriddenDiagnostic{}
var _ contextualFromConfigBody = overriddenDiagnostic{}

// OverrideAll accepts a set of Diagnostics and wraps them with a new severity
// and, optionally, a new ExtraInfo.
func OverrideAll(originals Diagnostics, severity Severity, createExtra func() DiagnosticExtraWrapper) Diagnostics {
	var diags Diagnostics
	for _, diag := range originals {
		diags = diags.Append(Override(diag, severity, createExtra))
	}
	return diags
}

// Override matches OverrideAll except it operates over a single Diagnostic
// rather than multiple Diagnostics.
func Override(original Diagnostic, severity Severity, createExtra func() DiagnosticExtraWrapper) Diagnostic {
	extra := original.ExtraInfo()
	if createExtra != nil {
		nw := createExtra()
		nw.WrapDiagnosticExtra(extra)
		extra = nw
	}

	return overriddenDiagnostic{
		original: original,
		severity: severity,
		extra:    extra,
	}
}

// UndoOverride will return the original diagnostic that was overridden within
// the OverrideAll function.
//
// If the provided Diagnostic was never overridden then it is simply returned
// unchanged.
func UndoOverride(diag Diagnostic) Diagnostic {
	if override, ok := diag.(overriddenDiagnostic); ok {
		return override.original
	}

	// Then it wasn't overridden, so we'll just return the diag unchanged.
	return diag
}

func (o overriddenDiagnostic) Severity() Severity {
	return o.severity
}

func (o overriddenDiagnostic) Description() Description {
	return o.original.Description()
}

func (o overriddenDiagnostic) Source() Source {
	return o.original.Source()
}

func (o overriddenDiagnostic) FromExpr() *FromExpr {
	return o.original.FromExpr()
}

func (o overriddenDiagnostic) ExtraInfo() interface{} {
	return o.extra
}

// ElaborateFromConfigBody implements contextualFromConfigBody.
//
// If the original diagnostic also implements contextualFromConfigBody then this
// delegates to its own ElaborateFromConfigBody implementation and then re-wraps
// the result with the same overrides.
//
// Otherwise, the reciever is returned unchanged.
func (o overriddenDiagnostic) ElaborateFromConfigBody(body hcl.Body, addr string) Diagnostic {
	innerContextual, ok := o.original.(contextualFromConfigBody)
	if !ok {
		return o // inner diagnostic is not contextual, so nothing to do
	}

	newOriginal := innerContextual.ElaborateFromConfigBody(body, addr)
	return overriddenDiagnostic{
		original: newOriginal,
		severity: o.severity,
		extra:    o.extra,
	}
}
