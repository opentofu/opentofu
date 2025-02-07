// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/initwd"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/registry"
	"github.com/opentofu/opentofu/internal/states"

	_ "github.com/opentofu/opentofu/internal/logging"
)

// This is the directory where our test fixtures are.
const fixtureDir = "./testdata"

func TestMain(m *testing.M) {
	flag.Parse()

	// We have fmt.Stringer implementations on lots of objects that hide
	// details that we very often want to see in tests, so we just disable
	// spew's use of String methods globally on the assumption that spew
	// usage implies an intent to see the raw values and ignore any
	// abstractions.
	spew.Config.DisableMethods = true

	os.Exit(m.Run())
}

func testModule(t testing.TB, name string) *configs.Config {
	t.Helper()
	c, _ := testModuleWithSnapshot(t, name)
	return c
}

func testModuleWithSnapshot(t testing.TB, name string) (*configs.Config, *configload.Snapshot) {
	t.Helper()

	dir := filepath.Join(fixtureDir, name)
	// FIXME: We're not dealing with the cleanup function here because
	// this testModule function is used all over and so we don't want to
	// change its interface at this late stage.
	loader, _ := configload.NewLoaderForTests(t)

	// We need to be able to exercise experimental features in our integration tests.
	loader.AllowLanguageExperiments(true)

	// Test modules usually do not refer to remote sources, and for local
	// sources only this ultimately just records all of the module paths
	// in a JSON file so that we can load them below.
	inst := initwd.NewModuleInstaller(loader.ModulesDir(), loader, registry.NewClient(nil, nil))
	_, instDiags := inst.InstallModules(context.Background(), dir, "tests", true, false, initwd.ModuleInstallHooksImpl{}, configs.RootModuleCallForTesting())
	if instDiags.HasErrors() {
		t.Fatal(instDiags.Err())
	}

	// Since module installer has modified the module manifest on disk, we need
	// to refresh the cache of it in the loader.
	if err := loader.RefreshModules(); err != nil {
		t.Fatalf("failed to refresh modules after installation: %s", err)
	}

	config, snap, diags := loader.LoadConfigWithSnapshot(dir, configs.RootModuleCallForTesting())
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	return config, snap
}

// testModuleInline takes a map of path -> config strings and yields a config
// structure with those files loaded from disk
func testModuleInline(t testing.TB, sources map[string]string) *configs.Config {
	t.Helper()

	cfgPath := t.TempDir()

	for path, configStr := range sources {
		dir := filepath.Dir(path)
		if dir != "." {
			err := os.MkdirAll(filepath.Join(cfgPath, dir), os.FileMode(0777))
			if err != nil {
				t.Fatalf("Error creating subdir: %s", err)
			}
		}
		// Write the configuration
		cfgF, err := os.Create(filepath.Join(cfgPath, path))
		if err != nil {
			t.Fatalf("Error creating temporary file for config: %s", err)
		}

		_, err = io.Copy(cfgF, strings.NewReader(configStr))
		cfgF.Close()
		if err != nil {
			t.Fatalf("Error creating temporary file for config: %s", err)
		}
	}

	loader, cleanup := configload.NewLoaderForTests(t)
	defer cleanup()

	// We need to be able to exercise experimental features in our integration tests.
	loader.AllowLanguageExperiments(true)

	// Test modules usually do not refer to remote sources, and for local
	// sources only this ultimately just records all of the module paths
	// in a JSON file so that we can load them below.
	inst := initwd.NewModuleInstaller(loader.ModulesDir(), loader, registry.NewClient(nil, nil))
	_, instDiags := inst.InstallModules(context.Background(), cfgPath, "tests", true, false, initwd.ModuleInstallHooksImpl{}, configs.RootModuleCallForTesting())
	if instDiags.HasErrors() {
		t.Fatal(instDiags.Err())
	}

	// Since module installer has modified the module manifest on disk, we need
	// to refresh the cache of it in the loader.
	if err := loader.RefreshModules(); err != nil {
		t.Fatalf("failed to refresh modules after installation: %s", err)
	}

	config, diags := loader.LoadConfigWithTests(cfgPath, "tests", configs.RootModuleCallForTesting())
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	return config
}

