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

type ExperimentalFlag string

const (
	ExperimentalFlagUnknown ExperimentalFlag = "Unknown"

	ExperimentalBugCancel            ExperimentalFlag = "Bug Context Cancel"
	ExperimentalBugStateProvider     ExperimentalFlag = "Bug State Provider"
	ExperimentalBugReferenceProvider ExperimentalFlag = "Bug Reference Provider"
	ExperimentalBugResourceReadNull  ExperimentalFlag = "Bug Read Resource Deleted"
	ExperimentalBugDataResource      ExperimentalFlag = "Bug Data Resource"
	ExperimentalBugVariableSensitive ExperimentalFlag = "Bug Variables Declared as Sensitive"
	ExperimentalBugForEach           ExperimentalFlag = "Bug For Each"         // TODO run existing evalchecks tests against new engine
	ExperimentalBugSpuriousReplace   ExperimentalFlag = "Bug Spurious Replace" // New runtime proposes replace where old runtime would've called for update

	ExperimentalChangeDiagWording  ExperimentalFlag = "Change Different Diagnostic Wording"
	ExperimentalChangeErrorEarly   ExperimentalFlag = "Change Detect Error Earlier"
	ExperimentalChangeDependencies ExperimentalFlag = "Change Precise Dependencies"

	ExperimentalFeatureCBD               ExperimentalFlag = "Missing Create Before Destroy"
	ExperimentalFeatureDeposed           ExperimentalFlag = "Missing Deposed"
	ExperimentalFeatureCondition         ExperimentalFlag = "Missing Pre/Post Conditions"
	ExperimentalFeatureLocalState        ExperimentalFlag = "Missing Store locals in state"
	ExperimentalFeatureChecks            ExperimentalFlag = "Missing Checks"
	ExperimentalFeatureChanges           ExperimentalFlag = "Missing Plan Changes"
	ExperimentalFeatureDeprecated        ExperimentalFlag = "Missing Deprecated"
	ExperimentalFeatureImport            ExperimentalFlag = "Missing Importing"
	ExperimentalFeatureRefresh           ExperimentalFlag = "Missing Refresh"
	ExperimentalFeatureValidate          ExperimentalFlag = "Missing Validate"
	ExperimentalFeatureDestroy           ExperimentalFlag = "Missing Destroy"
	ExperimentalFeatureMoved             ExperimentalFlag = "Missing Moved"
	ExperimentalFeatureRemoved           ExperimentalFlag = "Missing Removed"
	ExperimentalFeatureSkipDestroy       ExperimentalFlag = "Missing Lifecycle Destroy"
	ExperimentalFeatureUpgradeState      ExperimentalFlag = "Missing Upgrade Resource State"
	ExperimentalFeatureHooks             ExperimentalFlag = "Missing Hooks"
	ExperimentalFeatureTarget            ExperimentalFlag = "Missing Targeting"
	ExperimentalFeatureReplaceTB         ExperimentalFlag = "Missing replace_triggered_by"
	ExperimentalFeatureProvisioner       ExperimentalFlag = "Missing Provisioners"
	ExperimentalFeatureDependsOn         ExperimentalFlag = "Missing Depends On"
	ExperimentalFeatureIgnoreChanges     ExperimentalFlag = "Missing Ignore Changes"
	ExperimentalFeatureVarCondition      ExperimentalFlag = "Missing Variable Condiitions"
	ExperimentalFeaturePathAttrs         ExperimentalFlag = "Missing Path/Terraform/Tofu Attrs"
	ExperimentalFeaturePreventDestroy    ExperimentalFlag = "Missing Prevent Destroy"
	ExperimentalFeaturePlannedState      ExperimentalFlag = "Missing Planned State"
	ExperimentalFeatureForceReplace      ExperimentalFlag = "Missing Force Replace"
	ExperimentalFeatureRootOutput        ExperimentalFlag = "Missing Root Output"
	ExperimentalFeatureSensitivity       ExperimentalFlag = "Missing Sensitivity Handling"
	ExperimentalFeatureSelfReference     ExperimentalFlag = "Missing Self Reference"
	ExperimentalFeatureProviderMeta      ExperimentalFlag = "Missing Provider Meta"
	ExperimentalFeatureTaint             ExperimentalFlag = "Missing Taint"
	ExperimentalFeatureErrorHandling     ExperimentalFlag = "Missing Error Handling"
	ExperimentalFeatureProviderFunctions ExperimentalFlag = "Missing Provider Defined Functions"

	// ExperimentalNewStrategyNeeded is a special experimental flag that
	// represents that a test is failing not because the underlying behavior
	// is wrong but because the test was relying on poking around in the
	// internals of the old runtime to produce a synthetic result and that
	// poking is ineffective with the new runtime. If you use this one,
	// include a comment above the [SkipExperimental] call explaining what
	// aspect of the testing strategy is flawed in the new implementation.
	ExperimentalNewStrategyNeeded ExperimentalFlag = "New testing strategy needed"

	// Fixed
	ExperimentalBugExecGraph       ExperimentalFlag = "Bug in generated Exec Graph"
	ExperimentalBugDeclareProvider ExperimentalFlag = "Bug Declare Provider"
	ExperimentalBugVariableInput   ExperimentalFlag = "Bug Variable Input"

	// Implemented
	ExperimentalFeatureStateDependencies ExperimentalFlag = "Missing State Dependencies"
	ExperimentalFeatureProviderInstances ExperimentalFlag = "Missing Provider Instances"
)

func SkipExperimental(t *testing.T, features ...ExperimentalFlag) {
	if experimentalRuntimeEnabled() {
		var strs []string
		for _, feature := range features {
			switch feature {
			case
				ExperimentalBugExecGraph,
				ExperimentalBugDeclareProvider,
				ExperimentalFeatureProviderInstances,
				ExperimentalFeatureStateDependencies,
				ExperimentalBugVariableInput:
				// These ones are expected to be fixed already, so we don't skip.
			default:
				strs = append(strs, string(feature))
			}
		}
		if len(strs) > 0 {
			t.Skip("New Engine: " + strings.Join(strs, ", "))
		}
	}
}
