// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestNodeValidatableResource_ValidateProvisioner_valid(t *testing.T) {
	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	mp := &MockProvisioner{}
	ps := &configschema.Block{}
	ctx.ProvisionerSchemaSchema = ps
	ctx.ProvisionerProvisioner = mp

	pc := &configs.Provisioner{
		Type:   "baz",
		Config: hcl.EmptyBody(),
		Connection: &configs.Connection{
			Config: configs.SynthBody("", map[string]cty.Value{
				"host": cty.StringVal("localhost"),
				"type": cty.StringVal("ssh"),
				"port": cty.NumberIntVal(10022),
			}),
		},
	}

	rc := &configs.Resource{
		Mode:   addrs.ManagedResourceMode,
		Type:   "test_foo",
		Name:   "bar",
		Config: configs.SynthBody("", map[string]cty.Value{}),
	}

	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:   mustConfigResourceAddr("test_foo.bar"),
			Config: rc,
		},
	}

	diags := node.validateProvisioner(t.Context(), ctx, pc)
	if diags.HasErrors() {
		t.Fatalf("node.Eval failed: %s", diags.Err())
	}
	if !mp.ValidateProvisionerConfigCalled {
		t.Fatalf("p.ValidateProvisionerConfig not called")
	}
}

func TestNodeValidatableResource_ValidateProvisioner__warning(t *testing.T) {
	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	mp := &MockProvisioner{}
	ps := &configschema.Block{}
	ctx.ProvisionerSchemaSchema = ps
	ctx.ProvisionerProvisioner = mp

	pc := &configs.Provisioner{
		Type:   "baz",
		Config: hcl.EmptyBody(),
	}

	rc := &configs.Resource{
		Mode:    addrs.ManagedResourceMode,
		Type:    "test_foo",
		Name:    "bar",
		Config:  configs.SynthBody("", map[string]cty.Value{}),
		Managed: &configs.ManagedResource{},
	}

	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:   mustConfigResourceAddr("test_foo.bar"),
			Config: rc,
		},
	}

	{
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.SimpleWarning("foo is deprecated"))
		mp.ValidateProvisionerConfigResponse = provisioners.ValidateProvisionerConfigResponse{
			Diagnostics: diags,
		}
	}

	diags := node.validateProvisioner(t.Context(), ctx, pc)
	if len(diags) != 1 {
		t.Fatalf("wrong number of diagnostics in %s; want one warning", diags.ErrWithWarnings())
	}

	if got, want := diags[0].Description().Summary, mp.ValidateProvisionerConfigResponse.Diagnostics[0].Description().Summary; got != want {
		t.Fatalf("wrong warning %q; want %q", got, want)
	}
}

func TestNodeValidatableResource_ValidateProvisioner__connectionInvalid(t *testing.T) {
	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	mp := &MockProvisioner{}
	ps := &configschema.Block{}
	ctx.ProvisionerSchemaSchema = ps
	ctx.ProvisionerProvisioner = mp

	pc := &configs.Provisioner{
		Type:   "baz",
		Config: hcl.EmptyBody(),
		Connection: &configs.Connection{
			Config: configs.SynthBody("", map[string]cty.Value{
				"type":             cty.StringVal("ssh"),
				"bananananananana": cty.StringVal("foo"),
				"bazaz":            cty.StringVal("bar"),
			}),
		},
	}

	rc := &configs.Resource{
		Mode:    addrs.ManagedResourceMode,
		Type:    "test_foo",
		Name:    "bar",
		Config:  configs.SynthBody("", map[string]cty.Value{}),
		Managed: &configs.ManagedResource{},
	}

	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:   mustConfigResourceAddr("test_foo.bar"),
			Config: rc,
		},
	}

	diags := node.validateProvisioner(t.Context(), ctx, pc)
	if !diags.HasErrors() {
		t.Fatalf("node.Eval succeeded; want error")
	}
	if len(diags) != 3 {
		t.Fatalf("wrong number of diagnostics; want two errors\n\n%s", diags.Err())
	}

	errStr := diags.Err().Error()
	if !strings.Contains(errStr, "bananananananana") || !strings.Contains(errStr, "bazaz") {
		t.Fatalf("wrong errors %q; want something about each of our invalid connInfo keys", errStr)
	}
}

