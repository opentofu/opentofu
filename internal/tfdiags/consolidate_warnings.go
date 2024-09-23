// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import "fmt"

// Consolidate checks if there is an unreasonable amount of diagnostics
// with the same summary in the receiver and, if so, returns a new diagnostics
// with some of those diagnostics consolidated into a single diagnostic in order
// to reduce the verbosity of the output.
//
// This mechanism is here primarily for diagnostics printed out at the CLI. In
// other contexts it is likely better to just return the diagnostic directly,
// particularly if they are going to be interpreted by software rather than
// by a human reader.
//
// The returned slice always has a separate backing array from the receiver,
// but some diagnostic values themselves might be shared.
//
// The definition of "unreasonable" is given as the threshold argument. At most
// that many diagnostics with the same summary will be shown.
func (diags Diagnostics) Consolidate(threshold int, level Severity) Diagnostics {
	if len(diags) == 0 {
		return nil
	}

	newDiags := make(Diagnostics, 0, len(diags))

	// We'll track how many times we've seen each diagnostic summary so we can
	// decide when to start consolidating. Once we _have_ started consolidating,
	// we'll also track the object representing the consolidated diagnostic
	// so we can continue appending to it.
	diagnosticStats := make(map[string]int)
	diagnosticGroups := make(map[string]*consolidatedGroup)

	for _, diag := range diags {
		severity := diag.Severity()
		if severity != level || diag.Source().Subject == nil {
			// Only the given level can get special treatment, and we only
			// consolidate diagnostics that have source locations because
			// our primary goal here is to deal with the situation where
			// some configuration language feature is producing a diagnostic
			// each time it's used across a potentially-large config.
			newDiags = newDiags.Append(diag)
			continue
		}

		if DoNotConsolidateDiagnostic(diag) {
			// Then do not consolidate this diagnostic.
			newDiags = newDiags.Append(diag)
			continue
		}

		desc := diag.Description()
		summary := desc.Summary
		if g, ok := diagnosticGroups[summary]; ok {
			// We're already grouping this one, so we'll just continue it.
			g.Append(diag)
			continue
		}

		diagnosticStats[summary]++
		if diagnosticStats[summary] == threshold {
			// Initially creating the group doesn't really change anything
			// visibly in the result, since a group with only one diagnostic
			// is just a passthrough anyway, but once we do this any additional
			// diagnostics with the same summary will get appended to this group.
			g := &consolidatedGroup{}
			newDiags = newDiags.Append(g)
			diagnosticGroups[summary] = g
			g.Append(diag)
			continue
		}

		// If this diagnostic is not consolidating yet then we'll just append
		// it directly.
		newDiags = newDiags.Append(diag)
	}

	return newDiags
}

// A consolidatedGroup is one or more diagnostics grouped together for
// UI consolidation purposes.
//
// A consolidatedGroup with only one diagnostic in it is just a passthrough for
// that one diagnostic. If it has more than one then it will behave mostly
// like the first one but its detail message will include an additional
// sentence mentioning the consolidation. A consolidatedGroup with no diagnostics
// at all is invalid and will panic when used.
type consolidatedGroup struct {
	Consolidated Diagnostics
}

var _ Diagnostic = (*consolidatedGroup)(nil)

func (wg *consolidatedGroup) Severity() Severity {
	return wg.Consolidated[0].Severity()
}

func (wg *consolidatedGroup) Description() Description {
	desc := wg.Consolidated[0].Description()
	if len(wg.Consolidated) == 1 {
		return desc
	}
	extraCount := len(wg.Consolidated) - 1
	var msg string
	var diagType string
	switch wg.Severity() {
	case Error:
		diagType = "error"
	case Warning:
		diagType = "warning"
	default:
		panic(fmt.Sprintf("Invalid diagnostic severity: %#v", wg.Severity()))
	}

	switch extraCount {
	case 1:
		msg = fmt.Sprintf("(and one more similar %s elsewhere)", diagType)
	default:
		msg = fmt.Sprintf("(and %d more similar %ss elsewhere)", extraCount, diagType)
	}
	if desc.Detail != "" {
		desc.Detail = desc.Detail + "\n\n" + msg
	} else {
		desc.Detail = msg
	}
	return desc
}

func (wg *consolidatedGroup) Source() Source {
	return wg.Consolidated[0].Source()
}

func (wg *consolidatedGroup) FromExpr() *FromExpr {
	return wg.Consolidated[0].FromExpr()
}

func (wg *consolidatedGroup) ExtraInfo() interface{} {
	return wg.Consolidated[0].ExtraInfo()
}

func (wg *consolidatedGroup) Append(diag Diagnostic) {
	if len(wg.Consolidated) != 0 && diag.Severity() != wg.Severity() {
		panic("can't append a non-warning diagnostic to a warningGroup")
	}
	wg.Consolidated = append(wg.Consolidated, diag)
}

// ConsolidatedGroupSourceRanges can be used in conjunction with
// Diagnostics.Consolidate to recover the full set of original source
// locations from a consolidated diagnostic.
//
// For convenience, this function accepts any diagnostic and will just return
// the single Source value from any diagnostic that isn't a consolidated group.
func ConsolidatedGroupSourceRanges(diag Diagnostic) []Source {
	wg, ok := diag.(*consolidatedGroup)
	if !ok {
		return []Source{diag.Source()}
	}

	ret := make([]Source, len(wg.Consolidated))
	for i, wrappedDiag := range wg.Consolidated {
		ret[i] = wrappedDiag.Source()
	}
	return ret
}
