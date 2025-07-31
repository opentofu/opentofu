// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

// TODO #2386 / 1.11: consider if the external method changes and unify protocol with the external key provider.

// Magic is the magic string the external method needs to output in the Header.
const Magic = "OpenTofu-External-Encryption-Method"

// Header is the initial message the external method writes to stdout as a single-line JSON.
type Header struct {
	// Magic must always be "OpenTofu-External-Encryption-Method"
	Magic string `json:"magic"`
	// Version must always be 1.
	Version int `json:"version"`
}

// InputV1 is an encryption/decryption request from OpenTofu to the external method. OpenTofu writes this message
// to the standard input of the external method as a JSON message.
type InputV1 struct {
	// Key is the encryption or decryption key for this operation. On the wire, this is base64-encoded. If no key is
	// present, this will be nil. The method should exit with a non-zero exit code.
	Key []byte `json:"key,omitempty"`
	// Payload is the payload to encrypt/decrypt.
	Payload []byte `json:"payload"`
}

// OutputV1 is the returned encrypted/decrypted payload from the external method. The external method writes this
// to the standard output as JSON.
type OutputV1 struct {
	// Payload is the payload that has been encrypted/decrypted by the external method.
	Payload []byte `json:"payload"`
}
