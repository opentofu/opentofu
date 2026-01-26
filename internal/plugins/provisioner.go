// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/provisioners"
)

type ProvisionerFactories map[string]provisioners.Factory

func (p ProvisionerFactories) HasProvisioner(typ string) bool {
	_, ok := p[typ]
	return ok
}

func (p ProvisionerFactories) NewInstance(typ string) (provisioners.Interface, error) {
	f, ok := p[typ]
	if !ok {
		return nil, fmt.Errorf("unavailable provisioner %q", typ)
	}

	return f()
}
