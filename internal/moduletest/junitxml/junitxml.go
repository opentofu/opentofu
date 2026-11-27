// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package junitxml contains types that are used to generate JUnit
// compatible XML files from `tofu test` outputs.
//
// The structure found here are based on the structures documented in
// the https://github.com/testmoapp/junitxml repository. The
// structures have been adapted such that a JUnit test case maps to a
// `tofu test` run and each JUnit suite is mapped onto a tftest file.
package junitxml

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

// JunitError is used to include the message from a `tofu test` run
// that resulted in an error.
type JunitError struct {
	// Message is the error message from the test case.
	Message string `xml:"message,attr"`
}

// Failure is used to include the message from a `tofu test` run that
// resulted in a failure.
type Failure struct {
	// Message is the failure message from the test case.
	Message string `xml:"message,attr"`
}

// Skipped is used to include a message indicating a test was skipped
// in a `tofu test` run
type Skipped struct {
	Message string `xml:"message,attr"`
}

// RFC3339 is used to marshal time in a JUnit compatible time format.
type RFC3339 struct{ time.Time }

func (i RFC3339) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(i.Format(time.RFC3339), start)
}

// Testcase is used to represent the assertions and expect_failures in a `tofu test` run.
type Testcase struct {
	// Assertions is total number of assertions and `expect_failures` in the run.
	Assertions int `xml:"assertions,attr"`
	// File is the file the run is being executed from.
	File string `xml:"file,attr"`
	// Line is the line of assertion in the run.
	Line int `xml:"line,attr"`
	// Name is the name of the run.
	Name string `xml:"name,attr"`
	// Time is the total time in seconds for the run to complete.
	Time float64 `xml:"time,attr"`
	// Error contains the error message for test cases that resulted
	// in an `error` status.
	Error *JunitError `xml:"error"`
	// Failure contains the assertion failure message for test cases
	// that resulted in a `failed` status.
	Failure *Failure `xml:"failure"`
	// Skipped is used to indicate an assertion was skipped.
	Skipped *Skipped `xml:"skipped"`
}

// Testsuite represents all of the test runs that are contained within
// a tftest file.
type Testsuite struct {
	// Assertions is the total number of assertions found within the test suite.
	Assertions int `xml:"assertions,attr"`
	// Errors is the count of the total number of tests that ended in an error.
	Errors int `xml:"errors,attr"`
	// Failures is the count of the total number of tests that ended in failure.
	Failures int `xml:"failures,attr"`
	// File is the name of the file for this test suite.
	File string `xml:"file,attr"`
	// Name is the name of the file for this test suite.
	Name string `xml:"name,attr"`
	// Skipped is the total number of skipped tests from this test suite.
	Skipped int `xml:"skipped,attr"`
	// Tests is the total number of runs in this test suite.
	Tests int `xml:"tests,attr"`
	// Time is the total time taken for this test suite.
	Time float64 `xml:"time,attr"`
	// Timestamp is the date this test suite was run.
	Timestamp RFC3339 `xml:"timestamp,attr"`
	// Testcase contains the list of `tofu test` runs.
	Testcase []Testcase `xml:"testcase"`
}

// JUnit is a structure to represent a complete JUnit XML file.
type JUnit struct {
	XMLName xml.Name `xml:"testsuites"`
	// Suites represts all of the tftest files run when executing `tofu test`.
	Suites []Testsuite `xml:"testsuite"`
}

// WriteXML writes out the JUnitXML file to a writer.
func (j *JUnit) WriteXML(w io.Writer) error {
	encoder := xml.NewEncoder(w)
	defer encoder.Close()
	encoder.Indent("", "  ")
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return fmt.Errorf("failed to write JUnit xml header: %w", err)
	}
	if err := encoder.Encode(j); err != nil {
		return fmt.Errorf("failed to write JUnit xml: %w", err)
	}
	return nil
}
