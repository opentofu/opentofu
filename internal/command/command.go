// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"os"
	"runtime"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/version"
)

// Set to true when we're testing
var test bool = false

// PluginPathFile is the name of the file in the data dir which stores the list
// of directories supplied by the user with the `-plugin-dir` flag during init.
const PluginPathFile = "plugin_path"

// pluginMachineName is the directory name used in new plugin paths.
const pluginMachineName = runtime.GOOS + "_" + runtime.GOARCH

// DefaultPluginVendorDir is the location in the config directory to look for
// user-added plugin binaries. OpenTofu only reads from this path if it
// exists, it is never created by tofu.
const DefaultPluginVendorDir = "terraform.d/plugins/" + pluginMachineName

// DefaultVarsExtension is the default file extension used for vars
const DefaultVarsExtension = ".tfvars"

// DefaultVarsFilename is the default filename used for vars
const DefaultVarsFilename = "terraform" + DefaultVarsExtension

// DefaultBackupExtension is added to the state file to form the path
const DefaultBackupExtension = ".backup"

// DefaultParallelism is the limit Terraform places on total parallel
// operations as it walks the dependency graph.
const DefaultParallelism = 10

// ErrUnsupportedLocalOp is the common error message shown for operations
// that require a backend.Local.
const ErrUnsupportedLocalOp = `The configured backend doesn't support this operation.

The "backend" in OpenTofu defines how OpenTofu operates. The default
backend performs all operations locally on your machine. Your configuration
is configured to use a non-local backend. This backend doesn't support this
operation.
`

// modulePath returns the path to the root module and validates CLI arguments.
//
// This centralizes the logic for any commands that previously accepted
// a module path via CLI arguments. This will error if any extraneous arguments
// are given and suggest using the -chdir flag instead.
//
// If your command accepts more than one arg, then change the slice bounds
// to pass validation.
func modulePath(args []string) (string, error) {
	// TODO: test

	if len(args) > 0 {
		return "", fmt.Errorf("Too many command line arguments. Did you mean to use -chdir?")
	}

	path, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("Error getting pwd: %w", err)
	}

	return path, nil
}

// Group defines a command group and it's metadata
// In practice, this is only used for the special formatting in the root command
type Group struct {
	ID     string
	Title  string
	NoSort bool
}

var (
	MainCommandGroup  = Group{ID: "main", Title: "Main commands:", NoSort: true}
	OtherCommandGroup = Group{ID: "", Title: "All other commands:"}
)

// Command is the metadata and action associated with a command available to the CLI.
// This is eventually translated into a parsable/runnable CLI in cmd/tofu.
type Command struct {
	Name    string
	Aliases []string
	// Short is the text that accompanies it in the parent command's help text.
	Short string
	// Long is the full description that is printed as part of the command's help text.
	Long string
	// GroupID if set determines what group to show the command under in the parent command's help text.
	GroupID string
	// If this command should not be shown in the help text of the parent command.
	Hidden bool

	// Commands are the sub-commands of the current command.
	Commands []Command
	// Groups to split the sub Commands into
	Groups []Group

	// UsageOverride are custom overrides for special cases.
	// Ideally we can retire this eventually as we unify the help/usage text.
	UsageOverride UsageOverride

	// CommandLine defines what flags and arguments are available to this command.
	CommandLine arguments.CommandLine
	// Run is the action that will be executed if this command is selected.
	Run func(Meta) int

	// A legacy option that will be removed at some point
	// This was introduced during the migration to proper
	// diagnostic error printing during the meta-refactor
	DiagsWithNewline bool
}

type UsageOverride struct {
	// Weird formatting for the root command
	Usage string
	// Weird formatting for the root command
	SingleSpace bool
}

// RunResultHelp is a specific exit code that implies that help text should be shown.
// This will be modified or removed when we rip out mitchellh/cli in 1.14.x
var RunResultHelp = cli.RunResultHelp

// RunCommand is how the legacy command structure calls into the new command
// structure. This will be removed in 1.14.x
func RunCommand(cmd Command, meta Meta, args []string) int {
	return RunCli("", cmd, meta, cmd.CommandLine.ParseLegacy(args))
}

