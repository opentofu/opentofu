// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package junit

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/moduletest"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestJUnitXMLTestReport_BasicStatuses(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Fail,
		Files: map[string]*moduletest.File{
			"example.tftest.hcl": {
				Name:   "example.tftest.hcl",
				Status: moduletest.Fail,
				Runs: []*moduletest.Run{
					{
						Name:   "passes",
						Status: moduletest.Pass,
						Config: &configs.TestRun{},
					},
					{
						Name:   "fails",
						Index:  1,
						Status: moduletest.Fail,
						Config: &configs.TestRun{
							CheckRules: []*configs.CheckRule{
								{},
								{},
							},
						},
						Diagnostics: tfdiags.Diagnostics{
							tfdiags.Sourceless(tfdiags.Error, "Assertion failed", "Expected X, got Y"),
						},
					},
					{
						Name:   "skipped",
						Index:  2,
						Status: moduletest.Skip,
						Config: &configs.TestRun{},
					},
					{
						Name:   "errored",
						Index:  3,
						Status: moduletest.Error,
						Config: &configs.TestRun{},
						Diagnostics: tfdiags.Diagnostics{
							tfdiags.Sourceless(tfdiags.Error, "Something broke", "Details here"),
						},
					},
				},
			},
		},
	}

	stopped := false
	report := junitXMLTestReport(suite, nil, &stopped)

	if len(report.TestSuites) != 1 {
		t.Fatalf("expected 1 test suite, got %d", len(report.TestSuites))
	}

	ts := report.TestSuites[0]
	if ts.Name != "example.tftest.hcl" {
		t.Errorf("expected suite name %q, got %q", "example.tftest.hcl", ts.Name)
	}
	if ts.Tests != 4 {
		t.Errorf("expected 4 tests, got %d", ts.Tests)
	}
	if ts.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", ts.Failures)
	}
	if ts.Errors != 1 {
		t.Errorf("expected 1 error, got %d", ts.Errors)
	}
	if ts.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", ts.Skipped)
	}

	// Verify individual cases
	if ts.Cases[0].Failure != nil || ts.Cases[0].Skipped != nil || ts.Cases[0].Error != nil {
		t.Error("passing test should have no failure/skipped/error elements")
	}
	if ts.Cases[1].Failure == nil {
		t.Error("failing test should have failure element")
	}
	if ts.Cases[2].Skipped == nil {
		t.Error("skipped test should have skipped element")
	}
	if ts.Cases[3].Error == nil {
		t.Error("errored test should have error element")
	}
}

func TestJUnitXMLTestReport_MultipleFiles(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Pass,
		Files: map[string]*moduletest.File{
			"alpha.tftest.hcl": {
				Name:   "alpha.tftest.hcl",
				Status: moduletest.Pass,
				Runs: []*moduletest.Run{
					{
						Name:   "run_a",
						Status: moduletest.Pass,
						Config: &configs.TestRun{},
					},
				},
			},
			"beta.tftest.hcl": {
				Name:   "beta.tftest.hcl",
				Status: moduletest.Pass,
				Runs: []*moduletest.Run{
					{
						Name:   "run_b",
						Status: moduletest.Pass,
						Config: &configs.TestRun{},
					},
				},
			},
		},
	}

	stopped := false
	report := junitXMLTestReport(suite, nil, &stopped)

	if len(report.TestSuites) != 2 {
		t.Fatalf("expected 2 test suites, got %d", len(report.TestSuites))
	}

	// Verify alphabetical ordering
	if report.TestSuites[0].Name != "alpha.tftest.hcl" {
		t.Errorf("expected first suite to be alpha, got %q", report.TestSuites[0].Name)
	}
	if report.TestSuites[1].Name != "beta.tftest.hcl" {
		t.Errorf("expected second suite to be beta, got %q", report.TestSuites[1].Name)
	}
}

