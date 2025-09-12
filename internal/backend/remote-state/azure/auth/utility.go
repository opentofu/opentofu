// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"fmt"
	"os"
)

// consolidateFileAndValue takes the (potentially empty) values of a directly-set configuration string and
// the string value of a plaintext file and picks the one that's nonempty. If both are set and nonempty,
// it checks that they share an identical value and returns that value. If they're both empty, it returns
// an error unless acceptEmpty is true.
func consolidateFileAndValue(value, fileName, fieldName string, acceptEmpty bool) (string, error) {
	var fileValue string
	if fileName != "" {
		b, err := os.ReadFile(fileName)
		if err != nil {
			return "", fmt.Errorf("error reading %s file: %w", fieldName, err)
		}
		fileValue = string(b)
	}

	hasValue := value != ""
	hasFile := fileValue != ""

	if !hasValue && !hasFile {
		if acceptEmpty {
			return "", nil
		}
		return "", fmt.Errorf("missing %s, a %s is required", fieldName, fieldName)
	}

	if !hasValue {
		return fileValue, nil
	}

	if !hasFile {
		return value, nil
	}

	if value != fileValue {
		return "", fmt.Errorf("%s provided directly and through file do not match; either make them the same value or only provide one", fieldName)
	}
	return fileValue, nil
}
