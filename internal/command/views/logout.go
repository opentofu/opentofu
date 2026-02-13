// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Logout interface {
	Diagnostics(diags tfdiags.Diagnostics)
	HelpPrompt(credentialsFile string)

	NoCredentialsStored(dispHostname string)
	RemovingCredentialsFromHelper(dispHostname, helperType string)
	RemovingCredentialsFromFile(dispHostname, localFilename string)
	LogoutSuccess(dispHostname string)
}

// NewLogout returns an initialized Logout implementation for the given ViewType.
func NewLogout(args arguments.ViewOptions, view *View) Logout {
	var ret Logout
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &LogoutJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &LogoutHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &LogoutMulti{ret, &LogoutJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type LogoutMulti []Logout

var _ Logout = (LogoutMulti)(nil)

func (m LogoutMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m LogoutMulti) HelpPrompt(credentialsFile string) {
	for _, o := range m {
		o.HelpPrompt(credentialsFile)
	}
}

func (m LogoutMulti) NoCredentialsStored(dispHostname string) {
	for _, o := range m {
		o.NoCredentialsStored(dispHostname)
	}
}

func (m LogoutMulti) RemovingCredentialsFromHelper(dispHostname, helperType string) {
	for _, o := range m {
		o.RemovingCredentialsFromHelper(dispHostname, helperType)
	}
}

func (m LogoutMulti) RemovingCredentialsFromFile(dispHostname, localFilename string) {
	for _, o := range m {
		o.RemovingCredentialsFromFile(dispHostname, localFilename)
	}
}

func (m LogoutMulti) LogoutSuccess(dispHostname string) {
	for _, o := range m {
		o.LogoutSuccess(dispHostname)
	}
}

type LogoutHuman struct {
	view *View
}

var _ Logout = (*LogoutHuman)(nil)

func (v *LogoutHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *LogoutHuman) HelpPrompt(credentialsFile string) {
	helpText := fmt.Sprintf(`
Usage: tofu [global options] logout [hostname]

  Removes locally-stored credentials for specified hostname.

  Note: the API token is only removed from local storage, not destroyed on the
  remote server, so it will remain valid until manually revoked.
      %s
`, credentialsFile)
	_, _ = v.view.streams.Eprintln(helpText)
}

func (v *LogoutHuman) NoCredentialsStored(dispHostname string) {
	msg := fmt.Sprintf("No credentials for %s are stored.\n", dispHostname)
	_, _ = v.view.streams.Println(msg)
}

func (v *LogoutHuman) RemovingCredentialsFromHelper(dispHostname, helperType string) {
	msg := fmt.Sprintf("Removing the stored credentials for %s from the configured\n%q credentials helper.\n", dispHostname, helperType)
	_, _ = v.view.streams.Println(msg)
}

func (v *LogoutHuman) RemovingCredentialsFromFile(dispHostname, localFilename string) {
	msg := fmt.Sprintf("Removing the stored credentials for %s from the following file:\n    %s\n", dispHostname, localFilename)
	_, _ = v.view.streams.Println(msg)
}

func (v *LogoutHuman) LogoutSuccess(dispHostname string) {
	msg := fmt.Sprintf(v.view.colorize.Color("[green][bold]Success![reset] [bold]OpenTofu has removed the stored API token for %s.[reset]"), dispHostname)
	_, _ = v.view.streams.Println(msg + "\n")
}

type LogoutJSON struct {
	view *JSONView
}

var _ Logout = (*LogoutJSON)(nil)

func (v *LogoutJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *LogoutJSON) HelpPrompt(_ string) {}

func (v *LogoutJSON) NoCredentialsStored(dispHostname string) {
	v.view.Info(fmt.Sprintf("No credentials for %s are stored", dispHostname))
}

func (v *LogoutJSON) RemovingCredentialsFromHelper(dispHostname, helperType string) {
	v.view.Info(fmt.Sprintf("Removing the stored credentials for %s from the configured %q credentials helper", dispHostname, helperType))
}

func (v *LogoutJSON) RemovingCredentialsFromFile(dispHostname, localFilename string) {
	v.view.Info(fmt.Sprintf("Removing the stored credentials for %s from the following file: %s", dispHostname, localFilename))
}

func (v *LogoutJSON) LogoutSuccess(dispHostname string) {
	v.view.Info(fmt.Sprintf("Success! OpenTofu has removed the stored API token for %s", dispHostname))
}
