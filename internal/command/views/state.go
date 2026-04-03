// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"context"
	"fmt"
	"os"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/jsonformat"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/command/jsonstate"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

type State interface {
	Diagnostics(diags tfdiags.Diagnostics)

	// General `state` output
	StateNotFound()
	StateLoadingFailure(baseError string)
	StateSavingError(baseError string)

	// `tofu state list` specific
	StateListAddr(resAddr addrs.AbsResourceInstance)

	// `tofu state mv` specific
	ErrorMovingToAlreadyExistingDst()
	ResourceMoveStatus(dryRun bool, src, dest string)
	DryRunMovedStatus(moved int)
	MoveFinalStatus(moved int)

	// `tofu state pull` specific
	PrintPulledState(state string)

	// `tofu state replace-provider` specific
	NoMatchingResourcesForProviderReplacement()
	ReplaceProviderOverview(from, to addrs.Provider, willReplace []*states.Resource)
	ReplaceProviderCancelled()
	ProviderReplaced(forResources int)

	// `tofu state rm` specific
	ResourceRemoveStatus(dryRun bool, target string)
	DryRunRemovedStatus(removed int)
	RemoveFinalStatus(count int)

	// `tofu state show` specific
	UnsupportedLocalOp()
	AddressParsingError(resAddr string)
	NoInstanceFoundError()
	ShowResourceState(ctx context.Context, stateFile *statefile.File, schemas *tofu.Schemas) int

	// Backend returns the non-command view that contains methods to provide
	// progress output for the backend operations.
	Backend() Backend
}

// NewState returns an initialized State implementation for the given ViewType.
func NewState(args arguments.ViewOptions, view *View) State {
	var ret State
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &StateJSON{view: NewJSONView(view, nil), output: view.streams.Stdout.File}
	case arguments.ViewHuman:
		ret = &StateHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &StateMulti{ret, &StateJSON{view: NewJSONView(view, args.JSONInto), output: args.JSONInto}}
	}
	return ret
}

type StateMulti []State

var _ State = (StateMulti)(nil)

func (m StateMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m StateMulti) StateNotFound() {
	for _, o := range m {
		o.StateNotFound()
	}
}

func (m StateMulti) StateLoadingFailure(baseError string) {
	for _, o := range m {
		o.StateLoadingFailure(baseError)
	}
}

func (m StateMulti) StateSavingError(baseError string) {
	for _, o := range m {
		o.StateSavingError(baseError)
	}
}

func (m StateMulti) StateListAddr(resAddr addrs.AbsResourceInstance) {
	for _, o := range m {
		o.StateListAddr(resAddr)
	}
}

func (m StateMulti) ErrorMovingToAlreadyExistingDst() {
	for _, o := range m {
		o.ErrorMovingToAlreadyExistingDst()
	}
}

func (m StateMulti) ResourceMoveStatus(dryRun bool, src, dest string) {
	for _, o := range m {
		o.ResourceMoveStatus(dryRun, src, dest)
	}
}

func (m StateMulti) DryRunMovedStatus(moved int) {
	for _, o := range m {
		o.DryRunMovedStatus(moved)
	}
}

func (m StateMulti) MoveFinalStatus(moved int) {
	for _, o := range m {
		o.MoveFinalStatus(moved)
	}
}

func (m StateMulti) PrintPulledState(state string) {
	for _, o := range m {
		o.PrintPulledState(state)
	}
}

func (m StateMulti) NoMatchingResourcesForProviderReplacement() {
	for _, o := range m {
		o.NoMatchingResourcesForProviderReplacement()
	}
}

func (m StateMulti) ReplaceProviderOverview(from, to addrs.Provider, willReplace []*states.Resource) {
	for _, o := range m {
		o.ReplaceProviderOverview(from, to, willReplace)
	}
}

func (m StateMulti) ReplaceProviderCancelled() {
	for _, o := range m {
		o.ReplaceProviderCancelled()
	}
}

func (m StateMulti) ProviderReplaced(forResources int) {
	for _, o := range m {
		o.ProviderReplaced(forResources)
	}
}

func (m StateMulti) ResourceRemoveStatus(dryRun bool, target string) {
	for _, o := range m {
		o.ResourceRemoveStatus(dryRun, target)
	}
}

