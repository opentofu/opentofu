// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/parser"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// TestCommand represents the OpenTofu a given run block will execute, plan
// or apply. Defaults to apply.
type TestCommand rune

// TestMode represents the plan mode that OpenTofu will use for a given run
// block, normal or refresh-only. Defaults to normal.
type TestMode rune

const (
	// ApplyTestCommand causes the run block to execute a OpenTofu apply
	// operation.
	ApplyTestCommand TestCommand = 0

	// PlanTestCommand causes the run block to execute a OpenTofu plan
	// operation.
	PlanTestCommand TestCommand = 'P'

	// NormalTestMode causes the run block to execute in plans.NormalMode.
	NormalTestMode TestMode = 0

	// RefreshOnlyTestMode causes the run block to execute in
	// plans.RefreshOnlyMode.
	RefreshOnlyTestMode TestMode = 'R'
)

// TestFile represents a single test file within a `tofu test` execution.
//
// A test file is made up of a sequential list of run blocks, each designating
// a command to execute and a series of validations to check after the command.
type TestFile struct {
	// Variables defines a set of global variable definitions that should be set
	// for every run block within the test file.
	Variables map[string]hcl.Expression

	// Providers defines a set of providers that are available to run blocks
	// within this test file.
	//
	// If empty, tests should use the default providers for the module under
	// test.
	Providers map[string]*Provider

	// Runs defines the sequential list of run blocks that should be executed in
	// order.
	Runs []*TestRun

	// OverrideResources is a list of resources to be overridden with static values.
	// Underlying providers shouldn't be called for overridden resources.
	OverrideResources []*OverrideResource

	// OverrideModules is a list of modules to be overridden with static values.
	// Underlying modules shouldn't be called.
	OverrideModules []*OverrideModule

	// MockProviders is a map of providers that should be mocked. It is merged
	// with Providers map to use later when instantiating provider instance.
	MockProviders map[string]*MockProvider

	VariablesDeclRange hcl.Range
}

// Validate does a very simple and cursory check across the file blocks to look
// for simple issues we can highlight early on. It doesn't validate nested run blocks.
func (file *TestFile) Validate() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// It's not allowed to have multiple `override_resource`, `override_data` or `override_module` blocks
	// declared globally in a file with the same target address so we want to ensure there's no such cases.
	diags = diags.Append(checkForDuplicatedOverrideResources(file.OverrideResources))
	diags = diags.Append(checkForDuplicatedOverrideModules(file.OverrideModules))

	return diags
}

func (file *TestFile) getTestProviderOrMock(addr string) (*Provider, bool) {
	testProvider, ok := file.Providers[addr]
	if ok {
		return testProvider, true
	}

	mockProvider, ok := file.MockProviders[addr]
	if ok {
		p := &Provider{
			Name:              mockProvider.Name,
			NameRange:         mockProvider.NameRange,
			Alias:             mockProvider.Alias,
			AliasRange:        mockProvider.AliasRange,
			DeclRange:         mockProvider.DeclRange,
			IsMocked:          true,
			MockResources:     mockProvider.MockResources,
			OverrideResources: mockProvider.OverrideResources,
		}

		return p, true
	}

	return nil, false
}

// TestRun represents a single run block within a test file.
//
// Each run block represents a single OpenTofu command to be executed and a set
// of validations to run after the command.
type TestRun struct {
	Name string

	// Command is the OpenTofu command to execute.
	//
	// One of ['apply', 'plan'].
	Command TestCommand

	// Options contains the embedded plan options that will affect the given
	// Command. These should map to the options documented here:
	//   - https://opentofu.org/docs/cli/commands/plan/#planning-options
	//
	// Note, that the Variables are a top level concept and not embedded within
	// the options despite being listed as plan options in the documentation.
	Options *TestRunOptions

	// Variables defines a set of variable definitions for this command.
	//
	// Any variables specified locally that clash with the global variables will
	// take precedence over the global definition.
	Variables map[string]hcl.Expression

	// Providers specifies the set of providers that should be loaded into the
	// module for this run block.
	//
	// Providers specified here must be configured in one of the provider blocks
	// for this file. If empty, the run block will load the default providers
	// for the module under test.
	Providers []PassedProviderConfig

	// CheckRules defines the list of assertions/validations that should be
	// checked by this run block.
	CheckRules []*CheckRule

	// Module defines an address of another module that should be loaded and
	// executed as part of this run block instead of the module under test.
	//
	// We support loading from all the module sources, like local directories, registry,
	// generic git repos, github, bitbucket, s3 and gcs repositories.
	Module *TestRunModuleCall

	// ConfigUnderTest describes the configuration this run block should execute
	// against.
	//
	// In typical cases, this will be null and the config under test is the
	// configuration within the directory the tofu test command is
	// executing within. However, when Module is set the config under test is
	// whichever config is defined by Module. This field is then set during the
	// configuration load process and should be used when the test is executed.
	ConfigUnderTest *Config

	// ExpectFailures should be a list of checkable objects that are expected
	// to report a failure from their custom conditions as part of this test
	// run.
	ExpectFailures []hcl.Traversal

	// OverrideResources is a list of resources to be overridden with static values.
	// Underlying providers shouldn't be called for overridden resources.
	OverrideResources []*OverrideResource

	// OverrideModules is a list of modules to be overridden with static values.
	// Underlying modules shouldn't be called.
	OverrideModules []*OverrideModule

	NameDeclRange      hcl.Range
	VariablesDeclRange hcl.Range
	DeclRange          hcl.Range
}

