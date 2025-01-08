// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type Config struct {
	Command []string `hcl:"command"`
}

func (c *Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if len(c.Command) < 1 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "the command option is required",
		}
	}
	return &keyProvider{
		command: c.Command,
	}, &MetadataV1{}, nil
}
