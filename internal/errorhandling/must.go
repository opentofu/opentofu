package errorhandling

import (
	"errors"
	"github.com/hashicorp/hcl/v2"
)

// Must converts an error into a panic.
func Must(err error) {
	// Handle hcl.Diagnostics similar to errors:
	var diags hcl.Diagnostics
	if errors.As(err, &diags) {
		if diags.HasErrors() {
			panic(diags)
		}
		return
	}

	// Handle normal errors:
	if err != nil {
		panic(err)
	}
}

// Must2 converts an error into a panic, returning a value if no error happened.
func Must2[T any](value T, err error) T {
	Must(err)
	return value
}
