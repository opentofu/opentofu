// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"strings"
	"testing"
)

func TestModulePath(t *testing.T) {
	t.Run("no args returns working directory", func(t *testing.T) {
		td := t.TempDir()
		t.Chdir(td)

		got, err := modulePath(nil)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		want, err := os.Getwd()
		if err != nil {
			t.Fatalf("os.Getwd: %s", err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty args returns working directory", func(t *testing.T) {
		td := t.TempDir()
		t.Chdir(td)

		got, err := modulePath([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		want, err := os.Getwd()
		if err != nil {
			t.Fatalf("os.Getwd: %s", err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("extra args returns error", func(t *testing.T) {
		_, err := modulePath([]string{"extra-arg"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Too many command line arguments") {
			t.Errorf("unexpected error message: %s", err)
		}
	})
}
