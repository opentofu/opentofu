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
		{
			Module:   addrs.RootModuleInstance,
			Provider: addrs.NewDefaultProvider("aws"),
		},
		{
			Module:   addrs.RootModuleInstance,
			Provider: addrs.NewDefaultProvider("aws"),
			Key:      addrs.StringKey("foo"),
		},
		{
			Module:   addrs.RootModuleInstance,
			Provider: addrs.NewDefaultProvider("azure"),
		},
		{
			Module:   addrs.RootModuleInstance,
			Provider: addrs.NewDefaultProvider("null"),
		},
		{
			Module:   addrs.RootModuleInstance,
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
