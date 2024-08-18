// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestValidateHuman(t *testing.T) {
	testCases := map[string]struct {
		diag          tfdiags.Diagnostic
		wantSuccess   bool
		wantSubstring string
	}{
		"success": {
			nil,
			true,
			"The configuration is valid.",
		},
		"warning": {
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Your shoelaces are untied",
				"Watch out, or you'll trip!",
			),
			true,
			"The configuration is valid, but there were some validation warnings",
		},
		"error": {
			tfdiags.Sourceless(
				tfdiags.Error,
				"Configuration is missing random_pet",
				"Every configuration should have a random_pet.",
			),
			false,
			"Error: Configuration is missing random_pet",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)
			view.Configure(&arguments.View{NoColor: true})
			v := NewValidate(arguments.ViewHuman, view)

			var diags tfdiags.Diagnostics

			if tc.diag != nil {
				diags = diags.Append(tc.diag)
			}

			ret := v.Results(diags)

			if tc.wantSuccess && ret != 0 {
				t.Errorf("expected 0 return code, got %d", ret)
			} else if !tc.wantSuccess && ret != 1 {
				t.Errorf("expected 1 return code, got %d", ret)
			}

			got := done(t).All()
			if strings.Contains(got, "Success!") != tc.wantSuccess {
				t.Errorf("unexpected output:\n%s", got)
			}
			if !strings.Contains(got, tc.wantSubstring) {
				t.Errorf("expected output to include %q, but was:\n%s", tc.wantSubstring, got)
			}
		})
	}
}

func TestValidateHuman_InPedanticMode(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)
	view.PedanticMode = true

	validate := NewValidate(arguments.ViewHuman, view)
	diags := tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Warning, "Output as error", "")}

	retCode := validate.Results(diags)
	if retCode != 1 {
		t.Errorf("expected: 1 got: %v", retCode)
	}

	want := "Error: Output as error"
	got := strings.TrimSpace(done(t).Stderr())

	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected: %v got: %v", want, got)
	}

	if !view.LegacyViewErrorFlagged {
		t.Errorf("expected: true, got: %v", view.LegacyViewErrorFlagged)
	}
}

func TestValidateJSON(t *testing.T) {
	testCases := map[string]struct {
		diag        tfdiags.Diagnostic
		wantSuccess bool
	}{
		"success": {
			nil,
			true,
		},
		"warning": {
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Your shoelaces are untied",
				"Watch out, or you'll trip!",
			),
			true,
		},
		"error": {
			tfdiags.Sourceless(
				tfdiags.Error,
				"Configuration is missing random_pet",
				"Every configuration should have a random_pet.",
			),
			false,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)
			view.Configure(&arguments.View{NoColor: true})
			v := NewValidate(arguments.ViewJSON, view)

			var diags tfdiags.Diagnostics

			if tc.diag != nil {
				diags = diags.Append(tc.diag)
			}

			ret := v.Results(diags)

			if tc.wantSuccess && ret != 0 {
				t.Errorf("expected 0 return code, got %d", ret)
			} else if !tc.wantSuccess && ret != 1 {
				t.Errorf("expected 1 return code, got %d", ret)
			}

			got := done(t).All()

			// Make sure the result looks like JSON; we comprehensively test
			// the structure of this output in the command package tests.
			var result map[string]interface{}

			if err := json.Unmarshal([]byte(got), &result); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateJSON_InPedanticMode(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)
	view.PedanticMode = true
	validate := NewValidate(arguments.ViewJSON, view)

	diags := tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Warning, "Output as error", "")}

	retCode := validate.Results(diags)

	if retCode != 1 {
		t.Errorf("expected: 1 got: %v", retCode)
	}

	var got map[string]interface{}
	want := map[string]interface{}{
		"format_version": "1.0",
		"valid":          false,
		"error_count":    float64(1),
		"warning_count":  float64(0),
		"diagnostics": []interface{}{
			map[string]interface{}{
				"severity": "error",
				"summary":  "Output as error",
				"detail":   "",
			},
		},
	}

	if err := json.Unmarshal([]byte(done(t).Stdout()), &got); err != nil {
		t.Fatalf("error unmarsling json: %s", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected: %v got: %v", want, got)
	}

	if !view.LegacyViewErrorFlagged {
		t.Errorf("expected: true, got: %v", view.LegacyViewErrorFlagged)
	}
}
