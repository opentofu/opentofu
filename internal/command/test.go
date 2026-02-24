// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/lang"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/moduletest"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

const (
	MainStateIdentifier = ""
)

type TestCommand struct {
	Meta
}

func (c *TestCommand) Help() string {
	helpText := `
Usage: tofu [global options] test [options]

  Executes automated integration tests against the current OpenTofu 
  configuration.

  OpenTofu will search for .tftest.hcl files within the current configuration 
  and testing directories. OpenTofu will then execute the testing run blocks 
  within any testing files in order, and verify conditional checks and 
  assertions against the created infrastructure. 

  This command creates real infrastructure and will attempt to clean up the
  testing infrastructure on completion. Monitor the output carefully to ensure
  this cleanup process is successful.

Options:

  -compact-warnings     If OpenTofu produces any warnings that are not
                        accompanied by errors, show them in a more compact
                        form that includes only the summary messages.

  -consolidate-warnings If OpenTofu produces any warnings, no consolidation
                        will be performed. All locations, for all warnings
                        will be listed. Enabled by default.

  -consolidate-errors   If OpenTofu produces any errors, no consolidation
                        will be performed. All locations, for all errors
                        will be listed. Disabled by default

  -filter=testfile      If specified, OpenTofu will only execute the test files
                        specified by this flag. You can use this option multiple
                        times to execute more than one test file. The path should
                        be relative to the current working directory, even if
                        -test-directory is set.

  -json                 If specified, machine readable output will be printed in
                        JSON format

  -json-into=out.json   Produce the same output as -json, but sent directly
                        to the given file. This allows automation to preserve
                        the original human-readable output streams, while
                        capturing more detailed logs for machine analysis.

  -no-color             If specified, output won't contain any color.

  -test-directory=path  Set the OpenTofu test directory, defaults to "tests". When set, the
                        test command will search for test files in the current directory and
                        in the one specified by the flag.

  -var 'foo=bar'        Set a value for one of the input variables in the root
                        module of the configuration. Use this option more than
                        once to set more than one variable.

  -var-file=filename    Load variable values from the given file, in addition
                        to the default files terraform.tfvars and *.auto.tfvars.
                        Use this option more than once to include more than one
                        variables file.

  -verbose              Print the plan or state for each test run block as it
                        executes.

  -var 'foo=bar'        Set a value for one of the input variables in the root
                        module of the configuration. Use this option more than
                        once to set more than one variable.

  -var-file=filename    Load variable values from the given file, in addition
                        to the default files terraform.tfvars and *.auto.tfvars.
                        Use this option more than once to include more than one
                        variables file.
`
	return strings.TrimSpace(helpText)
}

func (c *TestCommand) Synopsis() string {
	return "Execute integration tests for OpenTofu modules"
}

func (c *TestCommand) Run(rawArgs []string) int {
	var diags tfdiags.Diagnostics
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	args, closer, diags := arguments.ParseTest(rawArgs)
	defer closer()
	if diags.HasErrors() {
		c.View.Diagnostics(diags)
		c.View.HelpPrompt("test")
		return 1
	}

	view := views.NewTest(args.ViewOptions, c.View)

	// Users can also specify variables via the command line, so we'll parse
	// all that here.
	items := args.Vars.All()
	c.variableArgs = flags.RawFlags{Items: &items}

	variables, variableDiags := c.collectVariableValuesWithTests(args.TestDirectory)
	diags = diags.Append(variableDiags)
	if variableDiags.HasErrors() {
		view.Diagnostics(nil, nil, diags)
		return 1
	}

	config, configDiags := c.loadConfigWithTests(ctx, ".", args.TestDirectory)
	diags = diags.Append(configDiags)
	if configDiags.HasErrors() {
		view.Diagnostics(nil, nil, diags)
		return 1
	}

	runCount := 0
	fileCount := 0

	var fileDiags tfdiags.Diagnostics
	suite := moduletest.Suite{
		Files: func() map[string]*moduletest.File {
			files := make(map[string]*moduletest.File)

			if len(args.Filter) > 0 {
				for _, name := range args.Filter {
					file, ok := config.Module.Tests[name]
					if !ok {
						// If the filter is invalid, we'll simply skip this
						// entry and print a warning. But we could still execute
						// any other tests within the filter.
						fileDiags.Append(tfdiags.Sourceless(
							tfdiags.Warning,
							"Unknown test file",
							fmt.Sprintf("The specified test file, %s, could not be found.", name)))
						continue
					}

					fileCount++

					var runs []*moduletest.Run
					for ix, run := range file.Runs {
						runs = append(runs, &moduletest.Run{
							Config: run,
							Index:  ix,
							Name:   run.Name,
						})
					}

					runCount += len(runs)
					files[name] = &moduletest.File{
						Config: file,
						Name:   name,
						Runs:   runs,
					}
				}

				return files
			}

			// Otherwise, we'll just do all the tests in the directory!
			for name, file := range config.Module.Tests {
				fileCount++

				var runs []*moduletest.Run
				for ix, run := range file.Runs {
					runs = append(runs, &moduletest.Run{
						Config: run,
						Index:  ix,
						Name:   run.Name,
					})
				}

				runCount += len(runs)
				files[name] = &moduletest.File{
					Config: file,
					Name:   name,
					Runs:   runs,
				}
			}
			return files
		}(),
	}

	log.Printf("[DEBUG] TestCommand: found %d files with %d run blocks", fileCount, runCount)

	if len(args.Filter) > 0 && len(suite.Files) == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Warning,
			"No tests were found",
			"-filter is being used but no tests were found. Make sure you're using a relative path to the current working directory.",
		))
	}

	diags = diags.Append(fileDiags)
	if fileDiags.HasErrors() {
		view.Diagnostics(nil, nil, diags)
		return 1
	}

	opts, err := c.contextOpts(ctx)
	if err != nil {
		diags = diags.Append(err)
		view.Diagnostics(nil, nil, diags)
		return 1
	}

	// Don't use encryption during testing
	opts.Encryption = encryption.Disabled()

	// Print out all the diagnostics we have from the setup. These will just be
	// warnings, and we want them out of the way before we start the actual
	// testing.
	view.Diagnostics(nil, nil, diags)

	// We have two levels of interrupt here. A 'stop' and a 'cancel'. A 'stop'
	// is a soft request to stop. We'll finish the current test, do the tidy up,
	// but then skip all remaining tests and run blocks. A 'cancel' is a hard
	// request to stop now. We'll cancel the current operation immediately
	// even if it's a delete operation, and we won't clean up any infrastructure
	// if we're halfway through a test. We'll print details explaining what was
	// stopped so the user can do their best to recover from it.

	runningCtx, done := context.WithCancel(context.WithoutCancel(ctx))
	stopCtx, stop := context.WithCancel(runningCtx)
	cancelCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	runner := &TestSuiteRunner{
		command: c,

		Suite:  &suite,
		Config: config,
		View:   view,

		GlobalVariables: variables,
		Opts:            opts,

		CancelledCtx: cancelCtx,
		StoppedCtx:   stopCtx,

		// Just to be explicit, we'll set the following fields even though they
		// default to these values.
		Cancelled: false,
		Stopped:   false,

		Verbose: args.Verbose,
	}

	view.Abstract(&suite)

	panicHandler := logging.PanicHandlerWithTraceFn()
	go func() {
		defer panicHandler()
		defer done()
		defer stop()
		defer cancel()

		runner.Start(ctx)
	}()

	// Wait for the operation to complete, or for an interrupt to occur.
	select {
	case <-c.ShutdownCh:
		// Nice request to be cancelled.

		view.Interrupted()
		runner.Stopped = true
		stop()

		select {
		case <-c.ShutdownCh:
			// The user pressed it again, now we have to get it to stop as
			// fast as possible.

			view.FatalInterrupt()
			runner.Cancelled = true
			cancel()

			// We'll wait 5 seconds for this operation to finish now, regardless
			// of whether it finishes successfully or not.
			select {
			case <-runningCtx.Done():
			case <-time.After(5 * time.Second):
			}

		case <-runningCtx.Done():
			// The application finished nicely after the request was stopped.
		}
	case <-runningCtx.Done():
		// tests finished normally with no interrupts.
	}

	if runner.Cancelled {
		// Don't print out the conclusion if the test was cancelled.
		return 1
	}

	view.Conclusion(&suite)

	if suite.Status != moduletest.Pass {
		return 1
	}
	return 0
}

