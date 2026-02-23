// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/svchost"

	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// LogoutCommand is a Command implementation which removes stored credentials
// for a remote service host.
type LogoutCommand struct {
	Meta
}

// Run implements cli.Command.
func (c *LogoutCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()
	ctx, span := tracing.Tracer().Start(ctx, "Logout")
	defer span.End()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Propagate -no-color for legacy use of Ui. The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	// Parse and validate flags
	args, closer, diags := arguments.ParseLogout(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewLogout(args.ViewOptions, c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}

	// FIXME: the -input flag value is needed to initialize the backend and the
	// operation, but there is no clear path to pass this value down, so we
	// continue to mutate the Meta object state for now.
	c.Meta.input = args.ViewOptions.InputEnabled

	givenHostname := args.Host

	hostname, err := svchost.ForComparison(givenHostname)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid hostname",
			fmt.Sprintf("The given hostname %q is not valid: %s.", givenHostname, err.Error()),
		))
		view.Diagnostics(diags)
		return 1
	}

	// From now on, since we've validated the given hostname, we should use
	// dispHostname in the UI to ensure we're presenting it in the canonical
	// form, in case that helps users with debugging when things aren't
	// working as expected. (Perhaps the normalization is part of the cause.)
	dispHostname := hostname.ForDisplay()

	creds := c.Services.CredentialsSource().(*cliconfig.CredentialsSource)
	filename, _ := creds.CredentialsFilePath()
	credsCtx := &loginCredentialsContext{
		Location:      creds.HostCredentialsLocation(hostname),
		LocalFilename: filename, // empty in the very unlikely event that we can't select a config directory for this user
		HelperType:    creds.CredentialsHelperType(),
	}

	if credsCtx.Location == cliconfig.CredentialsInOtherFile {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf("Credentials for %s are manually configured", dispHostname),
			"The \"tofu logout\" command cannot log out because credentials for this host are manually configured in a CLI configuration file.\n\nTo log out, revoke the existing credentials and remove that block from the CLI configuration.",
		))
	}

	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	switch credsCtx.Location {
	case cliconfig.CredentialsNotAvailable:
		view.NoCredentialsStored(dispHostname)
		return 0
	case cliconfig.CredentialsViaHelper:
		view.RemovingCredentialsFromHelper(dispHostname, credsCtx.HelperType)
	case cliconfig.CredentialsInPrimaryFile:
		view.RemovingCredentialsFromFile(dispHostname, credsCtx.LocalFilename)
	}

	err = creds.ForgetForHost(ctx, hostname)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to remove API token",
			fmt.Sprintf("Unable to remove stored API token: %s", err),
		))
	}

	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}

	view.LogoutSuccess(dispHostname)

	return 0
}

// Help implements cli.Command.
func (c *LogoutCommand) Help() string {
	defaultFile := c.defaultOutputFile()
	if defaultFile == "" {
		// Because this is just for the help message and it's very unlikely
		// that a user wouldn't have a functioning home directory anyway,
		// we'll just use a placeholder here. The real command has some
		// more complex behavior for this case. This result is not correct
		// on all platforms, but given how unlikely we are to hit this case
		// that seems okay.
		defaultFile = "~/.terraform/credentials.tfrc.json"
	}

	helpText := fmt.Sprintf(`
Usage: tofu [global options] logout [hostname]

  Removes locally-stored credentials for specified hostname.

  Note: the API token is only removed from local storage, not destroyed on the
  remote server, so it will remain valid until manually revoked.
      %s
`, defaultFile)
	return strings.TrimSpace(helpText)
}

// Synopsis implements cli.Command.
func (c *LogoutCommand) Synopsis() string {
	return "Remove locally-stored credentials for a remote host"
}

func (c *LogoutCommand) defaultOutputFile() string {
	if c.CLIConfigDir == "" {
		return "" // no default available
	}
	return filepath.Join(c.CLIConfigDir, "credentials.tfrc.json")
}
