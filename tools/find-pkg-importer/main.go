// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// find-pkg-importer is a utility for finding which packages in our dependency
// graph import a given package path.
//
// Run this from the root of your work tree for the OpenTofu repository so
// that it can find the project's "go.mod" file in the current working
// directory. For example, to find which packages import "html/template":
//
//	go tool find-pkg-importer html/template
//
// Note that this tool deals in individual packages rather than whole modules,
// so that we can use it to evaluate our exposure to advisories from the Go
// vulnerability database which describes specific packages and symbols that
// are affected. Use "go mod graph" or "go mod why" to answer similar questions
// about entire modules.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
)

func main() {
	// We don't have any options, but this'll report an error if someone tries to use one
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: go tool find-pkg-importer <PACKAGE-PATH>")
	}
	flag.Parse()
	if len(os.Args) != 2 {
		flag.Usage()
		os.Exit(1)
	}
	wantPkg := os.Args[1]

	cmd := exec.Command("go", "list", "-json=ImportPath,Imports", "all")
	out, err := cmd.StdoutPipe()
	if err != nil {
		fatalf("Can't open pipe for child process stdout: %s", out)
	}
	dec := json.NewDecoder(out)
	err = cmd.Start()
	if err != nil {
		fatalf("Failed to run 'go list': %s", err)
	}
	defer func() {
		// Error intentionally ignored becuse there's nothing useful we could
		// do about it anyway.
		_ = cmd.Wait()
	}()

	type Candidate struct {
		ImportPath string   `json:"ImportPath"`
		Imports    []string `json:"Imports"`
	}
	for {
		var candidate Candidate
		err := dec.Decode(&candidate)
		if err == io.EOF {
			break
		}
		if err != nil {
			fatalf("Failed to parse record from 'go list': %s", err)
		}
		if slices.Contains(candidate.Imports, wantPkg) {
			fmt.Println(candidate.ImportPath)
		}
	}
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
