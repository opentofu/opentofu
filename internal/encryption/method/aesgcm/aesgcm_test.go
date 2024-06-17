// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm_test

import (
	"errors"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
)

var config = &aesgcm.Config{
	Keys: keyprovider.Output{
		EncryptionKey: []byte("aeshi1quahb2Rua0ooquaiwahbonedoh"),
		DecryptionKey: []byte("aeshi1quahb2Rua0ooquaiwahbonedoh"),
	},
}

func TestDecryptEmptyData(t *testing.T) {
	m, err := config.Build()
	if err != nil {
		t.Fatalf("unexpected error (%v)", err)
	}

	_, err = m.Decrypt(nil)
	if err == nil {
		t.Fatalf("Expected error, none returned.")
	}

	var e *method.ErrDecryptionFailed
	if !errors.As(err, &e) {
		t.Fatalf("Incorrect error type returned: %T (%v)", err, err)
	}
}

func TestDecryptShortData(t *testing.T) {
	m, err := config.Build()
	if err != nil {
		t.Fatalf("unexpected error (%v)", err)
	}

	// Passing a non-empty, but shorter-than-nonce data
	_, err = m.Decrypt([]byte("1"))
	if err == nil {
		t.Fatalf("Expected error, none returned.")
	}

	var e *method.ErrDecryptionFailed
	if !errors.As(err, &e) {
		t.Fatalf("Incorrect error type returned: %T (%v)", err, err)
	}
}

func TestDecryptInvalidData(t *testing.T) {
	m, err := config.Build()
	if err != nil {
		t.Fatalf("unexpected error (%v)", err)
	}

	// Passing a non-empty, but shorter-than-nonce data
	_, err = m.Decrypt([]byte("abcdefghijklmnopqrstuvwxyz"))
	if err == nil {
		t.Fatalf("Expected error, none returned.")
	}

	var e *method.ErrDecryptionFailed
	if !errors.As(err, &e) {
		t.Fatalf("Incorrect error type returned: %T (%v)", err, err)
	}
}

func TestDecryptCorruptData(t *testing.T) {
	m, err := config.Build()
	if err != nil {
		t.Fatalf("unexpected error (%v)", err)
	}

	encrypted, err := m.Encrypt([]byte("Hello world!"))
	if err != nil {
		t.Fatalf("unexpected error (%v)", err)
	}

	encrypted = encrypted[:len(encrypted)-1]
	decrypted, err := m.Decrypt(encrypted)
	if err == nil {
		t.Fatalf("Expected error, got: %v", decrypted)
	}
	var e *method.ErrDecryptionFailed
	if !errors.As(err, &e) {
		t.Fatalf("Incorrect error type returned: %T (%v)", err, err)
	}
}
