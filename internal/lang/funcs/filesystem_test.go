// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/opentofu/opentofu/internal/lang/marks"
)

func TestFile(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  string
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.StringVal("Hello World"),
			``,
		},
		{
			cty.StringVal("testdata/icon.png"),
			cty.NilVal,
			`contents of "testdata/icon.png" are not valid UTF-8; use the filebase64 function to obtain the Base64 encoded contents or the other file functions (e.g. filemd5, filesha256) to obtain file hashing results instead`,
		},
		{
			cty.StringVal("testdata/icon.png").Mark(marks.Sensitive),
			cty.NilVal,
			`contents of (sensitive value) are not valid UTF-8; use the filebase64 function to obtain the Base64 encoded contents or the other file functions (e.g. filemd5, filesha256) to obtain file hashing results instead`,
		},
		{
			cty.StringVal("testdata/missing"),
			cty.NilVal,
			`no file exists at "testdata/missing"; this function works only with files that are distributed as part of the configuration source code, so if this file will be created by a resource in this configuration you must instead obtain this result from an attribute of that resource`,
		},
		{
			cty.StringVal("testdata/missing").Mark(marks.Sensitive),
			cty.NilVal,
			`no file exists at (sensitive value); this function works only with files that are distributed as part of the configuration source code, so if this file will be created by a resource in this configuration you must instead obtain this result from an attribute of that resource`,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("File(\".\", %#v)", test.Path), func(t *testing.T) {
			got, err := File(".", test.Path)

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got, want := err.Error(), test.Err; got != want {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
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

func TestTemplateFile(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Vars cty.Value
		Want cty.Value
		Err  string
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.EmptyObjectVal,
			cty.StringVal("Hello World"),
			``,
		},
		{
			cty.StringVal("testdata/icon.png"),
			cty.EmptyObjectVal,
			cty.NilVal,
			`contents of "testdata/icon.png" are not valid UTF-8; use the filebase64 function to obtain the Base64 encoded contents or the other file functions (e.g. filemd5, filesha256) to obtain file hashing results instead`,
		},
		{
			cty.StringVal("testdata/missing"),
			cty.EmptyObjectVal,
			cty.NilVal,
			`no file exists at "testdata/missing"; this function works only with files that are distributed as part of the configuration source code, so if this file will be created by a resource in this configuration you must instead obtain this result from an attribute of that resource`,
		},
		{
			cty.StringVal("testdata/secrets.txt").Mark(marks.Sensitive),
			cty.EmptyObjectVal,
			cty.NilVal,
			`no file exists at (sensitive value); this function works only with files that are distributed as part of the configuration source code, so if this file will be created by a resource in this configuration you must instead obtain this result from an attribute of that resource`,
		},
		{
			cty.StringVal("testdata/hello.tmpl"),
			cty.MapVal(map[string]cty.Value{
				"name": cty.StringVal("Jodie"),
			}),
			cty.StringVal("Hello, Jodie!"),
			``,
		},
		{
			cty.StringVal("testdata/hello.tmpl"),
			cty.MapVal(map[string]cty.Value{
				"name!": cty.StringVal("Jodie"),
			}),
			cty.NilVal,
			`invalid template variable name "name!": must start with a letter, followed by zero or more letters, digits, and underscores`,
		},
		{
			cty.StringVal("testdata/hello.tmpl"),
			cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("Jimbo"),
			}),
			cty.StringVal("Hello, Jimbo!"),
			``,
		},
		{
			cty.StringVal("testdata/hello.tmpl"),
			cty.EmptyObjectVal,
			cty.NilVal,
			`vars map does not contain key "name", referenced at testdata/hello.tmpl:1,10-14`,
		},
		{
			cty.StringVal("testdata/func.tmpl"),
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.ListVal([]cty.Value{
					cty.StringVal("a"),
					cty.StringVal("b"),
					cty.StringVal("c"),
				}),
			}),
			cty.StringVal("The items are a, b, c"),
			``,
		},
		{
			cty.StringVal("testdata/recursive.tmpl"),
			cty.MapValEmpty(cty.String),
			cty.NilVal,
			`maximum recursion depth 1024 reached in testdata/recursive.tmpl:1,3-16, testdata/recursive.tmpl:1,3-16 ... `,
		},
		{
			cty.StringVal("testdata/list.tmpl"),
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.ListVal([]cty.Value{
					cty.StringVal("a"),
					cty.StringVal("b"),
					cty.StringVal("c"),
				}),
			}),
			cty.StringVal("- a\n- b\n- c\n"),
			``,
		},
		{
			cty.StringVal("testdata/list.tmpl"),
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.True,
			}),
			cty.NilVal,
			`testdata/list.tmpl:1,13-17: Iteration over non-iterable value; A value of type bool cannot be used as the collection in a 'for' expression.`,
		},
		{
			cty.StringVal("testdata/bare.tmpl"),
			cty.ObjectVal(map[string]cty.Value{
				"val": cty.True,
			}),
			cty.True, // since this template contains only an interpolation, its true value shines through
			``,
		},
		{
			// write to a sensitive file path that exists
			cty.StringVal("testdata/hello.txt").Mark(marks.Sensitive),
			cty.EmptyObjectVal,
			cty.StringVal("Hello World").Mark(marks.Sensitive),
			``,
		},
		{
			cty.StringVal("testdata/bare.tmpl"),
			cty.ObjectVal(map[string]cty.Value{
				"val": cty.True.Mark(marks.Sensitive),
			}),
			cty.True.Mark(marks.Sensitive),
			``,
		},
	}

	templateFileFn := MakeTemplateFileFunc(".", func() map[string]function.Function {
		return map[string]function.Function{
			"join":         stdlib.JoinFunc,
			"templatefile": MakeFileFunc(".", false), // just a placeholder, since templatefile itself overrides this
		}
	})

	for _, test := range tests {
		t.Run(fmt.Sprintf("TemplateFile(%#v, %#v)", test.Path, test.Vars), func(t *testing.T) {
			got, err := templateFileFn.Call([]cty.Value{test.Path, test.Vars})

			var argErr function.ArgError
			if errors.As(err, &argErr) {
				if argErr.Index < 0 || argErr.Index > 1 {
					t.Errorf("ArgError index %d is out of range for templatefile (must be 0 or 1)", argErr.Index)
				}
			}

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got, want := err.Error(), test.Err; got != want {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
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

func Test_templateMaxRecursionDepth(t *testing.T) {
	tests := []struct {
		Input string
		Want  int
		Err   string
	}{
		{
			"",
			1024,
			``,
		}, {
			"4096",
			4096,
			``,
		}, {
			"apple",
			-1,
			`invalid value for TF_TEMPLATE_RECURSION_DEPTH: strconv.Atoi: parsing "apple": invalid syntax`,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("templateMaxRecursion(%s)", test.Input), func(t *testing.T) {
			os.Setenv("TF_TEMPLATE_RECURSION_DEPTH", test.Input)
			got, err := templateMaxRecursionDepth()
			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got, want := err.Error(), test.Err; got != want {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got != test.Want {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  string
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.BoolVal(true),
			``,
		},
		{
			cty.StringVal(""),
			cty.BoolVal(false),
			`"." is a directory, not a file`,
		},
		{
			cty.StringVal("testdata").Mark(marks.Sensitive),
			cty.BoolVal(false),
			`(sensitive value) is a directory, not a file`,
		},
		{
			cty.StringVal("testdata/missing"),
			cty.BoolVal(false),
			``,
		},
		{
			cty.StringVal("testdata/unreadable/foobar"),
			cty.BoolVal(false),
			`failed to stat "testdata/unreadable/foobar"`,
		},
		{
			cty.StringVal("testdata/unreadable/foobar").Mark(marks.Sensitive),
			cty.BoolVal(false),
			`failed to stat (sensitive value)`,
		},
	}

	// Ensure "unreadable" directory cannot be listed during the test run
	fi, err := os.Lstat("testdata/unreadable")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("testdata/unreadable", 0000); err != nil {
		t.Fatal(err)
	}
	defer func(mode os.FileMode) {
		if err := os.Chmod("testdata/unreadable", mode); err != nil {
			panic(err)
		}
	}(fi.Mode())

	for _, test := range tests {
		t.Run(fmt.Sprintf("FileExists(\".\", %#v)", test.Path), func(t *testing.T) {
			got, err := FileExists(".", test.Path)

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got, want := err.Error(), test.Err; got != want {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
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

func TestFileSet(t *testing.T) {
	tests := []struct {
		Path    cty.Value
		Pattern cty.Value
		Want    cty.Value
		Err     string
	}{
		{
			cty.StringVal("."),
			cty.StringVal("testdata*"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("{testdata,missing}"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata/missing"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata/missing*"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("*/missing"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("**/missing"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata/*.txt"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata/hello.txt"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata/hello.???"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata/hello*"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.tmpl"),
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("testdata/hello.{tmpl,txt}"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.tmpl"),
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("*/hello.txt"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("*/*.txt"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("*/hello*"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.tmpl"),
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("**/hello*"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.tmpl"),
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("**/hello.{tmpl,txt}"),
			cty.SetVal([]cty.Value{
				cty.StringVal("testdata/hello.tmpl"),
				cty.StringVal("testdata/hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("."),
			cty.StringVal("["),
			cty.SetValEmpty(cty.String),
			`failed to glob pattern "[": syntax error in pattern`,
		},
		{
			cty.StringVal("."),
			cty.StringVal("[").Mark(marks.Sensitive),
			cty.SetValEmpty(cty.String),
			`failed to glob pattern (sensitive value): syntax error in pattern`,
		},
		{
			cty.StringVal("."),
			cty.StringVal("\\"),
			cty.SetValEmpty(cty.String),
			`failed to glob pattern "\\": syntax error in pattern`,
		},
		{
			cty.StringVal("testdata"),
			cty.StringVal("missing"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("testdata"),
			cty.StringVal("missing*"),
			cty.SetValEmpty(cty.String),
			``,
		},
		{
			cty.StringVal("testdata"),
			cty.StringVal("*.txt"),
			cty.SetVal([]cty.Value{
				cty.StringVal("hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("testdata"),
			cty.StringVal("hello.txt"),
			cty.SetVal([]cty.Value{
				cty.StringVal("hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("testdata"),
			cty.StringVal("hello.???"),
			cty.SetVal([]cty.Value{
				cty.StringVal("hello.txt"),
			}),
			``,
		},
		{
			cty.StringVal("testdata"),
			cty.StringVal("hello*"),
			cty.SetVal([]cty.Value{
				cty.StringVal("hello.tmpl"),
				cty.StringVal("hello.txt"),
			}),
			``,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("FileSet(\".\", %#v, %#v)", test.Path, test.Pattern), func(t *testing.T) {
			got, err := FileSet(".", test.Path, test.Pattern)

			if test.Err != "" {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				if got, want := err.Error(), test.Err; got != want {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
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

func TestFileBase64(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  bool
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.StringVal("SGVsbG8gV29ybGQ="),
			false,
		},
		{
			cty.StringVal("testdata/icon.png"),
			cty.StringVal("iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAACXBIWXMAAAsTAAALEwEAmpwYAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAINSURBVHgBjZNdSBRRFMf/M7tttl9zsdBld7EbZdi6xRZR0UPUU0UvPaeLC71JHxCFWg+2RAkhNRVaSLELGRRIQYX41gci9Bz2UMSOkWgY7szO6vqxznVmxGFdd1f/cGDmnnN+5z/DuUAFERohetBKNbZyjQ5ndduxkPpay2vtC7ZabNseGJuTJ2VsJG8wfDV0sD7dc4ewia8ww3jef6g+5Qk0xorrOWtqMHzS48onoqenacvZ//C6tHXwJwM1+DAsSNI/R1wdH01an+D1h2/7dywkBrt/kRORLLY6WEl3R0MzOLxvlgyOkPNcVQ03r0595ldSjEbPLeKKuAtvvxCUk5G72deI5gstYOB2Gmf8arLpzDz6buWg5Kpx6vJejHx3Wz/s2w8XLokHoDjvYujjJ7RejFpQe+GEOp+GjtisDuPRlfSR98M5jE9twZ5wE14k2iB4PWadoqilAUqWh+DWTNDT9ixeDVXhz+INdFxrRTnxhS+9A3rDJG+CLFdBPyppDcCwL7iZCSqEbALASV1Jpz7dZgJWQFrJBiWjovf5S32B2NiahLFlkSMNqQdxmmY/fcyI/seU9b95xwzJSobd6+5hdaHjKbe+dKt9XPEEA7Q7sNR5vTlHzXTtQ/P8vvhM/i39jc9MjIqF9Vwpm8TXQPO8Pabb7CREkNNy5pHdkRVlSdr4MhWDCKWkUs0yuFTBNunfXEAAAAAASUVORK5CYII="),
			false,
		},
		{
			cty.StringVal("testdata/missing"),
			cty.NilVal,
			true, // no file exists
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("FileBase64(\".\", %#v)", test.Path), func(t *testing.T) {
			got, err := FileBase64(".", test.Path)

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

func TestBasename(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  bool
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.StringVal("hello.txt"),
			false,
		},
		{
			cty.StringVal("hello.txt"),
			cty.StringVal("hello.txt"),
			false,
		},
		{
			cty.StringVal(""),
			cty.StringVal("."),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Basename(%#v)", test.Path), func(t *testing.T) {
			got, err := Basename(test.Path)

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

func TestDirname(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  bool
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.StringVal("testdata"),
			false,
		},
		{
			cty.StringVal("testdata/foo/hello.txt"),
			cty.StringVal("testdata/foo"),
			false,
		},
		{
			cty.StringVal("hello.txt"),
			cty.StringVal("."),
			false,
		},
		{
			cty.StringVal(""),
			cty.StringVal("."),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Dirname(%#v)", test.Path), func(t *testing.T) {
			got, err := Dirname(test.Path)

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

func TestPathExpand(t *testing.T) {
	homePath, err := homedir.Dir()
	if err != nil {
		t.Fatalf("Error getting home directory: %v", err)
	}

	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  bool
	}{
		{
			cty.StringVal("~/test-file"),
			cty.StringVal(filepath.Join(homePath, "test-file")),
			false,
		},
		{
			cty.StringVal("~/another/test/file"),
			cty.StringVal(filepath.Join(homePath, "another/test/file")),
			false,
		},
		{
			cty.StringVal("/root/file"),
			cty.StringVal("/root/file"),
			false,
		},
		{
			cty.StringVal("/"),
			cty.StringVal("/"),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Dirname(%#v)", test.Path), func(t *testing.T) {
			got, err := Pathexpand(test.Path)

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
