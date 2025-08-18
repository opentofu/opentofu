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

// Resource is the key type for the state storage of a not-yet-expanded
// resource.
//
// The data associated with such a key is the metadata shared by all instances
// of the resource, but the main data is tracked on a per-instance basis
// under [ResourceInstance] keys.
type Resource struct {
	addr addrs.ConfigResource
}

const resourcePrefix = "rsrc"

func NewResource(addr addrs.ConfigResource) Resource {
	return Resource{addr}
}

func resourceFromStorage(addrStr string) (Key, error) {
	instAddr, diags := addrs.ParseAbsResourceInstanceStr(addrStr)
	if diags.HasErrors() {
		return nil, fmt.Errorf("invalid resource address syntax")
	}
	addr := instAddr.ConfigResource()
	if addr.String() != addrStr {
		return nil, fmt.Errorf("unexpected instance keys in resource address")
	}
	return Resource{addr}, nil
}

// ForStorage implements Key.
func (r Resource) ForStorage() statestore.Key {
	return makeStorageKey(resourcePrefix, r.addr.String())
}

func (r Resource) Address() addrs.ConfigResource {
	return r.addr
}

// keySigil implements Key.
func (r Resource) keySigil() {}
