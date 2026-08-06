// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/opentofu/opentofu/internal/command"
	"github.com/posener/complete"
	"github.com/posener/complete/cmd/install"
)

// This is a *very* rudimentary replacement for the posener/complete helper installation
// TODO:
//   * Multi platform support
//   * Multiple shell support (zsh, fish, pwsh)
//   * Fallbacks for really old versions of these shells?
//   * Consider upstreaming this to urfave/cli

func completions() []completion {
	return []completion{
		completionBash(),
	}
}

func installAutocomplete() error {
	if install.IsInstalled("tofu") {
		println("Uninstalling legacy command completion")
		install.Uninstall("tofu")
	}

	var errs []error
	for _, completion := range completions() {
		errs = append(errs, completion.Install())
	}
	return errors.Join(errs...)
}

func uninstallAutocomplete() error {
	var errs []error
	for _, completion := range completions() {
		errs = append(errs, completion.Uninstall())
	}
	return errors.Join(errs...)
}

type completion struct {
	Install   func() error
	Uninstall func() error
}

var completionNoop = completion{
	Install:   func() error { return nil },
	Uninstall: func() error { return nil },
}

func completionBash() completion {
	// Also consider checking for BASH_COMPLETION_VERSINFO
	_, err := exec.LookPath("bash")
	if err != nil {
		log.Printf("[DEBUG] Unable to locate bash: %s", err.Error())
		return completionNoop
	}

	exec, err := os.Executable()
	if err == nil {
		exec = os.Args[0]
	}
	bashCompletionString := fmt.Sprintf("source <(%s completion bash)", exec)

	home := os.Getenv("HOME")
	if home == "" {
		current, err := user.Current()
		if err == nil && current.HomeDir != "" {
			home = current.HomeDir
		}
	}

	// Supported for at least 25 years: https://github.com/scop/bash-completion/blame/dc0b6fbcf8d70f8a89cac7c035d551b201fcc543/README#L101C18-L101C29
	// We'd prefer to use one of the newer XDG standards, but their support is not yet universal, and detecting the actual supported features is crap.
	fileName := ".bash_completion"
	bashCompletionFile := filepath.Join(home, fileName)

	return completion{
		Install: func() error {
			if home == "" {
				return fmt.Errorf("unable to determine location to install bash user completion scripts")
			}

			stat, err := os.Stat(bashCompletionFile)
			if errors.Is(err, os.ErrNotExist) {
				log.Printf("[DEBUG] Creating bash completion file %s", bashCompletionFile)
				fmt.Printf("Installed bash autocompletion into %s\n", bashCompletionFile)
				return os.WriteFile(bashCompletionFile, []byte(bashCompletionString), 0755)
			} else if err != nil {
				// Invalid access
				return err
			}
			log.Printf("[DEBUG] Checking bash completion file %s for entry", bashCompletionFile)
			contents, err := os.ReadFile(bashCompletionFile)
			if err != nil {
				return err
			}

			for line := range strings.Lines(string(contents)) {
				if strings.TrimSpace(line) == bashCompletionString {
					return fmt.Errorf("bash completion already installed in %s", bashCompletionFile)
				}
			}

			log.Printf("[DEBUG] Appending bash completion file %s", bashCompletionFile)

			f, err := os.OpenFile(bashCompletionFile, os.O_APPEND|os.O_WRONLY, stat.Mode())
			if err != nil {
				return err
			}
			_, err = f.Write([]byte("\n" + bashCompletionString))
			fmt.Printf("Installed bash autocompletion into %s\n", bashCompletionFile)
			return errors.Join(err, f.Close())
		},
		Uninstall: func() error {
			if home == "" {
				return fmt.Errorf("unable to determine location to install bash user completion scripts")
			}

			stat, err := os.Stat(bashCompletionFile)
			if errors.Is(err, os.ErrNotExist) {
				log.Printf("[DEBUG] Bash completion file %s does not exist, nothing to remove", bashCompletionFile)
				return nil
			} else if err != nil {
				// Invalid access
				return err
			}

			log.Printf("[DEBUG] Checking bash completion file %s for entry", bashCompletionFile)
			contents, err := os.ReadFile(bashCompletionFile)
			if err != nil {
				return err
			}

			var withoutCompleteBuffer bytes.Buffer
			for line := range strings.Lines(string(contents)) {
				if strings.TrimSpace(line) != bashCompletionString {
					withoutCompleteBuffer.WriteString(line)
				}
			}
			if withoutCompleteBuffer.Len() == len(contents) {
				fmt.Printf("Bash completion not installed in %s, nothing to remove\n", bashCompletionFile)
				return nil
			}

			withoutComplete := withoutCompleteBuffer.Bytes()
			if strings.TrimSpace(string(withoutComplete)) == "" {
				log.Printf("[DEBUG] Removing bash completion file %s", bashCompletionFile)
				err := os.Remove(bashCompletionFile)
				if err != nil {
					return fmt.Errorf("unable to remove empty bash completion file: %w", err)
				}

				fmt.Printf("Removed bash autocompletion from %s\n", bashCompletionFile)
				return nil
			}

			backupFileName := bashCompletionFile + ".orig"
			err = os.WriteFile(backupFileName, contents, 0644)
			if err != nil {
				return fmt.Errorf("unable to create backup of bash user completion file: %w", err)
			}

			err = os.WriteFile(bashCompletionFile, withoutComplete, stat.Mode())
			if err != nil {
				return fmt.Errorf("unable to write to to bash user completion file: %w, a backup has been saved at %s.", err, backupFileName)
			}
			fmt.Printf("Removed bash autocompletion from %s\n", bashCompletionFile)

			err = os.Remove(backupFileName)
			if err != nil {
				return fmt.Errorf("unable to remove backup file %s", backupFileName)
			}

			return nil
		},
	}
}

func legacyAutocomplete(root command.Command) {
	var builder func(command.Command) complete.Command
	builder = func(cmd command.Command) complete.Command {
		comp := complete.Command{
			Flags: complete.Flags{},
			Sub:   complete.Commands{},
		}
		for name := range cmd.CommandLine.Flags {
			comp.Flags["-"+name] = complete.PredictNothing
			comp.Flags["--"+name] = complete.PredictNothing
		}
		for _, sub := range cmd.Commands {
			comp.Sub[sub.Name] = builder(sub)
		}

		return comp
	}

	complete.New("tofu", builder(root)).Complete()
}
