// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

// KeyProvider is the usable key provider. The Provide function is responsible for creating both the decryption and
// encryption key, as well as returning the metadata to be stored.
type KeyProvider interface {
	// Provide provides an encryption and decryption keys. If the process fails, it returns an error.
	//
	// The caller must pass in the same struct obtained from the Build function of the Config, with the decryption
	// metadata read in. If no decryption metadata is present, the caller must pass in the struct unmodified.
	Provide(decryptionMeta KeyMeta) (keysOutput Output, encryptionMeta KeyMeta, err error)
}
