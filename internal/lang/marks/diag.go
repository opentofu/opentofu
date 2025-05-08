// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package marks

import "github.com/opentofu/opentofu/internal/tfdiags"

// DiagnosticOutputDeprecationCause checks whether the given diagnostic is
// a deprecation warning, and if so returns the deprecation cause and
// true. If not, returns the zero value of DeprecationCause and false.
func DiagnosticOutputDeprecationCause(diag tfdiags.Diagnostic) (DeprecationCause, bool) {
	maybe := tfdiags.ExtraInfo[diagnosticExtraDeprecationCause](diag)
	if maybe == nil {
		return DeprecationCause{}, false
	}
	return maybe.diagnosticDeprecationCause(), true
}

type diagnosticExtraDeprecationCause interface {
	diagnosticDeprecationCause() DeprecationCause
}

// diagnosticDeprecationCause implements diagnosticExtraDeprecationCause
func (c DeprecationCause) diagnosticDeprecationCause() DeprecationCause {
	return c
}

// deprecatedOutputDiagnosticExtra is a container for the DeprecationCause used to decide later if the diagnostic
// needs to be shown or not.
// This definition is needed because in ExtractDeprecationDiagnosticsWithBody, we return tfdiags.AttributeValue which does
// not support adding extra information. Therefore, we wrap tfdiags.AttributeValue in tfdiags.Override that allows to
// add extraInfo the diagnostic. So this struct is actually a container for the extraInfo that we want to propagate in the
// diagnostic.
type deprecatedOutputDiagnosticExtra struct {
	Cause DeprecationCause

	wrapped interface{}
}

var (
	_ diagnosticExtraDeprecationCause = (*deprecatedOutputDiagnosticExtra)(nil)
	_ tfdiags.DiagnosticExtraWrapper  = (*deprecatedOutputDiagnosticExtra)(nil)
)

func (c *deprecatedOutputDiagnosticExtra) WrapDiagnosticExtra(inner interface{}) {
	if c.wrapped != nil {
		// This is a logical inconsistency, the caller should know whether they have already wrapped an extra or not.
		panic("Attempted to wrap a diagnostic extra into a deprecatedOutputDiagnosticExtra that is already wrapping a different extra. This is a bug in OpenTofu, please report it.")
	}
	c.wrapped = inner
}

func (c *deprecatedOutputDiagnosticExtra) diagnosticDeprecationCause() DeprecationCause {
	return c.Cause
}

// DeprecatedOutputDiagnosticOverride is mainly created for unit testing. This is done this way just to avoid
// exporting deprecatedOutputDiagnosticExtra from this package, which can create confusion when somebody would like to use this package.
func DeprecatedOutputDiagnosticOverride(cause DeprecationCause) func() tfdiags.DiagnosticExtraWrapper {
	return func() tfdiags.DiagnosticExtraWrapper {
		return &deprecatedOutputDiagnosticExtra{
			Cause: cause,
		}
	}
}
