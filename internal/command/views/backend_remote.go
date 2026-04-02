// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"time"

	"github.com/opentofu/opentofu/internal/command/jsonformat"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type BackendRemote interface {
	Basic
	Output(msg string, color bool)
	RunWarning(msg string)
	RenderLog(log *jsonformat.JSONLog) error
	RenderHumanPlan(plan jsonformat.Plan, mode plans.Mode)
	OperationCancelled()
	OperationNotCancelled()
	PreRefresh()
	InitialRetryError(isRemote bool)
	RepeatedRetryError(elapsed time.Duration)
	UnavailableVersionInBackend(localVersion, usedVersion string)
	ApplySavedHeader()
	LockTimeoutError()
	RemoteWorkspaceInRelativeDirectory(wd string, configDir string)
	WaitingForCostEstimation(elapsedHint string)
	WaitingForOperationToStart(opType string, elapsed string)
	WaitingForTheManuallyLockedWorkspace(elapsedHint string)
	WaitingForRuns(noOfRuns int, elapsedHint string)
	WaitingForQueuedRuns(noOfRuns int, elapsedHint string)
	OperationHeader(isApply bool, isRemote bool)
}

type BackendRemoteHuman struct {
	view     *View
	renderer *jsonformat.Renderer
}

// TODO meta-refactor: after the refactor where backend logic is extracted in its own component,
//  check if this view can be plugged correctly and returned by the Backend view instead.

// NewBackendRemote returns an implementation of BackendRemote which is meant to be used for the
// cloud and remote implementations.
// Contrary to the idea of this package to have only commands related views creation exposed and non-commands
// views to be retrieved from the commands related one, this is exposed to be created because this has no
// json implementation and because the plug of this view in the places where it's needed is not yet
// straightforward.
func NewBackendRemote(view *View) BackendRemote {
	return &BackendRemoteHuman{
		view: view,
		renderer: &jsonformat.Renderer{
			Streams:  view.streams,
			Colorize: view.colorize,
		}}
}

var _ BackendRemote = (*BackendRemoteHuman)(nil)

func (v *BackendRemoteHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *BackendRemoteHuman) Output(msg string, colored bool) {
	if colored {
		msg = v.view.colorize.Color(msg)
	}
	_, _ = v.view.streams.Println(msg)
}

func (v *BackendRemoteHuman) RunWarning(description string) {
	if description == "" {
		return
	}
	const runWarningHeader = `[reset][yellow]Warning:[reset] %s
`
	msg := fmt.Sprintf(runWarningHeader, description)
	_, _ = v.view.streams.Println(v.view.colorize.Color(msg))
}

func (v *BackendRemoteHuman) RenderLog(log *jsonformat.JSONLog) error {
	return v.renderer.RenderLog(log)
}

func (v *BackendRemoteHuman) RenderHumanPlan(plan jsonformat.Plan, mode plans.Mode) {
	v.renderer.RenderHumanPlan(plan, mode)
}

func (v *BackendRemoteHuman) OperationCancelled() {
	_, _ = v.view.streams.Println(v.view.colorize.Color(`[reset][red]The remote operation was successfully cancelled.[reset]`))
}

func (v *BackendRemoteHuman) OperationNotCancelled() {
	_, _ = v.view.streams.Println(v.view.colorize.Color(`[reset][red]The remote operation was not cancelled.[reset]`))
}

func (v *BackendRemoteHuman) PreRefresh() {
	_, _ = v.view.streams.Println(v.view.colorize.Color(`[bold][yellow]Proceeding with 'tofu apply -refresh-only -auto-approve'.[reset]
`))
}

func (v *BackendRemoteHuman) InitialRetryError(isRemote bool) {
	const initialRetryError = `[reset][yellow]There was an error connecting to the %s backend. Please do not exit
OpenTofu to prevent data loss! Trying to restore the connection...
[reset]`
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(initialRetryError, remoteBackendType(isRemote))))
}

