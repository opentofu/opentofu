package command

import (
	"flag"
	"os"

	"github.com/spf13/cobra"
)

type InitCobraCommand struct {
	m   *Meta
	cmd *cobra.Command
}

func newCobraInitCommand(m Meta, rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Prepare your working directory for other commands",
		Long: `Initialize a new or existing OpenTofu working directory by creating initial files, loading any remote state, downloading modules, etc.

  This is the first command that should be run for any new or existing OpenTofu configuration per machine. This sets up all the local data necessary to run OpenTofu that is typically not committed to version control.

  This command is always safe to run multiple times. Though subsequent runs may give errors, this command will never delete your configuration or state. Even so, if you have important information, please back it up prior to running this command, just in case.`,
		DisableFlagParsing: true,
		GroupID:            commandGroupIdMain.id(),
		// ValidArgs:                  nil,
		// ValidArgsFunction:          nil,
		// Args:                       nil,
	}
	tofuInitCmd := InitCobraCommand{
		m:   &m,
		cmd: cmd,
	}

	cfg := &initCfg{}
	flagSet := tofuInitCmd.configureInitCobraFlags(cfg)
	flagSet.Usage = func() {
		helpText := commandHelp()(cmd)
		m.Ui.Error(helpText)
	}
	// cmdFlags.Usage = func() { m.Ui.Error(c.Help()) } // TODO andrei check how to do it
	// f := flag.NewFlagSet(initCmd.Use, flag.ExitOnError)
	// initCmd.Flags().CopyToGoFlagSet(f)
	tofuInitCmd.cmd.Run = func(cmd *cobra.Command, args []string) {
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

	rootCmd.AddCommand(tofuInitCmd.cmd)
}

func (icc *InitCobraCommand) configureInitCobraFlags(flags *initCfg) *flag.FlagSet {
	basicFlags := icc.m.extendedFlagSet("init")
	icc.cmd.Flags().BoolVar(&flags.flagBackend, "backend", true, "Disable backend or cloud backend initialization for this configuration and use what was previously initialized instead.")
	icc.cmd.Flags().BoolVar(&flags.flagCloud, "cloud", true, "")
	// icc.cmd.Flags().Var(&flags.flagConfigExtra, "backend-config", "") // TODO andrei include this too
	icc.cmd.Flags().StringVar(&flags.flagFromModule, "from-module", "", "copy the source of the given module into the directory before init")
	icc.cmd.Flags().BoolVar(&flags.flagGet, "get", true, "")
	icc.cmd.Flags().BoolVar(&icc.m.forceInitCopy, "force-copy", false, "suppress prompts about copying state data")
	icc.cmd.Flags().BoolVar(&icc.m.stateLock, "lock", true, "lock state")
	icc.cmd.Flags().DurationVar(&icc.m.stateLockTimeout, "lock-timeout", 0, "lock timeout")
	icc.cmd.Flags().BoolVar(&icc.m.reconfigure, "reconfigure", false, "reconfigure")
	icc.cmd.Flags().BoolVar(&icc.m.migrateState, "migrate-state", false, "migrate state")
	icc.cmd.Flags().BoolVar(&flags.flagUpgrade, "upgrade", false, "")
	// icc.cmd.Flags().Var(&flags.flagPluginPath, "plugin-dir", "plugin directory") // TODO andrei include this too
	icc.cmd.Flags().StringVar(&flags.flagLockfile, "lockfile", "", "Set a dependency lockfile mode")
	icc.cmd.Flags().BoolVar(&icc.m.ignoreRemoteVersion, "ignore-remote-version", false, "continue even if remote and local OpenTofu versions are incompatible")
	icc.cmd.Flags().StringVar(&flags.testsDirectory, "test-directory", "tests", "test-directory")
	icc.cmd.Flags().BoolVar(&icc.m.outputInJSON, "json", false, "json")

	icc.cmd.Flags().CopyToGoFlagSet(basicFlags)
	return basicFlags
}
