// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tf

import (
	backendInit "github.com/terramate-io/opentofulib/internal/backend/init"
)

func init() {
	// Initialize the backends
	backendInit.Init(nil)
}
