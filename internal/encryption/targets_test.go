package encryption

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

func TestBaseEncryption_buildTargetMethods(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		rawConfig   string // must contain state target
		wantMethods []func(method.Method) bool
		wantErr     string
	}{
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
			wantErr: `Test Config Source:3,25-32: Unsupported attribute; This object does not have an attribute named "static".`,
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

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg, diags := config.LoadConfigFromString("Test Config Source", test.rawConfig)
			if diags.HasErrors() {
				panic(diags.Error())
			}

			base := &baseEncryption{
				enc: &encryption{
					cfg: cfg,
					reg: reg,
				},
				target:   cfg.State.AsTargetConfig(),
				enforced: cfg.State.Enforced,
				name:     "test",
				encMeta:  make(map[keyprovider.Addr][]byte),
			}

			methods, diags := base.buildTargetMethods(base.encMeta)

			if diags.HasErrors() {
				msgs := make([]string, len(diags))

				for i, d := range diags {
					msgs[i] = d.Error()
				}

				fullMsg := strings.Join(msgs, ";")
				if !strings.Contains(fullMsg, test.wantErr) {
					t.Fatalf("Unexpected error: %v", fullMsg)
				}
			}

			if !diags.HasErrors() && test.wantErr != "" {
				t.Fatalf("Expected error (got none): %v", test.wantErr)
			}

			if len(methods) != len(test.wantMethods) {
				t.Fatalf("Expected %d method(s), got %d", len(test.wantMethods), len(methods))
			}

			for i, m := range methods {
				if !test.wantMethods[i](m) {
					t.Fatalf("Got unexpected method: %v", m)
				}
			}
		})
	}
}
