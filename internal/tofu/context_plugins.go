// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/plugins"
)

type pluginsManager struct {
	providers               plugins.ProviderManager
	provisioners            plugins.ProvisionerManager
	providerFunctionTracker ProviderFunctionMapping
}

func newPluginsManager(library plugins.Library) *pluginsManager {
	return &pluginsManager{
		providerFunctionTracker: make(ProviderFunctionMapping),
		providers:               library.NewProviderManager(),
		provisioners:            library.NewProvisionerManager(),
	}
}
