// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"encoding/base64"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// There are no actual tests in this file. This is just a collection of helpers needed
// for tests in at least two other test files.

// normalizeFilePath is a testing helper that replaces any backslashes with forward
// slashes just to help us to exercise our entire test suite regardless of which
// platform the test suite is running on, without every test needing to compensate
// for the different path separator on Windows.
func normalizeFilePath(p string) string {
	// This is intentionally not filepath.FromSlash because that function does absolutely
	// nothing when the current platform isn't Windows.
	return strings.ReplaceAll(p, "\\", "/")
}

// base64Encode returns the result of base64-encoding the given string using the standard
// base64 alphabet.
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// cmpOptions are some options for go-cmp that allow comparing our types that would not
// normally be comparable due to having unexported fields.
//
// Tests must not modify this value or anything reachable through it, even though the
// Go type system cannot prevent that.
var cmpOptions = cmp.Options{
	cmp.AllowUnexported(Credentials{}),
}
