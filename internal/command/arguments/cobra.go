package arguments

import (
	"time"

	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/spf13/cobra"
)

type FlagSet interface {
	BoolVar(p *bool, name string, value bool, usage string)
	IntVar(p *int, name string, value int, usage string)
	StringVar(p *string, name string, value string, usage string)
	DurationVar(p *time.Duration, name string, value time.Duration, usage string)
}

func AddPre(cmd *cobra.Command, fn func() tfdiags.Diagnostics) {
	pre := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		diags := fn()
		if pre != nil {
			diags = diags.Append(pre(cmd, args))
		}
		return diags.Err()
	}
}

func AddPost(cmd *cobra.Command, fn func() tfdiags.Diagnostics) {
	post := cmd.PersistentPostRunE
	cmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		diags := fn()
		if post != nil {
			diags = diags.Append(post(cmd, args))
		}
		return diags.Err()
	}
}