func TestJUnitXMLTestReport_PendingStatus(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Pending,
		Files: map[string]*moduletest.File{
			"pending.tftest.hcl": {
				Name:   "pending.tftest.hcl",
				Status: moduletest.Pending,
				Runs: []*moduletest.Run{
					{
						Name:   "not_run",
						Status: moduletest.Pending,
						Config: &configs.TestRun{},
					},
				},
			},
		},
	}

	stopped := false
	report := junitXMLTestReport(suite, nil, &stopped)

	ts := report.TestSuites[0]
	if ts.Skipped != 1 {
		t.Errorf("expected 1 skipped for pending, got %d", ts.Skipped)
	}
	if ts.Cases[0].Skipped == nil {
		t.Fatal("expected skipped element for pending test")
	}
	if ts.Cases[0].Skipped.Message != "Test did not run" {
		t.Errorf("expected 'Test did not run', got %q", ts.Cases[0].Skipped.Message)
	}
}

func TestSkipDetails_Interrupt(t *testing.T) {
	stopped := true
	file := &moduletest.File{
		Runs: []*moduletest.Run{
			{Name: "run1", Status: moduletest.Pass},
		},
	}
	reason := skipDetails(1, file, &stopped, false)
	if reason != "Test skipped due to interrupt" {
		t.Errorf("expected interrupt reason, got %q", reason)
	}
}

func TestSkipDetails_FileLevelError(t *testing.T) {
	stopped := false
	file := &moduletest.File{
		Diagnostics: tfdiags.Diagnostics{
			tfdiags.Sourceless(tfdiags.Error, "file error", "details"),
		},
		Runs: []*moduletest.Run{},
	}
	reason := skipDetails(0, file, &stopped, true)
	if reason != "Test skipped due to earlier error in test file" {
		t.Errorf("expected file-level error reason, got %q", reason)
	}
}

func TestSkipDetails_PreviousRunError(t *testing.T) {
	stopped := false
	file := &moduletest.File{
		Runs: []*moduletest.Run{
			{Name: "run1", Status: moduletest.Error},
			{Name: "run2", Status: moduletest.Skip},
		},
	}
	reason := skipDetails(1, file, &stopped, false)
	if reason != "Test skipped due to earlier error" {
		t.Errorf("expected earlier error reason, got %q", reason)
	}
}

func TestSkipDetails_Default(t *testing.T) {
	stopped := false
	file := &moduletest.File{
		Runs: []*moduletest.Run{
			{Name: "run1", Status: moduletest.Pass},
			{Name: "run2", Status: moduletest.Skip},
		},
	}
	reason := skipDetails(1, file, &stopped, false)
	if reason != "Test skipped" {
		t.Errorf("expected default skip reason, got %q", reason)
	}
}

func TestFailureMessage(t *testing.T) {
	tests := []struct {
		failed   int
		total    int
		expected string
	}{
		{0, 0, "failed"},
		{1, 1, "1 of 1 assertion failed"},
		{1, 2, "1 of 2 assertions failed"},
		{3, 5, "3 of 5 assertions failed"},
	}

	for _, tt := range tests {
		got := failureMessage(tt.failed, tt.total)
		if got != tt.expected {
			t.Errorf("failureMessage(%d, %d) = %q, want %q", tt.failed, tt.total, got, tt.expected)
		}
	}
}

func TestSave_WritesFile(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "results.xml")

	suite := &moduletest.Suite{
		Status: moduletest.Pass,
		Files: map[string]*moduletest.File{
			"simple.tftest.hcl": {
				Name:   "simple.tftest.hcl",
				Status: moduletest.Pass,
				Runs: []*moduletest.Run{
					{
						Name:   "passes",
						Status: moduletest.Pass,
						Config: &configs.TestRun{},
					},
				},
			},
		},
	}

	stopped := false
	j := NewTestJUnitXMLFile(filename, nil, &stopped)
	diags := j.Save(suite)

	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read output file: %s", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "<?xml") {
		t.Error("output should start with XML declaration")
	}
	if !strings.Contains(content, "<testsuites>") {
		t.Error("output should contain testsuites element")
	}
	if !strings.Contains(content, `name="simple.tftest.hcl"`) {
		t.Error("output should contain test suite name")
	}
	if !strings.Contains(content, `name="passes"`) {
		t.Error("output should contain test case name")
	}
}

func TestSave_InvalidPath(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Pass,
		Files:  map[string]*moduletest.File{},
	}

	stopped := false
	j := NewTestJUnitXMLFile("/nonexistent/path/results.xml", nil, &stopped)
	diags := j.Save(suite)

	if !diags.HasErrors() {
		t.Fatal("expected error for invalid path")
	}
}

