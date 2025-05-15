// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tracing

import "testing"

func TestExtractImportPath(t *testing.T) {
	tests := []struct {
		fullName string
		expected string
	}{
		{
			fullName: "github.com/opentofu/opentofu/internal/getproviders.(*registryClient).Get",
			expected: "github.com/opentofu/opentofu/internal/getproviders",
		},
		{
			fullName: "github.com/opentofu/opentofu/pkg/module.Function",
			expected: "github.com/opentofu/opentofu/pkg/module",
		},
		{
			fullName: "main.main",
			expected: "main",
		},
		{
			fullName: "unknownFormat",
			expected: "unknown",
		},
	}

	for _, test := range tests {
		got := extractImportPath(test.fullName)
		if got != test.expected {
			t.Errorf("extractImportPath(%q) = %q; want %q", test.fullName, got, test.expected)
		}
	}
}
