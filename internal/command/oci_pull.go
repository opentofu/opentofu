package command

import (
	"os"

	"github.com/opentofu/opentofu/internal/oci"
	"github.com/opentofu/opentofu/internal/ociclient"
)

type OciPullCommand struct {
	Meta
}

func (c *OciPullCommand) Run(args []string) int {
	c.Ui.Output("OCI Pull command")
	ref, err := ociclient.ParseRef(args[0])
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	os.MkdirAll(ref.Name, 0755)

	if err := oci.PullModule(args[0], ref.Name); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}
	return 0
}

func (c *OciPullCommand) Help() string {
	return "Pull an OCI image."
}

func (c *OciPullCommand) Synopsis() string {
	return "push module to an OCI/Docker registry"
}