// test runner

type TestSuiteRunner struct {
	command *TestCommand

	Suite  *moduletest.Suite
	Config *configs.Config

	GlobalVariables map[string]backend.UnparsedVariableValue
	Opts            *tofu.ContextOpts

	View views.Test

	// Stopped and Cancelled track whether the user requested the testing
	// process to be interrupted. Stopped is a nice graceful exit, we'll still
	// tidy up any state that was created and mark the tests with relevant
	// `skipped` status updates. Cancelled is a hard stop right now exit, we
	// won't attempt to clean up any state left hanging, and tests will just
	// be left showing `pending` as the status. We will still print out the
	// destroy summary diagnostics that tell the user what state has been left
	// behind and needs manual clean up.
	Stopped   bool
	Cancelled bool

	// StoppedCtx and CancelledCtx allow in progress OpenTofu operations to
	// respond to external calls from the test command.
	StoppedCtx   context.Context
	CancelledCtx context.Context

	// Verbose tells the runner to print out plan files during each test run.
	Verbose bool
}

func (runner *TestSuiteRunner) Start(ctx context.Context) {
	var files []string
	for name := range runner.Suite.Files {
		files = append(files, name)
	}
	sort.Strings(files) // execute the files in alphabetical order

	runner.Suite.Status = moduletest.Pass
	for _, name := range files {
		if runner.Cancelled {
			return
		}

		file := runner.Suite.Files[name]

		fileRunner := &TestFileRunner{
			Suite: runner,
			States: map[string]*TestFileState{
				MainStateIdentifier: {
					Run:   nil,
					State: states.NewState(),
				},
			},
		}

		fileRunner.ExecuteTestFile(ctx, file)
		fileRunner.Cleanup(ctx, file)
		runner.Suite.Status = runner.Suite.Status.Merge(file.Status)
	}
}

type TestFileRunner struct {
	Suite *TestSuiteRunner

	States map[string]*TestFileState
}

type TestFileState struct {
	Run   *moduletest.Run
	State *states.State
}

