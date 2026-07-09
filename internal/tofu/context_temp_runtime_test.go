// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// maybeRunExperimentalNewRuntimeTests is called by this package's [TestMain]
// to give us an opportunity to handle testing differently if it seems like
// someone is trying to apply this package's tests against the new experimental
// language runtime through our shim layer.
//
// To use this, run something like the following command:
//
//	TOFU_X_EXPERIMENTAL_RUNTIME=1 go test ./internal/tofu
func maybeRunExperimentalNewRuntimeTests(m *testing.M) {
	if !experimentalRuntimeWanted() {
		return // we'll just let the normal TestMain function handle things, then
	}

	// In normal builds we require setting a special linker flag to permit
	// opting in to the experimental runtime shim, but for tests it's always
	// allowed and just enabled on request.
	SetExperimentalRuntimeAllowed(true)

	status := m.Run()

	// Before we exit we'll print some stats about how many times each of
	// the experimental flags caused us to skip running some part of a test.
	if status == 0 && testing.Verbose() && len(experimentalFlagSkipCounts) != 0 {
		// (We use stderr here just to avoid interfering with any
		// machine-readable output on stdout if we happen to be running tests
		// in JSON mode, or similar.)
		fmt.Fprintf(os.Stderr, "\n---- New runtime test-skip summary ----\n\n")
		fmt.Fprintf(os.Stderr, "-> %d tests were skipped due to one or more of the following...\n\n", experimentalFlagSkippedTests)
		type Record struct {
			name  string
			count int
		}
		var records []Record
		var nameLen int
		for name, count := range experimentalFlagSkipCounts {
			records = append(records, Record{name, count})
			if len(name) > nameLen {
				nameLen = len(name)
			}
		}
		slices.SortFunc(records, func(a, b Record) int {
			return cmp.Compare(b.count, a.count) // reverse sort, greatest count first
		})
		for _, record := range records {
			fmt.Fprintf(os.Stderr, "  %[3]*[1]s: %[2]d\n", record.name, record.count, nameLen)
		}
		fmt.Fprintln(os.Stderr, "")
	}

	// We'll exit here now, to avoid [TestMain] running the test suite over
	// again once we return.
	os.Exit(status)
}

type ExperimentalFlag struct {
	name    string
	enabled bool
}

var experimentalFlagSkipCounts map[string]int
var experimentalFlagSkippedTests int
var experimentalFlagSkipMu sync.Mutex