// Validate does a very simple and cursory check across the run block to look
// for simple issues we can highlight early on.
func (run *TestRun) Validate() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// We want to make sure all the ExpectFailure references
	// are the correct kind of reference.
	for _, traversal := range run.ExpectFailures {

		reference, refDiags := addrs.ParseRefFromTestingScope(traversal)
		diags = diags.Append(refDiags)
		if refDiags.HasErrors() {
			continue
		}

		switch reference.Subject.(type) {
		// You can only reference outputs, inputs, checks, and resources.
		case addrs.OutputValue, addrs.InputVariable, addrs.Check, addrs.ResourceInstance, addrs.Resource:
			// Do nothing, these are okay!
		default:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid `expect_failures` reference",
				Detail:   fmt.Sprintf("You cannot expect failures from %s. You can only expect failures from checkable objects such as input variables, output values, check blocks, managed resources and data sources.", reference.Subject.String()),
				Subject:  reference.SourceRange.ToHCL().Ptr(),
			})
		}

	}

	// It's not allowed to have multiple `override_resource`, `override_data` or `override_module` blocks
	// inside a single run block with the same target address so we want to ensure there's no such cases.
	diags = diags.Append(checkForDuplicatedOverrideResources(run.OverrideResources))
	diags = diags.Append(checkForDuplicatedOverrideModules(run.OverrideModules))

	return diags
}

// TestRunModuleCall specifies which module should be executed by a given run
// block.
type TestRunModuleCall struct {
	// Source is the source of the module to test.
	Source addrs.ModuleSource

	// Version is the version of the module to load from the registry.
	Version VersionConstraint

	DeclRange       hcl.Range
	SourceDeclRange hcl.Range
}

// TestRunOptions contains the plan options for a given run block.
type TestRunOptions struct {
	// Mode is the planning mode to run in. One of ['normal', 'refresh-only'].
	Mode TestMode

	// Refresh is analogous to the -refresh=false OpenTofu plan option.
	Refresh bool

	// Replace is analogous to the -refresh=ADDRESS OpenTofu plan option.
	Replace []hcl.Traversal

	// Target is analogous to the -target=ADDRESS OpenTofu plan option.
	Target []hcl.Traversal

	DeclRange hcl.Range
}

const (
	blockNameOverrideResource = "override_resource"
	blockNameOverrideData     = "override_data"
	//blockNameOverrideEphemeral = "override_ephemeral" // TODO ephemeral uncomment this when testing support will be added for ephemerals
)

// OverrideResource contains information about a resource or data block to be overridden.
type OverrideResource struct {
	// Target references resource or data block to override.
	Target       hcl.Traversal
	TargetParsed *addrs.ConfigResource

	// Mode indicates if the Target is resource or data block.
	Mode addrs.ResourceMode

	// Values represents fields to use as defaults
	// if they are not present in configuration.
	Values map[string]cty.Value
}

func (r OverrideResource) getBlockName() string {
	switch r.Mode {
	case addrs.ManagedResourceMode:
		return blockNameOverrideResource
	case addrs.DataResourceMode:
		return blockNameOverrideData
	case addrs.InvalidResourceMode:
		panic("BUG: invalid resource mode in override resource")
	default:
		panic("BUG: undefined resource mode in override resource: " + r.Mode.String())
	}
}