func (m StateMulti) DryRunRemovedStatus(removed int) {
	for _, o := range m {
		o.DryRunRemovedStatus(removed)
	}
}

func (m StateMulti) RemoveFinalStatus(count int) {
	for _, o := range m {
		o.RemoveFinalStatus(count)
	}
}

func (m StateMulti) UnsupportedLocalOp() {
	for _, o := range m {
		o.UnsupportedLocalOp()
	}
}

func (m StateMulti) AddressParsingError(resAddr string) {
	for _, o := range m {
		o.AddressParsingError(resAddr)
	}
}

func (m StateMulti) NoInstanceFoundError() {
	for _, o := range m {
		o.NoInstanceFoundError()
	}
}

func (m StateMulti) ShowResourceState(ctx context.Context, stateFile *statefile.File, schemas *tofu.Schemas) int {
	var ret int
	for _, o := range m {
		ret = max(ret, o.ShowResourceState(ctx, stateFile, schemas))
	}
	return ret
}

func (m StateMulti) Backend() Backend {
	ret := make([]Backend, len(m))
	for i, v := range m {
		ret[i] = v.Backend()
	}
	return BackendMulti(ret)
}

type StateHuman struct {
	view *View
}

var _ State = (*StateHuman)(nil)

func (v *StateHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *StateHuman) StateNotFound() {
	v.Diagnostics(tfdiags.Diagnostics{diagErrStateNotFound})
}

func (v *StateHuman) StateLoadingFailure(baseError string) {
	v.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Error,
		errStateLoadingStateSummary,
		fmt.Sprintf(errStateLoadingStateDescription, baseError),
	)})
}

func (v *StateHuman) StateSavingError(baseError string) {
	v.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Error,
		errStateRmPersistHeader,
		fmt.Sprintf(errStateRmPersistDescription, baseError),
	)})
}

func (v *StateHuman) StateListAddr(resAddr addrs.AbsResourceInstance) {
	_, _ = v.view.streams.Println(resAddr.String())
}

func (v *StateHuman) ErrorMovingToAlreadyExistingDst() {
	v.Diagnostics(tfdiags.Diagnostics{diagErrStateMvDstExists})
}

func (v *StateHuman) ResourceMoveStatus(dryRun bool, src, dest string) {
	if dryRun {
		_, _ = v.view.streams.Println(fmt.Sprintf("Would move %q to %q", src, dest))
		return
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("Move %q to %q", src, dest))
}

func (v *StateHuman) DryRunMovedStatus(moved int) {
	if moved == 0 {
		_, _ = v.view.streams.Println("Would have moved nothing.")
	}
}

func (v *StateHuman) MoveFinalStatus(moved int) {
	if moved == 0 {
		_, _ = v.view.streams.Println("No matching objects found.")
		return
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("Successfully moved %d object(s).", moved))
}

func (v *StateHuman) PrintPulledState(state string) {
	_, _ = v.view.streams.Println(state)
}

func (v *StateHuman) NoMatchingResourcesForProviderReplacement() {
	_, _ = v.view.streams.Println("No matching resources found.")
}

func (v *StateHuman) ReplaceProviderOverview(from, to addrs.Provider, willReplace []*states.Resource) {
	colorize := v.view.colorize.Color
	printer := func(args ...any) { _, _ = v.view.streams.Println(args...) }
	printer("OpenTofu will perform the following actions:\n")
	printer(colorize("  [yellow]~[reset] Updating provider:"))
	printer(colorize(fmt.Sprintf("    [red]-[reset] %s", from)))
	printer(colorize(fmt.Sprintf("    [green]+[reset] %s\n", to)))

	printer(colorize(fmt.Sprintf("[bold]Changing[reset] %d resources:\n", len(willReplace))))
	for _, resource := range willReplace {
		printer(colorize(fmt.Sprintf("  %s", resource.Addr)))
	}
}

func (v *StateHuman) ReplaceProviderCancelled() {
	_, _ = v.view.streams.Println("Cancelled replacing providers.")
}

func (v *StateHuman) ProviderReplaced(forResources int) {
	_, _ = v.view.streams.Println(fmt.Sprintf("Successfully replaced provider for %d resources.", forResources))
}

