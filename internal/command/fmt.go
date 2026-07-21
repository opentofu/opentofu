// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofufmt"
)

var (
	fmtSupportedExts = []string{
		".tf",
		".tofu",
		".tfvars",
		".tftest.hcl",
		".tofutest.hcl",
	}
)

// FmtCommand is a Command implementation that rewrites OpenTofu config
// files to a canonical format and style.
type FmtCommand struct {
	Meta
	input io.Reader // STDIN if nil
}

func (c *FmtCommand) Run(rawArgs []string) int {
	if c.input == nil {
		c.input = os.Stdin
	}

	// new view
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Parse and validate flags
	args, closer, diags := arguments.ParseFmt(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewFmt(c.View)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}

	var output io.Writer
	list := args.List // preserve the original value of -list
	if args.Check {
		// set to true so we can use the list output to check
		// if the input needs formatting
		args.List = true
		args.Write = false
		output = &bytes.Buffer{}
	} else {
		output = view.UserOutputWriter()
	}

	diags = diags.Append(c.fmt(args.Paths, c.input, output, *args))
	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 2
	}

	if args.Check {
		buf := output.(*bytes.Buffer)
		ok := buf.Len() == 0
		if list {
			if _, err := io.Copy(view.UserOutputWriter(), buf); err != nil {
				log.Printf("[ERROR] Unable to write UI output: %s", err)
			}
		}
		if !ok {
			return 3
		}
		return 0
	}
	return 0
}

func (c *FmtCommand) fmt(paths []string, stdin io.Reader, stdout io.Writer, args arguments.Fmt) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	if len(paths) == 0 { // Assuming stdin, then.
		if args.Write {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid arguments",
				"Option -write cannot be used when reading from stdin",
			))
			return diags
		}
		fileDiags := c.processFile("<stdin>", stdin, stdout, args)
		diags = diags.Append(fileDiags)
		return diags
	}

	for _, path := range paths {
		path = c.Meta.WorkingDir.NormalizePath(path)
		info, err := os.Stat(path)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid file or directory path",
				fmt.Sprintf("No file or directory at %s", path),
			))
			return diags
		}
		if info.IsDir() {
			dirDiags := c.processDir(path, stdout, args)
			diags = diags.Append(dirDiags)
		} else {
			fmtd := false
			for _, ext := range fmtSupportedExts {
				if strings.HasSuffix(path, ext) {
					f, err := os.Open(path)
					if err != nil {
						// Open does not produce error messages that are end-user-appropriate,
						// so we'll need to simplify here.
						diags = diags.Append(tfdiags.Sourceless(
							tfdiags.Error,
							"Failed to open file",
							fmt.Sprintf("Failed to read file %s", path),
						))
						continue
					}

					fileDiags := c.processFile(c.Meta.WorkingDir.NormalizePath(path), f, stdout, args)
					diags = diags.Append(fileDiags)
					_ = f.Close()

					// Take note that we processed the file.
					fmtd = true

					// Don't check the remaining extensions.
					break
				}
			}

			if !fmtd {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid file extension",
					"Only .tf, .tofu, .tfvars, .tftest.hcl, and .tofutest.hcl files can be processed with tofu fmt",
				))
				continue
			}
		}
	}

	return diags
}

func (c *FmtCommand) processFile(path string, r io.Reader, w io.Writer, args arguments.Fmt) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	log.Printf("[TRACE] tofu fmt: Formatting %s", path)

	src, err := io.ReadAll(r)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to read file",
			fmt.Sprintf("Failed to load the content of the %q file", path),
		))
		return diags
	}

	// Register this path as a synthetic configuration source, so that any
	// diagnostic errors can include the source code snippet
	c.configLoader().ForceFileSource(path, src)

	// File must be parseable as HCL native syntax before we'll try to format
	// it. If not, the formatter is likely to make drastic changes that would
	// be hard for the user to undo.
	_, syntaxDiags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if syntaxDiags.HasErrors() {
		diags = diags.Append(syntaxDiags)
		return diags
	}

	result := tofufmt.Format(src, path)

	if !bytes.Equal(src, result) {
		// Something was changed
		if args.List {
			_, _ = fmt.Fprintln(w, path)
		}
		if args.Write {
			err := os.WriteFile(path, result, 0644)
			if err != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Writing formatted file failed",
					fmt.Sprintf("Failed to write %s", path),
				))
				return diags
			}
		}
		if args.Diff {
			diff, err := bytesDiff(src, result, path)
			if err != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to generate diff",
					fmt.Sprintf("Failed to generate diff for %s: %s", path, err),
				))
				return diags
			}
			if _, err := w.Write(diff); err != nil {
				return diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to write format diff",
					err.Error(),
				))
			}
		}
	}

	if !args.List && !args.Write && !args.Diff {
		_, err = w.Write(result)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Writing results failed",
				fmt.Sprintf("Failed to write result: %s", err),
			))
		}
	}

	return diags
}

