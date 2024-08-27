// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2_test

import (
	"testing"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider/pbkdf2"
)

func TestHashFunctionName_Validate(t *testing.T) {
	tc := map[string]struct {
		hashFunctionName pbkdf2.HashFunctionName
		valid            bool
	}{
		"empty": {
			hashFunctionName: "",
			valid:            false,
		},
		"sha256": {
			hashFunctionName: pbkdf2.SHA256HashFunctionName,
			valid:            true,
		},
		"sha0": {
			hashFunctionName: "sha0",
			valid:            false,
		},
	}

	for name, testCase := range tc {
		t.Run(name, func(t *testing.T) {
			err := testCase.hashFunctionName.Validate()
			if testCase.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else if !testCase.valid && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func generateFixedStringHelper(length int) string {
	result := ""
	for i := 0; i < length; i++ {
		result += "a"
	}
	return result
}

func TestConfig_Build(t *testing.T) {
	knownGood := func() *pbkdf2.Config {
		return pbkdf2.New().TypedConfig().WithPassphrase(generateFixedStringHelper(pbkdf2.MinimumPassphraseLength))
	}
	tc := map[string]struct {
		config *pbkdf2.Config
		valid  bool
	}{
		"empty": {
			config: &pbkdf2.Config{},
			valid:  false,
		},
		"default": {
			// Missing passphrase
			config: pbkdf2.New().ConfigStruct().(*pbkdf2.Config),
			valid:  false,
		},
		"default-short-passphrase": {
			config: pbkdf2.New().TypedConfig().WithPassphrase(generateFixedStringHelper(pbkdf2.MinimumPassphraseLength - 1)),
			valid:  false,
		},
		"default-good-passphrase": {
			config: knownGood(),
			valid:  true,
		},
		"invalid-key-length": {
			config: knownGood().WithKeyLength(0),
			valid:  false,
		},
		"invalid-iterations": {
			config: knownGood().WithIterations(0),
			valid:  false,
		},
		"low-iterations": {
			config: knownGood().WithIterations(pbkdf2.MinimumIterations - 1),
			valid:  false,
		},
		"invalid-salt-length": {
			config: knownGood().WithSaltLength(0),
			valid:  false,
		},
		"invalid-hash-function": {
			config: knownGood().WithHashFunction(""),
			valid:  false,
		},
	}
	for name, testCase := range tc {
		t.Run(name, func(t *testing.T) {
			_, _, err := testCase.config.Build()
			if testCase.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else if !testCase.valid && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}