// testSetResourceInstanceCurrent is a helper function for tests that sets a Current,
// Ready resource instance for the given module.
func testSetResourceInstanceCurrent(module *states.Module, resource, attrsJson, provider string) {
	module.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(resource).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(attrsJson),
		},
		mustProviderConfig(provider),
		addrs.NoKey,
	)
}

// testSetResourceInstanceTainted is a helper function for tests that sets a Current,
// Tainted resource instance for the given module.
func testSetResourceInstanceTainted(module *states.Module, resource, attrsJson, provider string) {
	module.SetResourceInstanceCurrent(
		mustResourceInstanceAddr(resource).Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectTainted,
			AttrsJSON: []byte(attrsJson),
		},
		mustProviderConfig(provider),
		addrs.NoKey,
	)
}

func testProviderFuncFixed(rp providers.Interface) providers.Factory {
	if p, ok := rp.(*MockProvider); ok {
		// make sure none of the methods were "called" on this new instance
		p.GetProviderSchemaCalled = false
		p.ValidateProviderConfigCalled = false
		p.ValidateResourceConfigCalled = false
		p.ValidateDataResourceConfigCalled = false
		p.UpgradeResourceStateCalled = false
		p.ConfigureProviderCalled = false
		p.StopCalled = false
		p.ReadResourceCalled = false
		p.PlanResourceChangeCalled = false
		p.ApplyResourceChangeCalled = false
		p.ImportResourceStateCalled = false
		p.ReadDataSourceCalled = false
		p.CloseCalled = false
	}

	return func() (providers.Interface, error) {
		return rp, nil
	}
}

func testProvisionerFuncFixed(rp *MockProvisioner) provisioners.Factory {
	// make sure this provisioner has has not been closed
	rp.CloseCalled = false

	return func() (provisioners.Interface, error) {
		return rp, nil
	}
}

func mustResourceInstanceAddr(s string) addrs.AbsResourceInstance {
	addr, diags := addrs.ParseAbsResourceInstanceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr
}

func mustConfigResourceAddr(s string) addrs.ConfigResource {
	addr, diags := addrs.ParseAbsResourceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr.Config()
}

func mustAbsResourceAddr(s string) addrs.AbsResource {
	addr, diags := addrs.ParseAbsResourceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr
}

func mustProviderConfig(s string) addrs.AbsProviderConfig {
	p, diags := addrs.ParseAbsProviderConfigStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return p
}

func mustReference(s string) *addrs.Reference {
	p, diags := addrs.ParseRefStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return p
}

// HookRecordApplyOrder is a test hook that records the order of applies
// by recording the PreApply event.
type HookRecordApplyOrder struct {
	NilHook

	Active bool

	l      sync.Mutex
	IDs    []string
	States []cty.Value
	Diffs  []*plans.Change
}

func (h *HookRecordApplyOrder) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (HookAction, error) {
	if plannedNewState.RawEquals(priorState) {
		return HookActionContinue, nil
	}

	if h.Active {
		h.l.Lock()
		defer h.l.Unlock()

		h.IDs = append(h.IDs, addr.String())
		h.Diffs = append(h.Diffs, &plans.Change{
			Action: action,
			Before: priorState,
			After:  plannedNewState,
		})
		h.States = append(h.States, priorState)
	}

	return HookActionContinue, nil
}

// Below are all the constant strings that are the expected output for
// various tests.

const testTofuInputProviderOnlyStr = `
aws_instance.foo:
  ID = 
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = us-west-2
  type = 
`

const testTofuApplyStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyDataBasicStr = `
data.null_data_source.testing:
  ID = yo
  provider = provider["registry.opentofu.org/hashicorp/null"]
`

const testTofuApplyRefCountStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = 3
  type = aws_instance

  Dependencies:
    aws_instance.foo
aws_instance.foo.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
`

const testTofuApplyProviderAliasStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"].bar
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyProviderAliasConfigStr = `
another_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/another"].two
  type = another_instance
another_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/another"]
  type = another_instance
`

const testTofuApplyEmptyModuleStr = `
<no state>
Outputs:

end = XXXX
`

const testTofuApplyDependsCreateBeforeStr = `
aws_instance.lb:
  ID = baz
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  instance = foo
  type = aws_instance

  Dependencies:
    aws_instance.web
