// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestAddressedTypesAbs(t *testing.T) {
	providerAddrs := []addrs.AbsProviderConfig{
		addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("aws"),
		},
		addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("aws"),
			Alias:    "foo",
		},
		addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("azure"),
		},
		addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("null"),
		},
		addrs.AbsProviderConfig{
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
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result:\n" + diff)
	}
}
