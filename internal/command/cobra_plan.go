package command

import (
	"flag"
	"os"

	"github.com/spf13/cobra"
)

type PlanCobraCommand struct {
	m   *Meta
	cmd *cobra.Command
}

func newCobraPlanCommand(m Meta, rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:                "plan",
		Short:              "Show changes required by the current configuration",
		Long:               "",
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

func (icc *InitCobraCommand) configurePlanCobraFlags(flags *initCfg) *flag.FlagSet {
	basicFlags := icc.m.extendedFlagSet("init")
	icc.cmd.Flags().BoolVar(&flags.flagBackend, "backend", true, "")
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
