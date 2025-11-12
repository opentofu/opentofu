// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DeferralReason is an enumeration of the different reasons a provider might
// return when reporting why it is unable to perform a requested action with
// the currently-available context.
type DeferralReason int

//go:generate go tool golang.org/x/tools/cmd/stringer -type=DeferralReason

const (
	// DeferredReasonUnknown is the zero value of DeferralReason, used when
	// a provider returns an unsupported deferral reason.
	DeferredReasonUnknown DeferralReason = 0

	// DeferredBecauseResourceConfigUnknown indicates that the request cannot
	// be completed because of unknown values in the resource configuration.
	DeferredBecauseResourceConfigUnknown DeferralReason = 1

	// DeferredBecauseProviderConfigUnknown indicates that the request cannot
	// be completed because of unknown values in the provider configuration.
	DeferredBecauseProviderConfigUnknown DeferralReason = 2

	// DeferredBecausePrereqAbsent indicates that the request cannot be
	// completed because a hard dependency has not been satisfied.
	DeferredBecausePrereqAbsent DeferralReason = 3
)

// NewDeferralDiagnostic returns a contextual error diagnostic reporting that
// an operation cannot be completed for the given deferral reason.
//
// The returned diagnostic will cause [IsDeferralDiagnostic] to return true,
// so that callers can optionally annotate the diagnostic with suggestions
// for how to skip the affected request so that other unaffected requests can
// still be completed.
func NewDeferralDiagnostic(reason DeferralReason) tfdiags.Diagnostic {
	summary := DeferralReasonSummary(reason)

	var detail string
	switch reason {
	case DeferredBecauseResourceConfigUnknown:
		detail = "The provider was unable to act on this resource configuration because it makes use of values from other resources that will not be known until after apply."
	case DeferredBecauseProviderConfigUnknown:
		detail = "The provider was unable to work with this resource because the associated provider configuration makes use of values from other resources that will not be known until after apply."
	default:
		// This is the most general (and therefore least helpful) message, which
		// we'll return if the provider produces a reason that we don't know about
		// yet, such as one added in a later protocol version. This fallback
		// is very much a last resort.
		// (Note that this also currently handles DeferredBecausePrereqAbsent,
		// because there are not yet any known examples of providers using that
		// reason and so we don't yet know what it will turn out to mean in
		// practice. If it becomes used in more providers in future then we can
		// hopefully devise a better message that describes what those providers
		// use it to mean.)
		detail = "The provider reported that it is not able to perform the requested operation until more information is available."
	}

	// We start with a "whole-body" contextual diagnostic, so that the caller
	// can elaborate this with any configuration body whose presence caused
	// the request to be made and thus add useful source location information
	// that we cannot determine at the provider layer.
	contextual := tfdiags.WholeContainingBody(tfdiags.Error, summary, detail)

	// We then further wrap that contextual diagnostic in an override so that
	// we can annotate it with the "extra info" that IsDeferralDiagnostic
	// will use to recognize diagnostics returned from this function.
	return tfdiags.Override(contextual, tfdiags.Error, func() tfdiags.DiagnosticExtraWrapper {
		return &deferralDiagnosticExtraImpl{reason: reason}
	})
}

// DeferralReasonSummary returns a more informative string representation of the given DeferralReason to be used
// it other places too.
// For more details, check the comments from NewDeferralDiagnostic.
func DeferralReasonSummary(reason DeferralReason) string {
	switch reason {
	case DeferredBecauseResourceConfigUnknown:
		return "Resource configuration is incomplete"
	case DeferredBecauseProviderConfigUnknown:
		return "Provider configuration is incomplete"
	default:
		return "Operation cannot be completed yet"
	}
}

// IsDeferralDiagnostic returns true if the given diagnostic was constructed
// with NewDeferralDiagnostic, meaning that it describes a situation where a
// provider reported that it is not yet able to complete an operation with the
// currently-available context.
func IsDeferralDiagnostic(diag tfdiags.Diagnostic) bool {
	// NOTE: We're intentionally not actually exposing the deferral reason
	// in the first incarnation of this functionality because the associated
	// plugin protocol feature is not yet well-deployed and so we're trying
	// to keep our use of it as confined to the provider-related packages
	// as possible in case ongoing protocol evolution causes us to need to
	// revise this in a later version. However, once the underlying protocol
	// has settled it would be reasonable to actually expose the reason
	// so that callers can substitute more contextually-relevant versions of
	// the error diagnostics when appropriate, or indeed choose to handle
	// this situation in a way that doesn't return an error to the user at all.
	extra := tfdiags.ExtraInfo[deferralDiagnosticExtra](diag)
	return extra != nil
}

type deferralDiagnosticExtra interface {
	deferralReason() DeferralReason
}

// deferralDiagnosticExtraImpl is an indirection used to combine
// deferralDiagnosticExtra with tfdiags.DiagnosticExtraWrapper because the
// design of tfdiags.Override unfortunately exposes its implementation
// detail of possibly needing to preserve an existing "extra info"
// as part of its public API.
//
// (Maybe we can improve tfdiags.Override to encapsulate this better in
// future, so that the type representing the union of two "extra info"
// values can be an unexported struct within package tfdiags, but
// this API is already being used elsewhere and so is risky to change.)
type deferralDiagnosticExtraImpl struct {
	reason  DeferralReason
	wrapped any
}

// WrapDiagnosticExtra implements deferralDiagnosticExtra.
func (d *deferralDiagnosticExtraImpl) deferralReason() DeferralReason {
	return d.reason
}

// WrapDiagnosticExtra implements tfdiags.DiagnosticExtraWrapper.
func (d *deferralDiagnosticExtraImpl) WrapDiagnosticExtra(inner any) {
	d.wrapped = inner
}
