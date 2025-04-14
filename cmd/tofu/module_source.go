// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/opentofu/opentofu/internal/getmodules"
)

func remoteModulePackageFetcher() *getmodules.PackageFetcher {
	// TODO: Pass in a real getmodules.PackageFetcherEnvironment here,
	// which knows how to make use of the OCI authentication policy.
	return getmodules.NewPackageFetcher(nil)
}
