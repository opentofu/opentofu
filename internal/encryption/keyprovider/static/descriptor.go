// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static

import (
	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
)

func New() Descriptor {
	return &descriptor{}
}

// Descriptor is an additional interface to allow for providing custom methods.
type Descriptor interface {
	keyprovider.Descriptor
}

type descriptor struct {
}

func (f descriptor) ID() keyprovider.ID {
	return "static"
}

func (f descriptor) ConfigStruct() keyprovider.Config {
	return &Config{}
}