func (runner *TestFileRunner) ExecuteTestFile(ctx context.Context, file *moduletest.File) {
	log.Printf("[TRACE] TestFileRunner: executing test file %s", file.Name)

	file.Status = file.Status.Merge(moduletest.Pass)
	for _, run := range file.Runs {
		if runner.Suite.Cancelled {
			// This means a hard stop has been requested, in this case we don't
			// even stop to mark future tests as having been skipped. They'll
			// just show up as pending in the printed summary.
			return
		}

		if runner.Suite.Stopped {
			// Then the test was requested to be stopped, so we just mark each
			// following test as skipped and move on.
			run.Status = moduletest.Skip
			continue
		}

		if file.Status == moduletest.Error {
			// If the overall test file has errored, we don't keep trying to
			// execute tests. Instead, we mark all remaining run blocks as
			// skipped.
			run.Status = moduletest.Skip
			continue
		}

		key := MainStateIdentifier
		config := runner.Suite.Config
		if run.Config.ConfigUnderTest != nil {
			config = run.Config.ConfigUnderTest
			// Then we need to load an alternate state and not the main one.

			key = run.Config.Module.Source.String()
			if key == MainStateIdentifier {
				// This is bad. It means somehow the module we're loading has
				// the same key as main state and we're about to corrupt things.

				run.Diagnostics = run.Diagnostics.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid module source",
					Detail:   fmt.Sprintf("The source for the selected module evaluated to %s which should not be possible. This is a bug in OpenTofu - please report it!", key),
					Subject:  run.Config.Module.DeclRange.Ptr(),
				})

				run.Status = moduletest.Error
				file.Status = moduletest.Error
				continue // Abort!
			}

			if _, exists := runner.States[key]; !exists {
				runner.States[key] = &TestFileState{
					Run:   nil,
					State: states.NewState(),
				}
			}
		}

		state, updatedState := runner.ExecuteTestRun(ctx, run, file, runner.States[key].State, config)
		if updatedState {
			var err error

			// We need to simulate state serialization between multiple runs
			// due to its side effects. One of such side effects is removal
			// of destroyed non-root module outputs. This is not handled
			// during graph walk since those values are not stored in the
			// state file. This is more of a weird workaround instead of a
			// proper fix, unfortunately.
			state, err = simulateStateSerialization(state)
			if err != nil {
				run.Diagnostics = run.Diagnostics.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Failure during state serialization",
					Detail:   err.Error(),
				})

				// We cannot reuse state later so that's a hard stop.
				return
			}

			// Only update the most recent run and state if the state was
			// actually updated by this change. We want to use the run that
			// most recently updated the tracked state as the cleanup
			// configuration.
			runner.States[key].State = state
			runner.States[key].Run = run
		}

		file.Status = file.Status.Merge(run.Status)
	}

	runner.Suite.View.File(file)
	for _, run := range file.Runs {
		runner.Suite.View.Run(run, file)
	}
}

func (runner *TestFileRunner) ExecuteTestRun(ctx context.Context, run *moduletest.Run, file *moduletest.File, state *states.State, config *configs.Config) (*states.State, bool) {
	log.Printf("[TRACE] TestFileRunner: executing run block %s/%s", file.Name, run.Name)

	if runner.Suite.Cancelled {
		// Don't do anything, just give up and return immediately.
		// The surrounding functions should stop this even being called, but in
		// case of race conditions or something we can still verify this.
		return state, false
	}

	if runner.Suite.Stopped {
		// Basically the same as above, except we'll be a bit nicer.
		run.Status = moduletest.Skip
		return state, false
	}

	run.Diagnostics = run.Diagnostics.Append(file.Config.Validate())
	if run.Diagnostics.HasErrors() {
		run.Status = moduletest.Error
		return state, false
	}

	run.Diagnostics = run.Diagnostics.Append(run.Config.Validate())
	if run.Diagnostics.HasErrors() {
		run.Status = moduletest.Error
		return state, false
	}

	evalCtx, evalDiags := buildEvalContextForProviderConfigTransform(runner.States, run, file, config, runner.Suite.GlobalVariables)
	run.Diagnostics = run.Diagnostics.Append(evalDiags)
	if evalDiags.HasErrors() {
		run.Status = moduletest.Error
		return state, false
	}

	resetConfig, configDiags := config.TransformForTest(run.Config, file.Config, evalCtx)
	defer resetConfig()

	run.Diagnostics = run.Diagnostics.Append(configDiags)
	if configDiags.HasErrors() {
		run.Status = moduletest.Error
		return state, false
	}

	validateDiags := runner.validate(ctx, config, run, file)
	run.Diagnostics = run.Diagnostics.Append(validateDiags)
	if validateDiags.HasErrors() {
		run.Status = moduletest.Error
		return state, false
	}

	planCtx, plan, planDiags := runner.plan(ctx, config, state, run, file)
	if run.Config.Command == configs.PlanTestCommand {
		expectedFailures, sourceRanges := run.BuildExpectedFailuresAndSourceMaps()
		// Then we want to assess our conditions and diagnostics differently.
		planDiags = run.ValidateExpectedFailures(expectedFailures, sourceRanges, planDiags)
		run.Diagnostics = run.Diagnostics.Append(planDiags)
		if planDiags.HasErrors() {
			run.Status = moduletest.Error
			return state, false
		}

		variables, resetVariables, variableDiags := runner.prepareInputVariablesForAssertions(config, run, file, runner.Suite.GlobalVariables)
		defer resetVariables()

		run.Diagnostics = run.Diagnostics.Append(variableDiags)
		if variableDiags.HasErrors() {
			run.Status = moduletest.Error
			return state, false
		}

		if runner.Suite.Verbose {
			schemas, diags := planCtx.Schemas(ctx, config, plan.PlannedState)

			// If we're going to fail to render the plan, let's not fail the overall
			// test. It can still have succeeded. So we'll add the diagnostics, but
			// still report the test status as a success.
			if diags.HasErrors() {
				// This is very unlikely.
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Warning,
					"Failed to print verbose output",
					fmt.Sprintf("OpenTofu failed to print the verbose output for %s, other diagnostics will contain more details as to why.", path.Join(file.Name, run.Name))))
			} else {
				run.Verbose = &moduletest.Verbose{
					Plan:         plan,
					State:        plan.PlannedState,
					Config:       config,
					Providers:    schemas.Providers,
					Provisioners: schemas.Provisioners,
				}
			}

			run.Diagnostics = run.Diagnostics.Append(diags)
		}

		planCtx.TestContext(config, plan.PlannedState, plan, variables).EvaluateAgainstPlan(run)
		return state, false
	}

	expectedFailures, sourceRanges := run.BuildExpectedFailuresAndSourceMaps()

	planDiags = checkProblematicPlanErrors(expectedFailures, planDiags)

	// Otherwise any error during the planning prevents our apply from
	// continuing which is an error.
	run.Diagnostics = run.Diagnostics.Append(planDiags)
	if planDiags.HasErrors() {
		run.Status = moduletest.Error
		return state, false
	}

	// Since we're carrying on an executing the apply operation as well, we're
	// just going to do some post processing of the diagnostics. We remove the
	// warnings generated from check blocks, as the apply operation will either
	// reproduce them or fix them and we don't want fixed diagnostics to be
	// reported and we don't want duplicates either.
	var filteredDiags tfdiags.Diagnostics
	for _, diag := range run.Diagnostics {
		if rule, ok := addrs.DiagnosticOriginatesFromCheckRule(diag); ok && rule.Container.CheckableKind() == addrs.CheckableCheck {
			continue
		}
		filteredDiags = filteredDiags.Append(diag)
	}
	run.Diagnostics = filteredDiags

	applyCtx, updated, applyDiags := runner.apply(ctx, plan, state, config, run, file)

	// Remove expected diagnostics, and add diagnostics in case anything that should have failed didn't.
	applyDiags = run.ValidateExpectedFailures(expectedFailures, sourceRanges, applyDiags)

	run.Diagnostics = run.Diagnostics.Append(applyDiags)
	if applyDiags.HasErrors() {
		run.Status = moduletest.Error
		// Even though the apply operation failed, the graph may have done
		// partial updates and the returned state should reflect this.
		return updated, true
	}

	variables, resetVariables, variableDiags := runner.prepareInputVariablesForAssertions(config, run, file, runner.Suite.GlobalVariables)
	defer resetVariables()

	run.Diagnostics = run.Diagnostics.Append(variableDiags)
	if variableDiags.HasErrors() {
		run.Status = moduletest.Error
		return updated, true
	}

	if runner.Suite.Verbose {
		schemas, diags := planCtx.Schemas(ctx, config, plan.PlannedState)

		// If we're going to fail to render the plan, let's not fail the overall
		// test. It can still have succeeded. So we'll add the diagnostics, but
		// still report the test status as a success.
		if diags.HasErrors() {
			// This is very unlikely.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Failed to print verbose output",
				fmt.Sprintf("OpenTofu failed to print the verbose output for %s, other diagnostics will contain more details as to why.", path.Join(file.Name, run.Name))))
		} else {
			run.Verbose = &moduletest.Verbose{
				Plan:         plan,
				State:        updated,
				Config:       config,
				Providers:    schemas.Providers,
				Provisioners: schemas.Provisioners,
			}
		}

		run.Diagnostics = run.Diagnostics.Append(diags)
	}

	applyCtx.TestContext(config, updated, plan, variables).EvaluateAgainstState(run)
	return updated, true
}

