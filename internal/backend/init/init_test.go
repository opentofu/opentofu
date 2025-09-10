// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package init

import (
	"reflect"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption"
)

func TestInit_backend(t *testing.T) {
	// Initialize the backends and backendAliases maps
	Init(nil)

	backends := []struct {
		RequestedName string
		Type          string
		CanonicalName string
	}{
		{"local", "*local.Local", "local"},
		{"remote", "*remote.Remote", "remote"},
		{"azurerm", "*azure.Backend", "azurerm"},
		{"consul", "*consul.Backend", "consul"},
		{"cos", "*cos.Backend", "cos"},
		{"gcs", "*gcs.Backend", "gcs"},
		{"inmem", "*inmem.Backend", "inmem"},
		{"pg", "*pg.Backend", "pg"},
		{"s3", "*s3.Backend", "s3"},
	}

	// Make sure we get the requested backend
	for _, b := range backends {
		t.Run(b.RequestedName, func(t *testing.T) {
			f, canonName := Backend(b.RequestedName)
			if f == nil {
				t.Fatalf("backend %q is not present; should be", b.RequestedName)
			}
			bType := reflect.TypeOf(f(encryption.StateEncryptionDisabled())).String()
			if bType != b.Type {
				t.Errorf("expected backend %q to be %q, got: %q", b.RequestedName, b.Type, bType)
			}
			if b.CanonicalName != canonName {
				t.Errorf("expected canonical name to be %q, but got %q", b.CanonicalName, canonName)
			}
		})
	}
}

// TestInit_backendConsistency ensures that the "backends" and "backendAliases"
// maps are kept consistent with one another, so that:
//   - Every alias maps to a canonical backend name that is actually defined.
//   - No single type name is both an alias _and_ a canonical name.
//   - There must be a backend whose canonical name is "local" and no alias
//     of that name because package command relies on this in various special
//     cases.
func TestInit_backendConsistency(t *testing.T) {
	// Initialize the backends and backendAliases maps
	Init(nil)

	backendsLock.Lock()
	defer backendsLock.Unlock()

	for aliasType, canonType := range backendAliases {
		if _, ok := backends[canonType]; !ok {
			t.Errorf("alias %q maps to canonical name %q, but the canonical name is not in the backends map", aliasType, canonType)
		}
		if _, ok := backends[aliasType]; ok {
			t.Errorf("alias map has key %q, which is also a canonical name in the backends map", aliasType)
		}
	}

	if _, ok := backends["local"]; !ok {
		t.Error(`"local" must be defined as a an available backend type because lots of code in package command treats it as a special case`)
	}
	if _, ok := backendAliases["local"]; ok {
		t.Error(`"local" must not be defined as an alias because lots of code in package command treats it as a special case`)
	}
}
