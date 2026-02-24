// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// selected-go-version determines which version of Go is currently selected
// in the go.mod file.
//
// Note that running this requires that there already be some reasonable
// Go toolchain version (Go 1.21 or newer) installed on the system. The modern
// Go toolchain is able to manage its own versions automatically so it shouldn't
// typically matter which toolchain is used to run this command, and this
// command should only be used in situations where something other than the
// Go toolchain itself needs to make a decision about what to use, such as
// when building container images for testing purposes.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// Despite the name of this command, this is just printing a JSON
	// representation of the go.mod file and not actually editing it.
	cmd := exec.Command("go", "mod", "edit", "-json")
	raw, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get Go module metadata: %s\n", err)
		os.Exit(1)
	}

	var meta GoModJSON
	err = json.Unmarshal(raw, &meta)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse Go module metadata from JSON: %s\n", err)
		os.Exit(1)
	}

	// If the Toolchain field is present then we prefer to use that because
	// that matches what would be used when running "go" commands directly,
	// but we do need to trim of the "go" prefix to make it consistent with
	// how the "Go" field is formatted.
	//
	// Toolchain is described in https://go.dev/doc/toolchain .
	if toolchain, ok := strings.CutPrefix(meta.Toolchain, "go"); ok {
		// Note that this doesn't do any special handling of "custom toolchains"
		// because the OpenTofu project does not currently use those. If we
		// do decide to use custom toolchains in future then we'll need to
		// decide what that means for this program: should it return the base
		// version of the custom toolchain, or the full custom toolchain name?
		fmt.Println(toolchain)
		return
	}

	// Otherwise we use the "Go" field, since that's the only one present
	// when the toolchain directive isn't specified, and "go mod tidy" will
	// automatically remove the toolchain directive if the go directive matches.
	fmt.Println(meta.Go)
}

type GoModJSON struct {
	Go        string `json:"Go"`
	Toolchain string `json:"Toolchain"`
}
