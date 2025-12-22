// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package unencrypted

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

func New() method.Descriptor {
	return &descriptor{}
}

type descriptor struct{}

func (f *descriptor) ID() method.ID {
	return "unencrypted"
}
func (f *descriptor) DecodeConfig(_ method.EvalContext, _ hcl.Body) (method.Config, hcl.Diagnostics) {
	return new(methodConfig), nil
}

type methodConfig struct{}

func (c *methodConfig) Build() (method.Method, error) {
	return new(unenc), nil
}

type unenc struct{}

func (a *unenc) Encrypt(data []byte) ([]byte, error) {
	panic("Placeholder for type check!  Should never be called!")
}
func (a *unenc) Decrypt(data []byte) ([]byte, error) {
	panic("Placeholder for type check!  Should never be called!")
}

func Is(m method.Method) bool {
	_, ok := m.(*unenc)
	return ok
}

func IsConfig(m config.MethodConfig) bool {
	return m.Type == "unencrypted"
}