aws_instance.web:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  require_new = ami-new
  type = aws_instance
`

const testTofuApplyCreateBeforeStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  require_new = xyz
  type = aws_instance
`

const testTofuApplyCreateBeforeUpdateStr = `
aws_instance.bar:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = baz
  type = aws_instance
`

const testTofuApplyCancelStr = `
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
  value = 2
`

const testTofuApplyComputeStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = computed_value
  type = aws_instance

  Dependencies:
    aws_instance.foo
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  compute = value
  compute_value = 1
  num = 2
  type = aws_instance
  value = computed_value
`

const testTofuApplyCountDecStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo.0:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
aws_instance.foo.1:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
`

const testTofuApplyCountDecToOneStr = `
aws_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
`

const testTofuApplyCountDecToOneCorruptedStr = `
aws_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
`

const testTofuApplyCountDecToOneCorruptedPlanStr = `
DIFF:

DESTROY: aws_instance.foo[0]
  id:   "baz" => ""
  type: "aws_instance" => ""



STATE:

aws_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
aws_instance.foo.0:
  ID = baz
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
`

const testTofuApplyCountVariableStr = `
aws_instance.foo.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
aws_instance.foo.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
`

const testTofuApplyCountVariableRefStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = 2
  type = aws_instance

  Dependencies:
    aws_instance.foo
aws_instance.foo.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
`
const testTofuApplyForEachVariableStr = `
aws_instance.foo["b15c6d616d6143248c575900dff57325eb1de498"]:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
aws_instance.foo["c3de47d34b0a9f13918dd705c141d579dd6555fd"]:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
aws_instance.foo["e30a7edcc42a846684f2a4eea5f3cd261d33c46d"]:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  type = aws_instance
aws_instance.one["a"]:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.one["b"]:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.two["a"]:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance

  Dependencies:
    aws_instance.one
aws_instance.two["b"]:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance

  Dependencies:
    aws_instance.one`
const testTofuApplyMinimalStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
`

const testTofuApplyModuleStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance

module.child:
  aws_instance.baz:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    foo = bar
    type = aws_instance
`

const testTofuApplyModuleBoolStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = true
  type = aws_instance
`

const testTofuApplyModuleDestroyOrderStr = `
<no state>
`

const testTofuApplyMultiProviderStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
do_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/do"]
  num = 2
  type = do_instance
`

const testTofuApplyModuleOnlyProviderStr = `
<no state>
module.child:
  aws_instance.foo:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
  test_instance.foo:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/test"]
    type = test_instance
`

const testTofuApplyModuleProviderAliasStr = `
<no state>
module.child:
  aws_instance.foo:
    ID = foo
    provider = module.child.provider["registry.opentofu.org/hashicorp/aws"].eu
    type = aws_instance
`

const testTofuApplyModuleVarRefExistingStr = `
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance

module.child:
  aws_instance.foo:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
    value = bar

    Dependencies:
      aws_instance.foo
`

const testTofuApplyOutputOrphanStr = `
<no state>
Outputs:

foo = bar
`

const testTofuApplyOutputOrphanModuleStr = `
<no state>
`

const testTofuApplyProvisionerStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance

  Dependencies:
    aws_instance.foo
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  compute = value
  compute_value = 1
  num = 2
  type = aws_instance
  value = computed_value
`

const testTofuApplyProvisionerModuleStr = `
<no state>
module.child:
  aws_instance.bar:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    type = aws_instance
`

const testTofuApplyProvisionerFailStr = `
aws_instance.bar: (tainted)
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyProvisionerFailCreateStr = `
aws_instance.bar: (tainted)
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
`

const testTofuApplyProvisionerFailCreateNoIdStr = `
<no state>
`

const testTofuApplyProvisionerFailCreateBeforeDestroyStr = `
aws_instance.bar: (tainted) (1 deposed)
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  require_new = xyz
  type = aws_instance
  Deposed ID 1 = bar
`

const testTofuApplyProvisionerResourceRefStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyProvisionerSelfRefStr = `
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
`

const testTofuApplyProvisionerMultiSelfRefStr = `
aws_instance.foo.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = number 0
  type = aws_instance
