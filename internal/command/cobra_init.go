package command

import (
	"flag"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCobraInitCommand(m Meta, rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Prepare your working directory for other commands",
		Long: `Initialize a new or existing OpenTofu working directory by creating initial files, loading any remote state, downloading modules, etc.

This is the first command that should be run for any new or existing OpenTofu configuration per machine. This sets up all the local data necessary to run OpenTofu that is typically not committed to version control.

This command is always safe to run multiple times. Though subsequent runs may give errors, this command will never delete your configuration or state. Even so, if you have important information, please back it up prior to running this command, just in case.`,
		DisableFlagParsing: true,
		GroupID:            commandGroupIdMain.id(),
	}

	cfg := &initCfg{
		flagConfigExtra: newRawFlags("-backend-config"),
	}
	flagSet := configureInitCobraFlags(&m, cfg, cmd.Flags())
	flagSet.Usage = func() {
		helpText := commandHelp()(cmd)
		m.Ui.Error(helpText)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		nonFlagArgs, exitCode := parseFlags(&m, flagSet, args, cfg)
		if exitCode > 0 {
			return &ExitCodeError{ExitCode: exitCode}
		}

		acts := initActs{
			Meta:    &m,
			initCfg: cfg,
		}
		return &ExitCodeError{ExitCode: runInit(cmd.Context(), nonFlagArgs, &acts)}
	}

	rootCmd.AddCommand(cmd)
}

func configureInitCobraFlags(m *Meta, flags *initCfg, cmdFlags *pflag.FlagSet) *flag.FlagSet {
	basicFlags := m.extendedFlagSet("init")
	cmdFlags.BoolVar(&flags.flagBackend, "backend", true, "Disable backend or cloud backend initialization for this configuration and use what was previously initialized instead.")
	cmdFlags.BoolVar(&flags.flagCloud, "cloud", true, "")
	cmdFlags.Var(&flags.flagConfigExtra, "backend-config", "Configuration to be merged with what is in the configuration file's 'backend' block. This can be either a path to an HCL file with key/value assignments (same format as terraform.tfvars) or a 'key=value' format, and can be specified multiple times. The backend type must be in the configuration itself.")
	cmdFlags.StringVar(&flags.flagFromModule, "from-module", "", "Copy the contents of the given module into the target directory before initialization.")
	cmdFlags.BoolVar(&flags.flagGet, "get", true, "Disable downloading modules for this configuration.")
	cmdFlags.BoolVar(&m.forceInitCopy, "force-copy", false, `Suppress prompts about copying state data when initializing a new state backend. This is equivalent to providing a "yes" to all confirmation prompts.`)
	cmdFlags.BoolVar(&m.stateLock, "lock", true, "Don't hold a state lock during backend migration. This is dangerous if others might concurrently run commands against the same workspace.")
	cmdFlags.DurationVar(&m.stateLockTimeout, "lock-timeout", 0, "Duration to retry a state lock.")
	cmdFlags.BoolVar(&m.reconfigure, "reconfigure", false, "Reconfigure a backend, ignoring any saved configuration.")
	cmdFlags.BoolVar(&m.migrateState, "migrate-state", false, "Reconfigure a backend, and attempt to migrate any existing state.")
	cmdFlags.BoolVar(&flags.flagUpgrade, "upgrade", false, "Install the latest module and provider versions allowed within configured constraints, overriding the default behavior of selecting exactly the version recorded in the dependency lockfile.")
	cmdFlags.Var(&flags.flagPluginPath, "plugin-dir", "Directory containing plugin binaries. This overrides all default search paths for plugins, and prevents the automatic installation of plugins. This flag can be used multiple times.")
	cmdFlags.StringVar(&flags.flagLockfile, "lockfile", "", `Set a dependency lockfile mode. Currently only "readonly" is valid.`)
	cmdFlags.BoolVar(&m.ignoreRemoteVersion, "ignore-remote-version", false, "A rare option used for cloud backend and the remote backend only. Set this to ignore checking that the local and remote OpenTofu versions use compatible state representations, making an operation proceed even when there is a potential mismatch. See the documentation on configuring OpenTofu with cloud backend for more information.")
	cmdFlags.StringVar(&flags.testsDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`)
	cmdFlags.BoolVar(&m.outputInJSON, "json", false, "Produce output in a machine-readable JSON format, suitable for use in text editor integrations and other automated systems. Always disables color.")

	cmdFlags.CopyToGoFlagSet(basicFlags)
	return basicFlags
}
