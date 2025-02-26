// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
	"github.com/zclconf/go-cty/cty"
)

func TestBaseEncryption_buildTargetMethods(t *testing.T) {
	t.Parallel()

	tests := map[string]btmTestCase{
		"simple": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				state {
					method = method.aes_gcm.example
				}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"no-key-provider": {
			rawConfig: `
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				state {
					method = method.aes_gcm.example
				}
			`,
			wantErr: `Test Config Source:3,13-38: Undefined Key Provider; Key provider static.basic is missing from the encryption configuration.`,
		},
		"fallback": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				method "unencrypted" "example" {
				}
				state {
					method = method.aes_gcm.example
					fallback {
						method = method.unencrypted.example
					}
				}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
				unencrypted.Is,
			},
		},
		"enforced": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				method "unencrypted" "example" {
				}
				state {
					enforced = true
					method   = method.aes_gcm.example
				}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"enforced-with-unencrypted": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				method "unencrypted" "example" {
				}
				state {
					enforced = true
					method   = method.aes_gcm.example
					fallback {
						method = method.unencrypted.example
					}
				}
			`,
			wantErr: "<nil>: Unencrypted method is forbidden; Unable to use `unencrypted` method since the `enforced` flag is used.",
		},
		"key-from-vars": {
			rawConfig: `
				key_provider "static" "basic" {
					key = var.key
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				state {
					method = method.aes_gcm.example
				}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"key-from-complex-vars": {
			rawConfig: `
				key_provider "static" "basic" {
					key = var.obj[0].key
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				state {
					method = method.aes_gcm.example
				}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"undefined-key-from-vars": {
			rawConfig: `
				key_provider "static" "basic" {
					key = var.undefinedkey
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				state {
					method = method.aes_gcm.example
				}
			`,
			wantErr: "Test Config Source:3,12-28: Undefined variable; Undefined variable var.undefinedkey",
		},
		"bad-keyprovider-format": {
			rawConfig: `
				key_provider "static" "basic" {
					key = key_provider.static[0]
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				state {
					method = method.aes_gcm.example
				}
			`,
			wantErr: "Test Config Source:3,12-34: Invalid Key Provider expression format; Expected key_provider.<type>.<name>",
		},
	}

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

	mod := &configs.Module{
		Variables: map[string]*configs.Variable{
			"key": {
				Name:    "key",
				Default: cty.StringVal("6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"),
				Type:    cty.String,
			},
			"obj": {
				Name:    "obj",
				Default: cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169")})}),
			},
		},
	}

	getVars := func(v *configs.Variable) (cty.Value, hcl.Diagnostics) {
		return v.Default, nil
	}

	modCall := configs.NewStaticModuleCall(addrs.RootModule, getVars, "<testing>", "")

	staticEval := configs.NewStaticEvaluator(mod, modCall)

	for name, test := range tests {
		t.Run(name, test.newTestRun(reg, staticEval))
	}
}

type btmTestCase struct {
	rawConfig   string // must contain state target
	wantMethods []func(method.Method) bool
	wantErr     string
}

func (testCase btmTestCase) newTestRun(reg registry.Registry, staticEval *configs.StaticEvaluator) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		cfg, diags := config.LoadConfigFromString("Test Config Source", testCase.rawConfig)
		if diags.HasErrors() {
			panic(diags.Error())
		}

		meta := keyProviderMetadata{
			input:  make(map[keyprovider.MetaStorageKey][]byte),
			output: make(map[keyprovider.MetaStorageKey][]byte),
		}

		var methods []method.Method
		methodConfigs, diags := methodConfigsFromTarget(cfg, cfg.State.AsTargetConfig(), "test", cfg.State.Enforced)
		for _, methodConfig := range methodConfigs {
			m, mDiags := setupMethod(cfg, methodConfig, meta, reg, staticEval)
			diags = diags.Extend(mDiags)
			if !mDiags.HasErrors() {
				methods = append(methods, m)
			}
		}

		if diags.HasErrors() {
			if !hasDiagWithMsg(diags, testCase.wantErr) {
				t.Fatalf("Got unexpected error: %v", diags.Error())
			}
		}

		if !diags.HasErrors() && testCase.wantErr != "" {
			t.Fatalf("Expected error (got none): %v", testCase.wantErr)
		}

		if len(methods) != len(testCase.wantMethods) {
			t.Fatalf("Expected %d method(s), got %d", len(testCase.wantMethods), len(methods))
		}

		for i, m := range methods {
			if !testCase.wantMethods[i](m) {
				t.Fatalf("Got unexpected method: %v", reflect.TypeOf(m).String())
			}
		}
	}
}

func hasDiagWithMsg(diags hcl.Diagnostics, msg string) bool {
	for _, d := range diags {
		if d.Error() == msg {
			return true
		}
	}
	return false
}
