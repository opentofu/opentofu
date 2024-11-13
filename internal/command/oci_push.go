package command

import (
	"fmt"
	"strings"
)

type OciPushCommand struct {
	Meta
}

func (c *OciPushCommand) Run(args []string) int {
	if err := validateArgs(args); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	c.Ui.Output("OCI push command" + strings.Join(args, " "))
	return 0
}

func (c *OciPushCommand) Help() string {
	return "push module to an OCI registry"
}

func (c *OciPushCommand) Synopsis() string {
	return "push module to an OCI registry"
}

func validateArgs(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("invalid number of arguments")
	}

	return nil
}
