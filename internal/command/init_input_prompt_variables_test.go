// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

// based on the TestPlan_varsUnset test in plan_test.go
// this is to check if we use have an unset variable, does it
// accept input for it
func TestInit_single_variable_inputs(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-vars-prompt"), td)
	defer testChdir(t, td)()

	// as hashicorp/local is built in, we can avoid a few headaches getting the tests to play nice by using it
	providerSource, closeInput := newMockProviderSource(t, map[string][]string{
		"hashicorp/local": {"1.0.0"},
	})
	defer closeInput()

	p := testProvider()

	type testCase struct {
		answers       []string
		expectedCode  int
		expectedError string
	}

	tests := map[string]testCase{
		"test_dir": {
			answers:      []string{`test_dir`},
			expectedCode: 0,
		},
		"non_existant_dir": {
			answers:       []string{`non_existant_dir`},
			expectedCode:  1,
			expectedError: "Error: Unreadable module directory",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			ui := new(cli.MockUi)
			view, done := testView(t)
			c := &InitCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(p),
					View:             view,
					Ui:               ui,
					ProviderSource:   providerSource,
				},
			}
			closeInput := testInteractiveInput(t, tc.answers)
			defer closeInput()

			args := []string{}
			code := c.Run(args)
			done(t)
			if code != tc.expectedCode {
				errStr := ui.ErrorWriter.String()
				t.Fatalf("bad error code. actual: %d, expected: %d\n\n%s", code, tc.expectedCode, errStr)
			}
			if tc.expectedError != "" {
				errStr := ui.ErrorWriter.String()
				// we use strings.Contains as the test commands nearly always return warnings about lock files etc and that's a nightmare
				// to parse.
				if !strings.Contains(errStr, tc.expectedError) {
					t.Fatalf("Expected error: %s\n\n not found in:\n\n%s", tc.expectedError, errStr)
				}
			}
		})
	}
}

// based on the TestPlan_varsUnset test in plan_test.go
// this is to check if we use the variable multiple times
// does it only ask once
func TestInit_multiple_variable_inputs(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-multi-vars-prompt"), td)
	defer testChdir(t, td)()

	// as hashicorp/local is built in, we can avoid a few headaches getting the tests to play nice by using it
	providerSource, closeInput := newMockProviderSource(t, map[string][]string{
		"hashicorp/local": {"1.0.0"},
	})
	defer closeInput()

	p := testProvider()

	type testCase struct {
		answers       []string
		expectedCode  int
		expectedError string
	}

	tests := map[string]testCase{
		"test_dir": {
			answers:      []string{`test_dir`},
			expectedCode: 0,
		},
		"non_existant_dir": {
			answers:       []string{`non_existant_dir`},
			expectedCode:  1,
			expectedError: "Error: Unreadable module directory",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			ui := new(cli.MockUi)
			view, done := testView(t)
			c := &InitCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(p),
					View:             view,
					Ui:               ui,
					ProviderSource:   providerSource,
				},
			}
			closeInput := testInteractiveInput(t, tc.answers)
			defer closeInput()

			args := []string{}
			code := c.Run(args)
			done(t)
			if code != tc.expectedCode {
				errStr := ui.ErrorWriter.String()
				t.Fatalf("bad error code. actual: %d, expected: %d\n\n%s", code, tc.expectedCode, errStr)
			}
			if tc.expectedError != "" {
				errStr := ui.ErrorWriter.String()
				// we use strings.Contains as the test commands nearly always return warnings about lock files etc and that's a nightmare
				// to parse.
				if !strings.Contains(errStr, tc.expectedError) {
					t.Fatalf("Expected error: %s\n\n not found in:\n\n%s", tc.expectedError, errStr)
				}
			}
		})
	}
}