func TestNodeValidatableResource_ValidateResource_managedResource(t *testing.T) {
	mp := simpleMockProvider()
	mp.ValidateResourceConfigFn = func(req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
		if got, want := req.TypeName, "test_object"; got != want {
			t.Fatalf("wrong resource type\ngot:  %#v\nwant: %#v", got, want)
		}
		if got, want := req.Config.GetAttr("test_string"), cty.StringVal("bar"); !got.RawEquals(want) {
			t.Fatalf("wrong value for test_string\ngot:  %#v\nwant: %#v", got, want)
		}
		if got, want := req.Config.GetAttr("test_number"), cty.NumberIntVal(2); !got.RawEquals(want) {
			t.Fatalf("wrong value for test_number\ngot:  %#v\nwant: %#v", got, want)
		}
		return providers.ValidateResourceConfigResponse{}
	}

	p := providers.Interface(mp)
	rc := &configs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_object",
		Name: "foo",
		Config: configs.SynthBody("", map[string]cty.Value{
			"test_string": cty.StringVal("bar"),
			"test_number": cty.NumberIntVal(2).Mark(marks.Sensitive),
		}),
	}
	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:             mustConfigResourceAddr("test_foo.bar"),
			Config:           rc,
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	err := node.validateResource(t.Context(), ctx)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !mp.ValidateResourceConfigCalled {
		t.Fatal("Expected ValidateResourceConfig to be called, but it was not!")
	}
}

func TestNodeValidatableResource_ValidateResource_managedResourceCount(t *testing.T) {
	// Setup
	mp := simpleMockProvider()
	mp.ValidateResourceConfigFn = func(req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
		if got, want := req.TypeName, "test_object"; got != want {
			t.Fatalf("wrong resource type\ngot:  %#v\nwant: %#v", got, want)
		}
		if got, want := req.Config.GetAttr("test_string"), cty.StringVal("bar"); !got.RawEquals(want) {
			t.Fatalf("wrong value for test_string\ngot:  %#v\nwant: %#v", got, want)
		}
		return providers.ValidateResourceConfigResponse{}
	}

	p := providers.Interface(mp)

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	tests := []struct {
		name  string
		count hcl.Expression
	}{
		{
			"simple count",
			hcltest.MockExprLiteral(cty.NumberIntVal(2)),
		},
		{
			"marked count value",
			hcltest.MockExprLiteral(cty.NumberIntVal(3).Mark("marked")),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rc := &configs.Resource{
				Mode:  addrs.ManagedResourceMode,
				Type:  "test_object",
				Name:  "foo",
				Count: test.count,
				Config: configs.SynthBody("", map[string]cty.Value{
					"test_string": cty.StringVal("bar"),
				}),
			}
			node := NodeValidatableResource{
				NodeAbstractResource: &NodeAbstractResource{
					Addr:             mustConfigResourceAddr("test_foo.bar"),
					Config:           rc,
					ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
				},
			}

			diags := node.validateResource(t.Context(), ctx)
			if diags.HasErrors() {
				t.Fatalf("err: %s", diags.Err())
			}

			if !mp.ValidateResourceConfigCalled {
				t.Fatal("Expected ValidateResourceConfig to be called, but it was not!")
			}
		})
	}
}

func TestNodeValidatableResource_ValidateResource_dataSource(t *testing.T) {
	mp := simpleMockProvider()
	mp.ValidateDataResourceConfigFn = func(req providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
		if got, want := req.TypeName, "test_object"; got != want {
			t.Fatalf("wrong resource type\ngot:  %#v\nwant: %#v", got, want)
		}
		if got, want := req.Config.GetAttr("test_string"), cty.StringVal("bar"); !got.RawEquals(want) {
			t.Fatalf("wrong value for test_string\ngot:  %#v\nwant: %#v", got, want)
		}
		if got, want := req.Config.GetAttr("test_number"), cty.NumberIntVal(2); !got.RawEquals(want) {
			t.Fatalf("wrong value for test_number\ngot:  %#v\nwant: %#v", got, want)
		}
		return providers.ValidateDataResourceConfigResponse{}
	}

	p := providers.Interface(mp)
	rc := &configs.Resource{
		Mode: addrs.DataResourceMode,
		Type: "test_object",
		Name: "foo",
		Config: configs.SynthBody("", map[string]cty.Value{
			"test_string": cty.StringVal("bar"),
			"test_number": cty.NumberIntVal(2).Mark(marks.Sensitive),
		}),
	}

	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:             mustConfigResourceAddr("test_foo.bar"),
			Config:           rc,
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	diags := node.validateResource(t.Context(), ctx)
	if diags.HasErrors() {
		t.Fatalf("err: %s", diags.Err())
	}

	if !mp.ValidateDataResourceConfigCalled {
		t.Fatal("Expected ValidateDataSourceConfig to be called, but it was not!")
	}
}

func TestNodeValidatableResource_ValidateResource_valid(t *testing.T) {
	mp := simpleMockProvider()
	mp.ValidateResourceConfigFn = func(req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
		return providers.ValidateResourceConfigResponse{}
	}

	p := providers.Interface(mp)
	rc := &configs.Resource{
		Mode:   addrs.ManagedResourceMode,
		Type:   "test_object",
		Name:   "foo",
		Config: configs.SynthBody("", map[string]cty.Value{}),
	}
	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:             mustConfigResourceAddr("test_object.foo"),
			Config:           rc,
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	diags := node.validateResource(t.Context(), ctx)
	if diags.HasErrors() {
		t.Fatalf("err: %s", diags.Err())
	}
}