aws_instance.foo.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = number 1
  type = aws_instance
aws_instance.foo.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = number 2
  type = aws_instance
`

const testTofuApplyProvisionerMultiSelfRefSingleStr = `
aws_instance.foo.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = number 0
  type = aws_instance
aws_instance.foo.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = number 1
  type = aws_instance
aws_instance.foo.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = number 2
  type = aws_instance
`

const testTofuApplyProvisionerDiffStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
`

const testTofuApplyProvisionerSensitiveStr = `
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
`

const testTofuApplyDestroyStr = `
<no state>
`

const testTofuApplyErrorStr = `
aws_instance.bar: (tainted)
  ID = 
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = 2

  Dependencies:
    aws_instance.foo
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
  value = 2
`

const testTofuApplyErrorCreateBeforeDestroyStr = `
aws_instance.bar:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  require_new = abc
  type = aws_instance
`

const testTofuApplyErrorDestroyCreateBeforeDestroyStr = `
aws_instance.bar: (1 deposed)
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  require_new = xyz
  type = aws_instance
  Deposed ID 1 = bar
`

const testTofuApplyErrorPartialStr = `
aws_instance.bar:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance

  Dependencies:
    aws_instance.foo
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  type = aws_instance
  value = 2
`

const testTofuApplyResourceDependsOnModuleStr = `
aws_instance.a:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  ami = parent
  type = aws_instance

  Dependencies:
    module.child.aws_instance.child

module.child:
  aws_instance.child:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    ami = child
    type = aws_instance
`

const testTofuApplyResourceDependsOnModuleDeepStr = `
aws_instance.a:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  ami = parent
  type = aws_instance

  Dependencies:
    module.child.module.grandchild.aws_instance.c

module.child.grandchild:
  aws_instance.c:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    ami = grandchild
    type = aws_instance
`

const testTofuApplyResourceDependsOnModuleInModuleStr = `
<no state>
module.child:
  aws_instance.b:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    ami = child
    type = aws_instance

    Dependencies:
      module.child.module.grandchild.aws_instance.c
module.child.grandchild:
  aws_instance.c:
    ID = foo
    provider = provider["registry.opentofu.org/hashicorp/aws"]
    ami = grandchild
    type = aws_instance
`

const testTofuApplyTaintStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyTaintDepStr = `
aws_instance.bar:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  num = 2
  type = aws_instance

  Dependencies:
    aws_instance.foo
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyTaintDepRequireNewStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo
  require_new = yes
  type = aws_instance

  Dependencies:
    aws_instance.foo
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyOutputStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance

Outputs:

foo_num = 2
`

const testTofuApplyOutputAddStr = `
aws_instance.test.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo0
  type = aws_instance
aws_instance.test.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = foo1
  type = aws_instance

Outputs:

firstOutput = foo0
secondOutput = foo1
`

const testTofuApplyOutputListStr = `
aws_instance.bar.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.bar.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.bar.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance

Outputs:

foo_num = [bar,bar,bar]
`

const testTofuApplyOutputMultiStr = `
aws_instance.bar.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.bar.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.bar.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance

Outputs:

foo_num = bar,bar,bar
`

const testTofuApplyOutputMultiIndexStr = `
aws_instance.bar.0:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.bar.1:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.bar.2:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar
  type = aws_instance
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance

Outputs:

foo_num = bar
`

const testTofuApplyUnknownAttrStr = `
aws_instance.foo: (tainted)
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  num = 2
  type = aws_instance
`

const testTofuApplyVarsStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  bar = override
  baz = override
  foo = us-east-1
aws_instance.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  bar = baz
  list.# = 2
  list.0 = Hello
  list.1 = World
  map.Baz = Foo
  map.Foo = Bar
  map.Hello = World
  num = 2
`

const testTofuApplyVarsEnvStr = `
aws_instance.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  list.# = 2
  list.0 = Hello
  list.1 = World
  map.Baz = Foo
  map.Foo = Bar
  map.Hello = World
  string = baz
  type = aws_instance
`

const testTofuRefreshDataRefDataStr = `
data.null_data_source.bar:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/null"]
  bar = yes
data.null_data_source.foo:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/null"]
  foo = yes
`
