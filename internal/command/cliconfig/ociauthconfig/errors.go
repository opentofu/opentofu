// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"errors"
)

// NewCredentialsNotFoundError wraps the given error in an error value that would
// cause [IsCredentialsNotFoundError] to return true.
func NewCredentialsNotFoundError(inner error) error {
	if inner == nil {
		panic("wrapping nil error as 'credentials not found' error")
	}
	return credentialsNotFoundError{inner}
}

// IsCredentialsNotFoundError returns true if the given error is (or wraps)
// an error representing that a Docker credential helper lookup failed due
// to there being no credentials available for the requested server URL.
func IsCredentialsNotFoundError(err error) bool {
	var target credentialsNotFoundError
	return errors.As(err, &target)
}

type credentialsNotFoundError struct {
	inner error
}

func (e credentialsNotFoundError) Error() string {
	return e.inner.Error()
}

func (e credentialsNotFoundError) Unwrap() error {
	return e.inner
}
