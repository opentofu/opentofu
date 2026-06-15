// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"
)

func init() {
	// Allow running experimental engine tests with:
	// TOFU_X_EXPERIMENTAL_RUNTIME=1 go test ./internal/tofu
	SetExperimentalRuntimeAllowed(true)
}

type ExperimentalFlag struct {
	name    string
	enabled bool
}

var (
	ExperimentalFlagUnknown = ExperimentalFlag{"Unknown", false}

	ExperimentalBugExecGraph         = ExperimentalFlag{"Bug in generated Exec Graph", true}
	ExperimentalBugDeclareProvider   = ExperimentalFlag{"Bug Declare Provider", true}
	ExperimentalBugVariableInput     = ExperimentalFlag{"Bug Variable Input", true}
	ExperimentalBugCancel            = ExperimentalFlag{"Bug Context Cancel", false}
	ExperimentalBugStateProvider     = ExperimentalFlag{"Bug State Provider", false}
	ExperimentalBugReferenceProvider = ExperimentalFlag{"Bug Reference Provider", false}
	ExperimentalBugMissingProvider   = ExperimentalFlag{"Bug Missing Configuration For Provider", false}
	ExperimentalBugResourceReadNull  = ExperimentalFlag{"Bug Read Resource Deleted", false}
	ExperimentalBugDataResource      = ExperimentalFlag{"Bug Data Resource", false}
	ExperimentalBugVariableSensitive = ExperimentalFlag{"Bug Variables Declared as Sensitive", true}
	ExperimentalBugResourceMarks     = ExperimentalFlag{"Bug Not Transferring Marks from Resource Instance Config Value to Final Value", false}
	ExperimentalBugForEach           = ExperimentalFlag{"Bug For Each", true}
	ExperimentalBugSpuriousReplace   = ExperimentalFlag{"Bug Spurious Replace", false} // New runtime proposes replace where old runtime would've called for update

	ExperimentalChangeDiagWording     = ExperimentalFlag{"Change Different Diagnostic Wording", false}
	ExperimentalChangeErrorEarly      = ExperimentalFlag{"Change Detect Error Earlier", false}
	ExperimentalChangeDependencies    = ExperimentalFlag{"Change Precise Dependencies", false}
	ExperimentalChangeDeferredActions = ExperimentalFlag{"Change New Runtime Supports Deferred Actions", false}

	ExperimentalFeatureStateDependencies = ExperimentalFlag{"Missing State Dependencies", true}
	ExperimentalFeatureProviderInstances = ExperimentalFlag{"Missing Provider Instances", true}
	ExperimentalFeatureCBD               = ExperimentalFlag{"Missing Create Before Destroy", false}
	ExperimentalFeatureDeposed           = ExperimentalFlag{"Missing Deposed", false}
	ExperimentalFeatureCondition         = ExperimentalFlag{"Missing Pre/Post Conditions", false}
	ExperimentalFeatureLocalState        = ExperimentalFlag{"Missing Store locals in state", false}
	ExperimentalFeatureChecks            = ExperimentalFlag{"Missing Checks", false}
	ExperimentalFeatureChanges           = ExperimentalFlag{"Missing Plan Changes", false}
	ExperimentalFeatureDeprecated        = ExperimentalFlag{"Missing Deprecated", false}
	ExperimentalFeatureImport            = ExperimentalFlag{"Missing Importing", false}
	ExperimentalFeatureRefresh           = ExperimentalFlag{"Missing Refresh", false}
	ExperimentalFeatureValidate          = ExperimentalFlag{"Missing Validate", false}
	ExperimentalFeatureDestroy           = ExperimentalFlag{"Missing Destroy", false}
	ExperimentalFeatureMoved             = ExperimentalFlag{"Missing Moved", false}
	ExperimentalFeatureRemoved           = ExperimentalFlag{"Missing Removed", false}
	ExperimentalFeatureSkipDestroy       = ExperimentalFlag{"Missing Lifecycle Destroy", false}
	ExperimentalFeatureUpgradeState      = ExperimentalFlag{"Missing Upgrade Resource State", true}
	ExperimentalFeatureHooks             = ExperimentalFlag{"Missing Hooks", false}
	ExperimentalFeatureTarget            = ExperimentalFlag{"Missing Targeting", false}
	ExperimentalFeatureReplaceTB         = ExperimentalFlag{"Missing replace_triggered_by", false}
	ExperimentalFeatureProvisioner       = ExperimentalFlag{"Missing Provisioners", false}
	ExperimentalFeatureDependsOn         = ExperimentalFlag{"Missing Depends On", false}
	ExperimentalFeatureIgnoreChanges     = ExperimentalFlag{"Missing Ignore Changes", false}
	ExperimentalFeatureVarCondition      = ExperimentalFlag{"Missing Variable Condiitions", false}
	ExperimentalFeaturePathAttrs         = ExperimentalFlag{"Missing Path/Terraform/Tofu Attrs", false}
	ExperimentalFeaturePreventDestroy    = ExperimentalFlag{"Missing Prevent Destroy", false}
	ExperimentalFeaturePlannedState      = ExperimentalFlag{"Missing Planned State", false}
	ExperimentalFeatureForceReplace      = ExperimentalFlag{"Missing Force Replace", false}
	ExperimentalFeatureRootOutput        = ExperimentalFlag{"Missing Root Output", false}
	ExperimentalFeatureSensitivity       = ExperimentalFlag{"Missing Sensitivity Handling", false}
	ExperimentalFeatureSelfReference     = ExperimentalFlag{"Missing Self Reference", false}
	ExperimentalFeatureProviderMeta      = ExperimentalFlag{"Missing Provider Meta", false}
	ExperimentalFeatureTaint             = ExperimentalFlag{"Missing Taint", false}
	ExperimentalFeatureErrorHandling     = ExperimentalFlag{"Missing Error Handling", false}
	ExperimentalFeatureProviderFunctions = ExperimentalFlag{"Missing Provider Defined Functions", true}

	// Obsolete flags indicate a test which depends on a feature we do not
	// intend to carry forward into the new engine
	ExperimentalObsoleteFlatAttrs = ExperimentalFlag{"Obsolete Flat Mapped Attributes", false}

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
		for _, feature := range features {
			if feature.enabled {
				continue
			}
			strs = append(strs, feature.name)
		}
		if len(strs) > 0 {
			t.Skip("New Engine: " + strings.Join(strs, ", "))
		}
	}
}