func TestNodeValidatableResource_ValidateResource_warningsAndErrorsPassedThrough(t *testing.T) {
	mp := simpleMockProvider()
	mp.ValidateResourceConfigFn = func(req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.SimpleWarning("warn"))
		diags = diags.Append(errors.New("err"))
		return providers.ValidateResourceConfigResponse{
			Diagnostics: diags,
		}
	}

	p := providers.Interface(mp)
	rc := &configs.Resource{
		Mode:   addrs.ManagedResourceMode,
		Type:   "test_object",
		Name:   "foo",
		Config: configs.SynthBody("", map[string]cty.Value{}),
	}
	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:             mustConfigResourceAddr("test_foo.bar"),
			Config:           rc,
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	diags := node.validateResource(t.Context(), ctx)
	if !diags.HasErrors() {
		t.Fatal("unexpected success; want error")
	}

	bySeverity := map[tfdiags.Severity]tfdiags.Diagnostics{}
	for _, diag := range diags {
		bySeverity[diag.Severity()] = append(bySeverity[diag.Severity()], diag)
	}
	if len(bySeverity[tfdiags.Warning]) != 1 || bySeverity[tfdiags.Warning][0].Description().Summary != "warn" {
		t.Errorf("Expected 1 warning 'warn', got: %s", bySeverity[tfdiags.Warning].ErrWithWarnings())
	}
	if len(bySeverity[tfdiags.Error]) != 1 || bySeverity[tfdiags.Error][0].Description().Summary != "err" {
		t.Errorf("Expected 1 error 'err', got: %s", bySeverity[tfdiags.Error].Err())
	}
}

func TestNodeValidatableResource_ValidateResource_invalidDependsOn(t *testing.T) {
	mp := simpleMockProvider()
	mp.ValidateResourceConfigFn = func(req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
		return providers.ValidateResourceConfigResponse{}
	}

	// We'll check a _valid_ config first, to make sure we're not failing
	// for some other reason, and then make it invalid.
	p := providers.Interface(mp)
	rc := &configs.Resource{
		Mode:   addrs.ManagedResourceMode,
		Type:   "test_object",
		Name:   "foo",
		Config: configs.SynthBody("", map[string]cty.Value{}),
		DependsOn: []hcl.Traversal{
			// Depending on path.module is pointless, since it is immediately
			// available, but we allow all of the referenceable addrs here
			// for consistency: referencing them is harmless, and avoids the
			// need for us to document a different subset of addresses that
			// are valid in depends_on.
			// For the sake of this test, it's a valid address we can use that
			// doesn't require something else to exist in the configuration.
			{
				hcl.TraverseRoot{
					Name: "path",
				},
				hcl.TraverseAttr{
					Name: "module",
				},
			},
		},
	}
	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:             mustConfigResourceAddr("test_foo.bar"),
			Config:           rc,
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()

	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	diags := node.validateResource(t.Context(), ctx)
	if diags.HasErrors() {
		t.Fatalf("error for supposedly-valid config: %s", diags.ErrWithWarnings())
	}

	// Now we'll make it invalid by adding additional traversal steps at
	// the end of what we're referencing. This is intended to catch the
	// situation where the user tries to depend on e.g. a specific resource
	// attribute, rather than the whole resource, like aws_instance.foo.id.
	rc.DependsOn = append(rc.DependsOn, hcl.Traversal{
		hcl.TraverseRoot{
			Name: "path",
		},
		hcl.TraverseAttr{
			Name: "module",
		},
		hcl.TraverseAttr{
			Name: "extra",
		},
	})

	diags = node.validateResource(t.Context(), ctx)
	if !diags.HasErrors() {
		t.Fatal("no error for invalid depends_on")
	}
	if got, want := diags.Err().Error(), "Invalid depends_on reference"; !strings.Contains(got, want) {
		t.Fatalf("wrong error\ngot:  %s\nwant: Message containing %q", got, want)
	}

	// Test for handling an unknown root without attribute, like a
	// typo that omits the dot inbetween "path.module".
	rc.DependsOn = append(rc.DependsOn, hcl.Traversal{
		hcl.TraverseRoot{
			Name: "pathmodule",
		},
	})

	diags = node.validateResource(t.Context(), ctx)
	if !diags.HasErrors() {
		t.Fatal("no error for invalid depends_on")
	}
	if got, want := diags.Err().Error(), "Invalid depends_on reference"; !strings.Contains(got, want) {
		t.Fatalf("wrong error\ngot:  %s\nwant: Message containing %q", got, want)
	}
}

