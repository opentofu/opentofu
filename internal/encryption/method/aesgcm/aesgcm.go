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
)

// aesgcm contains the encryption/decryption methods according to AES-GCM (NIST SP 800-38D).
type aesgcm struct {
	encryptionKey []byte
	decryptionKey []byte
	aad           []byte
}

// Encrypt encrypts the passed data with AES-GCM. If the data the encryption fails, it returns an error.
func (a aesgcm) Encrypt(data []byte) ([]byte, error) {
	result, err := handlePanic(
		func() ([]byte, error) {
			gcm, err := a.getGCM(a.encryptionKey)
			if err != nil {
				return nil, &method.ErrEncryptionFailed{Cause: err}
			}

			nonce := make([]byte, gcm.NonceSize())
			if _, err := rand.Read(nonce); err != nil {
				return nil, &method.ErrEncryptionFailed{Cause: &method.ErrCryptoFailure{
					Message: "could not generate nonce",
					Cause:   err,
				}}
			}

			encrypted := gcm.Seal(nil, nonce, data, a.aad)

			return append(nonce, encrypted...), nil
		},
	)
	if err != nil {
		var encryptionFailed *method.ErrEncryptionFailed
		if errors.As(err, &encryptionFailed) {
			return nil, err
		}
		return nil, &method.ErrEncryptionFailed{Cause: &method.ErrCryptoFailure{Message: "unexpected error", Cause: err}}
	}
	return result, nil
}

// Decrypt decrypts an AES-GCM-encrypted data set. If the data set fails decryption, it returns an error.
func (a aesgcm) Decrypt(data []byte) ([]byte, error) {
	if len(a.decryptionKey) == 0 {
		return nil, &method.ErrDecryptionKeyUnavailable{}
	}
	result, err := handlePanic(
		func() ([]byte, error) {
			if len(data) == 0 {
				return nil, &method.ErrDecryptionFailed{
					Cause: method.ErrCryptoFailure{
						Message: "cannot decrypt empty data",
						Cause:   nil,
					},
				}
			}

			gcm, err := a.getGCM(a.decryptionKey)
			if err != nil {
				return nil, &method.ErrDecryptionFailed{Cause: err}
			}

			if len(data) < gcm.NonceSize() {
				return nil, &method.ErrDecryptionFailed{
					Cause: method.ErrCryptoFailure{
						Message: "cannot decrypt data because it is too small (likely data corruption)",
						Cause:   nil,
					},
				}
			}

			nonce := data[:gcm.NonceSize()]
			data = data[gcm.NonceSize():]

			decrypted, err := gcm.Open(nil, nonce, data, a.aad)
			if err != nil {
				return nil, &method.ErrDecryptionFailed{Cause: err}
			}
			return decrypted, nil
		},
	)
	if err != nil {
		var decryptionFailed *method.ErrDecryptionFailed
		if errors.As(err, &decryptionFailed) {
			return nil, err
		}
		return nil, &method.ErrDecryptionFailed{
			Cause: &method.ErrCryptoFailure{Message: "unexpected error", Cause: err},
		}
	}
	return result, nil
}

func (a aesgcm) getGCM(key []byte) (cipher.AEAD, error) {
	cipherBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, &method.ErrCryptoFailure{
			Message: "failed to create AES cypher block",
			Cause:   err,
		}
	}

	gcm, err := cipher.NewGCM(cipherBlock)
	if err != nil {
		return nil, &method.ErrCryptoFailure{
			Message: "failed to create AES GCM",
			Cause:   err,
		}
	}
	return gcm, nil
}

func Is(m method.Method) bool {
	_, ok := m.(*aesgcm)
	return ok
}
