// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statekeys

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states/statestore"
)

// RootModuleOutputValue is the key type for storing the values and metadata of
// output values belonging to the root module.
//
// Output values in other modules are not persisted at all, and are instead
// recalculated as needed based on the configuration and other state elements.
type RootModuleOutputValue struct {
	name string
}

const rootModuleOutputValuePrefix = "output"

func NewRootModuleOutputValue(addr addrs.AbsOutputValue) RootModuleOutputValue {
	if !addr.Module.IsRoot() {
		panic("NewRootModuleOutputValue with non-root output value")
	}
	return RootModuleOutputValue{name: addr.OutputValue.Name}
}

func rootModuleOutputValueFromStorage(raw string) (Key, error) {
	nameStr, err := decodeBase32(raw)
	if err != nil {
		return nil, err
	}
	if !hclsyntax.ValidIdentifier(nameStr) {
		return nil, fmt.Errorf("invalid output value name")
	}
	return RootModuleOutputValue{nameStr}, nil
}

// ForStorage implements Key.
func (r RootModuleOutputValue) ForStorage() statestore.Key {
	return makeStorageKey(rootModuleOutputValuePrefix, r.name)
}

func (r RootModuleOutputValue) Address() addrs.AbsOutputValue {
	return addrs.AbsOutputValue{
		Module: addrs.RootModuleInstance,
		OutputValue: addrs.OutputValue{
			Name: r.name,
		},
	}
}

// keySigil implements Key.
func (r RootModuleOutputValue) keySigil() {}
