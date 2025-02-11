// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/marks"
)

func TestBase64Decode(t *testing.T) {
	tests := []struct {
		String cty.Value
		Want   cty.Value
		Err    bool
	}{
		{
			cty.StringVal("YWJjMTIzIT8kKiYoKSctPUB+"),
			cty.StringVal("abc123!?$*&()'-=@~"),
			false,
		},
		{
			cty.StringVal("YWJjMTIzIT8kKiYoKSctPUB+").Mark(marks.Sensitive),
			cty.StringVal("abc123!?$*&()'-=@~").Mark(marks.Sensitive),
			false,
		},
		{ // Invalid base64 data decoding
			cty.StringVal("this-is-an-invalid-base64-data"),
			cty.UnknownVal(cty.String),
			true,
		},
		{ // Invalid utf-8
			cty.StringVal("\xc3\x28"),
			cty.UnknownVal(cty.String),
			true,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("base64decode(%#v)", test.String), func(t *testing.T) {
			got, err := Base64Decode(test.String)

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

func TestBase64Decode_error(t *testing.T) {
	tests := map[string]struct {
		String  cty.Value
		WantErr string
	}{
		"invalid base64": {
			cty.StringVal("dfg"),
			`failed to decode base64 data "dfg"`,
		},
		"sensitive invalid base64": {
			cty.StringVal("dfg").Mark(marks.Sensitive),
			`failed to decode base64 data (sensitive value)`,
		},
		"invalid utf-8": {
			cty.StringVal("whee"),
			"the result of decoding the provided string is not valid UTF-8",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Base64Decode(test.String)

			if err == nil {
				t.Fatal("succeeded; want error")
			}

			if err.Error() != test.WantErr {
				t.Errorf("wrong error result\ngot:  %#v\nwant: %#v", err.Error(), test.WantErr)
			}
		})
	}
}

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		String cty.Value
		Want   cty.Value
		Err    bool
	}{
		{
			cty.StringVal("abc123!?$*&()'-=@~"),
			cty.StringVal("YWJjMTIzIT8kKiYoKSctPUB+"),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("base64encode(%#v)", test.String), func(t *testing.T) {
			got, err := Base64Encode(test.String)

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

func TestBase64Gzip(t *testing.T) {
	tests := []struct {
		String cty.Value
		Want   cty.Value
		Err    bool
	}{
		{
			cty.StringVal("test"),
			cty.StringVal("H4sIAAAAAAAA/ypJLS4BAAAA//8BAAD//wx+f9gEAAAA"),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("base64gzip(%#v)", test.String), func(t *testing.T) {
			got, err := Base64Gzip(test.String)

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

func TestBase64Gunzip(t *testing.T) {
	tests := []struct {
		String cty.Value
		Want   cty.Value
		Err    string
	}{
		{
			cty.StringVal("H4sIAAAAAAAA/ypJLS4BAAAA//8BAAD//wx+f9gEAAAA"),
			cty.StringVal("test"),
			"",
		},
		{
			cty.StringVal("H4sIAAAAAAAA/ypJLS4BAAAA//8BAAD//wx+f9gEAAAA").Mark(marks.Sensitive),
			cty.StringVal("test").Mark(marks.Sensitive),
			"",
		},
		{
			cty.StringVal("hello"),
			cty.NilVal,
			`failed to decode base64 data "hello"`,
		},
		{
			cty.StringVal("hello").Mark(marks.Sensitive),
			cty.NilVal,
			`failed to decode base64 data (sensitive value)`,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("base64gunzip(%#v)", test.String), func(t *testing.T) {
			got, err := Base64Gunzip(test.String)

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if err.Error() != test.Err {
					t.Fatalf("got unexpected error: %v", err.Error())
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

func TestURLEncode(t *testing.T) {
	tests := []struct {
		String cty.Value
		Want   cty.Value
		Err    bool
	}{
		{
			cty.StringVal("abc123-_"),
			cty.StringVal("abc123-_"),
			false,
		},
		{
			cty.StringVal("foo:bar@localhost?foo=bar&bar=baz"),
			cty.StringVal("foo%3Abar%40localhost%3Ffoo%3Dbar%26bar%3Dbaz"),
			false,
		},
		{
			cty.StringVal("mailto:email?subject=this+is+my+subject"),
			cty.StringVal("mailto%3Aemail%3Fsubject%3Dthis%2Bis%2Bmy%2Bsubject"),
			false,
		},
		{
			cty.StringVal("foo/bar"),
			cty.StringVal("foo%2Fbar"),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("urlencode(%#v)", test.String), func(t *testing.T) {
			got, err := URLEncode(test.String)

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

func TestURLDecode(t *testing.T) {
	tests := []struct {
		String cty.Value
		Want   cty.Value
		Err    bool
	}{
		{
			cty.StringVal("abc123-_"),
			cty.StringVal("abc123-_"),
			false,
		},
		{
			cty.StringVal("foo%3Abar%40localhost%3Ffoo%3Dbar%26bar%3Dbaz"),
			cty.StringVal("foo:bar@localhost?foo=bar&bar=baz"),
			false,
		},
		{
			cty.StringVal("mailto%3Aemail%3Fsubject%3Dthis%2Bis%2Bmy%2Bsubject"),
			cty.StringVal("mailto:email?subject=this+is+my+subject"),
			false,
		},
		{
			cty.StringVal("foo%2Fbar"),
			cty.StringVal("foo/bar"),
			false,
		},
		{
			cty.StringVal("foo% bar"),
			cty.UnknownVal(cty.String),
			true,
		},
		{
			cty.StringVal("foo%2 bar"),
			cty.UnknownVal(cty.String),
			true,
		},
		{
			cty.StringVal("%GGfoo%2bar"),
			cty.UnknownVal(cty.String),
			true,
		},
		{
			cty.StringVal("foo%00, bar!"),
			cty.StringVal("foo\x00, bar!"),
			false,
		},
		{
			cty.StringVal("hello%20%E4%B8%96%E7%95%8C"), //Unicode character support
			cty.StringVal("hello 世界"),
			false,
		},
		{
			cty.StringVal("hello%20%D8%AF%D9%86%DB%8C%D8%A7"), //Unicode character support
			cty.StringVal("hello دنیا"),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("urldecode(%#v)", test.String), func(t *testing.T) {
			got, err := URLDecode(test.String)

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

func TestURLEncodeDecode(t *testing.T) {
	tests := []struct {
		String cty.Value
		Want   cty.Value
		Err    bool
	}{
		{
			cty.StringVal("abc123-_"),
			cty.StringVal("abc123-_"),
			false,
		},
		{
			cty.StringVal("foo:bar@localhost?foo=bar&bar=baz"),
			cty.StringVal("foo:bar@localhost?foo=bar&bar=baz"),
			false,
		},
		{
			cty.StringVal("mailto:email?subject=this+is+my+subject"),
			cty.StringVal("mailto:email?subject=this+is+my+subject"),
			false,
		},
		{
			cty.StringVal("foo/bar"),
			cty.StringVal("foo/bar"),
			false,
		},
		{
			cty.StringVal("foo%00, bar!"),
			cty.StringVal("foo%00, bar!"),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("url encode decode(%#v)", test.String), func(t *testing.T) {
			encoded, err := URLEncode(test.String)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			got, err := URLDecode(encoded)

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

func TestBase64TextEncode(t *testing.T) {
	tests := []struct {
		String   cty.Value
		Encoding cty.Value
		Want     cty.Value
		Err      string
	}{
		{
			cty.StringVal("abc123!?$*&()'-=@~"),
			cty.StringVal("UTF-8"),
			cty.StringVal("YWJjMTIzIT8kKiYoKSctPUB+"),
			``,
		},
		{
			cty.StringVal("abc123!?$*&()'-=@~"),
			cty.StringVal("UTF-16LE"),
			cty.StringVal("YQBiAGMAMQAyADMAIQA/ACQAKgAmACgAKQAnAC0APQBAAH4A"),
			``,
		},
		{
			cty.StringVal("abc123!?$*&()'-=@~"),
			cty.StringVal("CP936"),
			cty.StringVal("YWJjMTIzIT8kKiYoKSctPUB+"),
			``,
		},
		{
			cty.StringVal("abc123!?$*&()'-=@~"),
			cty.StringVal("NOT-EXISTS"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			`"NOT-EXISTS" is not a supported IANA encoding name or alias in this OpenTofu version`,
		},
		{
			cty.StringVal("🤔"),
			cty.StringVal("cp437"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			`the given string contains characters that cannot be represented in IBM437`,
		},
		{
			cty.UnknownVal(cty.String),
			cty.StringVal("windows-1250"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			``,
		},
		{
			cty.StringVal("hello world"),
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String).RefineNotNull(),
			``,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("textencodebase64(%#v, %#v)", test.String, test.Encoding), func(t *testing.T) {
			got, err := TextEncodeBase64(test.String, test.Encoding)

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got, want := err.Error(), test.Err; got != want {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", got, want)
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

func TestBase64TextDecode(t *testing.T) {
	tests := []struct {
		String   cty.Value
		Encoding cty.Value
		Want     cty.Value
		Err      string
	}{
		{
			cty.StringVal("YWJjMTIzIT8kKiYoKSctPUB+"),
			cty.StringVal("UTF-8"),
			cty.StringVal("abc123!?$*&()'-=@~"),
			``,
		},
		{
			cty.StringVal("YQBiAGMAMQAyADMAIQA/ACQAKgAmACgAKQAnAC0APQBAAH4A"),
			cty.StringVal("UTF-16LE"),
			cty.StringVal("abc123!?$*&()'-=@~"),
			``,
		},
		{
			cty.StringVal("YWJjMTIzIT8kKiYoKSctPUB+"),
			cty.StringVal("CP936"),
			cty.StringVal("abc123!?$*&()'-=@~"),
			``,
		},
		{
			cty.StringVal("doesn't matter"),
			cty.StringVal("NOT-EXISTS"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			`"NOT-EXISTS" is not a supported IANA encoding name or alias in this OpenTofu version`,
		},
		{
			cty.StringVal("<invalid base64>"),
			cty.StringVal("cp437"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			`the given value is has an invalid base64 symbol at offset 0`,
		},
		{
			cty.StringVal("gQ=="), // this is 0x81, which is not defined in windows-1250
			cty.StringVal("windows-1250"),
			cty.StringVal("�"),
			`the given string contains symbols that are not defined for windows-1250`,
		},
		{
			cty.UnknownVal(cty.String),
			cty.StringVal("windows-1250"),
			cty.UnknownVal(cty.String).RefineNotNull(),
			``,
		},
		{
			cty.StringVal("YQBiAGMAMQAyADMAIQA/ACQAKgAmACgAKQAnAC0APQBAAH4A"),
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String).RefineNotNull(),
			``,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("textdecodebase64(%#v, %#v)", test.String, test.Encoding), func(t *testing.T) {
			got, err := TextDecodeBase64(test.String, test.Encoding)

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got, want := err.Error(), test.Err; got != want {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", got, want)
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