const blockNameOverrideModule = "override_module"

// OverrideModule contains information about a module to be overridden.
type OverrideModule struct {
	// Target references module call to override.
	Target       hcl.Traversal
	TargetParsed addrs.Module

	// Outputs represents fields to use instead
	// of the real module call output.
	Outputs map[string]cty.Value
}

const blockNameMockProvider = "mock_provider"

// MockProvider represents mocked provider block. It partially matches
// the Provider configuration block (name, alias) and includes additional
// mocking data (mock resources).
type MockProvider struct {
	// Fields below are copied from configs.Provider struct:

	Name       string
	NameRange  hcl.Range
	Alias      string
	AliasRange *hcl.Range // nil if no alias set

	DeclRange hcl.Range

	// Fields below are specific to configs.MockProvider:

	MockResources     []*MockResource
	OverrideResources []*OverrideResource
}

// moduleUniqueKey is copied from Provider.moduleUniqueKey
func (p *MockProvider) moduleUniqueKey() string {
	if p.Alias != "" {
		return fmt.Sprintf("%s.%s", p.Name, p.Alias)
	}
	return p.Name
}

func (p *MockProvider) validateMockResources() hcl.Diagnostics {
	var diags hcl.Diagnostics

	managedResources := make(map[string]struct{})
	dataResources := make(map[string]struct{})

	for _, res := range p.MockResources {
		resources := managedResources
		if res.Mode == addrs.DataResourceMode {
			resources = dataResources
		}

		if _, ok := resources[res.Type]; ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Duplicated `%v` block", res.getBlockName()),
				Detail:   fmt.Sprintf("`%v.%v` is already defined in `mock_provider` block.", res.getBlockName(), res.Type),
				Subject:  p.DeclRange.Ptr(),
			})
			continue
		}

		resources[res.Type] = struct{}{}
	}

	return diags
}

func (p *MockProvider) validateOverrideResources() hcl.Diagnostics {
	var diags hcl.Diagnostics

	resources := make(map[string]struct{})

	for _, res := range p.OverrideResources {
		k := res.TargetParsed.String()

		if _, ok := resources[k]; ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Duplicated `%v` block", res.getBlockName()),
				Detail:   fmt.Sprintf("`%v` with target `%v` is already defined in `mock_provider` block.", res.getBlockName(), k),
				Subject:  p.DeclRange.Ptr(),
			})
			continue
		}

		resources[k] = struct{}{}
	}

	return diags
}

const (
	blockNameMockResource = "mock_resource"
	blockNameMockData     = "mock_data"
)

// MockResource represents mocked resource. It is similar to OverrideResource,
// except all the resources with the same type should be overridden (mocked).
type MockResource struct {
	Mode     addrs.ResourceMode
	Type     string
	Defaults map[string]cty.Value
}

func (r MockResource) getBlockName() string {
	switch r.Mode {
	case addrs.ManagedResourceMode:
		return blockNameMockResource
	case addrs.DataResourceMode:
		return blockNameMockData
	case addrs.InvalidResourceMode:
		panic("BUG: invalid resource mode in mock resource")
	default:
		panic("BUG: undefined resource mode in mock resource: " + r.Mode.String())
	}
}

