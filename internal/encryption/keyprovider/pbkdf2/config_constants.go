// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"crypto/sha256"
	"crypto/sha512"
)

const (
	SHA256HashFunctionName  HashFunctionName = "sha256"
	SHA512HashFunctionName  HashFunctionName = "sha512"
	DefaultHashFunctionName HashFunctionName = SHA512HashFunctionName
)

var hashFunctions = map[HashFunctionName]HashFunction{
	SHA256HashFunctionName: sha256.New,
	SHA512HashFunctionName: sha512.New,
}

const (
	MinimumIterations       int = 200000
	MinimumPassphraseLength int = 16
)
