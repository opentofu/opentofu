// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/providers"
)

func TestProviderForTest_ReadResource(t *testing.T) {
	mockProvider := &MockProvider{}

	provider := newProviderForTestWithSchema(mockProvider, mockProvider.GetProviderSchema())

	resp := provider.ReadResource(providers.ReadResourceRequest{
		TypeName: "test",
		Private:  []byte{},
	})

	if !resp.Diagnostics.HasErrors() {
		t.Fatalf("expected errors but none were found")
	}

	errMsg := resp.Diagnostics[0].Description().Summary
	if !strings.Contains(errMsg, "Unexpected null value for prior state") {
		t.Fatalf("expected prior state not found error but got: %s", errMsg)
	}
}
