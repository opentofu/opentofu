package command

import (
	"flag"
	"os"

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

	cfg := &initCfg{}
	flagSet := configureInitCobraFlags(&m, cfg, cmd.Flags())
	flagSet.Usage = func() {
		helpText := commandHelp()(cmd)
		m.Ui.Error(helpText)
	}
	cmd.Run = func(cmd *cobra.Command, args []string) {
		nonFlagArgs, exitCode := parseFlags(&m, flagSet, args, cfg)
		if exitCode > 0 {
			os.Exit(exitCode)
		}

		acts := initActs{
			Meta:    &m,
			initCfg: cfg,
		}
		os.Exit(runInit(cmd.Context(), nonFlagArgs, &acts))
	}

	rootCmd.AddCommand(cmd)
}

func configureInitCobraFlags(m *Meta, flags *initCfg, cmdFlags *pflag.FlagSet) *flag.FlagSet {
	basicFlags := m.extendedFlagSet("init")
	cmdFlags.BoolVar(&flags.flagBackend, "backend", true, "Disable backend or cloud backend initialization for this configuration and use what was previously initialized instead.")
	cmdFlags.BoolVar(&flags.flagCloud, "cloud", true, "")
	// cmdFlags.StringSliceVar(&flags.flagConfigExtra, "backend-config", "") // TODO andrei include this too
	cmdFlags.StringVar(&flags.flagFromModule, "from-module", "", "copy the source of the given module into the directory before init")
	cmdFlags.BoolVar(&flags.flagGet, "get", true, "")
	cmdFlags.BoolVar(&m.forceInitCopy, "force-copy", false, "suppress prompts about copying state data")
	cmdFlags.BoolVar(&m.stateLock, "lock", true, "lock state")
	cmdFlags.DurationVar(&m.stateLockTimeout, "lock-timeout", 0, "lock timeout")
	cmdFlags.BoolVar(&m.reconfigure, "reconfigure", false, "reconfigure")
	cmdFlags.BoolVar(&m.migrateState, "migrate-state", false, "migrate state")
	cmdFlags.BoolVar(&flags.flagUpgrade, "upgrade", false, "")
	// cmdFlags.Var(&flags.flagPluginPath, "plugin-dir", "plugin directory") // TODO andrei include this too
	cmdFlags.StringVar(&flags.flagLockfile, "lockfile", "", "Set a dependency lockfile mode")
	cmdFlags.BoolVar(&m.ignoreRemoteVersion, "ignore-remote-version", false, "continue even if remote and local OpenTofu versions are incompatible")
	cmdFlags.StringVar(&flags.testsDirectory, "test-directory", "tests", "test-directory")
	cmdFlags.BoolVar(&m.outputInJSON, "json", false, "json")

	cmdFlags.CopyToGoFlagSet(basicFlags)
	return basicFlags
}
