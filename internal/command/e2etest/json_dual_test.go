// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

func TestJsonIntoStream(t *testing.T) {
	fixturePath := filepath.Join("testdata", "json_dual")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)
	tfInto := e2e.NewBinary(t, tofuBin, fixturePath)
	logTimestampRe := regexp.MustCompile(`,"@timestamp":"[^"]*"`)
	resourceIdRe := regexp.MustCompile(`[a-z0-9\-]{36}`)

	sanitize := func(s string) string {
		s = logTimestampRe.ReplaceAllString(s, "")
		s = resourceIdRe.ReplaceAllString(s, "<ident>")
		return s
	}

	cases := []struct {
		title string
		args  []string
	}{
		{"init", []string{"init"}},
		{"validate", []string{"validate"}},
		{"plan", []string{"plan"}},
		{"apply", []string{"apply", "-auto-approve"}},
		{"output", []string{"output"}},
	}

	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			// Run init -json
			stdoutJson, stderr, err := tf.Run(append(tc.args, "-json")...)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if stderr != "" {
				t.Errorf("unexpected stderr output:\n%s", stderr)
			}
			if stdoutJson[0] != byte('{') {
				t.Errorf("Expected json output on stdout")
			}

			// Run init -json-into
			jsonIntoPath := tfInto.Path("into.json")
			stdoutJsonInto, stderr, err := tfInto.Run(append(tc.args, "-json-into="+jsonIntoPath)...)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if stderr != "" {
				t.Errorf("unexpected stderr output:\n%s", stderr)
			}
			if stdoutJsonInto[0] == byte('{') {
				t.Errorf("Unexpected json output on stdout")
			}

			// Read results
			fileJson, err := os.ReadFile(jsonIntoPath)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			// Compare
			fileJsonString := sanitize(string(fileJson))
			stdoutJson = sanitize(stdoutJson)

			if fileJsonString != stdoutJson {
				t.Errorf("\nGot:\n%s\n\nExpected:\n%s", fileJsonString, stdoutJson)
			}
		})
	}
}
