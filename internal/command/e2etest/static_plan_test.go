// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

// This is an e2e test as it relies on very specific configuration
// within the meta object that is currently very hard to mock out.
func TestStaticPlanVariables(t *testing.T) {
	fixtures := []string{
		"static_plan_variables",
		"static_plan_typed_variables",
	}
	for _, fixture := range fixtures {
		t.Run(fmt.Sprintf("TestStaticPlanVariables/%s", fixture), func(t *testing.T) {
			fixturePath := filepath.Join("testdata", fixture)
			tf := e2e.NewBinary(t, tofuBin, fixturePath)

			run := func(args ...string) tofuResult {
				stdout, stderr, err := tf.Run(args...)
				return tofuResult{t, stdout, stderr, err}
			}

			statePath := "custom.tfstate"
			stateVar := "-var=state_path=" + statePath
			modVar := "-var=src=./mod"
			planfile := "static.plan"

			modErr := "module.mod.source depends on var.src which is not available"
			backendErr := "backend.local depends on var.state_path which is not available"

			// Init
			run("init").Failure().StderrContains(modErr)
			run("init", stateVar, modVar).Success()

			// Get
			run("get").Failure().StderrContains(modErr)
			run("get", stateVar, modVar).Success()

			// Validate
			run("validate").Failure().StderrContains(modErr)
			run("validate", stateVar, modVar).Success()

			// Providers
			run("providers").Failure().StderrContains(modErr)
			run("providers", stateVar, modVar).Success()
			run("providers", "lock").Failure().StderrContains(modErr)
			run("providers", "lock", stateVar, modVar).Success()
			run("providers", "mirror", "./tempproviders").Failure().StderrContains(modErr)
			run("providers", "mirror", stateVar, modVar, "./tempproviders").Failure().StderrContains("Could not scan the output directory to get package metadata for the JSON")
			run("providers", "schema", "-json").Failure().StderrContains(backendErr)
			run("providers", "schema", "-json", stateVar, modVar).Success()

			// Check console init (early exits due to stdin setup)
			run("console").Failure().StderrContains(backendErr)
			run("console", stateVar, modVar).Success()

			// Check graph (without plan)
			run("graph").Failure().StderrContains(backendErr)
			run("graph", stateVar, modVar).Success()

			// Plan with static variable
			run("plan", stateVar, modVar, "-out="+planfile).Success()

			// Show plan without static variable (embedded)
			run("show", planfile).Success()

			// Check graph (without plan)
			run("graph", "-plan="+planfile).Success()

			// Apply plan without static variable (embedded)
			run("apply", planfile).Success()

			// Show State
			run("show", statePath).Failure().StderrContains(modErr)
			run("show", stateVar, modVar, statePath).Success().Contains(`out = "placeholder"`)

			// Force Unlock
			run("force-unlock", "ident").Failure().StderrContains(backendErr)
			run("force-unlock", stateVar, modVar, "ident").Failure().StderrContains("Local state cannot be unlocked by another process")

			// Output values
			run("output").Failure().StderrContains(backendErr)
			run("output", stateVar, modVar).Success().Contains(`out = "placeholder"`)

			// Refresh
			run("refresh").Failure().StderrContains(backendErr)
			run("refresh", stateVar, modVar).Success().Contains("There are currently no remote objects tracked in the state")

			// Import
			run("import", "resource.addr", "id").Failure().StderrContains(modErr)
			run("import", stateVar, modVar, "resource.addr", "id").Failure().StderrContains("Before importing this resource, please create its configuration in the root module.")

			// Taint
			run("taint", "resource.addr").Failure().StderrContains(modErr)
			run("taint", stateVar, modVar, "resource.addr").Failure().StderrContains("There is no resource instance in the state with the address resource.addr.")
			run("untaint", "resource.addr").Failure().StderrContains(backendErr)
			run("untaint", stateVar, modVar, "resource.addr").Failure().StderrContains("There is no resource instance in the state with the address resource.addr.")

			// State
			run("state", "list").Failure().StderrContains(backendErr)
			run("state", "list", stateVar, modVar).Success()
			run("state", "mv", "foo.bar", "foo.baz").Failure().StderrContains(modErr)
			run("state", "mv", stateVar, modVar, "foo.bar", "foo.baz").Failure().StderrContains("Cannot move foo.bar: does not match anything in the current state.")
			run("state", "pull").Failure().StderrContains(modErr)
			run("state", "pull", stateVar, modVar).Success().Contains(`"outputs":{"out":{"value":"placeholder","type":"string"}}`)
			run("state", "push", statePath).Failure().StderrContains(modErr)
			run("state", "push", stateVar, modVar, statePath).Success()
			run("state", "replace-provider", "foo", "bar").Failure().StderrContains(modErr)
			run("state", "replace-provider", stateVar, modVar, "foo", "bar").Success().Contains("No matching resources found.")
			run("state", "rm", "foo.bar").Failure().StderrContains(modErr)
			run("state", "rm", stateVar, modVar, "foo.bar").Failure().StderrContains("No matching objects found.")
			run("state", "show", "out").Failure().StderrContains(backendErr)
			run("state", "show", stateVar, modVar, "invalid.resource").Failure().StderrContains("No instance found for the given address!")

			// Workspace
			run("workspace", "list").Failure().StderrContains(backendErr)
			run("workspace", "list", stateVar, modVar).Success().Contains(`default`)
			run("workspace", "new", "foo").Failure().StderrContains(backendErr)
			run("workspace", "new", stateVar, modVar, "foo").Success().Contains(`foo`)
			run("workspace", "select", "default").Failure().StderrContains(backendErr)
			run("workspace", "select", stateVar, modVar, "default").Success().Contains(`default`)
			run("workspace", "delete", "foo").Failure().StderrContains(backendErr)
			run("workspace", "delete", stateVar, modVar, "foo").Success().Contains(`foo`)

			// Test
			run("test").Failure().StderrContains(modErr)
			run("test", stateVar, modVar).Success().Contains(`Success!`)

			// Destroy
			run("destroy", "-auto-approve").Failure().StderrContains(backendErr)
			run("destroy", stateVar, modVar, "-auto-approve").Success().Contains("You can apply this plan to save these new output values")
		})
	}
}
