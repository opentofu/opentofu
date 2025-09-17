// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package main

import (
	"os"
)

var ignoreSignals = []os.Signal{}
var forwardSignals = []os.Signal{os.Interrupt}
