// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"github.com/terramate-io/opentofulib/internal/tfdiags"
)

// diagnosticComparer is a Comparer function for use with cmp.Diff to compare two tfdiags.Diagnostic values
func diagnosticComparer(l, r tfdiags.Diagnostic) bool {
	if l.Severity() != r.Severity() {
		return false
	}
	if l.Description() != r.Description() {
		return false
	}

	lp := tfdiags.GetAttribute(l)
	rp := tfdiags.GetAttribute(r)
	if len(lp) != len(rp) {
		return false
	}
	return lp.Equals(rp)
}

// diagnosticSummaryComparer is a Comparer function for use with cmp.Diff to compare
// the Severity and Summary fields two tfdiags.Diagnostic values
func diagnosticSummaryComparer(l, r tfdiags.Diagnostic) bool {
	if l.Severity() != r.Severity() {
		return false
	}

	ld := l.Description()
	rd := r.Description()
	return ld.Summary == rd.Summary
}
