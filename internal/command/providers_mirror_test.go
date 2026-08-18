// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/workdir"
)

// More thorough tests for providers mirror can be found in the e2etest
func TestProvidersMirror(t *testing.T) {
	// noop example
	t.Run("noop", func(t *testing.T) {
		view, done := testView(t)

		meta := Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		}
		code := RunCommander(t, ProvidersMirrorCommander(), meta, []string{"."})
		output := done(t)
		if code != 0 {
			t.Fatalf("wrong exit code. expected 0, got %d\ngot output:\n%s", code, output.All())
		}
	})

	t.Run("missing arg error", func(t *testing.T) {
		view, done := testView(t)

		meta := Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		}
		code := RunCommander(t, ProvidersMirrorCommander(), meta, []string{"-no-color"})
		output := done(t)
		if code != cli.RunResultHelp {
			t.Fatalf("wrong exit code. expected 1, got %d", code)
		}

		got := output.Stderr()
		if !strings.Contains(got, "The providers mirror command requires an output directory") {
			t.Fatalf("missing directory error from output, got:\n%s\n", got)
		}
	})
}
