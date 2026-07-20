package arguments

import (
	"flag"
	"time"

	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Flags map[string]*Flag

func (f Flags) Attach(cmd *cobra.Command) {
	for _, flag := range f {
		flag.Cobra(cmd.Flags())
	}
}

func (f Flags) BoolVar(p *bool, name string, value bool, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.BoolVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.BoolVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}
func (f Flags) IntVar(p *int, name string, value int, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.IntVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.IntVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}
func (f Flags) StringVar(p *string, name string, value string, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.StringVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}
func (f Flags) DurationVar(p *time.Duration, name string, value time.Duration, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.DurationVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.DurationVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}

func (f Flags) StringArrayVar(p *[]string, name string, value []string, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringArrayVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.Var((*flags.FlagStringSlice)(p), name, usage) },
	}
	f[name] = flag
	return flag
}

func (f Flags) RawFlags(p flags.RawFlags, name string, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.Func(name, usage, func(s string) error { return p.Set(s) }) },
		Stdlib: func(f *flag.FlagSet) { f.Var(p, name, usage) },
	}
	f[name] = flag
	return flag
}

func (f Flags) Func(name string, usage string, fn func(string) error) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.Func(name, usage, fn) },
		Stdlib: func(f *flag.FlagSet) { panic("not implemented") },
	}
	f[name] = flag
	return flag
}

type Flag struct {
	Name    string
	Usage   string
	GroupID string
	Display string
	Hidden  bool

	Cobra  func(*pflag.FlagSet)
	Stdlib func(*flag.FlagSet)
}

func (f *Flag) SetGroup(id string) *Flag {
	f.GroupID = id
	return f
}
func (f *Flag) SetDisplay(display string) *Flag {
	f.Display = display
	return f
}
func (f *Flag) SetHidden(hidden bool) *Flag {
	f.Hidden = hidden
	return f
}

type DiagFn func() tfdiags.Diagnostics

type Hook struct {
	Pre, Post DiagFn
}
type Hooks []Hook

func (h Hooks) Attach(cmd *cobra.Command) {
	var pre []DiagFn
	var post []DiagFn
	for _, hook := range h {
		if hook.Pre != nil {
			pre = append(pre, hook.Pre)
		}
		if hook.Post != nil {
			post = append(post, hook.Post)
		}
	}
	if len(pre) != 0 {
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			var diags tfdiags.Diagnostics
			for _, fn := range pre {
				diags = diags.Append(fn)
			}
			return diags.Err()
		}
	}

	if len(post) != 0 {
		cmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
			var diags tfdiags.Diagnostics
			for _, fn := range post {
				diags = diags.Append(fn)
			}
			return diags.Err()
		}
	}
}

type FlagGroup struct {
	ID          string
	Title       string
	Description string
}