func (runner *TestFileRunner) validate(ctx context.Context, config *configs.Config, run *moduletest.Run, file *moduletest.File) tfdiags.Diagnostics {
	log.Printf("[TRACE] TestFileRunner: called validate for %s/%s", file.Name, run.Name)

	var diags tfdiags.Diagnostics

	tfCtx, ctxDiags := tofu.NewContext(runner.Suite.Opts)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		return diags
	}

	runningCtx, done := context.WithCancel(context.WithoutCancel(ctx))

	var validateDiags tfdiags.Diagnostics
	panicHandler := logging.PanicHandlerWithTraceFn()
	go func() {
		defer panicHandler()
		defer done()

		log.Printf("[DEBUG] TestFileRunner: starting validate for %s/%s", file.Name, run.Name)
		validateDiags = tfCtx.Validate(ctx, config)
		log.Printf("[DEBUG] TestFileRunner: completed validate for  %s/%s", file.Name, run.Name)
	}()
	waitDiags, cancelled := runner.wait(tfCtx, runningCtx, run, file, nil)

	if cancelled {
		diags = diags.Append(tfdiags.Sourceless(tfdiags.Error, "Test interrupted", "The test operation could not be completed due to an interrupt signal. Please read the remaining diagnostics carefully for any sign of failed state cleanup or dangling resources."))
	}

	diags = diags.Append(waitDiags)
	diags = diags.Append(validateDiags)

	return diags
}

func (runner *TestFileRunner) destroy(ctx context.Context, config *configs.Config, state *states.State, run *moduletest.Run, file *moduletest.File) (*states.State, tfdiags.Diagnostics) {
	log.Printf("[TRACE] TestFileRunner: called destroy for %s/%s", file.Name, run.Name)

	if state.Empty() {
		// Nothing to do!
		return state, nil
	}

	var diags tfdiags.Diagnostics

	evalCtx, evalDiags := buildEvalContextForProviderConfigTransform(runner.States, run, file, config, runner.Suite.GlobalVariables)
	run.Diagnostics = run.Diagnostics.Append(evalDiags)
	if evalDiags.HasErrors() {
		return state, nil
	}

	variables, variableDiags := buildInputVariablesForTest(run, file, config, runner.Suite.GlobalVariables, evalCtx)
	diags = diags.Append(variableDiags)

	if diags.HasErrors() {
		return state, diags
	}

	planOpts := &tofu.PlanOpts{
		Mode:         plans.DestroyMode,
		SetVariables: variables,
	}

	tfCtx, ctxDiags := tofu.NewContext(runner.Suite.Opts)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		return state, diags
	}

	runningCtx, done := context.WithCancel(context.WithoutCancel(ctx))

	var plan *plans.Plan
	var planDiags tfdiags.Diagnostics
	panicHandler := logging.PanicHandlerWithTraceFn()
	go func() {
		defer panicHandler()
		defer done()

		log.Printf("[DEBUG] TestFileRunner: starting destroy plan for %s/%s", file.Name, run.Name)
		plan, planDiags = tfCtx.Plan(ctx, config, state, planOpts)
		log.Printf("[DEBUG] TestFileRunner: completed destroy plan for %s/%s", file.Name, run.Name)
	}()
	waitDiags, cancelled := runner.wait(tfCtx, runningCtx, run, file, nil)

	if cancelled {
		diags = diags.Append(tfdiags.Sourceless(tfdiags.Error, "Test interrupted", "The test operation could not be completed due to an interrupt signal. Please read the remaining diagnostics carefully for any sign of failed state cleanup or dangling resources."))
	}

	diags = diags.Append(waitDiags)
	diags = diags.Append(planDiags)

	if diags.HasErrors() {
		return state, diags
	}

	_, updated, applyDiags := runner.apply(ctx, plan, state, config, run, file)
	diags = diags.Append(applyDiags)
	return updated, diags
}

