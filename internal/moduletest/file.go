package moduletest

import (
	"github.com/placeholderplaceholderplaceholder/opentf/internal/configs"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/tfdiags"
)

type File struct {
	Config *configs.TestFile

	Name   string
	Status Status

	Runs []*Run

	Diagnostics tfdiags.Diagnostics
}