func loadTestFile(body hcl.Body) (*TestFile, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	var parsed parser.TestFile
	decodeDiags := gohcl.DecodeBody(body, nil, &parsed)
	diags = append(diags, decodeDiags...)

	tf := TestFile{
		Providers:     make(map[string]*Provider),
		MockProviders: make(map[string]*MockProvider),
	}

	for _, block := range parsed.Runs {
		run, runDiags := decodeTestRunBlock(block)
		diags = append(diags, runDiags...)
		if !runDiags.HasErrors() {
			tf.Runs = append(tf.Runs, run)
		}
	}

	for _, block := range parsed.Variables {
		if tf.Variables != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Multiple \"variables\" blocks",
				Detail:   fmt.Sprintf("This test file already has a variables block defined at %s.", tf.VariablesDeclRange),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		tf.Variables = make(map[string]hcl.Expression)
		tf.VariablesDeclRange = block.DefRange

		vars, varsDiags := block.Body.JustAttributes()
		diags = append(diags, varsDiags...)
		for _, v := range vars {
			tf.Variables[v.Name] = v.Expr
		}
	}

	for _, block := range parsed.ProviderConfigs {
		provider, providerDiags := decodeProviderBlock(block)
		diags = append(diags, providerDiags...)
		if provider != nil {
			tf.Providers[provider.moduleUniqueKey()] = provider
		}
	}

	for _, block := range parsed.OverrideResources {
		overrideRes, overrideResDiags := decodeOverrideResourceBlock(block, addrs.ManagedResourceMode)
		diags = append(diags, overrideResDiags...)
		if !overrideResDiags.HasErrors() {
			tf.OverrideResources = append(tf.OverrideResources, overrideRes)
		}
	}

	for _, block := range parsed.OverrideData {
		overrideRes, overrideResDiags := decodeOverrideResourceBlock(block, addrs.DataResourceMode)
		diags = append(diags, overrideResDiags...)
		if !overrideResDiags.HasErrors() {
			tf.OverrideResources = append(tf.OverrideResources, overrideRes)
		}
	}

	for _, block := range parsed.OverrideModules {
		overrideMod, overrideModDiags := decodeOverrideModuleBlock(block)
		diags = append(diags, overrideModDiags...)
		if !overrideModDiags.HasErrors() {
			tf.OverrideModules = append(tf.OverrideModules, overrideMod)
		}
	}

	for _, block := range parsed.MockProviders {
		mockProvider, mockProviderDiags := decodeMockProviderBlock(block)
		diags = append(diags, mockProviderDiags...)

		if !mockProviderDiags.HasErrors() {
			k := mockProvider.moduleUniqueKey()

			if _, ok := tf.MockProviders[k]; ok {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicated `mock_provider` block",
					Detail:   fmt.Sprintf("It is not allowed to have multiple `mock_provider` blocks with the same address: `%v`.", k),
					Subject:  mockProvider.DeclRange.Ptr(),
				})
			} else {
				tf.MockProviders[k] = mockProvider
			}
		}
	}

	return &tf, diags
}

func decodeTestRunBlock(block *parser.TestRun) (*TestRun, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	r := TestRun{
		Name:          block.Name,
		NameDeclRange: block.NameRange,
		DeclRange:     block.DefRange,
	}

	if !hclsyntax.ValidIdentifier(r.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid run block name",
			Detail:   badIdentifierDetail,
			Subject:  &block.NameRange,
		})
	}

	for _, block := range block.Asserts {
		cr, crDiags := decodeCheckRuleBlock(block, "assert", false)
		diags = append(diags, crDiags...)
		if !crDiags.HasErrors() {
			r.CheckRules = append(r.CheckRules, cr)
		}
	}

	for _, block := range block.PlanOptions {
		if r.Options != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Multiple \"plan_options\" blocks",
				Detail:   fmt.Sprintf("This run block already has a plan_options block defined at %s.", r.Options.DeclRange),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		opts, optsDiags := decodeTestRunOptionsBlock(block)
		diags = append(diags, optsDiags...)
		if !optsDiags.HasErrors() {
			r.Options = opts
		}
	}

	for _, block := range block.Variables {
		if r.Variables != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Multiple \"variables\" blocks",
				Detail:   fmt.Sprintf("This run block already has a variables block defined at %s.", r.VariablesDeclRange),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		r.Variables = make(map[string]hcl.Expression)
		r.VariablesDeclRange = block.DefRange

		vars, varsDiags := block.Body.JustAttributes()
		diags = append(diags, varsDiags...)
		for _, v := range vars {
			r.Variables[v.Name] = v.Expr
		}
	}

	for _, block := range block.Module {
		if r.Module != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Multiple \"module\" blocks",
				Detail:   fmt.Sprintf("This run block already has a module block defined at %s.", r.Module.DeclRange),
				Subject:  block.DefRange.Ptr(),
			})
		}

		module, moduleDiags := decodeTestRunModuleBlock(block)
		diags = append(diags, moduleDiags...)
		if !moduleDiags.HasErrors() {
			r.Module = module
		}
	}

	for _, block := range block.OverrideResources {
		overrideRes, overrideResDiags := decodeOverrideResourceBlock(block, addrs.ManagedResourceMode)
		diags = append(diags, overrideResDiags...)
		if !overrideResDiags.HasErrors() {
			r.OverrideResources = append(r.OverrideResources, overrideRes)
		}
	}
	for _, block := range block.OverrideData {
		overrideRes, overrideResDiags := decodeOverrideResourceBlock(block, addrs.DataResourceMode)
		diags = append(diags, overrideResDiags...)
		if !overrideResDiags.HasErrors() {
			r.OverrideResources = append(r.OverrideResources, overrideRes)
		}
	}

	for _, block := range block.OverrideModules {
		overrideMod, overrideModDiags := decodeOverrideModuleBlock(block)
		diags = append(diags, overrideModDiags...)
		if !overrideModDiags.HasErrors() {
			r.OverrideModules = append(r.OverrideModules, overrideMod)
		}
	}

	if r.Variables == nil {
		// There is no distinction between a nil map of variables or an empty
		// map, but we can avoid any potential nil pointer exceptions by just
		// creating an empty map.
		r.Variables = make(map[string]hcl.Expression)
	}

	if r.Options == nil {
		// Create an options with default values if the user didn't specify
		// anything.
		r.Options = &TestRunOptions{
			Mode:    NormalTestMode,
			Refresh: true,
		}
	}

	if block.Command != nil {
		switch hcl.ExprAsKeyword(block.Command.Expr) {
		case "apply":
			r.Command = ApplyTestCommand
		case "plan":
			r.Command = PlanTestCommand
		default:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid \"command\" keyword",
				Detail:   "The \"command\" argument requires one of the following keywords without quotes: apply or plan.",
				Subject:  block.Command.Expr.Range().Ptr(),
			})
		}
	} else {
		r.Command = ApplyTestCommand // Default to apply
	}

	if block.Providers != nil {
		providers, providerDiags := decodePassedProviderConfigs(block.Providers)
		diags = append(diags, providerDiags...)
		r.Providers = append(r.Providers, providers...)
	}

	if block.ExpectFailures != nil {
		failures, failDiags := decodeDependsOn(block.ExpectFailures)
		diags = append(diags, failDiags...)
		r.ExpectFailures = failures
	}

	return &r, diags
}