func (v *StateHuman) ResourceRemoveStatus(dryRun bool, target string) {
	if dryRun {
		_, _ = v.view.streams.Println(fmt.Sprintf("Would remove %s", target))
		return
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("Removed %s", target))
}

func (v *StateHuman) DryRunRemovedStatus(removed int) {
	if removed == 0 {
		_, _ = v.view.streams.Println("Would have removed nothing.")
	}
}

func (v *StateHuman) RemoveFinalStatus(count int) {
	if count == 0 {
		// NOTE: printing nothing here since this case needs to be handled by the caller
		return
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("Successfully removed %d resource instance(s).", count))
}

func (v *StateHuman) UnsupportedLocalOp() {
	v.Diagnostics(tfdiags.Diagnostics{diagUnsupportedLocalOp})
}

func (v *StateHuman) AddressParsingError(resAddr string) {
	v.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Error,
		fmt.Sprintf(errParsingAddressHeader, resAddr),
		errParsingAddressDescription,
	)})
}

func (v *StateHuman) NoInstanceFoundError() {
	v.Diagnostics(tfdiags.Diagnostics{diagErrNoInstanceFound})
}

func (v *StateHuman) ShowResourceState(_ context.Context, stateFile *statefile.File, schemas *tofu.Schemas) int {
	renderer := jsonformat.Renderer{
		Colorize:            v.view.colorize,
		Streams:             v.view.streams,
		RunningInAutomation: v.view.runningInAutomation,
		ShowSensitive:       v.view.showSensitive,
	}

	if stateFile == nil {
		_, _ = v.view.streams.Println("No state.")
		return 0
	}

	root, outputs, err := jsonstate.MarshalForRenderer(stateFile, schemas)
	if err != nil {
		v.Diagnostics(tfdiags.Diagnostics{}.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to marshal state to json",
			fmt.Sprintf("Error while marshalling state to json: %s", err),
		)))
		return 1
	}

	jstate := jsonformat.State{
		StateFormatVersion:    jsonstate.FormatVersion,
		ProviderFormatVersion: jsonprovider.FormatVersion,
		RootModule:            root,
		RootModuleOutputs:     outputs,
		ProviderSchemas:       jsonprovider.MarshalForRenderer(schemas),
	}

	renderer.RenderHumanState(jstate)
	return 0
}

func (v *StateHuman) Backend() Backend {
	return &BackendHuman{
		view: v.view,
	}
}

type StateJSON struct {
	view   *JSONView
	output *os.File
}

var _ State = (*StateJSON)(nil)

func (v *StateJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *StateJSON) StateNotFound() {
	v.Diagnostics(tfdiags.Diagnostics{diagErrStateNotFound})
}

func (v *StateJSON) StateLoadingFailure(baseError string) {
	v.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Error,
		errStateLoadingStateSummary,
		fmt.Sprintf(errStateLoadingStateDescription, baseError),
	)})
}

func (v *StateJSON) StateSavingError(baseError string) {
	v.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Error,
		errStateRmPersistHeader,
		fmt.Sprintf(errStateRmPersistDescription, baseError),
	)})
}

func (v *StateJSON) StateListAddr(resAddr addrs.AbsResourceInstance) {
	v.view.log.Info(resAddr.String(), "type", "resource_address")
}

func (v *StateJSON) ErrorMovingToAlreadyExistingDst() {
	v.Diagnostics(tfdiags.Diagnostics{diagErrStateMvDstExists})
}

func (v *StateJSON) ResourceMoveStatus(dryRun bool, src, dest string) {
	if dryRun {
		v.view.Info(fmt.Sprintf("Would move %q to %q", src, dest))
		return
	}
	v.view.Info(fmt.Sprintf("Move %q to %q", src, dest))
}

func (v *StateJSON) DryRunMovedStatus(moved int) {
	if moved == 0 {
		v.view.Info("Would have moved nothing")
	}
}

func (v *StateJSON) MoveFinalStatus(moved int) {
	if moved == 0 {
		v.view.Info("No matching objects found")
		return
	}
	v.view.Info(fmt.Sprintf("Successfully moved %d object(s)", moved))
}

func (v *StateJSON) PrintPulledState(_ string) {
	v.view.Error("printing the pulled state is not available in the JSON view. The `tofu state pull` should not be configured with the `-json` flag")
}

