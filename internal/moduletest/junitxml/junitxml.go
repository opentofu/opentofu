// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package junitxml

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

type JunitError struct {
	Message string `xml:"message,attr"`
}

type Failure struct {
	Message string `xml:"message,attr"`
}

type Skipped struct {
	Message string `xml:"message,attr"`
}

type RFC3339 struct{ time.Time }

func (i RFC3339) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(i.Format(time.RFC3339), start)
}

type Testcase struct {
	Assertions int         `xml:"assertions,attr"`
	File       string      `xml:"file,attr"`
	Line       int         `xml:"line,attr"`
	Name       string      `xml:"name,attr"`
	Time       float64     `xml:"time,attr"`
	Error      *JunitError `xml:"error"`
	Failure    *Failure    `xml:"failure"`
	Skipped    *Skipped    `xml:"skipped"`
	SystemErr  *string     `xml:"system-err"`
	SystemOut  *string     `xml:"system-out"`
}

type Testsuite struct {
	Assertions int        `xml:"assertions,attr"`
	Errors     int        `xml:"errors,attr"`
	Failures   int        `xml:"failures,attr"`
	File       string     `xml:"file,attr"`
	Name       string     `xml:"name,attr"`
	Skipped    int        `xml:"skipped,attr"`
	Tests      int        `xml:"tests,attr"`
	Time       float64    `xml:"time,attr"`
	Timestamp  RFC3339    `xml:"timestamp,attr"`
	Testcase   []Testcase `xml:"testcase"`
}

type JUnit struct {
	XMLName xml.Name    `xml:"testsuites"`
	Suites  []Testsuite `xml:"testsuite"`
}

func (j *JUnit) WriteXML(w io.Writer) error {
	encoder := xml.NewEncoder(w)
	defer encoder.Close()
	encoder.Indent("", "  ")
	if _, err := w.Write([]byte(xml.Header)); err != nil {

	}
	if err := encoder.Encode(w); err != nil {
		return fmt.Errorf("failed to write JUnit xml: %w", err)
	}
	return nil
}