func (runner *TestFileRunner) plan(ctx context.Context, config *configs.Config, state *states.State, run *moduletest.Run, file *moduletest.File) (*tofu.Context, *plans.Plan, tfdiags.Diagnostics) {
	log.Printf("[TRACE] TestFileRunner: called plan for %s/%s", file.Name, run.Name)

	var diags tfdiags.Diagnostics

	targets, targetDiags := run.GetTargets()
	diags = diags.Append(targetDiags)

	replaces, replaceDiags := run.GetReplaces()
	diags = diags.Append(replaceDiags)

	references, referenceDiags := run.GetReferences()
	diags = diags.Append(referenceDiags)

	evalCtx, ctxDiags := getEvalContextForTest(runner.States, config, runner.Suite.GlobalVariables)
	diags = diags.Append(ctxDiags)

	variables, variableDiags := buildInputVariablesForTest(run, file, config, runner.Suite.GlobalVariables, evalCtx)
	diags = diags.Append(variableDiags)

	if diags.HasErrors() {
		return nil, nil, diags
	}

	planOpts := &tofu.PlanOpts{
		Mode: func() plans.Mode {
			switch run.Config.Options.Mode {
			case configs.RefreshOnlyTestMode:
				return plans.RefreshOnlyMode
			default:
				return plans.NormalMode
			}
		}(),
		Targets:            targets,
		ForceReplace:       replaces,
		SkipRefresh:        !run.Config.Options.Refresh,
		SetVariables:       variables,
		ExternalReferences: references,
	}

	tfCtx, ctxDiags := tofu.NewContext(runner.Suite.Opts)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		return nil, nil, diags
	}

	runningCtx, done := context.WithCancel(context.WithoutCancel(ctx))

	var plan *plans.Plan
	var planDiags tfdiags.Diagnostics
	panicHandler := logging.PanicHandlerWithTraceFn()
	go func() {
		defer panicHandler()
		defer done()

		log.Printf("[DEBUG] TestFileRunner: starting plan for %s/%s", file.Name, run.Name)
		plan, planDiags = tfCtx.Plan(ctx, config, state, planOpts)
		log.Printf("[DEBUG] TestFileRunner: completed plan for %s/%s", file.Name, run.Name)
	}()
	waitDiags, cancelled := runner.wait(tfCtx, runningCtx, run, file, nil)

	if cancelled {
		diags = diags.Append(tfdiags.Sourceless(tfdiags.Error, "Test interrupted", "The test operation could not be completed due to an interrupt signal. Please read the remaining diagnostics carefully for any sign of failed state cleanup or dangling resources."))
	}

	diags = diags.Append(waitDiags)
	diags = diags.Append(planDiags)

	return tfCtx, plan, diags
}

func (runner *TestFileRunner) apply(ctx context.Context, plan *plans.Plan, state *states.State, config *configs.Config, run *moduletest.Run, file *moduletest.File) (*tofu.Context, *states.State, tfdiags.Diagnostics) {
	log.Printf("[TRACE] TestFileRunner: called apply for %s/%s", file.Name, run.Name)

	var diags tfdiags.Diagnostics

	// If things get cancelled while we are executing the apply operation below
	// we want to print out all the objects that we were creating so the user
	// can verify we managed to tidy everything up possibly.
	//
	// Unfortunately, this creates a race condition as the apply operation can
	// edit the plan (by removing changes once they are applied) while at the
	// same time our cancellation process will try to read the plan.
	//
	// We take a quick copy of the changes we care about here, which will then
	// be used in place of the plan when we print out the objects to be created
	// as part of the cancellation process.
	var created []*plans.ResourceInstanceChangeSrc
	for _, change := range plan.Changes.Resources {
		if change.Action != plans.Create {
			continue
		}
		created = append(created, change)
	}

	tfCtx, ctxDiags := tofu.NewContext(runner.Suite.Opts)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		return nil, state, diags
	}

	runningCtx, done := context.WithCancel(context.WithoutCancel(ctx))

	var updated *states.State
	var applyDiags tfdiags.Diagnostics

	panicHandler := logging.PanicHandlerWithTraceFn()
	go func() {
		defer panicHandler()
		defer done()
		log.Printf("[DEBUG] TestFileRunner: starting apply for %s/%s", file.Name, run.Name)
		updated, applyDiags = tfCtx.Apply(ctx, plan, config, nil)
		log.Printf("[DEBUG] TestFileRunner: completed apply for %s/%s", file.Name, run.Name)
	}()
	waitDiags, cancelled := runner.wait(tfCtx, runningCtx, run, file, created)

	if cancelled {
		diags = diags.Append(tfdiags.Sourceless(tfdiags.Error, "Test interrupted", "The test operation could not be completed due to an interrupt signal. Please read the remaining diagnostics carefully for any sign of failed state cleanup or dangling resources."))
	}

	diags = diags.Append(waitDiags)
	diags = diags.Append(applyDiags)

	return tfCtx, updated, diags
}

