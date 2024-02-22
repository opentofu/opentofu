package aesgcm_test

import (
	"errors"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/errorhandling"
	"testing"
)

func TestDecryptEmptyData(t *testing.T) {
	m := errorhandling.Must2(aesgcm.New().TypedConfig().WithKey([]byte("aeshi1quahb2Rua0ooquaiwahbonedoh")).Build())

	_, err := m.Decrypt(nil)
	if err == nil {
		t.Fatalf("Expected error, none returned.")
	}

	var e *method.ErrDecryptionFailed
	if !errors.As(err, &e) {
		t.Fatalf("Incorrect error type returned: %T (%v)", err, err)
	}
}

func TestDecryptShortData(t *testing.T) {
	m := errorhandling.Must2(aesgcm.New().TypedConfig().WithKey([]byte("aeshi1quahb2Rua0ooquaiwahbonedoh")).Build())

	// Passing a non-empty, but shorted-than-nonce data
	_, err := m.Decrypt([]byte("1"))
	if err == nil {
		t.Fatalf("Expected error, none returned.")
	}

	var e *method.ErrDecryptionFailed
	if !errors.As(err, &e) {
		t.Fatalf("Incorrect error type returned: %T (%v)", err, err)
	}
}

func TestDecryptInvalidData(t *testing.T) {
	m := errorhandling.Must2(aesgcm.New().TypedConfig().WithKey([]byte("aeshi1quahb2Rua0ooquaiwahbonedoh")).Build())

	// Passing a non-empty, but shorted-than-nonce data
	_, err := m.Decrypt([]byte("abcdefghijklmnopqrstuvwxyz"))
	if err == nil {
		t.Fatalf("Expected error, none returned.")
	}

	var e *method.ErrDecryptionFailed
	if !errors.As(err, &e) {
		t.Fatalf("Incorrect error type returned: %T (%v)", err, err)
	}
}

func TestDecryptCorruptData(t *testing.T) {
	m := errorhandling.Must2(aesgcm.New().TypedConfig().WithKey([]byte("aeshi1quahb2Rua0ooquaiwahbonedoh")).Build())

	encrypted := errorhandling.Must2(m.Encrypt([]byte("Hello world!")))

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
