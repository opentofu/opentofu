// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestPbkdf2KeyProvider_generateMetadata(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: testRandomSource{t},
			Passphrase:   "Hello world!",
			KeyLength:    32,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}
	metadata, err := provider.generateMetadata()
	if err != nil {
		t.Fatalf("%v", err)
	}

	if len(metadata.Salt) != 12 {
		t.Fatalf("Invalid generated salt length: %d", len(metadata.Salt))
	}
	// This is read from the random source, which is the test function name in this case.
	// Note: this relies on the internal behavior of generateMetadata, but it's a non-exported
	// function, so in this case that's acceptable.
	if !bytes.Equal(metadata.Salt, []byte("TestPbkdf2Ke")) {
		t.Fatalf("Invalid generated salt: %s", metadata.Salt)
	}

	if metadata.KeyLength != 32 {
		t.Fatalf("Invalid key length: %d", metadata.KeyLength)
	}
	if metadata.Iterations != MinimumIterations {
		t.Fatalf("Invalid iterations: %d", metadata.Iterations)
	}
	if metadata.HashFunction != SHA256HashFunctionName {
		t.Fatalf("Invalid hash function name: %s", SHA256HashFunctionName)
	}
}

type badReader struct{}

func (b badReader) Read(target []byte) (int, error) {
	return 0, io.EOF
}

func TestBadReader(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: badReader{},
			Passphrase:   "Hello world!",
			KeyLength:    32,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}

	if _, err := provider.generateMetadata(); err == nil {
		t.Fatalf("expected error")
	}

	if _, _, err := provider.Provide(&Metadata{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInvalidMetadata(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: testRandomSource{t},
			Passphrase:   "Hello world!",
			KeyLength:    32,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}

	if _, _, err := provider.Provide(&Metadata{
		Iterations:   1,
		Salt:         []byte("Hello world!"),
		HashFunction: SHA256HashFunctionName,
		KeyLength:    -1,
	}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNilMetadata(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: testRandomSource{t},
			Passphrase:   "Hello world!",
			KeyLength:    32,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}

	if _, _, err := provider.Provide(nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestOutputMetadata(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: testRandomSource{t},
			Passphrase:   "Hello world!",
			KeyLength:    32,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}

	_, meta, err := provider.Provide(&Metadata{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !meta.(*Metadata).isPresent() {
		t.Fatalf("result metadata is not present")
	}
	if err := meta.(*Metadata).validate(); err != nil {
		t.Fatalf("result metadata is not valid (%v)", err)
	}
}

func TestFullCircle(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: rand.Reader,
			Passphrase:   "Hello world!",
			KeyLength:    32,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}

	encryptionKeys, meta, err := provider.Provide(&Metadata{})
	if err != nil {
		t.Fatalf("%v", err)
	}

	decryptionKeys, _, err := provider.Provide(meta)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !bytes.Equal(encryptionKeys.EncryptionKey, decryptionKeys.DecryptionKey) {
		t.Fatalf("The two keys don't match: %x / %x", encryptionKeys.EncryptionKey, decryptionKeys.DecryptionKey)
	}
}

func TestKeyLength(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: rand.Reader,
			Passphrase:   "Hello world!",
			KeyLength:    128,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}
	keys, _, err := provider.Provide(&Metadata{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if length := len(keys.EncryptionKey); length != 128 {
		t.Fatalf("incorrect key length: %d", length)
	}
}

func TestNoDecryptionKeyOnEmptyInputMeta(t *testing.T) {
	provider := pbkdf2KeyProvider{
		Config{
			randomSource: rand.Reader,
			Passphrase:   "Hello world!",
			KeyLength:    128,
			Iterations:   MinimumIterations,
			HashFunction: SHA256HashFunctionName,
			SaltLength:   12,
		},
	}
	keys, _, err := provider.Provide(&Metadata{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(keys.DecryptionKey) != 0 {
		t.Fatalf("decryption key generated despite no input metadata")
	}
}
