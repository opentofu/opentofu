// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
)

type CachedSchema struct {
	sync.Mutex
	Filter addrs.ProviderResourceRequirments
	Value  *ProviderSchema
}