func decodeTestRunModuleBlock(block *parser.TestRunModule) (*TestRunModuleCall, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	module := TestRunModuleCall{
		DeclRange: block.DefRange,
	}

	haveVersionArg := false
	if block.Version != nil {
		var versionDiags hcl.Diagnostics
		module.Version, versionDiags = decodeVersionConstraint(block.Version)
		diags = append(diags, versionDiags...)
		haveVersionArg = true
	}

	if attr := block.Source; attr != nil {
		module.SourceDeclRange = attr.Range

		var raw string
		rawDiags := gohcl.DecodeExpression(attr.Expr, nil, &raw)
		diags = append(diags, rawDiags...)
		if !rawDiags.HasErrors() {
			var err error
			if haveVersionArg {
				module.Source, err = addrs.ParseModuleSourceRegistry(raw)
			} else {
				module.Source, err = addrs.ParseModuleSource(raw)
			}
			if err != nil {
				// NOTE: We leave mc.SourceAddr as nil for any situation where the
				// source attribute is invalid, so any code which tries to carefully
				// use the partial result of a failed config decode must be
				// resilient to that.
				module.Source = nil

				// NOTE: In practice it's actually very unlikely to end up here,
				// because our source address parser can turn just about any string
				// into some sort of remote package address, and so for most errors
				// we'll detect them only during module installation. There are
				// still a _few_ purely-syntax errors we can catch at parsing time,
				// though, mostly related to remote package sub-paths and local
				// paths.
				switch err := err.(type) {
				case *getmodules.MaybeRelativePathErr:
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid module source address",
						Detail: fmt.Sprintf(
							"OpenTofu failed to determine your intended installation method for remote module package %q.\n\nIf you intended this as a path relative to the current module, use \"./%s\" instead. The \"./\" prefix indicates that the address is a relative filesystem path.",
							err.Addr, err.Addr,
						),
						Subject: module.SourceDeclRange.Ptr(),
					})
				default:
					if haveVersionArg {
						// In this case we'll include some extra context that
						// we assumed a registry source address due to the
						// version argument.
						diags = append(diags, &hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid registry module source address",
							Detail:   fmt.Sprintf("Failed to parse module registry address: %s.\n\nOpenTofu assumed that you intended a module registry source address because you also set the argument \"version\", which applies only to registry modules.", err),
							Subject:  module.SourceDeclRange.Ptr(),
						})
					} else {
						diags = append(diags, &hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid module source address",
							Detail:   fmt.Sprintf("Failed to parse module source address: %s.", err),
							Subject:  module.SourceDeclRange.Ptr(),
						})
					}
				}
			}
		}
	} else {
		// Must have a source attribute.
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing \"source\" attribute for module block",
			Detail:   "You must specify a source attribute when executing alternate modules during test executions.",
			Subject:  module.DeclRange.Ptr(),
		})
	}

	return &module, diags
}

