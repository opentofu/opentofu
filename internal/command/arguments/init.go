// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	flagspkg "github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Init represents the command-line arguments for the init command.
type Init struct {
	// Copy the contents of the given module into the target directory before initialisation
	FlagFromModule string
	// Lockfile operation mode. Currently only "readonly" is valid.
	FlagLockfile string
	// Set the OpenTofu test directory. When set, the
	// test command will search for test files in the current directory and
	// in the one specified by the flag.
	TestsDirectory string
	// When set to false, disables modules downloading for the current configuration
	FlagGet bool
	// Install the latest module and provider versions allowed within configured constraints, overriding the
	// default behavior of selecting exactly the version recorded in the dependency lockfile.
	FlagUpgrade bool
	// Directory containing plugin binaries. This overrides all default search paths for plugins, and prevents the
	// automatic installation of plugins. This flag can be used multiple times.
	FlagPluginPath []string
	// Configuration to be merged with what is in the configuration file's 'backend' block. This can be
	// either a path to an HCL file with key/value assignments (same format as terraform.tfvars) or a
	// 'key=value' format, and can be specified multiple times. The backend type must be in the configuration itself.
	FlagConfigExtra flagspkg.RawFlags
	// Disable backend or cloud backend initialization for this configuration and use what was previously
	// initialized instead. This and the FlagCloud cannot be toggled in the same time.
	FlagBackend bool
	FlagCloud   bool

	// Bools indicating that the FlagBackend and FlagCloud have been found into the arguments list of the
	// process.
	BackendFlagSet bool
	CloudFlagSet   bool

	// View represents the global view options
	View *View

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
	// Backend holds and providers information for the flags related to the backend operations, like locking
	// locking timeout, force migration, etc.
	Backend *Backend
	// State is used for the state related flags
	State *State
}

// BindInit registers CLI arguments, returning a Init value and it's corresponding hooks.
func BindInit(cli *CommandLine) *Init {
	init := Init{
		View:    BindView(cli, viewFlagAll),
		Vars:    BindVars(cli),
		State:   BindState(cli, stateFlagLock),
		Backend: BindBackendWithMigration(cli),

		FlagConfigExtra: flagspkg.NewRawFlags("-backend-config"),
	}

	backend := cli.BoolVar(&init.FlagBackend, "backend", true, "Disable backend or cloud backend initialization for this configuration and use what was previously initialized instead.").SetDisplay("=false")
	cloud := cli.BoolVar(&init.FlagCloud, "cloud", true, "").SetHidden(true)
	cli.RawFlags(init.FlagConfigExtra, "backend-config", "Configuration to be merged with what is in the configuration file's 'backend' block. This can be either a path to an HCL file with key/value assignments (same format as terraform.tfvars) or a 'key=value' format, and can be specified multiple times. The backend type must be in the configuration itself.").SetDisplay("=path")
	cli.StringVar(&init.FlagFromModule, "from-module", "", "Copy the contents of the given module into the target directory before initialization.").SetDisplay("=SOURCE")
	cli.BoolVar(&init.FlagGet, "get", true, "Disable downloading modules for this configuration.").SetDisplay("=false")
	cli.BoolVar(&init.FlagUpgrade, "upgrade", false, "Install the latest module and provider versions allowed within configured constraints, overriding the default behavior of selecting exactly the version recorded in the dependency lockfile.")
	cli.StringArrayVar(&init.FlagPluginPath, "plugin-dir", nil, "Directory containing plugin binaries. This overrides all default search paths for plugins, and prevents the automatic installation of plugins. This flag can be used multiple times.")
	cli.StringVar(&init.FlagLockfile, "lockfile", "", `Set a dependency lockfile mode. Currently only "readonly" is valid.`).SetDisplay("=MODE")
	cli.StringVar(&init.TestsDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")

	cli.PreHook(func() tfdiags.Diagnostics {
		init.BackendFlagSet = backend.IsSet()
		init.CloudFlagSet = cloud.IsSet()

		switch {
		case init.BackendFlagSet && init.CloudFlagSet:
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Wrong combination of options",
				"The -backend and -cloud options are aliases of one another and mutually-exclusive in their use",
			))
		case init.BackendFlagSet:
			init.FlagCloud = init.FlagBackend
		case init.CloudFlagSet:
			init.FlagBackend = init.FlagCloud
		}
		return nil
	})

	return &init
}

// ParseInit processes CLI arguments, returning an Init value, a closer function, and errors.
// If errors are encountered, an Init value is still returned representing
// the best effort interpretation of the arguments.
func ParseInit(args []string) (*Init, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	init := BindInit(cli)
	closer, diags := cli.parseWithHooks("init", args)
	return init, closer, diags
}
