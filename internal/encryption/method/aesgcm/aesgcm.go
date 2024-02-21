// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/golang"

	// This unsafe is required for go:linkname
	_ "unsafe"
)

// Note: the linking below is a workaround for Go issue #42470. We want to support setting both the nonce and the tag
// size specifically because if the defaults for any of these parameters change due to new cryptography research,
// people should still be able to decode their old state files.

//go:linkname newGCMWithNonceAndTagSize crypto/cipher.newGCMWithNonceAndTagSize
func newGCMWithNonceAndTagSize(cipher cipher.Block, nonceSize, tagSize int) (cipher.AEAD, error)

// aesgcm contains the encryption/decryption methods according to AES-GCM (NIST SP 800-38D).
type aesgcm struct {
	key       []byte
	aad       []byte
	nonceSize int
	tagSize   int
}

// Encrypt encrypts the passed data with AES-GCM. If the data the encryption fails, it returns an error.
func (a aesgcm) Encrypt(data []byte) ([]byte, error) {
	// Ew! Ew! Ew! This is a try-catch! Yes, we know.
	//
	// The GCM implementation in Golang uses panics for invalid inputs. This block makes sure that users get an
	// intelligible error message and that calling functions can rely on this function being panic-free.
	return golang.Safe2w(
		func() ([]byte, error) {
			gcm, err := a.getGCM()
			if err != nil {
				return nil, &method.ErrEncryptionFailed{Cause: err}
			}

			nonce := make([]byte, gcm.NonceSize())
			if _, err := rand.Read(nonce); err != nil {
				return nil, &method.ErrEncryptionFailed{Cause: &method.ErrCryptoFailure{Message: "could not generate nonce", Cause: err}}
			}

			encrypted := gcm.Seal(nil, nonce, data, a.aad)

			return append(nonce, encrypted...), nil
		},
		func(e error) error {
			var encryptionFailed *method.ErrEncryptionFailed
			if errors.As(e, &encryptionFailed) {
				return e
			}
			return &method.ErrEncryptionFailed{Cause: &method.ErrCryptoFailure{Message: "unexpected panic", Cause: e}}
		},
	)
}

// Decrypt decrypts an AES-GCM-encrypted data set. If the data set fails decryption, it returns an error.
func (a aesgcm) Decrypt(data []byte) ([]byte, error) {
	// Ew! Ew! Ew! This is a try-catch! Yes, we know.
	//
	// The GCM implementation in Golang uses panics for invalid inputs. This block makes sure that users get an
	// intelligible error message and that calling functions can rely on this function being panic-free.
	return golang.Safe2w(
		func() ([]byte, error) {
			if len(data) == 0 {
				return nil, &method.ErrDecryptionFailed{
					Cause: method.ErrCryptoFailure{
						Message: "cannot decrypt empty data",
						Cause:   nil,
					},
				}
			}
			if len(data) < a.nonceSize {
				return nil, &method.ErrDecryptionFailed{
					Cause: method.ErrCryptoFailure{
						Message: "cannot decrypt data because it is too small (likely data corruption)",
						Cause:   nil,
					},
				}
			}

			nonce := data[:a.nonceSize]
			data = data[a.nonceSize:]

			gcm, err := a.getGCM()
			if err != nil {
				return nil, &method.ErrDecryptionFailed{Cause: err}
			}

			decrypted, err := gcm.Open(nil, nonce, data, a.aad)
			if err != nil {
				return nil, &method.ErrDecryptionFailed{Cause: err}
			}
			return decrypted, nil
		},
		func(e error) error {
			var decryptionFailed *method.ErrDecryptionFailed
			if errors.As(e, &decryptionFailed) {
				return e
			}
			return &method.ErrDecryptionFailed{Cause: &method.ErrCryptoFailure{Message: "unexpected panic", Cause: e}}
		},
	)
}

func (a aesgcm) getGCM() (cipher.AEAD, error) {
	cipherBlock, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, &method.ErrCryptoFailure{
			Message: "failed to create AES cypher block",
			Cause:   err,
		}
	}

	gcm, err := newGCMWithNonceAndTagSize(cipherBlock, a.nonceSize, a.tagSize)
	if err != nil {
		return nil, &method.ErrCryptoFailure{
			Message: "failed to create AES GCM",
			Cause:   err,
		}
	}
	return gcm, nil
}
