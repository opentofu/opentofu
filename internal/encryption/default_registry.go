// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/aws_kms"
	externalKeyProvider "github.com/opentofu/opentofu/internal/encryption/keyprovider/external"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/gcp_kms"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/openbao"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/pbkdf2"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	externalMethod "github.com/opentofu/opentofu/internal/encryption/method/external"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

var DefaultRegistry = lockingencryptionregistry.New()

func init() {
	if err := DefaultRegistry.RegisterKeyProvider(pbkdf2.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(aws_kms.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(gcp_kms.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(openbao.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(externalKeyProvider.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterMethod(externalMethod.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterMethod(unencrypted.New()); err != nil {
		panic(err)
	}
}
