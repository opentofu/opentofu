//go:build js

package main

import (
	"os"
	"syscall/js"

	"github.com/spf13/afero"
)

func main() {
	fs := afero.NewMemMapFs()
	afero.Afero{fs}.WriteFile("main.tofu", []byte(`
resource "terraform_data" "first" {
	count = 200
	input = count.index
}

resource "terraform_data" "second" {
	count = 200
	input = terraform_data.first[count.index].input
}
	`), 644)

	c := make(chan any)

	os.Setenv("TOFU_X_EXPERIMENTAL_RUNTIME", "yes")

	js.Global().Set("tofu", js.FuncOf(func(this js.Value, args []js.Value) any {
		os.Args = []string{"tofu"}
		for _, arg := range args {
			os.Args = append(os.Args, arg.String())
		}

		return realMain(fs)
	}))

	_ = <-c
}
