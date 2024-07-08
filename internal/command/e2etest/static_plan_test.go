package e2etest

import (
	"path/filepath"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

// This is an e2e test as it relies on very specific configuration
// within the meta object that is currently very hard to mock out.
func TestStaticPlanVariables(t *testing.T) {
	fixturePath := filepath.Join("testdata", "static_plan_variables")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	run := func(args ...string) tofuResult {
		stdout, stderr, err := tf.Run(args...)
		return tofuResult{t, stdout, stderr, err}
	}

	statePath := "custom.tfstate"
	stateVar := "-var=state_path=" + statePath
	modVar := "-var=src=./mod"
	planfile := "static.plan"

	// Init without static variable
	run("init").Failure()

	// Init with static variable
	run("init", stateVar, modVar).Success()

	// Plan with static variable
	run("plan", stateVar, modVar, "-out="+planfile).Success()

	// Show plan without static variable (embedded)
	run("show", planfile).Success()

	// Apply plan without static variable (embedded)
	run("apply", planfile).Success()
}
