package moduletest

import (
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type File struct {
	Config *configs.TestFile

	Name   string
	Status Status

	Runs []*Run

	Diagnostics tfdiags.Diagnostics
}
