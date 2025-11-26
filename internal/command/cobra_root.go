package command

import (
	"github.com/spf13/cobra"
)

func CobraCommands(m Meta) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:  "tofu",
		Long: "The available commands for execution are listed below. The primary workflow commands are given first, followed by less common or more advanced commands.",
		// We disable the flag parsing from cobra since we are doing this in the Run method to be able
		// to use go standard `flag` parsing.
		DisableFlagParsing: true,
		// These 2 are needed to disable printing usage and errors because each command will return
		// an error type with the exit code, even the command execution succeeded.
		SilenceUsage:  true,
		SilenceErrors: true,
		// We still need to discover how this works
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	// adding groups to be able to group the commands accordingly in the help text
	rootCmd.AddGroup(commandGroupIdMain.group(), commandGroupIdOther.group())

	// This function is generating a help text similar to the one that we have with the other CLI lib
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		helpText := commandHelp()(cmd)
		m.Ui.Output(helpText)
	})

	// basic flags for the root command
	rootCmd.Flags().String("chdir", "", "Switch to a different working directory before executing the given subcommand")
	rootCmd.Flags().Bool("help", false, "Show this help output, or the help for a specified subcommand")
	rootCmd.Flags().Bool("version", false, `Alias to "version" command`)

	// init the other sub commands
	newCobraInitCommand(m, rootCmd)
	newCobraMainCommands(m, rootCmd)
	newCobraOtherCommands(m, rootCmd)

	return rootCmd
}
