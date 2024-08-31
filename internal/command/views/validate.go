// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/format"
	viewsjson "github.com/opentofu/opentofu/internal/command/views/json"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// The Validate is used for the validate command.
type Validate interface {
	// Results renders the diagnostics returned from a validation walk, and
	// returns a CLI exit code: 0 if there are no errors, 1 otherwise
	Results(diags tfdiags.Diagnostics) int

	HasErrors(diags tfdiags.Diagnostics) bool

	// Diagnostics renders early diagnostics, resulting from argument parsing.
	Diagnostics(diags tfdiags.Diagnostics)
}

// NewValidate returns an initialized Validate implementation for the given ViewType.
func NewValidate(vt arguments.ViewType, view *View) Validate {
	switch vt {
	case arguments.ViewJSON:
		return &ValidateJSON{view: view}
	case arguments.ViewHuman:
		return &ValidateHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", vt))
	}
}

// The ValidateHuman implementation renders diagnostics in a human-readable form,
// along with a success/failure message if OpenTofu is able to execute the
// validation walk.
type ValidateHuman struct {
	view *View
}

var _ Validate = (*ValidateHuman)(nil)

func (v *ValidateHuman) Results(diags tfdiags.Diagnostics) int {
	columns := v.view.outputColumns()

	if len(diags) == 0 {
		v.view.streams.Println(format.WordWrap(v.view.colorize.Color(validateSuccess), columns))
	} else {
		v.Diagnostics(diags)

		if !v.HasErrors(diags) {
			v.view.streams.Println(format.WordWrap(v.view.colorize.Color(validateWarnings), columns))
		}
	}

	if v.HasErrors(diags) {
		return 1
	}
	return 0
}

const validateSuccess = "[green][bold]Success![reset] The configuration is valid."

const validateWarnings = "[green][bold]Success![reset] The configuration is valid, but there were some validation warnings as shown above."

func (v *ValidateHuman) HasErrors(diags tfdiags.Diagnostics) bool {
	return v.view.HasErrors(diags)
}

func (v *ValidateHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

// The ValidateJSON implementation renders validation results as a JSON object.
// This object includes top-level fields summarizing the result, and an array
// of JSON diagnostic objects.
type ValidateJSON struct {
	view *View
}

var _ Validate = (*ValidateJSON)(nil)

func (v *ValidateJSON) Results(diags tfdiags.Diagnostics) int {
	// FormatVersion represents the version of the json format and will be
	// incremented for any change to this format that requires changes to a
	// consuming parser.
	const FormatVersion = "1.0"

	type Output struct {
		FormatVersion string `json:"format_version"`

		// We include some summary information that is actually redundant
		// with the detailed diagnostics, but avoids the need for callers
		// to re-implement our logic for deciding these.
		Valid        bool                    `json:"valid"`
		ErrorCount   int                     `json:"error_count"`
		WarningCount int                     `json:"warning_count"`
		Diagnostics  []*viewsjson.Diagnostic `json:"diagnostics"`
	}

	output := Output{
		FormatVersion: FormatVersion,
		Valid:         true, // until proven otherwise
	}
	configSources := v.view.configSources()

	if v.view.PedanticMode {
		// Convert warnings to errors
		var isOverridden bool
		diags, isOverridden = tfdiags.OverrideAllFromTo(diags, tfdiags.Warning, tfdiags.Error, nil)
		if isOverridden {
			v.view.PedanticWarningFlagged = true
		}
	}

	for _, diag := range diags {
		output.Diagnostics = append(output.Diagnostics, viewsjson.NewDiagnostic(diag, configSources))

		switch diag.Severity() {
		case tfdiags.Error:
			output.ErrorCount++
			output.Valid = false
		case tfdiags.Warning:
			output.WarningCount++
		}
	}
	if output.Diagnostics == nil {
		// Make sure this always appears as an array in our output, since
		// this is easier to consume for dynamically-typed languages.
		output.Diagnostics = []*viewsjson.Diagnostic{}
	}

	j, err := json.MarshalIndent(&output, "", "  ")
	if err != nil {
		// Should never happen because we fully-control the input here
		panic(err)
	}
	v.view.streams.Println(string(j))

	if v.HasErrors(diags) {
		return 1
	}
	return 0
}

func (v *ValidateJSON) HasErrors(diags tfdiags.Diagnostics) bool {
	return v.view.HasErrors(diags)
}

// Diagnostics should only be called if the validation walk cannot be executed.
// In this case, we choose to render human-readable diagnostic output,
// primarily for backwards compatibility.
func (v *ValidateJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}
