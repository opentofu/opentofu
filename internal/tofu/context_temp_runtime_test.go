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

	ExperimentalBugCancel           ExperimentalFlag = "Bug Context Cancel"
	ExperimentalBugStateProvider    ExperimentalFlag = "Bug State Provider"
	ExperimentalBugDeclareProvider  ExperimentalFlag = "Bug Declare Provider"
	ExperimentalBugResourceReadNull ExperimentalFlag = "Bug Read Resource Deleted"

	ExperimentalFeatureCBD          ExperimentalFlag = "Missing Create Before Destroy"
	ExperimentalFeatureDeposed      ExperimentalFlag = "Missing Deposed"
	ExperimentalFeatureCondition    ExperimentalFlag = "Missing Pre/Post Conditions"
	ExperimentalFeatureLocalState   ExperimentalFlag = "Missing Store locals in state"
	ExperimentalFeatureChecks       ExperimentalFlag = "Missing Checks"
	ExperimentalFeatureChanges      ExperimentalFlag = "Missing Plan Changes"
	ExperimentalFeatureDeprecated   ExperimentalFlag = "Missing Deprecated"
	ExperimentalFeatureImport       ExperimentalFlag = "Missing Importing"
	ExperimentalFeatureRefresh      ExperimentalFlag = "Missing Refresh"
	ExperimentalFeatureMoved        ExperimentalFlag = "Missing Moved"
	ExperimentalFeatureRemoved      ExperimentalFlag = "Missing Removed"
	ExperimentalFeatureSkipDestroy  ExperimentalFlag = "Missing Lifecycle Destroy"
	ExperimentalFeatureUpgradeState ExperimentalFlag = "Missing Upgrade Resource State"
	ExperimentalFeatureHooks        ExperimentalFlag = "Missing Hooks"
	ExperimentalFeatureTarget       ExperimentalFlag = "Missing Targeting"
)

func SkipExperimental(t *testing.T, features ...ExperimentalFlag) {
	if experimentalRuntimeEnabled() {
		var strs []string
		for _, feature := range features {
			strs = append(strs, string(feature))
		}
		t.Skip("New Engine: " + strings.Join(strs, ", "))
	}
}
