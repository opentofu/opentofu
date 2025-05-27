// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"testing"

	"github.com/go-test/deep"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestAddressedTypesAbs(t *testing.T) {
	providerAddrs := []addrs.AbsProviderInstance{
		addrs.AbsProviderInstance{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("aws"),
		},
		addrs.AbsProviderInstance{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("aws"),
			Alias:    "foo",
		},
		addrs.AbsProviderInstance{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("azure"),
		},
		addrs.AbsProviderInstance{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("null"),
		},
		addrs.AbsProviderInstance{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("null"),
		},
	}

	got := AddressedTypesAbs(providerAddrs)
	want := []addrs.Provider{
		addrs.NewDefaultProvider("aws"),
		addrs.NewDefaultProvider("azure"),
		addrs.NewDefaultProvider("null"),
	}
	for _, problem := range deep.Equal(got, want) {
		t.Error(problem)
	}
}
