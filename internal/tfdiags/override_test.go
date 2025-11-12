// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
)

func TestOverride_UpdatesSeverity(t *testing.T) {
	original := Sourceless(Error, "summary", "detail")
	override := Override(original, Warning, nil)

	if override.Severity() != Warning {
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
	override := Override(original, Warning, nil)

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
	override := Override(original, Warning, func() DiagnosticExtraWrapper {
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

func TestOverride_Contextual(t *testing.T) {
	// Wrapping a body-contextual diagnostic with Override should still allow
	// for the final elaboration of the wrapped diagnostic, while also
	// preserving the override data.
	original := WholeContainingBody(
		Error,
		"Placeholder error",
		"...",
	)
	override := Override(original, Warning, func() DiagnosticExtraWrapper {
		return &extraWrapper{
			mine: "mine",
		}
	})
	var diags Diagnostics
	diags = diags.Append(override)
	diags = diags.InConfigBody(hcltest.MockBody(&hcl.BodyContent{}), "placeholder address")

	if got, want := len(diags), 1; got != want {
		t.Fatalf("wrong number of diagnostics after elaboration %d; want %d", got, want)
	}
	diag := diags[0]
	desc := diag.Description()
	if got, want := desc.Address, "placeholder address"; got != want {
		t.Errorf("wrong value for diag.Description().Address\ngot:  %q\nwant: %q", got, want)
	}
	if got, want := diag.Severity(), Warning; got != want {
		t.Errorf("wrong final severity %s; want %s", got, want)
	}
	wrapper, ok := diag.ExtraInfo().(*extraWrapper)
	if !ok {
		t.Fatalf("final diagnostic has wrong ExtraInfo type\ngot:  %T\nwant: %T", diag.ExtraInfo(), wrapper)
	}
	if got, want := wrapper.mine, "mine"; got != want {
		t.Errorf("wrong ExtraInfo value\ngot:  %q\nwant: %q", got, want)
	}
}

func TestUndoOverride(t *testing.T) {
	original := Sourceless(Error, "summary", "detail")
	override := Override(original, Warning, nil)
	restored := UndoOverride(override)

	if restored.Severity() != Error {
		t.Errorf("expected warning but was %s", restored.Severity())
	}
}

func TestUndoOverride_NotOverridden(t *testing.T) {
	original := Sourceless(Error, "summary", "detail")
	restored := UndoOverride(original) // Shouldn't do anything bad.

	if restored.Severity() != Error {
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
