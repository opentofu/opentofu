// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

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
)

func TestBaseEncryption_methodConfigsFromTargetAndSetup(t *testing.T) {
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
			wantErr: `Test Config Source:3,13-38: Reference to undeclared key provider; There is no key_provider "static" "basic" block declared in the encryption block.`,
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
			wantErr: "Test Config Source:0,0-0: Unencrypted method is forbidden; Unable to use unencrypted method since the enforced flag is set.",
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
			wantErr: "Test Config Source:3,12-34: Invalid Key Provider expression format; The key_provider symbol must be followed by two more attribute names specifying the type and name of the selected key provider.",
		},
		"unused-key-provider": {
			rawConfig: `
				key_provider "static" "unused" {
					key = key_provider.static[0] # Even though this is invalid and won't function, since it's not used by another key_provider or method it should not produce an error
				}
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
		"chained-key-provider": {
			rawConfig: `
				key_provider "static" "basic" {
					key = sha256(jsonencode(key_provider.static.source)) # This is *not recommended or secure* but serves to demonstrate the chain
				}
				# Since these are processed "in-order", putting the dependency after the dependent checks that the chain functions as expected
				key_provider "static" "source" {
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
		"method-using-vars": {
			rawConfig: `
				key_provider "static" "basic" {
					key = var.key
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
					aad = var.aad
				}
				state {
					method = method.aes_gcm.example
				}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"state-loaded-even-when-remote-alias-conflicts": {
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
				key_provider "static" "for_remote_state" {
					key = "test"
					encrypted_metadata_alias = "key_provider.static.basic"
				}
				method "external" "for_remote_state" {
				  keys = key_provider.static.for_remote_state
				}

				remote_state_data_sources {
				  remote_state_data_source "r" {
					method = method.aes_gcm.for_remote_state
				  }
				}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"remote-method-loaded-even-when-alias-conflicts-state-provider": {
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
				key_provider "static" "for_remote_state" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
					encrypted_metadata_alias = "key_provider.static.basic" # this conflicts with the first key_provider
				}
				method "aes_gcm" "for_remote_state" {
					keys = key_provider.static.for_remote_state
				}

				remote_state_data_sources {
				  remote_state_data_source "r" {
					method = method.aes_gcm.for_remote_state
				  }
				}
			`,
			useRemoteTarget: true,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"invalid-method-identifier-format-missing-method-keyword": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				method "unencrypted" "for_migration" {
				}
				state {
					# Missing method. prefix - this will trigger the invalid format error
					method = aes_gcm.example
					fallback {
						method = method.unencrypted.for_migration
					}
				}
			`,
			wantErr: "Test Config Source:12,15-30: Invalid encryption method identifier; Expected method of form method.<type>.<name>",
		},
		"invalid-method-identifier-format-incorrect-method-keyword": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				method "unencrypted" "for_migration" {
				}
				state {
					# using methodzzzzz. prefix - this will trigger the invalid format error
					method = methodzzzzz.aes_gcm.example
				}
			`,
			wantErr: "Test Config Source:12,15-42: Invalid encryption method identifier; Expected method of form method.<type>.<name>",
		},
		"invalid-method-identifier-format-incorrect-method-keyword-with-fallback": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				method "unencrypted" "for_migration" {
				}
				state {
					# using methodzzzzz. prefix - this will trigger the invalid format error
					method = methodzzzzz.aes_gcm.example
					fallback {
						method = method.unencrypted.for_migration
					}
				}
			`,
			wantErr: "Test Config Source:12,15-42: Invalid encryption method identifier; Expected method of form method.<type>.<name>",
		},
		"reference-to-undeclared-method-with-fallback": {
			rawConfig: `
				key_provider "static" "basic" {
					key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				}
				method "aes_gcm" "example" {
					keys = key_provider.static.basic
				}
				method "unencrypted" "for_migration" {
				}
				state {
					# Correct format but referencing a non-existent method
					method = method.aes_gcm.nonexistent # this is interpolated strictly to verify that things work as expected with interpolation of methods in the state block
					fallback {
						method = method.unencrypted.for_migration
					}
				}
`,
			wantErr: `Test Config Source:12,15-41: Reference to undeclared encryption method; There is no method "aes_gcm" "nonexistent" block declared in the encryption block.`,
			wantMethods: []func(method.Method) bool{
				unencrypted.Is,
			},
		},
		// In https://github.com/opentofu/opentofu/issues/3482 was discovered that interpolation for
		// target.method does not work, but only literal reference.
		// This solves the inconsistencies between the way string expressions are evaluated for state.method vs method.keys.
		"json-config-loads-state-method-interpolated": {
			rawConfig: `{
			  "key_provider": {
				"static": {
				  "basic": {
					"key": "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				  }
				}
			  },
			  "method": {
				"aes_gcm": {
				  "example": {
					"keys": "${key_provider.static.basic}"
				  }
				}
			  },
			  "state": {
				"enforced": true,
				"method": "method.aes_gcm.example"
			  }
			}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
		},
		"json-config-loads-state-method-not-interpolated": {
			rawConfig: `{
			  "key_provider": {
				"static": {
				  "basic": {
					"key": "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
				  }
				}
			  },
			  "method": {
				"aes_gcm": {
				  "example": {
					"keys": "key_provider.static.basic"
				  }
				}
			  },
			  "state": {
				"enforced": true,
				"method": "method.aes_gcm.example"
			  }
			}
			`,
			wantMethods: []func(method.Method) bool{
				aesgcm.Is,
			},
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
			"aad": {
				Name:    "aad",
				Default: cty.ListVal([]cty.Value{cty.NumberIntVal(4)}),
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
	rawConfig       string // must contain state target
	wantMethods     []func(method.Method) bool
	wantErr         string
	useRemoteTarget bool
}

func (testCase btmTestCase) newTestRun(reg registry.Registry, staticEval *configs.StaticEvaluator) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		cfg, diags := config.LoadConfigFromString("Test Config Source", testCase.rawConfig)
		if diags.HasErrors() {
			panic(diags.Error())
		}

		target, err := selectTarget(cfg, testCase.useRemoteTarget)
		if err != nil {
			t.Fatalf("Error selecting the target to run with: %v", err)
		}

		meta := keyProviderMetadata{
			input:  make(map[keyprovider.MetaStorageKey][]byte),
			output: make(map[keyprovider.MetaStorageKey][]byte),
		}

		var methods []method.Method
		methodConfigs, diags := methodConfigsFromTarget(cfg, target, "test", cfg.State.Enforced)
		for _, methodConfig := range methodConfigs {
			m, mDiags := setupMethod(t.Context(), cfg, methodConfig, meta, reg, staticEval)
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

func selectTarget(encryptionConfig *config.EncryptionConfig, useRemote bool) (*config.TargetConfig, error) {
	if !useRemote {
		return encryptionConfig.State.AsTargetConfig(), nil
	}
	if encryptionConfig.Remote.Default != nil {
		return encryptionConfig.Remote.Default, nil
	}
	if len(encryptionConfig.Remote.Targets) == 0 {
		return nil, fmt.Errorf("configured to run with remote target but there is nothing configured. Check the rawConfig")
	}
	return encryptionConfig.Remote.Targets[0].AsTargetConfig(), nil
}

func hasDiagWithMsg(diags hcl.Diagnostics, msg string) bool {
	for _, d := range diags {
		if d.Error() == msg {
			return true
		}
	}
	return false
}
