//go:build js

package main

import (
	"bytes"
	"os"
	"strings"
	"syscall/js"

	"github.com/spf13/afero"
)

func main() {
	fs := afero.NewMemMapFs()

	c := make(chan any)

	os.Setenv("TOFU_X_EXPERIMENTAL_RUNTIME", "yes")

	js.Global().Set("tofu", js.FuncOf(func(this js.Value, args []js.Value) any {
		mainTofu := js.Global().Get("document").Call("getElementById", "input").Get("value").String()
		afero.Afero{fs}.WriteFile("main.tofu", []byte(mainTofu), 644)

		os.Args = []string{"tofu"}
		for _, arg := range args {
			os.Args = append(os.Args, strings.Split(arg.String(), " ")...) // this is wrong
		}

		oldLog := js.Global().Get("console").Get("log")
		defer func() {
			js.Global().Get("console").Set("log", oldLog)
		}()

		log := js.Global().Get("document").Call("getElementById", "log")

		out := new(bytes.Buffer)
		js.Global().Get("console").Set("log", js.FuncOf(func(this js.Value, args []js.Value) any {
			out.Write([]byte(args[0].String() + "\n"))
			log.Set("innerHTML", string(out.Bytes()))
			log.Call("scrollTo", 0, log.Get("scrollHeight"))
			return nil
		}))

		main := realMain(fs)

		js.Global().Get("document").Call("getElementById", "log").Set("innerHTML", string(out.Bytes()))

		state, err := afero.Afero{fs}.ReadFile("terraform.tfstate")
		if err == nil {
			js.Global().Get("document").Call("getElementById", "state").Set("innerHTML", string(state))
		}

		return main
	}))

	_ = <-c
}
