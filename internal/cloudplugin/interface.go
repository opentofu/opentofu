// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloudplugin

import (
	"io"
)

type Cloud1 interface {
	Execute(args []string, stdout, stderr io.Writer) int
}
