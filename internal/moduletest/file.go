package moduletest

import (
	"github.com/opentffoundation/opentf/internal/configs"
	"github.com/opentffoundation/opentf/internal/tfdiags"
)

type File struct {
	Config *configs.TestFile

	Name   string
	Status Status

	Runs []*Run

	Diagnostics tfdiags.Diagnostics
}
