// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/command/flags"
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
	FlagPluginPath flags.FlagStringSlice
	// Configuration to be merged with what is in the configuration file's 'backend' block. This can be
	// either a path to an HCL file with key/value assignments (same format as terraform.tfvars) or a
	// 'key=value' format, and can be specified multiple times. The backend type must be in the configuration itself.
	FlagConfigExtra flags.RawFlags
	// Disable backend or cloud backend initialization for this configuration and use what was previously
	// initialized instead. This and the FlagCloud cannot be toggled in the same time.
	FlagBackend bool
	FlagCloud   bool

	// Bools indicating that the FlagBackend and FlagCloud have been found into the arguments list of the
	// process.
	BackendFlagSet bool
	CloudFlagSet   bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
	// Backend holds and providers information for the flags related to the backend operations, like locking
	// locking timeout, force migration, etc.
	Backend *Backend
}

// ParseInit processes CLI arguments, returning an Init value, a closer function, and errors.
// If errors are encountered, an Init value is still returned representing
// the best effort interpretation of the arguments.
func ParseInit(args []string) (*Init, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	init := &Init{
		Vars:            &Vars{},
		Backend:         &Backend{},
		FlagConfigExtra: flags.NewRawFlags("-backend-config"),
	}

	cmdFlags := extendedFlagSet("init", nil, nil, init.Vars)
	init.Backend.AddIgnoreRemoteVersionFlag(cmdFlags)
	init.Backend.AddStateFlags(cmdFlags)
	init.Backend.AddMigrationFlags(cmdFlags)
	cmdFlags.BoolVar(&init.FlagBackend, "backend", true, "")
	cmdFlags.BoolVar(&init.FlagCloud, "cloud", true, "")
	cmdFlags.Var(init.FlagConfigExtra, "backend-config", "")
	cmdFlags.StringVar(&init.FlagFromModule, "from-module", "", "copy the source of the given module into the directory before init")
	cmdFlags.BoolVar(&init.FlagGet, "get", true, "")
	cmdFlags.BoolVar(&init.FlagUpgrade, "upgrade", false, "")
	cmdFlags.Var(&init.FlagPluginPath, "plugin-dir", "plugin directory")
	cmdFlags.StringVar(&init.FlagLockfile, "lockfile", "", "Set a dependency lockfile mode")
	cmdFlags.StringVar(&init.TestsDirectory, "test-directory", "tests", "test-directory")

	init.ViewOptions.AddFlags(cmdFlags, true)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	closer, moreDiags := init.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return init, closer, diags
	}
	init.BackendFlagSet = flags.FlagIsSet(cmdFlags, "backend")
	init.CloudFlagSet = flags.FlagIsSet(cmdFlags, "cloud")

	switch {
	case init.BackendFlagSet && init.CloudFlagSet:
		return init, closer, diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Wrong combination of options",
			"The -backend and -cloud options are aliases of one another and mutually-exclusive in their use",
		))
	case init.BackendFlagSet:
		init.FlagCloud = init.FlagBackend
	case init.CloudFlagSet:
		init.FlagBackend = init.FlagCloud
	}

	diags = diags.Append(init.Backend.migrationFlagsCheck())

	if len(cmdFlags.Args()) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"Too many command line arguments. Did you mean to use -chdir?",
		))
	}
	return init, closer, diags
}