func TestNodeValidatableResource_ValidateResource_invalidIgnoreChangesNonexistent(t *testing.T) {
	mp := simpleMockProvider()
	mp.ValidateResourceConfigFn = func(req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
		return providers.ValidateResourceConfigResponse{}
	}

	// We'll check a _valid_ config first, to make sure we're not failing
	// for some other reason, and then make it invalid.
	p := providers.Interface(mp)
	rc := &configs.Resource{
		Mode:   addrs.ManagedResourceMode,
		Type:   "test_object",
		Name:   "foo",
		Config: configs.SynthBody("", map[string]cty.Value{}),
		Managed: &configs.ManagedResource{
			IgnoreChanges: []hcl.Traversal{
				{
					hcl.TraverseAttr{
						Name: "test_string",
					},
				},
			},
		},
	}
	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:             mustConfigResourceAddr("test_foo.bar"),
			Config:           rc,
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()

	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	diags := node.validateResource(t.Context(), ctx)
	if diags.HasErrors() {
		t.Fatalf("error for supposedly-valid config: %s", diags.ErrWithWarnings())
	}

	// Now we'll make it invalid by attempting to ignore a nonexistent
	// attribute.
	rc.Managed.IgnoreChanges = append(rc.Managed.IgnoreChanges, hcl.Traversal{
		hcl.TraverseAttr{
			Name: "nonexistent",
		},
	})

	diags = node.validateResource(t.Context(), ctx)
	if !diags.HasErrors() {
		t.Fatal("no error for invalid ignore_changes")
	}
	if got, want := diags.Err().Error(), "Unsupported attribute: This object has no argument, nested block, or exported attribute named \"nonexistent\""; !strings.Contains(got, want) {
		t.Fatalf("wrong error\ngot:  %s\nwant: Message containing %q", got, want)
	}
}

func TestNodeValidatableResource_ValidateResource_invalidIgnoreChangesComputed(t *testing.T) {
	// construct a schema with a computed attribute
	ms := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"test_string": {
				Type:     cty.String,
				Optional: true,
			},
			"computed_string": {
				Type:     cty.String,
				Computed: true,
				Optional: false,
			},
		},
	}

	mp := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: ms},
			ResourceTypes: map[string]providers.Schema{
				"test_object": providers.Schema{Block: ms},
			},
		},
	}

	mp.ValidateResourceConfigFn = func(req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
		return providers.ValidateResourceConfigResponse{}
	}

	// We'll check a _valid_ config first, to make sure we're not failing
	// for some other reason, and then make it invalid.
	p := providers.Interface(mp)
	rc := &configs.Resource{
		Mode:   addrs.ManagedResourceMode,
		Type:   "test_object",
		Name:   "foo",
		Config: configs.SynthBody("", map[string]cty.Value{}),
		Managed: &configs.ManagedResource{
			IgnoreChanges: []hcl.Traversal{
				{
					hcl.TraverseAttr{
						Name: "test_string",
					},
				},
			},
		},
	}
	node := NodeValidatableResource{
		NodeAbstractResource: &NodeAbstractResource{
			Addr:             mustConfigResourceAddr("test_foo.bar"),
			Config:           rc,
			ResolvedProvider: ResolvedProvider{ProviderConfig: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`)},
		},
	}

	ctx := &MockEvalContext{}
	ctx.installSimpleEval()

	ctx.ProviderSchemaSchema = mp.GetProviderSchema()
	ctx.ProviderProvider = p

	diags := node.validateResource(t.Context(), ctx)
	if diags.HasErrors() {
		t.Fatalf("error for supposedly-valid config: %s", diags.ErrWithWarnings())
	}

	// Now we'll make it invalid by attempting to ignore a computed
	// attribute.
	rc.Managed.IgnoreChanges = append(rc.Managed.IgnoreChanges, hcl.Traversal{
		hcl.TraverseAttr{
			Name: "computed_string",
		},
	})

	diags = node.validateResource(t.Context(), ctx)
	if diags.HasErrors() {
		t.Fatalf("got unexpected error: %s", diags.ErrWithWarnings())
	}
	if got, want := diags.ErrWithWarnings().Error(), `Redundant ignore_changes element: Adding an attribute name to ignore_changes tells OpenTofu to ignore future changes to the argument in configuration after the object has been created, retaining the value originally configured.

The attribute computed_string is decided by the provider alone and therefore there can be no configured value to compare with. Including this attribute in ignore_changes has no effect. Remove the attribute from ignore_changes to quiet this warning.`; !strings.Contains(got, want) {
		t.Fatalf("wrong error\ngot:  %s\nwant: Message containing %q", got, want)
	}
}
