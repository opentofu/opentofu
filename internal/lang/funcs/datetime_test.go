// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"
	"testing"
	"time"

	"github.com/zclconf/go-cty/cty"
)

func TestTimestamp(t *testing.T) {
	currentTime := time.Now().UTC()
	result, err := Timestamp()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	resultTime, err := time.Parse(time.RFC3339, result.AsString())
	if err != nil {
		t.Fatalf("Error parsing timestamp: %s", err)
	}

	if resultTime.Sub(currentTime).Seconds() > 10.0 {
		t.Fatalf("Timestamp Diff too large. Expected: %s\nReceived: %s", currentTime.Format(time.RFC3339), result.AsString())
	}

}

func TestTimeadd(t *testing.T) {
	tests := []struct {
		Time     cty.Value
		Duration cty.Value
		Want     cty.Value
		Err      bool
	}{
		{
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.StringVal("1s"),
			cty.StringVal("2017-11-22T00:00:01Z"),
			false,
		},
		{
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.StringVal("10m1s"),
			cty.StringVal("2017-11-22T00:10:01Z"),
			false,
		},
		{ // also support subtraction
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.StringVal("-1h"),
			cty.StringVal("2017-11-21T23:00:00Z"),
			false,
		},
		{ // Invalid format timestamp
			cty.StringVal("2017-11-22"),
			cty.StringVal("-1h"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			true,
		},
		{ // Invalid format duration (day is not supported by ParseDuration)
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.StringVal("1d"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			true,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("TimeAdd(%#v, %#v)", test.Time, test.Duration), func(t *testing.T) {
			got, err := TimeAdd(test.Time, test.Duration)

			if test.Err {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !got.RawEquals(test.Want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}

func TestTimeCmp(t *testing.T) {
	tests := []struct {
		TimeA, TimeB cty.Value
		Want         cty.Value
		Err          string
	}{
		{
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.Zero,
			``,
		},
		{
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.StringVal("2017-11-22T01:00:00+01:00"),
			cty.Zero,
			``,
		},
		{
			cty.StringVal("2017-11-22T00:00:01Z"),
			cty.StringVal("2017-11-22T01:00:00+01:00"),
			cty.NumberIntVal(1),
			``,
		},
		{
			cty.StringVal("2017-11-22T01:00:00Z"),
			cty.StringVal("2017-11-22T00:59:00-01:00"),
			cty.NumberIntVal(-1),
			``,
		},
		{
			cty.StringVal("2017-11-22T01:00:00+01:00"),
			cty.StringVal("2017-11-22T01:00:00-01:00"),
			cty.NumberIntVal(-1),
			``,
		},
		{
			cty.StringVal("2017-11-22T01:00:00-01:00"),
			cty.StringVal("2017-11-22T01:00:00+01:00"),
			cty.NumberIntVal(1),
			``,
		},
		{
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.StringVal("bloop"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			`not a valid RFC3339 timestamp: cannot use "bloop" as year`,
		},
		{
			cty.StringVal("2017-11-22 00:00:00Z"),
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			`not a valid RFC3339 timestamp: missing required time introducer 'T'`,
		},
		{
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.Number).RefineNotNull(),
			``,
		},
		{
			cty.UnknownVal(cty.String),
			cty.StringVal("2017-11-22T00:00:00Z"),
			cty.UnknownVal(cty.Number).RefineNotNull(),
			``,
		},
		{
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.Number).RefineNotNull(),
			``,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("TimeCmp(%#v, %#v)", test.TimeA, test.TimeB), func(t *testing.T) {
			got, err := TimeCmp(test.TimeA, test.TimeB)

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got := err.Error(); got != test.Err {
					t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, test.Err)
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !got.RawEquals(test.Want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}

func TestMakeStaticTimestampFunc(t *testing.T) {
	tests := []struct {
		Name string
		// Setup made like this to bind the generated time value to the wanted value.
		Setup func() (time.Time, cty.Value)
	}{
		{
			Name: "zero",
			Setup: func() (time.Time, cty.Value) {
				in := time.Time{}
				out := cty.UnknownVal(cty.String)
				return in, out
			},
		},
		{
			Name: "now",
			Setup: func() (time.Time, cty.Value) {
				in := time.Now()
				out := cty.StringVal(in.Format(time.RFC3339))
				return in, out
			},
		},
		{
			Name: "one year later",
			Setup: func() (time.Time, cty.Value) {
				in := time.Now().Add(8766 * time.Hour) // 1 year later
				out := cty.StringVal(in.Format(time.RFC3339))
				return in, out
			},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("MakeStaticTimestampFunc(%s)", test.Name), func(t *testing.T) {
			in, want := test.Setup()
			got, err := MakeStaticTimestampFunc(in).Call(nil)
			if err != nil {
				t.Fatalf("MakeStaticTimestampFunc is not meant to return error but got one: %v", err)
			}
			if !got.RawEquals(want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, want)
			}
		})
	}
}
