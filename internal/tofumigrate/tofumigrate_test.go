// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofumigrate

import (
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/states"
)

func TestMigrateStateProviderAddresses(t *testing.T) {
	loader := configload.NewLoaderForTests(t)
	mustParseInstAddr := func(s string) addrs.AbsResourceInstance {
		addr, err := addrs.ParseAbsResourceInstanceStr(s)
		if err != nil {
			t.Fatal(err)
		}
		return addr
	}

	makeRootProviderAddr := func(s string) addrs.AbsProviderConfig {
		return addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.MustParseProviderSourceString(s),
		}
	}

	type args struct {
		configDir string
		state     *states.State
	}
	tests := []struct {
		name string
		args args
		want *states.State
	}{
		{
			name: "if there are no code references, migrate",
			args: args{
				configDir: "testdata/nomention",
				state: states.BuildState(func(s *states.SyncState) {
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("random_id.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.terraform.io/hashicorp/random"),
						addrs.NoKey,
					)
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("aws_instance.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.terraform.io/hashicorp/aws"),
						addrs.NoKey,
					)
				}),
			},
			want: states.BuildState(func(s *states.SyncState) {
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("random_id.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.opentofu.org/hashicorp/random"),
					addrs.NoKey,
				)
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("aws_instance.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.opentofu.org/hashicorp/aws"),
					addrs.NoKey,
				)
			}),
		},
		{
			name: "if there are some full-form references in the code, only migrate the ones not referenced",
			args: args{
				configDir: "testdata/mention",
				state: states.BuildState(func(s *states.SyncState) {
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("random_id.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.terraform.io/hashicorp/random"),
						addrs.NoKey,
					)
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("aws_instance.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.terraform.io/hashicorp/aws"),
						addrs.NoKey,
					)
				}),
			},
			want: states.BuildState(func(s *states.SyncState) {
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("random_id.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.opentofu.org/hashicorp/random"),
					addrs.NoKey,
				)
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("aws_instance.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.terraform.io/hashicorp/aws"),
					addrs.NoKey,
				)
			}),
		},
		{
			name: "if the state file contains no legacy references, return statefile unchanged",
			args: args{
				configDir: "testdata/nomention",
				state: states.BuildState(func(s *states.SyncState) {
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("random_id.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.opentofu.org/hashicorp/random"),
						addrs.NoKey,
					)
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("aws_instance.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.opentofu.org/hashicorp/aws"),
						addrs.NoKey,
					)
				}),
			},
			want: states.BuildState(func(s *states.SyncState) {
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("random_id.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.opentofu.org/hashicorp/random"),
					addrs.NoKey,
				)
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("aws_instance.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.opentofu.org/hashicorp/aws"),
					addrs.NoKey,
				)
			}),
		},
		{
			name: "if there is no code, migrate",
			args: args{
				configDir: "",
				state: states.BuildState(func(s *states.SyncState) {
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("random_id.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.terraform.io/hashicorp/random"),
						addrs.NoKey,
					)
					s.SetResourceInstanceCurrent(
						mustParseInstAddr("aws_instance.example"),
						&states.ResourceInstanceObjectSrc{
							Status:    states.ObjectReady,
							AttrsJSON: []byte(`{}`),
						},
						makeRootProviderAddr("registry.terraform.io/hashicorp/aws"),
						addrs.NoKey,
					)
				}),
			},
			want: states.BuildState(func(s *states.SyncState) {
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("random_id.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.opentofu.org/hashicorp/random"),
					addrs.NoKey,
				)
				s.SetResourceInstanceCurrent(
					mustParseInstAddr("aws_instance.example"),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{}`),
					},
					makeRootProviderAddr("registry.opentofu.org/hashicorp/aws"),
					addrs.NoKey,
				)
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *configs.Config
			if tt.args.configDir != "" {
				var hclDiags hcl.Diagnostics
				cfg, hclDiags = loader.LoadConfig(t.Context(), tt.args.configDir, configs.RootModuleCallForTesting())
				if hclDiags.HasErrors() {
					t.Fatalf("invalid configuration: %s", hclDiags.Error())
				}
			}

			got, err := MigrateStateProviderAddresses(cfg, tt.args.state)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MigrateStateProviderAddresses() got = %v, want %v", got, tt.want)
			}
			if err != nil {
				t.Errorf("MigrateStateProviderAddresses() err = %v, want %v", err, nil)
			}
		})
	}
}
