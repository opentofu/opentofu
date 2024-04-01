// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package method

// Method is a low-level encryption method interface that is responsible for encrypting a binary blob of data. It should
// not try to interpret what kind of data it is encrypting.
type Method interface {
	// Encrypt encrypts the specified data with the set configuration. This method should treat any data passed as
	// opaque and should not try to interpret its contents. The interpretation is the job of the encryption.Encryption
	// interface.
	Encrypt(data []byte) ([]byte, error)
	// Decrypt decrypts the specified data with the set configuration. This method should treat any data passed as
	// opaque and should not try to interpret its contents. The interpretation is the job of the encryption.Encryption
	// interface.
	Decrypt(data []byte) ([]byte, error)
}
