package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
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

func InitCobra(m Meta) *cobra.Command {
	rootCmd.AddGroup(commandGroupIdMain.group(), commandGroupIdOther.group())
	rootCmd.Flags().String("chdir", "", "Switch to a different working directory before executing the given subcommand")
	rootCmd.Flags().Bool("help", false, "Show this help output, or the help for a specified subcommand")
	rootCmd.Flags().Bool("version", false, `Alias to "version" command`)
	newCobraInitCommand(m, rootCmd)
	newCobraPlanCommand(m, rootCmd)
	newCobraValidateCommand(m, rootCmd)
	newCobraApplyCommand(m, rootCmd)
	newCobraOtherCommands(m, rootCmd)
	newCobraDestroyCommand(m, rootCmd)

	// NOTE: uncomment the following block to have a similar `tofu -h` output with the one without refactoring
	// This still doesn't work as wanted but it's a example of what's possible
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, i []string) {
		helpText := commandHelp()(cmd)
		fmt.Println(helpText)
	})

	return rootCmd
}
