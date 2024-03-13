// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package enctest

// This package is used for supplying a fully configured encryption instance for use in unit and integration tests

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

// TODO docstrings once this stabilizes

func EncryptionDirect(configData string) encryption.Encryption {
	reg := lockingencryptionregistry.New()
	if err := reg.RegisterKeyProvider(static.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}

	cfg, diags := config.LoadConfigFromString("Test Config Source", configData)

	handleDiags(diags)

	enc, diags := encryption.New(reg, cfg)
	handleDiags(diags)

	return enc
}

func EncryptionRequired() encryption.Encryption {
	return EncryptionDirect(`
		key_provider "static" "basic" {
			key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
		}
		method "aes_gcm" "example" {
			keys = key_provider.static.basic
		}
		state {
			method = method.aes_gcm.example
		}
		plan {
			method = method.aes_gcm.example
		}
		remote_state_data_sources {
			default {
				method = method.aes_gcm.example
			}
		}
	`)
}

func EncryptionWithFallback() encryption.Encryption {
	return EncryptionDirect(`
		key_provider "static" "basic" {
			key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
		}
		method "aes_gcm" "example" {
			keys = key_provider.static.basic
		}
		state {
			method = method.aes_gcm.example
			fallback {}
		}
		plan {
			method = method.aes_gcm.example
			fallback {}
		}
		remote_state_data_sources {
			default {
				method = method.aes_gcm.example
				fallback {}
			}
		}
	`)
}

func handleDiags(diags hcl.Diagnostics) {
	for _, d := range diags {
		println(d.Error())
	}
	if diags.HasErrors() {
		panic(diags.Error())
	}
}
