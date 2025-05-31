// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statekeys

import (
	"encoding/base32"
	"fmt"
)

func decodeBase32(raw string) (string, error) {
	bytes, err := base32.HexEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base32 encoding")
	}
	return string(bytes), nil
}

func encodeBase32(val string) string {
	return base32.HexEncoding.EncodeToString([]byte(val))
}
