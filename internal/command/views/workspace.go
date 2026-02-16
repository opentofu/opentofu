// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"bytes"
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Workspace interface {
	Diagnostics(diags tfdiags.Diagnostics)

	// General workspace messages
	WorkspaceDoesNotExist(name string)
	WorkspaceInvalidName(name string)
	WorkspaceCreated(name string)
	WarnWhenUsedAsEnvCmd(usedAsEnvCmd bool)

	// `tofu workspace new` specific
	WorkspaceAlreadyExists(name string)
	WorkspaceIsOverriddenNewError()

	// `tofu workspace list` specific
	ListWorkspaces(workspaces []string, current string)
	WorkspaceOverwrittenByEnvVarWarn()

	// `tofu workspace select` specific
	WorkspaceChanged(name string)
	WorkspaceIsOverriddenSelectError()

	// `tofu workspace delete` specific
	WorkspaceDeleted(name string)
	DeletedWorkspaceNotEmpty(name string)
	CannotDeleteCurrentWorkspace(name string)

	// `tofu workspace show` specific
	WorkspaceShow(name string)
}

// NewWorkspace returns an initialized Workspace implementation for the given ViewType.
func NewWorkspace(args arguments.ViewOptions, view *View) Workspace {
	var ret Workspace
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &WorkspaceJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &WorkspaceHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &WorkspaceMulti{ret, &WorkspaceJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type WorkspaceMulti []Workspace

var _ Workspace = (WorkspaceMulti)(nil)

func (m WorkspaceMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m WorkspaceMulti) WorkspaceAlreadyExists(name string) {
	for _, o := range m {
		o.WorkspaceAlreadyExists(name)
	}
}

func (m WorkspaceMulti) WorkspaceDoesNotExist(name string) {
	for _, o := range m {
		o.WorkspaceDoesNotExist(name)
	}
}

func (m WorkspaceMulti) WorkspaceInvalidName(name string) {
	for _, o := range m {
		o.WorkspaceInvalidName(name)
	}
}

func (m WorkspaceMulti) ListWorkspaces(workspaces []string, current string) {
	for _, o := range m {
		o.ListWorkspaces(workspaces, current)
	}
}

func (m WorkspaceMulti) WorkspaceOverwrittenByEnvVarWarn() {
	for _, o := range m {
		o.WorkspaceOverwrittenByEnvVarWarn()
	}
}

func (m WorkspaceMulti) WorkspaceCreated(name string) {
	for _, o := range m {
		o.WorkspaceCreated(name)
	}
}

func (m WorkspaceMulti) WorkspaceChanged(name string) {
	for _, o := range m {
		o.WorkspaceChanged(name)
	}
}

func (m WorkspaceMulti) WorkspaceIsOverriddenSelectError() {
	for _, o := range m {
		o.WorkspaceIsOverriddenSelectError()
	}
}

func (m WorkspaceMulti) WorkspaceIsOverriddenNewError() {
	for _, o := range m {
		o.WorkspaceIsOverriddenNewError()
	}
}

func (m WorkspaceMulti) WorkspaceDeleted(name string) {
	for _, o := range m {
		o.WorkspaceDeleted(name)
	}
}

func (m WorkspaceMulti) DeletedWorkspaceNotEmpty(name string) {
	for _, o := range m {
		o.DeletedWorkspaceNotEmpty(name)
	}
}

func (m WorkspaceMulti) CannotDeleteCurrentWorkspace(name string) {
	for _, o := range m {
		o.CannotDeleteCurrentWorkspace(name)
	}
}

func (m WorkspaceMulti) WorkspaceShow(name string) {
	for _, o := range m {
		o.WorkspaceShow(name)
	}
}

func (m WorkspaceMulti) WarnWhenUsedAsEnvCmd(usedAsEnvCmd bool) {
	for _, o := range m {
		o.WarnWhenUsedAsEnvCmd(usedAsEnvCmd)
	}
}

type WorkspaceHuman struct {
	view *View
}

var _ Workspace = (*WorkspaceHuman)(nil)

func (v *WorkspaceHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *WorkspaceHuman) WorkspaceAlreadyExists(name string) {
	msg := fmt.Sprintf("Workspace %q already exists", name)
	v.view.errorln(msg)
}

func (v *WorkspaceHuman) WorkspaceDoesNotExist(name string) {
	msg := fmt.Sprintf(`
Workspace %q doesn't exist.

You can create this workspace with the "new" subcommand 
or include the "-or-create" flag with the "select" subcommand.`, name)
	v.view.errorln(msg)
}

func (v *WorkspaceHuman) WorkspaceInvalidName(name string) {
	msg := fmt.Sprintf(`
The workspace name %q is not allowed. The name must contain only URL safe
characters, and no path separators.
`, name)
	v.view.errorln(msg)
}

func (v *WorkspaceHuman) ListWorkspaces(workspaces []string, current string) {
	var out bytes.Buffer
	for _, s := range workspaces {
		if s == current {
			out.WriteString("* ")
		} else {
			out.WriteString("  ")
		}
		out.WriteString(s + "\n")
	}
	_, _ = v.view.streams.Println(out.String())
}

func (v *WorkspaceHuman) WorkspaceOverwrittenByEnvVarWarn() {
	msg := `

The active workspace is being overridden using the TF_WORKSPACE environment
variable.
`
	_, _ = v.view.streams.Println(msg)
}

func (v *WorkspaceHuman) WorkspaceCreated(name string) {
	const msg = `[reset][green][bold]Created and switched to workspace %q![reset][green]

You're now on a new, empty workspace. Workspaces isolate their state,
so if you run "tofu plan" OpenTofu will not see any existing state
for this configuration.`
	colorisedMsg := fmt.Sprintf(v.view.colorize.Color(msg), name)
	_, _ = v.view.streams.Println(colorisedMsg)
}

func (v *WorkspaceHuman) WorkspaceChanged(name string) {
	const msg = `[reset][green]Switched to workspace %q.`
	colorisedMsg := fmt.Sprintf(v.view.colorize.Color(msg), name)
	_, _ = v.view.streams.Println(colorisedMsg)
}

func (v *WorkspaceHuman) WorkspaceIsOverriddenSelectError() {
	msg := `
The selected workspace is currently overridden using the TF_WORKSPACE
environment variable.

To select a new workspace, either update this environment variable or unset
it and then run this command again.
`
	v.view.errorln(msg)
}

func (v *WorkspaceHuman) WorkspaceIsOverriddenNewError() {
	msg := `
The workspace is currently overridden using the TF_WORKSPACE environment
variable. You cannot create a new workspace when using this setting.

To create a new workspace, either unset this environment variable or update it
to match the workspace name you are trying to create, and then run this command
again.
`
	v.view.errorln(msg)
}

func (v *WorkspaceHuman) WorkspaceDeleted(name string) {
	const msg = `[reset][green]Deleted workspace %q!`
	colorisedMsg := fmt.Sprintf(v.view.colorize.Color(msg), name)
	_, _ = v.view.streams.Println(colorisedMsg)
}

func (v *WorkspaceHuman) DeletedWorkspaceNotEmpty(name string) {
	const msg = `WARNING: %q was non-empty.
The resources managed by the deleted workspace may still exist,
but are no longer manageable by OpenTofu since the state has
been deleted.`
	colorisedMsg := fmt.Sprintf(v.view.colorize.Color(msg), name)
	v.view.warnln(colorisedMsg)
}

func (v *WorkspaceHuman) CannotDeleteCurrentWorkspace(name string) {
	msg := fmt.Sprintf(`
Workspace %[1]q is your active workspace.

You cannot delete the currently active workspace. Please switch
to another workspace and try again.`, name)
	v.view.errorln(msg)
}

func (v *WorkspaceHuman) WorkspaceShow(name string) {
	_, _ = v.view.streams.Println(name)
}

func (v *WorkspaceHuman) WarnWhenUsedAsEnvCmd(usedAsEnvCmd bool) {
	if !usedAsEnvCmd {
		return
	}
	msg := `Warning: the "tofu env" family of commands is deprecated.

"Workspace" is now the preferred term for what earlier OpenTofu versions
called "environment", to reduce ambiguity caused by the latter term colliding
with other concepts.

The "tofu workspace" commands should be used instead. "tofu env"
will be removed in a future OpenTofu version.
`
	v.view.warnln(msg)
}

type WorkspaceJSON struct {
	view *JSONView
}

var _ Workspace = (*WorkspaceJSON)(nil)

func (v *WorkspaceJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *WorkspaceJSON) WorkspaceAlreadyExists(name string) {
	v.view.Error(fmt.Sprintf("Workspace %q already exists", name))
}

func (v *WorkspaceJSON) WorkspaceDoesNotExist(name string) {
	v.view.Error(fmt.Sprintf("Workspace %q doesn't exist. You can create this workspace with the \"new\" subcommand or include the \"-or-create\" flag with the \"select\" subcommand", name))
}

func (v *WorkspaceJSON) WorkspaceInvalidName(name string) {
	v.view.Error(fmt.Sprintf("The workspace name %q is not allowed. The name must contain only URL safe characters, and no path separators", name))
}

func (v *WorkspaceJSON) ListWorkspaces(workspaces []string, current string) {
	for _, workspace := range workspaces {
		isCurrent := current == workspace
		v.view.log.Info("workspace listing", "workspace", workspace, "is_current", isCurrent)
	}
}

func (v *WorkspaceJSON) WorkspaceOverwrittenByEnvVarWarn() {
	v.view.Info("The active workspace is being overridden using the TF_WORKSPACE environment variable")
}

func (v *WorkspaceJSON) WorkspaceCreated(name string) {
	v.view.Info(fmt.Sprintf("Created and switched to workspace %q. You're now on a new, empty workspace. Workspaces isolate their state, so if you run \"tofu plan\" OpenTofu will not see any existing state for this configuration", name))
}

func (v *WorkspaceJSON) WorkspaceChanged(name string) {
	v.view.Info(fmt.Sprintf("Switched to workspace %q", name))
}

func (v *WorkspaceJSON) WorkspaceIsOverriddenSelectError() {
	v.view.Error("The selected workspace is currently overridden using the TF_WORKSPACE environment variable. To select a new workspace, either update this environment variable or unset it and then run this command again")
}

func (v *WorkspaceJSON) WorkspaceIsOverriddenNewError() {
	v.view.Error("The workspace is currently overridden using the TF_WORKSPACE environment variable. You cannot create a new workspace when using this setting. To create a new workspace, either unset this environment variable or update it to match the workspace name you are trying to create, and then run this command again")
}

func (v *WorkspaceJSON) WorkspaceDeleted(name string) {
	v.view.Info(fmt.Sprintf("Deleted workspace %q", name))
}

func (v *WorkspaceJSON) DeletedWorkspaceNotEmpty(name string) {
	v.view.Warn(fmt.Sprintf("WARNING: %q was non-empty. The resources managed by the deleted workspace may still exist, but are no longer manageable by OpenTofu since the state has been deleted", name))
}

func (v *WorkspaceJSON) CannotDeleteCurrentWorkspace(name string) {
	v.view.Error(fmt.Sprintf("Workspace %q is your active workspace. You cannot delete the currently active workspace. Please switch to another workspace and try again", name))
}

func (v *WorkspaceJSON) WorkspaceShow(name string) {
	v.view.Info(name)
}

func (v *WorkspaceJSON) WarnWhenUsedAsEnvCmd(usedAsEnvCmd bool) {
	if !usedAsEnvCmd {
		return
	}
	v.view.Warn("The \"tofu env\" family of commands is deprecated. Use \"tofu workspace\" instead")
}
