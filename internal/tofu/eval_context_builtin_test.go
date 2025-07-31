// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"reflect"
	"sync"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

func TestBuiltinEvalContextProviderInput(t *testing.T) {
	var lock sync.Mutex
	cache := make(map[string]map[string]cty.Value)

	ctx1 := testBuiltinEvalContext(t)
	ctx1 = ctx1.WithPath(addrs.RootModuleInstance).(*BuiltinEvalContext)
	ctx1.ProviderInputConfig = cache
	ctx1.ProviderLock = &lock

	ctx2 := testBuiltinEvalContext(t)
	ctx2 = ctx2.WithPath(addrs.RootModuleInstance.Child("child", addrs.NoKey)).(*BuiltinEvalContext)
	ctx2.ProviderInputConfig = cache
	ctx2.ProviderLock = &lock

	providerAddr1 := addrs.AbsProviderConfig{
		Module:   addrs.RootModule,
		Provider: addrs.NewDefaultProvider("foo"),
	}
	providerAddr2 := addrs.AbsProviderConfig{
		Module:   addrs.RootModule.Child("child"),
		Provider: addrs.NewDefaultProvider("foo"),
	}

	expected1 := map[string]cty.Value{"value": cty.StringVal("foo")}
	ctx1.SetProviderInput(t.Context(), providerAddr1, expected1)

	try2 := map[string]cty.Value{"value": cty.StringVal("bar")}
	ctx2.SetProviderInput(t.Context(), providerAddr2, try2) // ignored because not a root module

	actual1 := ctx1.ProviderInput(t.Context(), providerAddr1)
	actual2 := ctx2.ProviderInput(t.Context(), providerAddr2)

	if !reflect.DeepEqual(actual1, expected1) {
		t.Errorf("wrong result 1\ngot:  %#v\nwant: %#v", actual1, expected1)
	}
	if actual2 != nil {
		t.Errorf("wrong result 2\ngot:  %#v\nwant: %#v", actual2, nil)
	}
}

func TestBuildingEvalContextInitProvider(t *testing.T) {
	var lock sync.Mutex

	testP := &MockProvider{}

	ctx := testBuiltinEvalContext(t)
	ctx = ctx.WithPath(addrs.RootModuleInstance).(*BuiltinEvalContext)
	ctx.ProviderLock = &lock
	ctx.ProviderCache = make(map[string]map[addrs.InstanceKey]providers.Interface)
	ctx.Plugins = newContextPlugins(map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): providers.FactoryFixed(testP),
	}, nil)

	providerAddrDefault := addrs.AbsProviderConfig{
		Module:   addrs.RootModule,
		Provider: addrs.NewDefaultProvider("test"),
	}
	providerAddrAlias := addrs.AbsProviderConfig{
		Module:   addrs.RootModule,
		Provider: addrs.NewDefaultProvider("test"),
		Alias:    "foo",
	}

	_, err := ctx.InitProvider(t.Context(), providerAddrDefault, addrs.NoKey)
	if err != nil {
		t.Fatalf("error initializing provider test: %s", err)
	}
	_, err = ctx.InitProvider(t.Context(), providerAddrAlias, addrs.NoKey)
	if err != nil {
		t.Fatalf("error initializing provider test.foo: %s", err)
	}
}

func testBuiltinEvalContext(t *testing.T) *BuiltinEvalContext {
	return &BuiltinEvalContext{}
}
