// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tf

import (
	backendInit "github.com/placeholderplaceholderplaceholder/opentf/internal/backend/init"
)

func init() {
	// Initialize the backends
	backendInit.Init(nil)
}
