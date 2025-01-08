// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

func New() Descriptor {
	return &descriptor{}
}

type Descriptor interface {
	keyprovider.Descriptor

	TypedConfig() *Config
}

type descriptor struct {
}

func (f descriptor) ID() keyprovider.ID {
	return "external"
}

func (f descriptor) TypedConfig() *Config {
	return &Config{}
}

func (f descriptor) ConfigStruct() keyprovider.Config {
	return f.TypedConfig()
}
