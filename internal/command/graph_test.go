// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
)

func TestGraph(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("graph"), td)
	t.Chdir(td)

	ui := new(cli.MockUi)
	c := &GraphCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(applyFixtureProvider()),
			Ui:               ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: \n%s", ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()
	if !strings.Contains(output, `provider[\"registry.opentofu.org/hashicorp/test\"]`) {
		t.Fatalf("doesn't look like digraph: %s", output)
	}
}

func TestGraph_multipleArgs(t *testing.T) {
	ui := new(cli.MockUi)
	c := &GraphCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(applyFixtureProvider()),
			Ui:               ui,
		},
	}

	args := []string{
		"bad",
		"bad",
	}
	if code := c.Run(args); code != 1 {
		t.Fatalf("bad: \n%s", ui.OutputWriter.String())
	}
}

func TestGraph_noArgs(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("graph"), td)
	t.Chdir(td)

	ui := new(cli.MockUi)
	c := &GraphCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(applyFixtureProvider()),
			Ui:               ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: \n%s", ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()
	if !strings.Contains(output, `provider[\"registry.opentofu.org/hashicorp/test\"]`) {
		t.Fatalf("doesn't look like digraph: %s", output)
	}
}

func TestGraph_noConfig(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	ui := new(cli.MockUi)
	c := &GraphCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(applyFixtureProvider()),
			Ui:               ui,
		},
	}

	// Running the graph command without a config should not panic,
	// but this may be an error at some point in the future.
	args := []string{"-type", "apply"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: \n%s", ui.ErrorWriter.String())
	}
}

func TestGraph_plan(t *testing.T) {
	testCwdTemp(t)

	plan := &plans.Plan{
		Changes: plans.NewChanges(),
	}
	plan.Changes.Resources = append(plan.Changes.Resources, &plans.ResourceInstanceChangeSrc{
		Addr: addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test_instance",
			Name: "bar",
		}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
		ChangeSrc: plans.ChangeSrc{
			Action: plans.Delete,
			Before: plans.DynamicValue(`{}`),
			After:  plans.DynamicValue(`null`),
		},
		ProviderAddr: addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
	})
	beConfig := cty.ObjectVal(map[string]cty.Value{
		"path":          cty.NilVal,
		"workspace_dir": cty.NilVal,
	})
	emptyConfig, err := plans.NewDynamicValue(beConfig, beConfig.Type())
	if err != nil {
		t.Fatal(err)
	}
	plan.Backend = plans.Backend{
		Type:   "local",
		Config: emptyConfig,
	}
	_, configSnap := testModuleWithSnapshot(t, "graph")

	planPath := testPlanFile(t, configSnap, states.NewState(), plan)

	ui := new(cli.MockUi)
	c := &GraphCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(applyFixtureProvider()),
			Ui:               ui,
		},
	}

	args := []string{
		"-plan", planPath,
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: \n%s", ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()
	if !strings.Contains(output, `provider[\"registry.opentofu.org/hashicorp/test\"]`) {
		t.Fatalf("doesn't look like digraph: %s", output)
	}
}
