// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing/fstest"
	"text/template"
)

type testFileSystem struct {
	fsys fstest.MapFS
}

func (tfs *testFileSystem) ReadFile(name string) ([]byte, error) {
	return tfs.fsys.ReadFile(tfs.trim(name))
}

func (tfs *testFileSystem) Stat(name string) (os.FileInfo, error) {
	return tfs.fsys.Stat(tfs.trim(name))
}

func (tfs *testFileSystem) ReadDir(name string) ([]os.DirEntry, error) {
	return tfs.fsys.ReadDir(tfs.trim(name))
}

func (tfs *testFileSystem) Open(name string) (fs.File, error) {
	return tfs.fsys.Open(tfs.trim(name))
}

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

// buildSubdomains creates a list of pairs of indices, encoded as subdomain names.
// These subdomains are of the form "{i}and{j}", where i is always strictly less than j.
// When we merge two configurations, we want to know which one has "precedence", so we
// can look at the "module version" value. We use the index for that module version,
// since configuration files are identified by their index in the test's list of files.
func buildSubDomains(i, n int) []string {
	subDomains := make([]string, n-1)
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
		subDomains[x-adjust] = fmt.Sprintf("%dand%d", j, k)
	}
	return subDomains
}

// getFile generates an example configuration file for use in the locations tests.
// Config files are identified by their name's index in a given test's file list.
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
	subDomains := buildSubDomains(i, n)
	templateFileName := "config-location.tpl"
	templateFileLocation := filepath.Join(fixtureDir, templateFileName)

	tpl := template.Must(template.ParseFiles(templateFileLocation))

	byteW := new(bytes.Buffer)
	err := tpl.ExecuteTemplate(byteW, templateFileName, testTemplateInfo{Subdomains: subDomains, Index: i})
	if err != nil {
		return nil, err
	}
	outB, err := io.ReadAll(byteW)
	if err != nil {
		return nil, err
	}
	return outB, nil
}