func (runner *TestFileRunner) wait(ctx *tofu.Context, runningCtx context.Context, run *moduletest.Run, file *moduletest.File, created []*plans.ResourceInstanceChangeSrc) (diags tfdiags.Diagnostics, cancelled bool) {
	var identifier string
	if file == nil {
		identifier = "validate"
	} else {
		identifier = file.Name
		if run != nil {
			identifier = fmt.Sprintf("%s/%s", identifier, run.Name)
		}
	}
	log.Printf("[TRACE] TestFileRunner: waiting for execution during %s", identifier)

	// This function handles what happens when the user presses the second
	// interrupt. This is a "hard cancel", we are going to stop doing whatever
	// it is we're doing. This means even if we're halfway through creating or
	// destroying infrastructure we just give up.
	handleCancelled := func() {
		log.Printf("[DEBUG] TestFileRunner: test execution cancelled during %s", identifier)

		states := make(map[*moduletest.Run]*states.State)
		states[nil] = runner.States[MainStateIdentifier].State
		for key, module := range runner.States {
			if key == MainStateIdentifier {
				continue
			}
			states[module.Run] = module.State
		}
		runner.Suite.View.FatalInterruptSummary(run, file, states, created)

		cancelled = true
		go ctx.Stop()

		// Just wait for things to finish now, the overall test execution will
		// exit early if this takes too long.
		<-runningCtx.Done()
	}

	// This function handles what happens when the user presses the first
	// interrupt. This is essentially a "soft cancel", we're not going to do
	// anything but just wait for things to finish safely. But, we do listen
	// for the crucial second interrupt which will prompt a hard stop / cancel.
	handleStopped := func() {
		log.Printf("[DEBUG] TestFileRunner: test execution stopped during %s", identifier)

		select {
		case <-runner.Suite.CancelledCtx.Done():
			// We've been asked again. This time we stop whatever we're doing
			// and abandon all attempts to do anything reasonable.
			handleCancelled()
		case <-runningCtx.Done():
			// Do nothing, we finished safely and skipping the remaining tests
			// will be handled elsewhere.
		}

	}

	select {
	case <-runner.Suite.StoppedCtx.Done():
		handleStopped()
	case <-runner.Suite.CancelledCtx.Done():
		handleCancelled()
	case <-runningCtx.Done():
		// The operation exited normally.
	}

	return diags, cancelled
}

func (runner *TestFileRunner) Cleanup(ctx context.Context, file *moduletest.File) {
	log.Printf("[TRACE] TestStateManager: cleaning up state for %s", file.Name)

	if runner.Suite.Cancelled {
		// Don't try and clean anything up if the execution has been cancelled.
		log.Printf("[DEBUG] TestStateManager: skipping state cleanup for %s due to cancellation", file.Name)
		return
	}

	var states []*TestFileState
	for key, state := range runner.States {
		if state.Run == nil {
			if state.State.Empty() {
				// We can see a run block being empty when the state is empty if
				// a module was only used to execute plan commands. So this is
				// okay, and means we have nothing to cleanup so we'll just
				// skip it.
				continue
			}

			if key == MainStateIdentifier {
				log.Printf("[ERROR] TestFileRunner: found inconsistent run block and state file in %s", file.Name)
			} else {
				log.Printf("[ERROR] TestFileRunner: found inconsistent run block and state file in %s for module %s", file.Name, key)
			}

			// Otherwise something bad has happened, and we have no way to
			// recover from it. This shouldn't happen in reality, but we'll
			// print a diagnostic instead of panicking later.

			var diags tfdiags.Diagnostics
			diags = diags.Append(tfdiags.Sourceless(tfdiags.Error, "Inconsistent state", fmt.Sprintf("Found inconsistent state while cleaning up %s. This is a bug in OpenTofu - please report it", file.Name)))
			runner.Suite.View.DestroySummary(diags, nil, file, state.State)
			continue
		}

		states = append(states, state)
	}

	slices.SortFunc(states, func(a, b *TestFileState) int {
		// We want to clean up later run blocks first. So, we'll sort this in
		// reverse according to index. This means larger indices first.
		return b.Run.Index - a.Run.Index
	})

	// Clean up all the states (for main and custom modules) in reverse order.
	for _, state := range states {
		log.Printf("[DEBUG] TestStateManager: cleaning up state for %s/%s", file.Name, state.Run.Name)

		if runner.Suite.Cancelled {
			// In case the cancellation came while a previous state was being
			// destroyed.
			log.Printf("[DEBUG] TestStateManager: skipping state cleanup for %s/%s due to cancellation", file.Name, state.Run.Name)
			return
		}

		var diags tfdiags.Diagnostics
		var runConfig *configs.Config

		isMainState := state.Run.Config.Module == nil
		if isMainState {
			runConfig = runner.Suite.Config
		} else {
			runConfig = state.Run.Config.ConfigUnderTest
		}

		evalCtx, evalDiags := buildEvalContextForProviderConfigTransform(runner.States, state.Run, file, runConfig, runner.Suite.GlobalVariables)
		if evalDiags.HasErrors() {
			return
		}

		reset, configDiags := runConfig.TransformForTest(state.Run.Config, file.Config, evalCtx)
		diags = diags.Append(configDiags)

		updated := state.State
		if !diags.HasErrors() {
			var destroyDiags tfdiags.Diagnostics
			updated, destroyDiags = runner.destroy(ctx, runConfig, state.State, state.Run, file)
			diags = diags.Append(destroyDiags)
		}
		runner.Suite.View.DestroySummary(diags, state.Run, file, updated)

		if updated.HasManagedResourceInstanceObjects() {
			views.SaveErroredTestStateFile(updated, state.Run, file, runner.Suite.View)
		}
		reset()
	}
}

// helper functions

