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
		c := &ProvidersMirrorCommand{
			Meta{
				WorkingDir: workdir.NewDir("."),
			},
		}
		code := c.Run([]string{"."})
		if code != 0 {
			t.Fatalf("wrong exit code. expected 0, got %d", code)
		}
	})

	t.Run("missing arg error", func(t *testing.T) {
		ui := new(cli.MockUi)
		c := &ProvidersMirrorCommand{
			Meta: Meta{
				WorkingDir: workdir.NewDir("."),
				Ui:         ui,
			},
		}
		code := c.Run([]string{})
		if code != 1 {
			t.Fatalf("wrong exit code. expected 1, got %d", code)
		}

		got := ui.ErrorWriter.String()
		if !strings.Contains(got, "Error: No output directory specified") {
			t.Fatalf("missing directory error from output, got:\n%s\n", got)
		}
	})
}