// RunCli is how the cli package executes a command after performing flag and argument
// parsing. The oddity here is that certain arg handling diags need to be printed
// from a properly configured meta and are therefore passed in here.
func RunCli(namespace string, cmd Command, meta Meta, diags tfdiags.Diagnostics) int {
	// Different exit code if args parse error vs hook error
	isArgsErr := diags.HasErrors()

	diags = diags.Append(cmd.CommandLine.PreHooks.Run())
	defer cmd.CommandLine.PostHooks.Run()

	if cmd.CommandLine.View == nil {
		cmd.CommandLine.View = &arguments.View{}
	}

	meta.View.Configure(cmd.CommandLine.View)

	if cmd.DiagsWithNewline {
		// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
		// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
		meta.View.DiagsWithNewline()
	}

	if diags.HasErrors() {
		if cmd.CommandLine.View.JSONInto != nil {
			views.NewJSONView(meta.View, cmd.CommandLine.View.JSONInto).Diagnostics(diags)
		}

		if cmd.CommandLine.View.ViewType == arguments.ViewJSON {
			views.NewJSONView(meta.View, nil).Diagnostics(diags)
			return 1
		}

		meta.View.Diagnostics(diags)
		if isArgsErr {
			return RunResultHelp
		}

		meta.View.HelpPrompt(namespace + cmd.Name)
		return 1
	}

	// FIXME: the -input flag value is needed to initialize the backend and the
	// operation, but there is no clear path to pass this value down, so we
	// continue to mutate the Meta object state for now.
	meta.input = cmd.CommandLine.View.InputEnabled

	if cmd.CommandLine.State != nil {
		meta.stateArgs = *cmd.CommandLine.State
	}
	if cmd.CommandLine.Backend != nil {
		meta.backendArgs = *cmd.CommandLine.Backend
	}
	if cmd.CommandLine.Operation != nil {
		// FIXME: the -parallelism flag is used to control the concurrency of
		// OpenTofu operations. At the moment, this value is used both to
		// initialize the backend via the ContextOpts field inside CLIOpts, and to
		// set a largely unused field on the Operation request. Again, there is no
		// clear path to pass this value down, so we continue to mutate the Meta
		// object state for now.
		meta.parallelism = cmd.CommandLine.Operation.Parallelism
	}
	if cmd.CommandLine.Vars != nil {
		// Inject variables from args into meta for static evaluation
		meta.variableArgs = cmd.CommandLine.Vars.All()
	}

	return cmd.Run(meta)
}

// RootCommander builds the standard tofu root command.
func RootCommander(help *bool, ver *bool, chdir *string) Command {
	root := Command{
		Name: "",
		Long: `The available commands for execution are listed below. The primary workflow commands are given first, followed by less common or more advanced commands.`,

		Commands: []Command{
			// Main Commands
			InitCommander(),
			ValidateCommander(),
			PlanCommander(),
			ApplyCommander(),
			DestroyCommander(),

			// Other Commands
			ConsoleCommander(),
			WorkspaceCommander(true),
			FmtCommander(nil),
			GetCommander(),
			GraphCommander(),
			ImportCommander(),
			LoginCommander(),
			LogoutCommander(),
			MetadataCommander(),
			OutputCommander(),
			ProvidersCommander(),
			RefreshCommander(),
			ShowCommander(),
			TaintCommander(),
			TestCommander(),
			VersionCommander(version.Version, version.Prerelease, getproviders.CurrentPlatform),
			UntaintCommander(),
			WorkspaceCommander(false),
			UnlockCommander(),
			StateCommander(),
			DocgenCommander(),
		},
		Groups: []Group{MainCommandGroup, OtherCommandGroup},

		UsageOverride: UsageOverride{
			Usage:       "tofu [global options] <subcommand> [args]",
			SingleSpace: true,
		},
	}

	root.CommandLine.FlagGroups = []arguments.FlagGroup{{
		Title: "Global options (use these before the subcommand, if any)",
	}}

	root.CommandLine.StringVar(chdir, "chdir", "", "Switch to a different working directory before executing the given subcommand.").SetDisplay("=DIR")
	root.CommandLine.BoolVar(ver, "version", false, `An alias for the "version" subcommand.`)
	root.CommandLine.BoolVar(help, "help", false, "Show this help output, or the help for a specified subcommand.")

	return root
}
