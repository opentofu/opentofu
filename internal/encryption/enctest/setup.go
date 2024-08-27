// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package enctest

// This package is used for supplying a fully configured encryption instance for use in unit and integration tests

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/terramate-io/opentofulib/internal/configs"
	"github.com/terramate-io/opentofulib/internal/encryption"
	"github.com/terramate-io/opentofulib/internal/encryption/config"
	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider/static"
	"github.com/terramate-io/opentofulib/internal/encryption/method/aesgcm"
	"github.com/terramate-io/opentofulib/internal/encryption/method/unencrypted"
	"github.com/terramate-io/opentofulib/internal/encryption/registry/lockingencryptionregistry"
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
	if err := reg.RegisterMethod(unencrypted.New()); err != nil {
		panic(err)
	}

	cfg, diags := config.LoadConfigFromString("Test Config Source", configData)

	handleDiags(diags)

	staticEval := configs.NewStaticEvaluator(nil, configs.RootModuleCallForTesting())

	enc, diags := encryption.New(reg, cfg, staticEval)
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
		method "unencrypted" "migration" {}
		state {
			method = method.aes_gcm.example
			fallback {
				method = method.unencrypted.migration
			}
		}
		plan {
			method = method.aes_gcm.example
			fallback {
				method = method.unencrypted.migration
			}
		}
		remote_state_data_sources {
			default {
				method = method.aes_gcm.example
				fallback {
					method = method.unencrypted.migration
				}
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
