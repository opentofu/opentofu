// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"text/template"
)

type locationTestParameters struct {
	name        string
	files       []string
	directories []string
	envVars     map[string]string
}

// locationTest fully specifies the relevant filesystem contents and the expected config.
// The config files are automatically generated with the getFile function, which only
// generates host information, so we only compare expected and actual host field.
type locationTest struct {
	locationTestParameters
	expected map[string]*ConfigHost
}

// directoryLocationTest specifies which directories are present in the filesystem and
// the expected directories.
type directoryLocationTest struct {
	locationTestParameters
	expected []string
}

type testTemplateInfo struct {
	Subdomains []string
	Index      int
}

// getFile generates an example configuration file for use in the locations tests.
// The generated configuration needs to satisfy two criteria. When looking at the
// final configuration, we should be able to determine:
//   - which files contributed to it
//   - the order those files were merged
//
// These are satisfied with a index host and a pairwise "comparison host", whose "module.vX"
// value receives X from the index of the configuration file with highest precedence.
// The resulting configuration will only have hosts information, which is sufficient
// for the purposes of the location tests.
func getFile(i, n int) ([]byte, error) {
	subDs := make([]string, n-1)
	adjust := 0
	for x := range n {
		if x == i {
			adjust = 1
			continue
		}
		j, k := i, x
		if x < i {
			j, k = x, i
		}
		subDs[x-adjust] = fmt.Sprintf("%dand%d", j, k)
	}

	templateFileName := "config-location.tpl"
	templateFileLocation := filepath.Join(fixtureDir, templateFileName)

	tpl := template.Must(template.ParseFiles(templateFileLocation))

	byteW := new(bytes.Buffer)
	err := tpl.ExecuteTemplate(byteW, templateFileName, testTemplateInfo{Subdomains: subDs, Index: i})
	if err != nil {
		return nil, err
	}
	outB, err := io.ReadAll(byteW)
	if err != nil {
		return nil, err
	}
	return outB, nil
}