func (v *BackendRemoteHuman) RepeatedRetryError(elapsed time.Duration) {
	const repeatedRetryError = `[reset][yellow]Still trying to restore the connection... (%s elapsed)[reset]`
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(repeatedRetryError, elapsed)))
}

func (v *BackendRemoteHuman) UnavailableVersionInBackend(localVersion, usedVersion string) {
	const unavailableTerraformVersion = `
[reset][yellow]The local OpenTofu version (%s) is not available in the cloud backend, or your
organization does not have access to it. The new workspace will use %s. You can
change this later in the workspace settings.[reset]`
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(unavailableTerraformVersion, localVersion, usedVersion)))
}

func (v *BackendRemoteHuman) ApplySavedHeader() {
	const applySavedHeader = `[reset][yellow]Running apply in the cloud backend. Output will stream here. Pressing Ctrl-C
will stop streaming the logs, but will not stop the apply running remotely.[reset]

Preparing the remote apply...
`
	_, _ = v.view.streams.Println(v.view.colorize.Color(applySavedHeader))
}

func (v *BackendRemoteHuman) LockTimeoutError() {
	const lockTimeoutErr = `[reset][red]Lock timeout exceeded, sending interrupt to cancel the remote operation.
[reset]`
	_, _ = v.view.streams.Println(v.view.colorize.Color(lockTimeoutErr))
}

func (v *BackendRemoteHuman) RemoteWorkspaceInRelativeDirectory(wd string, configDir string) {
	const msg = `The remote workspace is configured to work with configuration at
%s relative to the target repository.

OpenTofu will upload the contents of the following directory,
excluding files or directories as defined by a .terraformignore file
at %s/.terraformignore (if it is present),
in order to capture the filesystem context the remote workspace expects:
    %s
`
	_, _ = v.view.streams.Println(fmt.Sprintf(msg, wd, configDir, configDir))
}

func (v *BackendRemoteHuman) WaitingForCostEstimation(elapsedHint string) {
	const msg = "Waiting for cost estimate to complete...%s\n"
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(msg, elapsedHint)))
}

func (v *BackendRemoteHuman) WaitingForOperationToStart(opType string, elapsed string) {
	const msg = "Waiting for the %s to start..."
	if elapsed != "" {
		final := fmt.Sprintf("%s%s\n", msg, elapsed)
		_, _ = v.view.streams.Println(v.view.colorize.Color(final))
		return
	}
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(msg, opType)))
}

func (v *BackendRemoteHuman) WaitingForTheManuallyLockedWorkspace(elapsedHint string) {
	const msg = "Waiting for the manually locked workspace to be unlocked...%s"
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(msg, elapsedHint)))
}

func (v *BackendRemoteHuman) WaitingForRuns(noOfRuns int, elapsedHint string) {
	const msg = "Waiting for %d run(s) to finish before being queued...%s"
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(msg, noOfRuns, elapsedHint)))
}

func (v *BackendRemoteHuman) WaitingForQueuedRuns(noOfRuns int, elapsedHint string) {
	const msg = "Waiting for %d queued run(s) to finish before starting...%s"
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(msg, noOfRuns, elapsedHint)))
}

func (v *BackendRemoteHuman) OperationHeader(isApply bool, isRemote bool) {
	const (
		planDefaultHeader = `[reset][yellow]Running plan in the %s backend. Output will stream here. Pressing Ctrl-C
will stop streaming the logs, but will not stop the plan running remotely.[reset]

Preparing the remote plan...
`
		applyDefaultHeader = `[reset][yellow]Running apply in the %s backend. Output will stream here. Pressing Ctrl-C
will cancel the remote apply if it's still pending. If the apply started it
will stop streaming the logs, but will not stop the apply running remotely.[reset]

Preparing the remote apply...
`
	)

	if isApply {
		_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(applyDefaultHeader, remoteBackendType(isRemote))))
		return
	}
	_, _ = v.view.streams.Println(v.view.colorize.Color(fmt.Sprintf(planDefaultHeader, remoteBackendType(isRemote))))
}

func remoteBackendType(isRemote bool) string {
	if isRemote {
		return "remote"
	}
	return "cloud"
}
