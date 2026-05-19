package main

import (
	"context"
	"os"

	"github.com/opentofu/opentofu/internal/streamapi"
)

func main() {
	// To avoid blocking the caller's event loop we just immediately start
	// a goroutine and return here. The caller is expected to implement
	// the wasip1 API, an in particular implements the "poll_oneoff" function
	// to check whether data is available on stdin/stdout or whether a timer
	// has expired.
	//
	// If `poll_oneoff` indicates that nothing is ready then the Go runtime
	// will return to the WebAssembly caller and expect to be given an
	// opportunity to run once the subscriptions are satisfied, by a call
	// to [pollEvents].
	go func() {
		streamapi.MainLoop(context.Background(), os.Stdin, os.Stdout)
	}()
}

// pollEvents should be called by the WebAssembly caller whenever at least one
// of the event subscriptions from the most recent call to "poll_oneoff" has
// become ready. This gives the Go runtime a chance to try calling "poll_oneoff"
// again and continue execution of any background goroutines that become
// runnable.
//
//go:wasmexport pollEvents
func pollEvents() {
	// This is intentionally empty: entering this function just gives the
	// Go runtime an opportunity to deal with background goroutines.
}