func TestJUnitXMLTestReport_XMLValidity(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Fail,
		Files: map[string]*moduletest.File{
			"test.tftest.hcl": {
				Name:   "test.tftest.hcl",
				Status: moduletest.Fail,
				Runs: []*moduletest.Run{
					{
						Name:   "pass_run",
						Status: moduletest.Pass,
						Config: &configs.TestRun{},
					},
					{
						Name:   "fail_run",
						Index:  1,
						Status: moduletest.Fail,
						Config: &configs.TestRun{},
					},
				},
			},
		},
	}

	stopped := false
	report := junitXMLTestReport(suite, nil, &stopped)

	data, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal XML: %s", err)
	}

	// Verify the XML is valid by unmarshaling it back
	var parsed testSuites
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated XML is not valid: %s", err)
	}

	if len(parsed.TestSuites) != 1 {
		t.Fatalf("expected 1 test suite after round-trip, got %d", len(parsed.TestSuites))
	}
	if len(parsed.TestSuites[0].Cases) != 2 {
		t.Fatalf("expected 2 test cases after round-trip, got %d", len(parsed.TestSuites[0].Cases))
	}
}

func TestGetDiagString(t *testing.T) {
	diags := tfdiags.Diagnostics{
		tfdiags.Sourceless(tfdiags.Error, "Test error", "Details of the error"),
	}

	result := getDiagString(diags, nil)
	if result == "" {
		t.Error("expected non-empty diagnostic string")
	}
	if !strings.Contains(result, "Test error") {
		t.Error("diagnostic string should contain the error summary")
	}
}

func TestJUnitXMLTestReport_EmptySuite(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Pass,
		Files:  map[string]*moduletest.File{},
	}

	stopped := false
	report := junitXMLTestReport(suite, nil, &stopped)

	if len(report.TestSuites) != 0 {
		t.Errorf("expected 0 test suites for empty suite, got %d", len(report.TestSuites))
	}
}

func TestJUnitXMLTestReport_Classname(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Pass,
		Files: map[string]*moduletest.File{
			"myfile.tftest.hcl": {
				Name:   "myfile.tftest.hcl",
				Status: moduletest.Pass,
				Runs: []*moduletest.Run{
					{
						Name:   "my_run",
						Status: moduletest.Pass,
						Config: &configs.TestRun{},
					},
				},
			},
		},
	}

	stopped := false
	report := junitXMLTestReport(suite, nil, &stopped)

	tc := report.TestSuites[0].Cases[0]
	if tc.Classname != "myfile.tftest.hcl" {
		t.Errorf("expected classname %q, got %q", "myfile.tftest.hcl", tc.Classname)
	}
	if tc.Name != "my_run" {
		t.Errorf("expected name %q, got %q", "my_run", tc.Name)
	}
}

func TestJUnitXMLTestReport_ErrorWithDiags(t *testing.T) {
	suite := &moduletest.Suite{
		Status: moduletest.Error,
		Files: map[string]*moduletest.File{
			"err.tftest.hcl": {
				Name:   "err.tftest.hcl",
				Status: moduletest.Error,
				Runs: []*moduletest.Run{
					{
						Name:   "error_run",
						Status: moduletest.Error,
						Config: &configs.TestRun{},
						Diagnostics: tfdiags.Diagnostics{
							tfdiags.Sourceless(tfdiags.Error, "Config error", "Invalid configuration"),
							tfdiags.Sourceless(tfdiags.Warning, "Deprecation", "This is deprecated"),
						},
					},
				},
			},
		},
	}

	stopped := false
	sources := map[string]*hcl.File{}
	report := junitXMLTestReport(suite, sources, &stopped)

	tc := report.TestSuites[0].Cases[0]
	if tc.Error == nil {
		t.Fatal("expected error element")
	}
	if tc.Error.Message != "Encountered an error" {
		t.Errorf("expected error message, got %q", tc.Error.Message)
	}
	if !strings.Contains(tc.Error.Body, "Config error") {
		t.Error("error body should contain diagnostic text")
	}
}