var (
	ExperimentalFlagUnknown = ExperimentalFlag{"Unknown", false}

	ExperimentalBugExecGraph         = ExperimentalFlag{"Bug in generated Exec Graph", true}
	ExperimentalBugDeclareProvider   = ExperimentalFlag{"Bug Declare Provider", true}
	ExperimentalBugVariableInput     = ExperimentalFlag{"Bug Variable Input", true}
	ExperimentalBugCancel            = ExperimentalFlag{"Bug Context Cancel", false}
	ExperimentalBugStateProvider     = ExperimentalFlag{"Bug State Provider", true}
	ExperimentalBugStateCBD          = ExperimentalFlag{"Bug CreateBeforeDestroy Not Tracked In State", false}
	ExperimentalBugStateUpdateHook   = ExperimentalFlag{"Bug State Update Hook", false}
	ExperimentalBugReferenceProvider = ExperimentalFlag{"Bug Reference Provider", true}
	ExperimentalBugMissingProvider   = ExperimentalFlag{"Bug Missing Configuration For Provider", true}
	ExperimentalBugMissingResource   = ExperimentalFlag{"Bug Missing Configuration For Resource Instance", false}
	ExperimentalBugResourceReadNull  = ExperimentalFlag{"Bug Read Resource Deleted", false}
	ExperimentalBugDataResource      = ExperimentalFlag{"Bug Data Resource", false}
	ExperimentalBugVariableSensitive = ExperimentalFlag{"Bug Variables Declared as Sensitive", true}
	ExperimentalBugResourceMarks     = ExperimentalFlag{"Bug Not Transferring Marks from Resource Instance Config Value to Final Value", false}
	ExperimentalBugTaintOnCreateFail = ExperimentalFlag{"Bug Not Tainted When Create Fails", false}
	ExperimentalBugForEach           = ExperimentalFlag{"Bug For Each", true}
	ExperimentalBugSpuriousReplace   = ExperimentalFlag{"Bug Spurious Replace", true} // New runtime proposes replace where old runtime would've called for update
	ExperimentalBugProviderPrivate   = ExperimentalFlag{"Bug Provider Private Data Not Preserved", false}
	ExperimentalBugCircularReference = ExperimentalFlag{"Bug Circular Reference", false}

	ExperimentalChangeDiagWording     = ExperimentalFlag{"Change Different Diagnostic Wording", false}
	ExperimentalChangeErrorEarly      = ExperimentalFlag{"Change Detect Error Earlier", false}
	ExperimentalChangeDependencies    = ExperimentalFlag{"Change Precise Dependencies", false}
	ExperimentalChangeDeferredActions = ExperimentalFlag{"Change New Runtime Supports Deferred Actions", false}
	ExperimentalChangeNoNoOp          = ExperimentalFlag{"Change New Runtime Doesn't Generate NoOp Changes", false}
	ExperimentalChangePreReqdProvider = ExperimentalFlag{"Change New Runtime Doesn't Inherit Full Pre \"required_providers\" Behavior", false}
	ExperimentalChangeDestroyOrder    = ExperimentalFlag{"Change Destroy Order", false}
	ExperimentalChangeModuleOutput    = ExperimentalFlag{"Change Module Outputs (test only)", false}
	ExperimentalChangeCursedSelfRef   = ExperimentalFlag{"Change Cursed Self Reference (legacy race condition)", false}

	ExperimentalFeatureStateDependencies = ExperimentalFlag{"Missing State Dependencies", true}
	ExperimentalFeatureProviderInstances = ExperimentalFlag{"Missing Provider Instances", true}
	ExperimentalFeatureCBD               = ExperimentalFlag{"Missing Create Before Destroy", true}
	ExperimentalFeatureDeposed           = ExperimentalFlag{"Missing Deposed", false}
	ExperimentalFeatureCondition         = ExperimentalFlag{"Missing Pre/Post Conditions", false}
	ExperimentalFeatureLocalState        = ExperimentalFlag{"Missing Store locals in state", false}
	ExperimentalFeatureChecks            = ExperimentalFlag{"Missing Checks", false}
	ExperimentalFeatureChanges           = ExperimentalFlag{"Missing Plan Changes", false}
	ExperimentalFeatureDeprecated        = ExperimentalFlag{"Missing Deprecated", false}
	ExperimentalFeatureImport            = ExperimentalFlag{"Missing Importing", false}
	ExperimentalFeatureRefresh           = ExperimentalFlag{"Missing Refresh", false}
	ExperimentalFeatureRefreshOnly       = ExperimentalFlag{"Missing Refresh-only Planning Mode", false}
	ExperimentalFeatureValidate          = ExperimentalFlag{"Missing Validate", false}
	ExperimentalFeatureDestroy           = ExperimentalFlag{"Missing Destroy-mode Planning", true}
	ExperimentalFeatureMoved             = ExperimentalFlag{"Missing Moved", false}
	ExperimentalFeatureRemoved           = ExperimentalFlag{"Missing Removed", false}
	ExperimentalFeatureSkipDestroy       = ExperimentalFlag{"Missing Lifecycle Destroy", false}
	ExperimentalFeatureUpgradeState      = ExperimentalFlag{"Missing Upgrade Resource State", true}
	ExperimentalFeatureUpgradeUnwanted   = ExperimentalFlag{"Missing Upgrade Orphan or Deposed Resource Instance State", false}
	ExperimentalFeatureHooks             = ExperimentalFlag{"Missing Hooks", true}
	ExperimentalFeatureTarget            = ExperimentalFlag{"Missing Targeting", false}
	ExperimentalFeatureReplaceTB         = ExperimentalFlag{"Missing replace_triggered_by", true}
	ExperimentalFeatureProvisioner       = ExperimentalFlag{"Missing Provisioners", true}
	ExperimentalFeatureDependsOn         = ExperimentalFlag{"Missing Depends On", true}
	ExperimentalFeatureIgnoreChanges     = ExperimentalFlag{"Missing Ignore Changes", true}
	ExperimentalFeatureVarCondition      = ExperimentalFlag{"Missing Variable Condiitions", false}
	ExperimentalFeaturePathAttrs         = ExperimentalFlag{"Missing Path/Terraform/Tofu Attrs", true}
	ExperimentalFeaturePreventDestroy    = ExperimentalFlag{"Missing Prevent Destroy", false}
	ExperimentalFeaturePlannedState      = ExperimentalFlag{"Missing Planned State", false}
	ExperimentalFeatureForceReplace      = ExperimentalFlag{"Missing Force Replace", false}
	ExperimentalFeatureRootOutput        = ExperimentalFlag{"Missing Root Output", true}
	ExperimentalFeatureSensitivity       = ExperimentalFlag{"Missing Sensitivity Handling", false}
	ExperimentalFeatureSelfReference     = ExperimentalFlag{"Missing Self Reference", false}
	ExperimentalFeatureProviderMeta      = ExperimentalFlag{"Missing Provider Meta", false}
	ExperimentalFeatureTaint             = ExperimentalFlag{"Missing Taint", false}
	ExperimentalFeatureErrorHandling     = ExperimentalFlag{"Missing Error Handling", false}
	ExperimentalFeatureProviderFunctions = ExperimentalFlag{"Missing Provider Defined Functions", true}
	ExperimentalFeatureProviderInput     = ExperimentalFlag{"Missing Provider Input Prompting", false}
	ExperimentalFeatureModuleEnabled     = ExperimentalFlag{"Missing Module Lifecycle Enabled", false}

	// Obsolete flags indicate a test which depends on a feature we do not
	// intend to carry forward into the new engine
	ExperimentalObsoleteFlatAttrs        = ExperimentalFlag{"Obsolete Flat Mapped Attributes", false}
	ExperimentalObsoleteDestroyData      = ExperimentalFlag{"Obsolete Explicit Destroy of Data Resource Instances", false}
	ExperimentalObsoleteEphemeralInState = ExperimentalFlag{"Obsolete Ephemeral Resource Instances in State", false}

	// ExperimentalNewStrategyNeeded is a special experimental flag that
	// represents that a test is failing not because the underlying behavior
	// is wrong but because the test was relying on poking around in the
	// internals of the old runtime to produce a synthetic result and that
	// poking is ineffective with the new runtime. If you use this one,
	// include a comment above the [SkipExperimental] call explaining what
	// aspect of the testing strategy is flawed in the new implementation.
	ExperimentalNewStrategyNeeded = ExperimentalFlag{"New testing strategy needed", false}
)

func SkipExperimental(t *testing.T, features ...ExperimentalFlag) {
	if experimentalRuntimeEnabled() {
		var strs []string
		experimentalFlagSkipMu.Lock()
		defer experimentalFlagSkipMu.Unlock()
		if experimentalFlagSkipCounts == nil {
			experimentalFlagSkipCounts = make(map[string]int)
		}
		for _, feature := range features {
			if feature.enabled {
				continue
			}
			strs = append(strs, feature.name)
			experimentalFlagSkipCounts[feature.name]++
		}
		if len(strs) > 0 {
			experimentalFlagSkippedTests++
			t.Skip("New Engine: " + strings.Join(strs, ", "))
		}
	}
}