func decodeTestRunOptionsBlock(block *parser.TestRunOptions) (*TestRunOptions, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	opts := TestRunOptions{
		DeclRange: block.DefRange,
	}

	if attr := block.Mode; attr != nil {
		switch hcl.ExprAsKeyword(attr.Expr) {
		case "refresh-only":
			opts.Mode = RefreshOnlyTestMode
		case "normal":
			opts.Mode = NormalTestMode
		default:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid \"mode\" keyword",
				Detail:   "The \"mode\" argument requires one of the following keywords without quotes: normal or refresh-only",
				Subject:  attr.Expr.Range().Ptr(),
			})
		}
	} else {
		opts.Mode = NormalTestMode // Default to normal
	}

	if block.Refresh != nil {
		opts.Refresh = *block.Refresh
	} else {
		// Defaults to true.
		opts.Refresh = true
	}

	if block.Replace != nil {
		reps, repsDiags := decodeDependsOn(block.Replace)
		diags = append(diags, repsDiags...)
		opts.Replace = reps
	}

	if block.Target != nil {
		tars, tarsDiags := decodeDependsOn(block.Target)
		diags = append(diags, tarsDiags...)
		opts.Target = tars
	}

	if !opts.Refresh && opts.Mode == RefreshOnlyTestMode {
		// These options are incompatible.
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Incompatible plan options",
			Detail:   "The \"refresh\" option cannot be set to false when running a test in \"refresh-only\" mode.",
			Subject:  block.RefreshRange.Ptr(),
		})
	}

	return &opts, diags
}

func decodeOverrideResourceBlock(block *parser.OverrideResource, mode addrs.ResourceMode) (*OverrideResource, hcl.Diagnostics) {
	parseTarget := func(attr *hcl.Attribute) (hcl.Traversal, *addrs.ConfigResource, hcl.Diagnostics) {
		traversal, traversalDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags := traversalDiags
		if traversalDiags.HasErrors() {
			return nil, nil, diags
		}

		configRes, configResDiags := addrs.ParseConfigResource(traversal)
		diags = append(diags, configResDiags.ToHCL()...)
		if configResDiags.HasErrors() {
			return nil, nil, diags
		}

		return traversal, &configRes, diags
	}

	var diags hcl.Diagnostics
	res := &OverrideResource{
		Mode: mode,
	}

	target, parsed, moreDiags := parseTarget(&block.Target)
	res.Target, res.TargetParsed = target, parsed
	diags = append(diags, moreDiags...)

	if block.Values != nil {
		v, moreDiags := parseObjectAttrWithNoVariables(block.Values)
		res.Values, diags = v, append(diags, moreDiags...)
	}

	return res, diags
}

func decodeOverrideModuleBlock(block *parser.OverrideModule) (*OverrideModule, hcl.Diagnostics) {
	parseTarget := func(attr *hcl.Attribute) (hcl.Traversal, addrs.Module, hcl.Diagnostics) {
		traversal, traversalDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags := traversalDiags
		if traversalDiags.HasErrors() {
			return nil, nil, diags
		}

		target, targetDiags := addrs.ParseModule(traversal)
		diags = append(diags, targetDiags.ToHCL()...)
		if targetDiags.HasErrors() {
			return nil, nil, diags
		}

		return traversal, target, diags
	}

	var diags hcl.Diagnostics
	mod := &OverrideModule{}

	traversal, target, moreDiags := parseTarget(&block.Target)
	mod.Target, mod.TargetParsed = traversal, target
	diags = append(diags, moreDiags...)

	if block.Outputs != nil {
		outputs, moreDiags := parseObjectAttrWithNoVariables(block.Outputs)
		mod.Outputs, diags = outputs, append(diags, moreDiags...)
	}

	return mod, diags
}

