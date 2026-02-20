// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package junit

import (
	"encoding/xml"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/format"
	"github.com/opentofu/opentofu/internal/moduletest"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// JUnit is the interface for writing JUnit XML test reports.
type JUnit interface {
	Save(suite *moduletest.Suite) tfdiags.Diagnostics
}

// TestJUnitXMLFile writes JUnit XML test results to a file.
type TestJUnitXMLFile struct {
	filename string
	sources  map[string]*hcl.File
	stopped  *bool
}

// NewTestJUnitXMLFile creates a new JUnit XML file writer.
func NewTestJUnitXMLFile(filename string, sources map[string]*hcl.File, stopped *bool) *TestJUnitXMLFile {
	return &TestJUnitXMLFile{
		filename: filename,
		sources:  sources,
		stopped:  stopped,
	}
}

// Save generates JUnit XML from the test suite results and writes it to the
// configured file.
func (j *TestJUnitXMLFile) Save(suite *moduletest.Suite) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	report := junitXMLTestReport(suite, j.sources, j.stopped)

	data, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to serialize JUnit XML",
			fmt.Sprintf("Could not marshal JUnit XML report: %s", err),
		))
		return diags
	}

	content := []byte(xml.Header)
	content = append(content, data...)
	content = append(content, '\n')

	if err := os.WriteFile(j.filename, content, 0644); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to write JUnit XML file",
			fmt.Sprintf("Could not write JUnit XML report to %q: %s", j.filename, err),
		))
		return diags
	}

	return diags
}

// XML structs for JUnit output

type testSuites struct {
	XMLName    xml.Name    `xml:"testsuites"`
	TestSuites []testSuite `xml:"testsuite"`
}

type testSuite struct {
	Name     string     `xml:"name,attr"`
	Tests    int        `xml:"tests,attr"`
	Skipped  int        `xml:"skipped,attr"`
	Failures int        `xml:"failures,attr"`
	Errors   int        `xml:"errors,attr"`
	Cases    []testCase `xml:"testcase"`
}

type testCase struct {
	Name      string       `xml:"name,attr"`
	Classname string       `xml:"classname,attr"`
	Failure   *withMessage `xml:"failure,omitempty"`
	Skipped   *withMessage `xml:"skipped,omitempty"`
	Error     *withMessage `xml:"error,omitempty"`
}

type withMessage struct {
	Message string `xml:"message,attr,omitempty"`
	Body    string `xml:",chardata"`
}

// junitXMLTestReport builds the full JUnit XML report from a test suite.
func junitXMLTestReport(suite *moduletest.Suite, sources map[string]*hcl.File, stopped *bool) testSuites {
	var suites []testSuite

	for _, fileName := range slices.Sorted(maps.Keys(suite.Files)) {
		file := suite.Files[fileName]

		ts := testSuite{
			Name: fileName,
		}

		hasFileLevelErrors := file.Diagnostics.HasErrors()

		for _, run := range file.Runs {
			ts.Tests++

			tc := testCase{
				Name:      run.Name,
				Classname: fileName,
			}

			switch run.Status {
			case moduletest.Skip:
				ts.Skipped++
				reason := skipDetails(run.Index, file, stopped, hasFileLevelErrors)
				tc.Skipped = &withMessage{
					Message: reason,
				}

			case moduletest.Fail:
				ts.Failures++
				failedCount, checkCount := countAssertions(run)
				tc.Failure = &withMessage{
					Message: failureMessage(failedCount, checkCount),
					Body:    getDiagString(filterAssertionDiags(run.Diagnostics), sources),
				}

			case moduletest.Error:
				ts.Errors++
				tc.Error = &withMessage{
					Message: "Encountered an error",
					Body:    getDiagString(run.Diagnostics, sources),
				}

			case moduletest.Pass:
				// No extra elements needed for passing tests.

			case moduletest.Pending:
				// Pending tests are treated as skipped in JUnit.
				ts.Skipped++
				tc.Skipped = &withMessage{
					Message: "Test did not run",
				}
			}

			ts.Cases = append(ts.Cases, tc)
		}

		suites = append(suites, ts)
	}

	return testSuites{
		TestSuites: suites,
	}
}

// countAssertions counts failed assertions and total check rules for a run.
func countAssertions(run *moduletest.Run) (failed, total int) {
	total = len(run.Config.CheckRules)
	for _, diag := range run.Diagnostics {
		if diagnosticCausedByCheckAssertion(diag) {
			failed++
		}
	}
	return failed, total
}

// failureMessage formats a human-readable failure summary.
func failureMessage(failedCount, checkCount int) string {
	if checkCount == 0 {
		return "failed"
	}
	checkWord := "assertions"
	if checkCount == 1 {
		checkWord = "assertion"
	}
	return fmt.Sprintf("%d of %d %s failed", failedCount, checkCount, checkWord)
}

// skipDetails determines why a run was skipped and returns a reason string.
func skipDetails(runIndex int, file *moduletest.File, stopped *bool, hasFileLevelErrors bool) string {
	if stopped != nil && *stopped {
		return "Test skipped due to interrupt"
	}
	if hasFileLevelErrors {
		return "Test skipped due to earlier error in test file"
	}
	// Check if a previous run in the same file had an error.
	for i := 0; i < runIndex && i < len(file.Runs); i++ {
		if file.Runs[i].Status == moduletest.Error {
			return "Test skipped due to earlier error"
		}
	}
	return "Test skipped"
}

// filterAssertionDiags returns only diagnostics caused by check assertions.
func filterAssertionDiags(diags tfdiags.Diagnostics) tfdiags.Diagnostics {
	var filtered tfdiags.Diagnostics
	for _, diag := range diags {
		if diagnosticCausedByCheckAssertion(diag) {
			filtered = filtered.Append(diag)
		}
	}
	return filtered
}

// getDiagString converts diagnostics to plain text for embedding in XML.
func getDiagString(diags tfdiags.Diagnostics, sources map[string]*hcl.File) string {
	var parts []string
	for _, diag := range diags {
		rendered := format.DiagnosticPlain(diag, sources, 80)
		rendered = strings.TrimSpace(rendered)
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, "\n\n")
}

// diagnosticCausedByCheckAssertion returns true if the given diagnostic
// originates from a check assertion rule. This is the OpenTofu equivalent
// of Terraform's DiagnosticCausedByTestFailure.
func diagnosticCausedByCheckAssertion(diag tfdiags.Diagnostic) bool {
	rule, ok := addrs.DiagnosticOriginatesFromCheckRule(diag)
	if !ok {
		return false
	}
	return rule.Type == addrs.CheckAssertion
}