func (c *FmtCommand) processDir(path string, stdout io.Writer, args arguments.Fmt) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	log.Printf("[TRACE] tofu fmt: looking for files in %s", path)

	entries, err := os.ReadDir(path)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid path",
				fmt.Sprintf("There is no configuration directory at %s", path),
			))
		default:
			// ReadDir does not produce error messages that are end-user-appropriate,
			// so we'll need to simplify here.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid path",
				fmt.Sprintf("Cannot read directory %s", path),
			))
		}
		return diags
	}

	for _, info := range entries {
		name := info.Name()
		if configs.IsIgnoredFile(name) {
			continue
		}
		subPath := filepath.Join(path, name)
		if info.IsDir() {
			if args.Recursive {
				subDiags := c.processDir(subPath, stdout, args)
				diags = diags.Append(subDiags)
			}

			// We do not recurse into child directories by default because we
			// want to mimic the file-reading behavior of "tofu plan", etc,
			// operating on one module at a time.
			continue
		}

		for _, ext := range fmtSupportedExts {
			if strings.HasSuffix(name, ext) {
				f, err := os.Open(subPath)
				if err != nil {
					// Open does not produce error messages that are end-user-appropriate,
					// so we'll need to simplify here.
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Failed to open file",
						fmt.Sprintf("Failed to read file %s", path),
					))
					continue
				}

				fileDiags := c.processFile(c.Meta.WorkingDir.NormalizePath(subPath), f, stdout, args)
				diags = diags.Append(fileDiags)
				_ = f.Close()

				// Don't need to check the remaining extensions.
				break
			}
		}
	}

	return diags
}

func (c *FmtCommand) Help() string {
	helpText := `
Usage: tofu [global options] fmt [options] [target...]

  Rewrites all OpenTofu configuration files to a canonical format. All
  configuration files (.tf), variables files (.tfvars), and testing files 
  (.tftest.hcl) are updated. JSON files (.tf.json, .tfvars.json, or 
  .tftest.json) are not modified.

  By default, fmt scans the current directory for configuration files. If you
  provide a directory for the target argument, then fmt will scan that
  directory instead. If you provide a file, then fmt will process just that
  file. If you provide a single dash ("-"), then fmt will read from standard
  input (STDIN).

  The content must be in the OpenTofu language native syntax; JSON is not
  supported.

Options:

  -list=false    Don't list files whose formatting differs
                 (always disabled if using STDIN)

  -write=false   Don't write to source files
                 (always disabled if using STDIN or -check)

  -diff          Display diffs of formatting changes

  -check         Check if the input is formatted. Exit status will be 0 if all
                 input is properly formatted and non-zero otherwise.

  -no-color      If specified, output won't contain any color.

  -recursive     Also process files in subdirectories. By default, only the
                 given directory (or current directory) is processed.
`
	return strings.TrimSpace(helpText)
}

func (c *FmtCommand) Synopsis() string {
	return "Reformat your configuration in the standard style"
}

func withTempFile(b []byte, fn func(*os.File) error) error {
	f, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = fn(f)
	}
	err = errors.Join(err, f.Close())
	err = errors.Join(err, os.Remove(f.Name()))
	return err
}

func bytesDiff(b1, b2 []byte, path string) (data []byte, err error) {
	err = withTempFile(b1, func(f1 *os.File) error {
		return withTempFile(b2, func(f2 *os.File) error {
			data, err = exec.Command("diff", "--label=old/"+path, "--label=new/"+path, "-u", f1.Name(), f2.Name()).CombinedOutput()
			if len(data) > 0 {
				// diff exits with a non-zero status when the files don't match.
				// Ignore that failure as long as we get output.
				err = nil
			}
			return err
		})
	})

	return
}
