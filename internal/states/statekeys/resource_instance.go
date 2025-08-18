// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statekeys

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states/statestore"
)

type ResourceInstance struct {
	addr addrs.AbsResourceInstance
}

const resourceInstancePrefix = "rsrcinst"

func NewResourceInstance(addr addrs.AbsResourceInstance) ResourceInstance {
	return ResourceInstance{addr}
}

func resourceInstanceFromStorage(addrStr string) (Key, error) {
	addr, diags := addrs.ParseAbsResourceInstanceStr(addrStr)
	if diags.HasErrors() {
		return nil, fmt.Errorf("invalid resource address syntax")
	}
	return ResourceInstance{addr}, nil
}

// ForStorage implements Key.
func (r ResourceInstance) ForStorage() statestore.Key {
	return makeStorageKey(resourceInstancePrefix, r.addr.String())
}

func (r ResourceInstance) Address() addrs.AbsResourceInstance {
	return r.addr
}

// keySigil implements Key.
func (r ResourceInstance) keySigil() {}
