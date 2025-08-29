// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"github.com/opentofu/opentofu/internal/addrs"
)

// ProviderInstance represents the configuration for an instance of a provider.
//
// Note that this type's name is slightly misleading because it does not
// represent an already-running provider that requests can be sent to, but
// rather the configuration that sould be sent to a running instance of
// this provider in order to prepare it for use. This package does not deal
// with "configured" providers directly at all, instead expecting its caller
// (e.g. an implementation or the plan or apply phase) to handle the provider
// instance lifecycle.
type ProviderInstance struct {
	ProviderAddr addrs.Provider
	// TODO: everything else
}
