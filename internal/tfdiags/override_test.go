// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
)

func TestOverride_UpdatesSeverity(t *testing.T) {
	original := Sourceless(NewSeverity(ErrorLevel), "summary", "detail")
	override := Override(original, NewSeverity(WarningLevel), nil)

	if override.Severity().SeverityLevel != WarningLevel {
		t.Errorf("expected warning but was %s", override.Severity())
	}
}

func TestOverride_MaintainsExtra(t *testing.T) {
	original := hclDiagnostic{&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "summary",
		Detail:   "detail",
		Extra:    "extra",
	}}
	override := Override(original, NewSeverity(WarningLevel), nil)

	if override.ExtraInfo().(string) != "extra" {
		t.Errorf("invalid extra info %v", override.ExtraInfo())
	}
}

func TestOverride_WrapsExtra(t *testing.T) {
	original := hclDiagnostic{&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "summary",
		Detail:   "detail",
		Extra:    "extra",
	}}
	override := Override(original, NewSeverity(WarningLevel), func() DiagnosticExtraWrapper {
		return &extraWrapper{
			mine: "mine",
		}
	})

	wrapper := override.ExtraInfo().(*extraWrapper)
	if wrapper.mine != "mine" {
		t.Errorf("invalid extra info %v", override.ExtraInfo())
	}
	if wrapper.original.(string) != "extra" {
		t.Errorf("invalid wrapped extra info %v", override.ExtraInfo())
	}
}

func TestUndoOverride(t *testing.T) {
	original := Sourceless(NewSeverity(ErrorLevel), "summary", "detail")
	override := Override(original, NewSeverity(WarningLevel), nil)
	restored := UndoOverride(override)

	if restored.Severity().SeverityLevel != ErrorLevel {
		t.Errorf("expected warning but was %s", restored.Severity())
	}
}

func TestUndoOverride_NotOverridden(t *testing.T) {
	original := Sourceless(NewSeverity(ErrorLevel), "summary", "detail")
	restored := UndoOverride(original) // Shouldn't do anything bad.

	if restored.Severity().SeverityLevel != ErrorLevel {
		t.Errorf("expected warning but was %s", restored.Severity())
	}
}

type extraWrapper struct {
	mine     string
	original interface{}
}

func (e *extraWrapper) WrapDiagnosticExtra(inner interface{}) {
	e.original = inner
}
