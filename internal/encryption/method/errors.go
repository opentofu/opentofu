// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package method

import "fmt"

// ErrCryptoFailure indicates a generic cryptographic failure. This error should be embedded into
// ErrEncryptionFailed, ErrDecryptionFailed, or ErrInvalidConfiguration.
type ErrCryptoFailure struct {
	Message string
	Cause   error
}

func (e ErrCryptoFailure) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e ErrCryptoFailure) Unwrap() error {
	return e.Cause
}

// ErrEncryptionFailed indicates that encrypting a set of data failed.
type ErrEncryptionFailed struct {
	Cause error
}

func (e ErrEncryptionFailed) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("encryption failed: %v", e.Cause)
	}
	return "encryption failed"
}

func (e ErrEncryptionFailed) Unwrap() error {
	return e.Cause
}

// ErrDecryptionFailed indicates that decrypting a set of data failed.
type ErrDecryptionFailed struct {
	Cause error
}

func (e ErrDecryptionFailed) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("decryption failed: %v", e.Cause)
	}
	return "decryption failed"
}

func (e ErrDecryptionFailed) Unwrap() error {
	return e.Cause
}

// ErrDecryptionKeyUnavailable indicates that no decryption key is available.
type ErrDecryptionKeyUnavailable struct {
}

func (e ErrDecryptionKeyUnavailable) Error() string {
	return "no decryption key available"
}

// ErrInvalidConfiguration indicates that the method configuration is incorrect.
type ErrInvalidConfiguration struct {
	Cause error
}

func (e ErrInvalidConfiguration) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("invalid method configuration: %v", e.Cause)
	}
	return "invalid method configuration"
}

func (e ErrInvalidConfiguration) Unwrap() error {
	return e.Cause
}
