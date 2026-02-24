// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package main

import (
	"os"
)

var ignoreSignals = []os.Signal{os.Interrupt}
var forwardSignals []os.Signal
