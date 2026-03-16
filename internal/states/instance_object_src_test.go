// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package states

import (
	"testing"
)

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func TestResourceInstanceObjectSrcEqual(t *testing.T) {
	tests := map[string]struct {
		a    *ResourceInstanceObjectSrc
		b    *ResourceInstanceObjectSrc
		want bool
	}{
		"identical base objects": {
			a:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`)},
			b:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`)},
			want: true,
		},
		"identity schema version both nil": {
			a:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: nil},
			b:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: nil},
			want: true,
		},
		"identity schema version first nil": {
			a:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: nil},
			b:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: uint64Ptr(1)},
			want: false,
		},
		"identity schema version second nil": {
			a:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: uint64Ptr(1)},
			b:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: nil},
			want: false,
		},
		"identity schema version different values": {
			a:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: uint64Ptr(1)},
			b:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: uint64Ptr(2)},
			want: false,
		},
		"identity schema version same values": {
			a:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: uint64Ptr(1)},
			b:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentitySchemaVersion: uint64Ptr(1)},
			want: true,
		},
		"same identity json different schema version": {
			a:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentityJSON: []byte(`{"name":"bar"}`), IdentitySchemaVersion: uint64Ptr(1)},
			b:    &ResourceInstanceObjectSrc{Status: ObjectReady, AttrsJSON: []byte(`{"id":"foo"}`), IdentityJSON: []byte(`{"name":"bar"}`), IdentitySchemaVersion: uint64Ptr(2)},
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
