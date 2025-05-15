// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"bytes"
	"errors"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

func TestConfig_Build(t *testing.T) {
	var testCases = []struct {
		name      string
		config    *Config
		errorType any
		expected  aesgcm
	}{
		{
			name: "key-32-bytes",
			config: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathe"),
					DecryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				},
			},
			errorType: nil,
			expected: aesgcm{
				encryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				decryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathd"),
			},
		},
		{
			name: "key-24-bytes",
			config: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte("bohwu9zoo7Zool5olaileefe"),
					DecryptionKey: []byte("bohwu9zoo7Zool5olaileefd"),
				},
			},
			errorType: nil,
			expected: aesgcm{
				encryptionKey: []byte("bohwu9zoo7Zool5olaileefe"),
				decryptionKey: []byte("bohwu9zoo7Zool5olaileefd"),
			},
		},
		{
			name: "key-16-bytes",
			config: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte("bohwu9zoo7Zool5e"),
					DecryptionKey: []byte("bohwu9zoo7Zool5d"),
				},
			},
			errorType: nil,
			expected: aesgcm{
				encryptionKey: []byte("bohwu9zoo7Zool5e"),
				decryptionKey: []byte("bohwu9zoo7Zool5d"),
			},
		},
		{
			name:      "no-key",
			config:    &Config{},
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "encryption-key-15-bytes",
			config: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte("bohwu9zoo7Ze15"),
					DecryptionKey: []byte("bohwu9zoo7Zod16"),
				},
			},
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "decryption-key-15-bytes",
			config: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte("bohwu9zoo7Zooe16"),
					DecryptionKey: []byte("bohwu9zoo7Zod15"),
				},
			},
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "aad",
			config: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathe"),
					DecryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				},
				AAD: []byte("foobar"),
			},
			expected: aesgcm{
				encryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				decryptionKey: []byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				aad:           []byte("foobar"),
			},
			errorType: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			built, err := tc.config.Build()
			if tc.errorType == nil {
				if err != nil {
					t.Fatalf("Unexpected error returned: %v", err)
				}

				built := built.(*aesgcm)

				if !bytes.Equal(tc.expected.encryptionKey, built.encryptionKey) {
					t.Fatalf("Incorrect encryption key built: %v != %v", tc.expected.encryptionKey, built.encryptionKey)
				}
				if !bytes.Equal(tc.expected.decryptionKey, built.decryptionKey) {
					t.Fatalf("Incorrect decryption key built: %v != %v", tc.expected.decryptionKey, built.decryptionKey)
				}
				if !bytes.Equal(tc.expected.aad, built.aad) {
					t.Fatalf("Incorrect aad built: %v != %v", tc.expected.aad, built.aad)
				}

			} else if tc.errorType != nil {
				if err == nil {
					t.Fatal("Expected error, none received")
				}
				if !errors.As(err, &tc.errorType) {
					t.Fatalf("Incorrect error type received: %T", err)
				}
				t.Logf("Correct error of type %T received: %v", err, err)
			}

		})
	}
}
