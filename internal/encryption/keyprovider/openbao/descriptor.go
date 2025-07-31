// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package openbao

import "github.com/opentofu/opentofu/internal/encryption/keyprovider"

func New() keyprovider.Descriptor {
	return &descriptor{}
}

type descriptor struct {
}

func (f descriptor) ID() keyprovider.ID {
	return "openbao"
}

func (f descriptor) ConfigStruct() keyprovider.Config {
	return &Config{}
}
