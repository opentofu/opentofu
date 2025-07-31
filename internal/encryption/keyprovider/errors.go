// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

import "fmt"

// ErrKeyProviderFailure indicates a generic key provider failure.
type ErrKeyProviderFailure struct {
	Message string
	Cause   error
}

func (e ErrKeyProviderFailure) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e ErrKeyProviderFailure) Unwrap() error {
	return e.Cause
}

// ErrInvalidConfiguration indicates that the key provider configuration is incorrect.
type ErrInvalidConfiguration struct {
	Message string
	Cause   error
}

func (e ErrInvalidConfiguration) Error() string {
	if e.Cause != nil {

		if e.Message != "" {
			return fmt.Sprintf("%s: %v", e.Message, e.Cause)
		}
		return fmt.Sprintf("invalid key provider configuration: %v", e.Cause)
	}
	if e.Message != "" {
		return e.Message
	}
	return "invalid provider configuration"
}

func (e ErrInvalidConfiguration) Unwrap() error {
	return e.Cause
}

// ErrInvalidMetadata indicates that the key provider has received an incorrect metadata and cannot decrypt.
type ErrInvalidMetadata struct {
	Message string
	Cause   error
}

func (e ErrInvalidMetadata) Error() string {
	if e.Cause != nil {
		if e.Message != "" {
			return fmt.Sprintf("%s: %v", e.Message, e.Cause)
		}
		return fmt.Sprintf("invalid key provider metadata: %v", e.Cause)
	}
	if e.Message != "" {
		return e.Message
	}
	return "invalid provider metadata"
}

func (e ErrInvalidMetadata) Unwrap() error {
	return e.Cause
}
