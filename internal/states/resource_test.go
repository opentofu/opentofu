// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package states

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/addrs"
	"testing"
)

func TestResourceInstanceDeposeCurrentObject(t *testing.T) {
	obj := &ResourceInstanceObjectSrc{
		// Empty for the sake of this test, because we're just going to
		// compare by pointer below anyway.
	}

	is := NewResourceInstance()
	is.Current = obj
	var dk DeposedKey

	t.Run("first depose", func(t *testing.T) {
		dk = is.deposeCurrentObject(NotDeposed) // dk is randomly-generated but should be eight characters long
		t.Logf("deposedKey is %q", dk)

		if got := is.Current; got != nil {
			t.Errorf("current is %#v; want nil", got)
		}
		if got, want := is.Deposed[dk], obj; got != want {
			t.Errorf("deposed object pointer is %#v; want %#v", got, want)
		}
		if got, want := len(is.Deposed), 1; got != want {
			t.Errorf("wrong len(is.Deposed) %d; want %d", got, want)
		}
		if got, want := len(dk), 8; got != want {
			t.Errorf("wrong len(deposedkey) %d; want %d", got, want)
		}
	})

	t.Run("second depose", func(t *testing.T) {
		notDK := is.deposeCurrentObject(NotDeposed)
		if notDK != NotDeposed {
			t.Errorf("got deposedKey %q; want NotDeposed", notDK)
		}

		// Make sure we really did abort early, and haven't corrupted the
		// state somehow.
		if got := is.Current; got != nil {
			t.Errorf("current is %#v; want nil", got)
		}
		if got, want := is.Deposed[dk], obj; got != want {
			t.Errorf("deposed object pointer is %#v; want %#v", got, want)
		}
		if got, want := len(is.Deposed), 1; got != want {
			t.Errorf("wrong len(is.Deposed) %d; want %d", got, want)
		}
		if got, want := len(dk), 8; got != want {
			t.Errorf("wrong len(deposedkey) %d; want %d", got, want)
		}
	})
}

func TestResolveInstanceProvider(t *testing.T) {
	ik := addrs.StringKey("first")
	absResourceAddr := mustAbsResourceAddr("null_resource.resource")
	provider, _ := addrs.ParseAbsProviderConfigStr("provider[\"registry.opentofu.org/hashicorp/aws\"]")
	providerSecond, _ := addrs.ParseAbsProviderConfigStr("provider[\"registry.opentofu.org/hashicorp/aws\"].second")
	emptyProvider := addrs.AbsProviderConfig{}

	// Test cases for the method
	tests := []struct {
		name                    string
		resourceProvider        addrs.AbsProviderConfig
		currentInstanceProvider addrs.AbsProviderConfig
		deposedInstanceProvider addrs.AbsProviderConfig
		expectPanic             bool
		expectedPanicMessage    string
		expectFromResource      bool
	}{
		{
			name:                    "should return resourceProvider if set",
			resourceProvider:        provider,
			currentInstanceProvider: emptyProvider,
			expectPanic:             false,
			expectFromResource:      true,
		},
		{
			name:                    "should return instanceProvider if resourceProvider is not set",
			resourceProvider:        emptyProvider,
			currentInstanceProvider: provider,
			expectPanic:             false,
			expectFromResource:      false,
		},
		{
			name:                    "should return resourceProvider if set (even when deposed has deposedInstanceProvider set)",
			resourceProvider:        provider,
			currentInstanceProvider: emptyProvider,
			deposedInstanceProvider: providerSecond,
			expectPanic:             false,
			expectFromResource:      true,
		},
		{
			name:                    "should return current instanceProvider if resourceProvider is not set (even when deposed has deposedInstanceProvider set)",
			resourceProvider:        emptyProvider,
			currentInstanceProvider: provider,
			deposedInstanceProvider: providerSecond,
			expectPanic:             false,
			expectFromResource:      false,
		},
		{
			name:                    "should return instanceProvider from deposed instances",
			resourceProvider:        emptyProvider,
			currentInstanceProvider: emptyProvider,
			deposedInstanceProvider: provider,
			expectPanic:             false,
			expectFromResource:      false,
		},
		{
			name:                    "should panic if neither resourceProvider nor instanceProvider is set and no deposed instance provider found",
			resourceProvider:        emptyProvider,
			currentInstanceProvider: emptyProvider,
			deposedInstanceProvider: emptyProvider,
			expectPanic:             true,
			expectedPanicMessage:    fmt.Sprintf("InstanceProvider for %s (instance key %s) failed to read provider from the state", absResourceAddr, ik),
		},
		{
			name:                    "should panic if both resourceProvider and instanceProvider are set",
			resourceProvider:        provider,
			currentInstanceProvider: providerSecond,
			expectPanic:             true,
			expectedPanicMessage:    fmt.Sprintf("InstanceProvider for %s (instance key %s) found two providers in state for the instance", absResourceAddr, ik),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &ResourceInstance{
				Current: &ResourceInstanceObjectSrc{
					InstanceProvider: tt.currentInstanceProvider,
				},
				Deposed: map[DeposedKey]*ResourceInstanceObjectSrc{"deposedKey": {
					InstanceProvider: tt.deposedInstanceProvider,
				}},
			}

			rs := &Resource{
				Addr:           absResourceAddr,
				ProviderConfig: tt.resourceProvider,
				Instances:      map[addrs.InstanceKey]*ResourceInstance{ik: instance},
			}

			if tt.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic, but got none")
					} else if r != tt.expectedPanicMessage {
						t.Errorf("Expected panic message '%s', but got '%v'", tt.expectedPanicMessage, r)
					}
				}()
				rs.InstanceProvider(ik)
			} else {
				resultProvider, fromResource := rs.InstanceProvider(ik)

				if !resultProvider.IsSet() {
					t.Errorf("Expected provider to be set, but it was not")
				}
				if resultProvider.String() != provider.String() {
					t.Errorf("Expected provider %v, but got %v", provider, resultProvider)
				}
				if fromResource != tt.expectFromResource {
					t.Errorf("Expected fromResource to be %v, but got %v", tt.expectFromResource, fromResource)
				}
			}
		})
	}
}