// buildEvalContextForProviderConfigTransform constructs a hcl.EvalContext based on the provided map of
// TestFileState instances, configuration and global variables. Also, creates a tofu.InputValues mapping for
// variable values that are relevant to the config being tested. And merges the variables into the evalCtx.
// This is required to transform provider configs defined inside the test file, which are using run block output.
//
// Variable pre-evaluation and merging into the context is required, to evaluate more complex expressions involving both
// run outputs and var (For example, as a function arguments). Without this step the test file won't be able to overwrite
// input variables with `variables` block.
//
// The evalCtx returned from this, contains built-in functions for the same reason.
func buildEvalContextForProviderConfigTransform(states map[string]*TestFileState, run *moduletest.Run, file *moduletest.File, config *configs.Config, globals map[string]backend.UnparsedVariableValue) (*hcl.EvalContext, tfdiags.Diagnostics) {
	evalCtx, diags := getEvalContextForTest(states, config, globals)
	vars, varDiags := buildInputVariablesForTest(run, file, config, globals, evalCtx)
	diags = diags.Append(varDiags)
	if diags.HasErrors() {
		return evalCtx, diags
	}

	varMap := evalCtx.Variables["var"].AsValueMap()
	if varMap == nil {
		varMap = make(map[string]cty.Value)
	}
	for name, val := range vars {
		// If the input variable is not defined on test file, we skip it.
		if val == nil || val.Value.IsNull() {
			continue
		}
		varMap[name] = val.Value
	}
	evalCtx.Variables["var"] = cty.ObjectVal(varMap)

	scope := &lang.Scope{}
	evalCtx.Functions = scope.Functions()
	return evalCtx, diags
}

// buildInputVariablesForTest creates a tofu.InputValues mapping for
// variable values that are relevant to the config being tested.
//
// Crucially, it differs from prepareInputVariablesForAssertions in that it only
// includes variables that are reference by the config and not everything that
// is defined within the test run block and test file.
func buildInputVariablesForTest(run *moduletest.Run, file *moduletest.File, config *configs.Config, globals map[string]backend.UnparsedVariableValue, evalCtx *hcl.EvalContext) (tofu.InputValues, tfdiags.Diagnostics) {
	variables := make(map[string]backend.UnparsedVariableValue)
	for name := range config.Module.Variables {
		if run != nil {
			if expr, exists := run.Config.Variables[name]; exists {
				// Local variables take precedence.
				variables[name] = testVariableValueExpression{
					expr:       expr,
					sourceType: tofu.ValueFromConfig,
					ctx:        evalCtx,
				}
				continue
			}
		}

		if file != nil {
			if expr, exists := file.Config.Variables[name]; exists {
				// If it's not set locally, it maybe set for the entire file.
				variables[name] = testVariableValueExpression{
					expr:       expr,
					sourceType: tofu.ValueFromConfig,
					ctx:        evalCtx,
				}
				continue
			}
		}

		if globals != nil {
			// If it's not set locally or at the file level, maybe it was
			// defined globally.
			if variable, exists := globals[name]; exists {
				variables[name] = variable
			}
		}

		// If it's not set at all that might be okay if the variable is optional
		// so we'll just not add anything to the map.
	}

	return backend.ParseVariableValues(variables, config.Module.Variables)
}

// getEvalContextForTest constructs an hcl.EvalContext based on the provided map of
// TestFileState instances, configuration and global variables.
// It extracts the relevant information from the input parameters to create a
// context suitable for HCL evaluation.
func getEvalContextForTest(states map[string]*TestFileState, config *configs.Config, globals map[string]backend.UnparsedVariableValue) (*hcl.EvalContext, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	runCtx := make(map[string]cty.Value)
	for _, state := range states {
		if state.Run == nil {
			continue
		}
		outputs := make(map[string]cty.Value)
		mod := state.State.Modules[""] // Empty string is what is used by the module in the test runner
		for outName, out := range mod.OutputValues {
			outputs[outName] = out.Value
		}
		runCtx[state.Run.Name] = cty.ObjectVal(outputs)
	}

	// If the variable is referenced in the tfvars file or TF_VAR_ environment variable, then lookup the value
	// in global variables; otherwise, assign the default value.
	inputValues, diags := parseAndApplyDefaultValues(globals, config.Module.Variables)
	diags.Append(diags)

	varCtx := make(map[string]cty.Value)
	for name, val := range inputValues {
		varCtx[name] = val.Value
	}

	scope := &lang.Scope{}
	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"run": cty.ObjectVal(runCtx),
			"var": cty.ObjectVal(varCtx),
		},
		Functions: scope.Functions(),
	}
	return ctx, diags
}

type testVariableValueExpression struct {
	expr       hcl.Expression
	sourceType tofu.ValueSourceType
	ctx        *hcl.EvalContext
}

func (v testVariableValueExpression) ParseVariableValue(mode configs.VariableParsingMode) (*tofu.InputValue, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	val, hclDiags := v.expr.Value(v.ctx)
	diags = diags.Append(hclDiags)

	rng := tfdiags.SourceRangeFromHCL(v.expr.Range())

	return &tofu.InputValue{
		Value:       val,
		SourceType:  v.sourceType,
		SourceRange: rng,
	}, diags
}