// Some code of decodeMockProviderBlock function was copied from decodeProviderBlock.
func decodeMockProviderBlock(block *parser.MockProvider) (*MockProvider, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	// Provider names must be localized. Produce an error with a message
	// indicating the action the user can take to fix this message if the local
	// name is not localized.
	name := block.Name
	nameDiags := checkProviderNameNormalized(name, block.DefRange)
	diags = append(diags, nameDiags...)
	if nameDiags.HasErrors() {
		// If the name is invalid then we mustn't produce a result because
		// downstreams could try to use it as a provider type and then crash.
		return nil, diags
	}

	provider := &MockProvider{
		Name:      name,
		NameRange: block.NameRange,
		DeclRange: block.DefRange,
	}

	if block.Alias != nil {
		provider.Alias = *block.Alias
		provider.AliasRange = block.AliasRange.Ptr()

		if !hclsyntax.ValidIdentifier(provider.Alias) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid mock provider configuration alias",
				Detail:   fmt.Sprintf("An alias must be a valid name. %s", badIdentifierDetail),
				Subject:  provider.AliasRange,
			})
		}
	}

	for _, block := range block.MockResources {
		res, resDiags := decodeMockResourceBlock(block, addrs.ManagedResourceMode)
		diags = append(diags, resDiags...)
		if !resDiags.HasErrors() {
			provider.MockResources = append(provider.MockResources, res)
		}
	}
	for _, block := range block.MockData {
		res, resDiags := decodeMockResourceBlock(block, addrs.DataResourceMode)
		diags = append(diags, resDiags...)
		if !resDiags.HasErrors() {
			provider.MockResources = append(provider.MockResources, res)
		}
	}

	for _, block := range block.OverrideResources {
		res, resDiags := decodeOverrideResourceBlock(block, addrs.ManagedResourceMode)
		diags = append(diags, resDiags...)
		if !resDiags.HasErrors() {
			provider.OverrideResources = append(provider.OverrideResources, res)
		}
	}

	for _, block := range block.OverrideResources {
		res, resDiags := decodeOverrideResourceBlock(block, addrs.DataResourceMode)
		diags = append(diags, resDiags...)
		if !resDiags.HasErrors() {
			provider.OverrideResources = append(provider.OverrideResources, res)
		}
	}

	diags = append(diags, provider.validateMockResources()...)
	diags = append(diags, provider.validateOverrideResources()...)

	return provider, diags
}

func decodeMockResourceBlock(block *parser.MockResource, mode addrs.ResourceMode) (*MockResource, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	res := &MockResource{
		Mode: mode,
		Type: block.Type,
	}

	if block.Defaults != nil {
		v, moreDiags := parseObjectAttrWithNoVariables(block.Defaults)
		res.Defaults, diags = v, append(diags, moreDiags...)
	}

	return res, diags
}

func parseObjectAttrWithNoVariables(attr *hcl.Attribute) (map[string]cty.Value, hcl.Diagnostics) {
	attrVal, valDiags := attr.Expr.Value(nil)
	diags := valDiags
	if valDiags.HasErrors() {
		return nil, diags
	}

	if !attrVal.Type().IsObjectType() {
		return nil, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Object expected",
			Detail:   fmt.Sprintf("The attribute `%v` must be an object.", attr.Name),
			Subject:  attr.Range.Ptr(),
		})
	}

	return attrVal.AsValueMap(), diags
}

func checkForDuplicatedOverrideResources(resources []*OverrideResource) hcl.Diagnostics {
	var diags hcl.Diagnostics

	overrideResources := make(map[string]struct{}, len(resources))
	for _, res := range resources {
		k := res.TargetParsed.String()

		if _, ok := overrideResources[k]; ok {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Duplicated `%v` block", res.getBlockName()),
				Detail:   fmt.Sprintf("It is not allowed to have multiple `%v` blocks with the same target: `%v`.", res.getBlockName(), res.TargetParsed),
				Subject:  res.Target.SourceRange().Ptr(),
			})
			continue
		}

		overrideResources[k] = struct{}{}
	}

	return diags
}

func checkForDuplicatedOverrideModules(modules []*OverrideModule) hcl.Diagnostics {
	var diags hcl.Diagnostics

	overrideModules := make(map[string]struct{}, len(modules))
	for _, mod := range modules {
		k := mod.TargetParsed.String()

		if _, ok := overrideModules[k]; ok {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicated `override_module` block",
				Detail:   fmt.Sprintf("It is not allowed to have multiple `override_module` blocks with the same target: `%v`.", mod.TargetParsed),
				Subject:  mod.Target.SourceRange().Ptr(),
			})
			continue
		}

		overrideModules[k] = struct{}{}
	}

	return diags
}
