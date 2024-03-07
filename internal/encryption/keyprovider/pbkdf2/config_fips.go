// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build fips140_2

// TODO: read the FIPS specification and make sure all parameters are compliant.

package pbkdf2

import "crypto/sha256"

const (
	SHA256HashFunctionName  HashFunctionName = "sha256"
	DefaultHashFunctionName HashFunctionName = SHA256HashFunctionName
)

var hashFunctions = map[HashFunctionName]hashFunction{
	SHA256HashFunctionName: {
		sha256.New,
	},
}
