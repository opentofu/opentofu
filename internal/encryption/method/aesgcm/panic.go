// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import "fmt"

// handlePanic runs the specified function and returns its result value or returned error. If a panic occurs, it returns the
// panic as an error.
func handlePanic(f func() ([]byte, error)) (result []byte, err error) {
	result, e := func() ([]byte, error) {
		defer func() {
			var ok bool
			e := recover()
			if e == nil {
				return
			}
			if err, ok = e.(error); !ok {
				// In case the panic is not an error
				err = fmt.Errorf("%v", e)
			}
		}()
		return f()
	}()
	if err != nil {
		return nil, err
	}
	return result, e
}
