//go:build !js

package main

import (
	"os"

	"github.com/spf13/afero"
)

func main() {
	os.Exit(realMain(afero.NewOsFs()))
}
