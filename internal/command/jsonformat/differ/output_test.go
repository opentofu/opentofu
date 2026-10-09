// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package differ

import (
	"testing"

	"github.com/mitchellh/colorstring"

	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured/attribute_path"
)

// noopColorize returns a colorstring.Colorize that strips color codes so that
// test assertions can compare plain strings without ANSI escape sequences.
func noopColorize() *colorstring.Colorize {
	return &colorstring.Colorize{Colors: colorstring.DefaultColors, Disable: true, Reset: true}
}

// TestComputeDiffForOutput_SensitivityChange verifies that WarningsHuman fires
// exactly when the sensitivity status actually changes between before and after.
//
// This tests the rendering layer fix for issue #2680: once
// applyRendererOutputSensitivity has reconstructed distinct BeforeSensitive /
// AfterSensitive values, ComputeDiffForOutput (via checkForSensitiveType) and
// WarningsHuman must emit a warning if and only if the sensitivity changes.
func TestComputeDiffForOutput_SensitivityChange(t *testing.T) {
	tests := map[string]struct {
		beforeSensitive interface{}
		afterSensitive  interface{}
		beforeValue     interface{}
		afterValue      interface{}
		wantWarning     bool
	}{
		"becomes sensitive — warning expected": {
			beforeSensitive: false,
			afterSensitive:  true,
			beforeValue:     "hello",
			afterValue:      "hello",
			wantWarning:     true,
		},
		"no longer sensitive — warning expected": {
			beforeSensitive: true,
			afterSensitive:  false,
			beforeValue:     "hello",
			afterValue:      "hello",
			wantWarning:     true,
		},
		"both sensitive, value unchanged — no warning": {
			beforeSensitive: true,
			afterSensitive:  true,
			beforeValue:     "hello",
			afterValue:      "hello",
			wantWarning:     false,
		},
		"both not sensitive, value changes — no warning": {
			beforeSensitive: false,
			afterSensitive:  false,
			beforeValue:     "old",
			afterValue:      "new",
			wantWarning:     false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			change := structured.Change{
				Before:             tc.beforeValue,
				BeforeExplicit:     true,
				After:              tc.afterValue,
				AfterExplicit:      true,
				Unknown:            false,
				BeforeSensitive:    tc.beforeSensitive,
				AfterSensitive:     tc.afterSensitive,
				ReplacePaths:       attribute_path.Empty(false),
				RelevantAttributes: attribute_path.AlwaysMatcher(),
			}

			diff := ComputeDiffForOutput(change)

			opts := computed.NewRenderHumanOpts(noopColorize(), false)
			warnings := diff.WarningsHuman(0, opts)

			gotWarning := len(warnings) > 0
			if gotWarning != tc.wantWarning {
				t.Errorf("WarningsHuman returned %d warning(s), wantWarning=%t\nwarnings: %v",
					len(warnings), tc.wantWarning, warnings)
			}
		})
	}
}