func (v *StateJSON) NoMatchingResourcesForProviderReplacement() {
	v.view.log.Info("No matching resources found")
}

func (v *StateJSON) ReplaceProviderOverview(from, to addrs.Provider, willReplace []*states.Resource) {
	replacedResources := make([]string, len(willReplace))
	for i, resource := range willReplace {
		replacedResources[i] = resource.Addr.String()
	}
	msg := fmt.Sprintf("OpenTofu will replace provider from %s to %s for %d resources", from, to, len(willReplace))
	v.view.log.Info(msg, "resources", replacedResources, "type", "replace_provider", "from", from.String(), "to", to.String())
}

func (v *StateJSON) ReplaceProviderCancelled() {
	v.view.Info("Cancelled replacing providers")
}

func (v *StateJSON) ProviderReplaced(forResources int) {
	v.view.Info(fmt.Sprintf("Successfully replaced provider for %d resources", forResources))
}

func (v *StateJSON) ResourceRemoveStatus(dryRun bool, target string) {
	if dryRun {
		v.view.Info(fmt.Sprintf("Would remove %s", target))
		return
	}
	v.view.Info(fmt.Sprintf("Removed %s", target))
}

func (v *StateJSON) DryRunRemovedStatus(removed int) {
	if removed == 0 {
		v.view.Info("Would have removed nothing")
	}
}

func (v *StateJSON) RemoveFinalStatus(count int) {
	if count == 0 {
		// NOTE: printing nothing here since this case needs to be handled by the caller
		return
	}
	v.view.Info(fmt.Sprintf("Successfully removed %d resource instance(s)", count))
}

func (v *StateJSON) UnsupportedLocalOp() {
	v.Diagnostics(tfdiags.Diagnostics{diagUnsupportedLocalOp})
}

func (v *StateJSON) AddressParsingError(resAddr string) {
	v.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Error,
		fmt.Sprintf(errParsingAddressHeader, resAddr),
		errParsingAddressDescription,
	)})
}

func (v *StateJSON) NoInstanceFoundError() {
	v.Diagnostics(tfdiags.Diagnostics{diagErrNoInstanceFound})
}

func (v *StateJSON) ShowResourceState(_ context.Context, stateFile *statefile.File, schemas *tofu.Schemas) int {
	if stateFile == nil {
		v.view.Info("no state")
		return 0
	}

	rawState, err := jsonstate.Marshal(stateFile, schemas)
	if err != nil {
		v.Diagnostics(tfdiags.Diagnostics{}.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to marshal state to json",
			fmt.Sprintf("Error while marshalling state to json: %s", err),
		)))
		return 1
	}
	_, _ = fmt.Fprintln(v.output, string(rawState))
	return 0
}

func (v *StateJSON) Backend() Backend {
	return &BackendJSON{
		view: v.view,
	}
}

var (
	diagErrStateNotFound = tfdiags.Sourceless(
		tfdiags.Error,
		"No state file was found",
		`State management commands require a state file. Run this command in a directory where OpenTofu has been run or use the -state flag to point the command to a specific state location.`,
	)
	diagErrStateMvDstExists = tfdiags.Sourceless(
		tfdiags.Error,
		"Destination module already exists",
		`Please ensure your addresses and state paths are valid. No state was persisted. Your existing states are untouched.`,
	)
	diagErrNoInstanceFound = tfdiags.Sourceless(
		tfdiags.Error,
		"No instance found for the given address",
		`This command requires that the address references one specific instance. To view the available instances, use "tofu state list". Please modify the address to reference a specific instance.`,
	)
)

const (
	errStateLoadingStateSummary     = "Error loading the state"
	errStateLoadingStateDescription = `Please ensure that your OpenTofu state exists and that you've configured it properly. You can use the "-state" flag to point OpenTofu at another state file.

Cause: %s`
)

const (
	errStateRmPersistHeader      = `Error saving the state`
	errStateRmPersistDescription = `The state was not saved. No items were removed from the persisted state. No backup was created since no modification occurred. Please resolve the issue above and try again.

Cause: %s`
)

const (
	errParsingAddressHeader      = `Error parsing instance address %q`
	errParsingAddressDescription = `This command requires that the address references one specific instance. To view the available instances, use "tofu state list". Please modify the address to reference a specific instance.`
)
