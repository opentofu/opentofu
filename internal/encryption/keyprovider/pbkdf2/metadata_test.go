// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import "testing"

func TestMetadata_validate(t *testing.T) {
	for name, tc := range map[string]struct {
		meta    Metadata
		present bool
		valid   bool
	}{
		"empty": {
			Metadata{},
			false,
			true,
		},
		"invalid-iterations": {
			Metadata{
				Iterations:   -1,
				KeyLength:    1,
				HashFunction: SHA256HashFunctionName,
				Salt:         []byte("Hello world!"),
			},
			true,
			false,
		},
		"invalid-keylength": {
			Metadata{
				Iterations:   1,
				KeyLength:    -1,
				HashFunction: SHA256HashFunctionName,
				Salt:         []byte("Hello world!"),
			},
			true,
			false,
		},
		"invalid-hashfunction": {
			Metadata{
				Iterations:   1,
				KeyLength:    1,
				HashFunction: "sha0",
				Salt:         []byte("Hello world!"),
			},
			true,
			false,
		},
		"no-salt": {
			Metadata{
				Iterations:   1,
				KeyLength:    1,
				HashFunction: SHA256HashFunctionName,
				Salt:         []byte{},
			},
			false,
			true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if present := tc.meta.isPresent(); present != tc.present {
				t.Fatalf("incorrect value for 'present': %t", present)
			}
			if err := tc.meta.validate(); (err == nil) != tc.valid {
				t.Fatalf("incorrect return value from 'validate': %v", err)
			}
		})
	}
}
