package marks

import "github.com/opentofu/opentofu/internal/tfdiags"

// DiagnosticDeprecationCause checks whether the given diagnostic is
// a deprecation warning, and if so returns the deprecation cause and
// true. If not, returns the zero value of DeprecationCause and false.
func DiagnosticDeprecationCause(diag tfdiags.Diagnostic) (DeprecationCause, bool) {
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

// DeprecatedOutputDiagnosticExtra is a container for the DeprecationCause used to decide later if the diagnostic
// needs to be shown or not
type DeprecatedOutputDiagnosticExtra struct {
	Cause DeprecationCause

	wrapped interface{}
}

var (
	_ diagnosticExtraDeprecationCause = (*DeprecatedOutputDiagnosticExtra)(nil)
	_ tfdiags.DiagnosticExtraWrapper  = (*DeprecatedOutputDiagnosticExtra)(nil)
)

func (c *DeprecatedOutputDiagnosticExtra) WrapDiagnosticExtra(inner interface{}) {
	if c.wrapped != nil {
		// This is a logical inconsistency, the caller should know whether they have already wrapped an extra or not.
		panic("Attempted to wrap a diagnostic extra into a DeprecatedOutputDiagnosticExtra that is already wrapping a different extra. This is a bug in OpenTofu, please report it.")
	}
	c.wrapped = inner
}

func (c *DeprecatedOutputDiagnosticExtra) diagnosticDeprecationCause() DeprecationCause {
	return c.Cause
}
