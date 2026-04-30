// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build wasip1 || js

package flock

import (
	"context"
	"os"
)

func Lock(f *os.File) error {
	return nil
}

func LockBlocking(ctx context.Context, f *os.File) error {
	return nil
}

func Unlock(f *os.File) error {
	return nil
}
