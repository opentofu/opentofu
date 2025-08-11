// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"testing"
)

func TestSuffixUIInput_impl(t *testing.T) {
	var _ UIInput = new(SuffixUIInput)
}

func TestSuffixUIInput(t *testing.T) {
	input := new(MockUIInput)
	suffix := &SuffixUIInput{
		QuerySuffix: " (custom suffix)",
		UIInput:     input,
	}

	_, err := suffix.Input(context.Background(), &InputOpts{Id: "bar", Query: "var.bar"})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if input.InputOpts.Query != "var.bar (custom suffix)" {
		t.Fatalf("bad: %#v", input.InputOpts)
	}
}

func TestNewEphemeralSuffixUIInput(t *testing.T) {
	input := new(MockUIInput)
	suffix := NewEphemeralSuffixUIInput(input)

	_, err := suffix.Input(context.Background(), &InputOpts{Id: "bar", Query: "var.bar"})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if input.InputOpts.Query != "var.bar (ephemeral)" {
		t.Fatalf("bad: %#v", input.InputOpts)
	}
}
