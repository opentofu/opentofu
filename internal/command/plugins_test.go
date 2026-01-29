// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"reflect"
	"testing"

	"github.com/opentofu/opentofu/internal/command/workdir"
)

func TestPluginPath(t *testing.T) {
	td := testTempDirRealpath(t)
	t.Chdir(td)

	pluginPath := []string{"a", "b", "c"}

	m := Meta{
		WorkingDir: workdir.NewDir("."),
	}
	if err := m.storePluginPath(pluginPath); err != nil {
		t.Fatal(err)
	}

	restoredPath, err := m.loadPluginPath()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(pluginPath, restoredPath) {
		t.Fatalf("expected plugin path %#v, got %#v", pluginPath, restoredPath)
	}
}

func TestInternalProviders(t *testing.T) {
	m := Meta{
		WorkingDir: workdir.NewDir("."),
	}
	internal := m.internalProviders()
	tfProvider, err := internal["terraform"]()
	if err != nil {
		t.Fatal(err)
	}

	schema := tfProvider.GetProviderSchema(t.Context())
	_, found := schema.DataSources["terraform_remote_state"]
	if !found {
		t.Errorf("didn't find terraform_remote_state in internal \"terraform\" provider")
	}
}
