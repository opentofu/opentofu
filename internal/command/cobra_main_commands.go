package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Placeholders for most of the "main" commands. Once a command from this list is properly implemented, remove it from here.
// This function creates placeholder commands
func newCobraMainCommands(_ Meta, rootCmd *cobra.Command) {
	other := map[string]string{
		"apply":    "Create or update infrastructure",
		"destroy":  "Destroy previously-created infrastructure",
		"plan":     "Show changes required by the current configuration",
		"validate": "Check whether the configuration is valid",
	}
	for cmdName, desc := range other {
		rootCmd.Commands()
		cmd := &cobra.Command{
			Use:                cmdName,
			Short:              desc,
			Long:               "",
			DisableFlagParsing: true,
			GroupID:            commandGroupIdMain.id(),
		}
		cmd.Run = func(cmd *cobra.Command, args []string) {
			fmt.Println("execute", cmdName)
		}
		rootCmd.AddCommand(cmd)
	}

}
