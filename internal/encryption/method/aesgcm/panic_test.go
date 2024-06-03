// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"fmt"
)

func Example_handlePanic() {
	_, err := handlePanic(func() ([]byte, error) {
		panic("Hello world!")
	})
	fmt.Printf("%v", err)
	// Output: Hello world!
}
