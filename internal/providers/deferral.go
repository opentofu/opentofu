// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

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
