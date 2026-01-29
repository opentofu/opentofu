// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"reflect"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/posener/complete"
)

func TestMetaCompletePredictWorkspaceName(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	t.Chdir(td)

	// make sure a vars file doesn't interfere
	err := os.WriteFile(DefaultVarsFilename, nil, 0644)
	if err != nil {
		t.Fatal(err)
	}

	ui := new(cli.MockUi)
	meta := &Meta{
		WorkingDir: workdir.NewDir("."),
		Ui:         ui,
	}

	predictor := meta.completePredictWorkspaceName(t.Context())

	got := predictor.Predict(complete.Args{
		Last: "",
	})
	want := []string{"default"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, want)
	}
}