// parseAndApplyDefaultValues parses the given unparsed variables into tofu.InputValues
// and applies default values from the configuration variables where applicable.
// This ensures all variables are correctly initialized and returns the resulting tofu.InputValues.
func parseAndApplyDefaultValues(unparsedVariables map[string]backend.UnparsedVariableValue, configVariables map[string]*configs.Variable) (tofu.InputValues, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	inputs := make(tofu.InputValues, len(unparsedVariables))
	for name, variable := range unparsedVariables {
		value, valueDiags := variable.ParseVariableValue(configs.VariableParseLiteral)
		diags = diags.Append(valueDiags)

		// Even so the variable is declared, some of the fields could
		// be empty and filled in via type default values.
		if confVariable, ok := configVariables[name]; ok && confVariable.TypeDefaults != nil {
			value.Value = confVariable.TypeDefaults.Apply(value.Value)
		}

		inputs[name] = value
	}

	// Now, we're going to apply any default values from the configuration.
	// We do this after the conversion into tofu.InputValues, as the
	// defaults have already been converted into cty.Value objects.
	for name, variable := range configVariables {
		if _, exists := unparsedVariables[name]; exists {
			// Then we don't want to apply the default for this variable as we
			// already have a value.
			continue
		}

		if variable.Default != cty.NilVal {
			inputs[name] = &tofu.InputValue{
				Value:       variable.Default,
				SourceType:  tofu.ValueFromConfig,
				SourceRange: tfdiags.SourceRangeFromHCL(variable.DeclRange),
			}
		}
	}

	return inputs, diags
}

// prepareInputVariablesForAssertions creates a tofu.InputValues mapping
// that contains all the variables defined for a given run and file, alongside
// any unset variables that have defaults within the provided config.
//
// Crucially, it differs from buildInputVariablesForTest in that the returned
// input values include all variables available even if they are not defined
// within the config. This allows the assertions to refer to variables defined
// solely within the test file, and not only those within the configuration.
//
// It also allows references to previously run test module's outputs as variable
// expressions.  This relies upon the evaluation order and will not sort the test cases
// to run in the dependent order.
//
// In addition, it modifies the provided config so that any variables that are
// available are also defined in the config. It returns a function that resets
// the config which must be called so the config can be reused going forward.
func (runner *TestFileRunner) prepareInputVariablesForAssertions(config *configs.Config, run *moduletest.Run, file *moduletest.File, globals map[string]backend.UnparsedVariableValue) (tofu.InputValues, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ctx, ctxDiags := getEvalContextForTest(runner.States, config, globals)
	diags = diags.Append(ctxDiags)

	variables := make(map[string]backend.UnparsedVariableValue)

	if run != nil {
		for name, expr := range run.Config.Variables {
			variables[name] = testVariableValueExpression{
				expr:       expr,
				sourceType: tofu.ValueFromConfig,
				ctx:        ctx,
			}
		}
	}

	if file != nil {
		for name, expr := range file.Config.Variables {
			if _, exists := variables[name]; exists {
				// Then this variable was defined at the run level and we want
				// that value to take precedence.
				continue
			}
			variables[name] = testVariableValueExpression{
				expr:       expr,
				sourceType: tofu.ValueFromConfig,
				ctx:        ctx,
			}
		}
	}

	for name, variable := range globals {
		if _, exists := variables[name]; exists {
			// Then this value was already defined at either the run level
			// or the file level, and we want those values to take
			// precedence.
			continue
		}
		variables[name] = variable
	}

	// We've gathered all the values we have, let's convert them into
	// tofu.InputValues so they can be passed into the OpenTofu graph.
	// Also, apply default values from the configuration variables where applicable.
	inputs, valDiags := parseAndApplyDefaultValues(variables, config.Module.Variables)
	diags.Append(valDiags)

	// Finally, we're going to do a some modifications to the config.
	// If we have got variable values from the test file we need to make sure
	// they have an equivalent entry in the configuration. We're going to do
	// that dynamically here.

	// First, take a backup of the existing configuration so we can easily
	// restore it later.
	currentVars := make(map[string]*configs.Variable)
	for name, variable := range config.Module.Variables {
		currentVars[name] = variable
	}

	// Next, let's go through our entire inputs and add any that aren't already
	// defined into the config.
	for name, value := range inputs {
		if _, exists := config.Module.Variables[name]; exists {
			continue
		}

		config.Module.Variables[name] = &configs.Variable{
			Name:           name,
			Type:           value.Value.Type(),
			ConstraintType: value.Value.Type(),
			DeclRange:      value.SourceRange.ToHCL(),
		}
	}

	// We return our input values, a function that will reset the variables
	// within the config so it can be used again, and any diagnostics reporting
	// variables that we couldn't parse.

	return inputs, func() {
		config.Module.Variables = currentVars
	}, diags
}

// checkProblematicPlanErrors checks for plan errors that are also "expected" by the tests. In some cases we expect an error, however,
// what causes the error might not be what we expected. So we try to warn about that here.
func checkProblematicPlanErrors(expectedFailures addrs.Map[addrs.Referenceable, bool], planDiags tfdiags.Diagnostics) tfdiags.Diagnostics {
	for _, diag := range planDiags {
		rule, ok := addrs.DiagnosticOriginatesFromCheckRule(diag)
		if !ok {
			continue
		}
		if rule.Container.CheckableKind() != addrs.CheckableInputVariable {
			continue
		}

		addr, ok := rule.Container.(addrs.AbsInputVariableInstance)
		if ok && expectedFailures.Has(addr.Variable) {
			planDiags = planDiags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Invalid Variable in test file",
				fmt.Sprintf("Variable %s, has an invalid value within the test. Although this was an expected failure, it has meant the apply stage was unable to run so the overall test will fail.", rule.Container.String())))
		}
	}
	return planDiags
}

// simulateStateSerialization takes a state, serializes it, deserializes it
// and then returns. This is useful for state writing side effects without
// actually writing a state file.
func simulateStateSerialization(state *states.State) (*states.State, error) {
	buff := &bytes.Buffer{}

	f := statefile.New(state, "", 0)

	err := statefile.Write(f, buff, encryption.StateEncryptionDisabled())
	if err != nil {
		return nil, fmt.Errorf("writing state to buffer: %w", err)
	}

	f, err = statefile.Read(buff, encryption.StateEncryptionDisabled())
	if err != nil {
		return nil, fmt.Errorf("reading state from buffer: %w", err)
	}

	return f.State, nil
}
