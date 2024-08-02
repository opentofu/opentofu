// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestMeta_addVarsFromFile(t *testing.T) {
	d := t.TempDir()
	defer testChdir(t, d)()

	hclData := `foo = "bar"`
	jsonData := `{"foo": "bar"}`

	cases := []struct {
		filename string
		contents string
		errors   bool
	}{
		{
			filename: "input.tfvars",
			contents: hclData,
			errors:   false,
		},
		{
			filename: "input.json",
			contents: jsonData,
			errors:   false,
		},
		{
			filename: "input_a.unknown",
			contents: hclData,
			errors:   false,
		},
		{
			filename: "input_b.unknown",
			contents: jsonData,
			errors:   false,
		},
		{
			filename: "mismatch.tfvars",
			contents: jsonData,
			errors:   true,
		},
		{
			filename: "mismatch.json",
			contents: hclData,
			errors:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			target := filepath.Join(d, tc.filename)
			err := os.WriteFile(target, []byte(tc.contents), 0600)
			if err != nil {
				t.Fatalf("err: %s", err)
			}

			m := new(Meta)
			to := make(map[string]backend.UnparsedVariableValue)
			diags := m.addVarsFromFile(target, tofu.ValueFromAutoFile, to)
			if tc.errors != diags.HasErrors() {
				t.Log(diags.Err())
				t.Errorf("Expected: %v, got %v", tc.errors, diags.HasErrors())
			}
		})
	}
}
