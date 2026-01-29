// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/zclconf/go-cty/cty"
)

func TestProvidersSchema_error(t *testing.T) {
	ui := new(cli.MockUi)
	c := &ProvidersSchemaCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	if code := c.Run(nil); code != 1 {
		fmt.Println(ui.OutputWriter.String())
		t.Fatalf("expected error: \n%s", ui.OutputWriter.String())
	}
}

func TestProvidersSchema_output(t *testing.T) {
	fixtureDir := "testdata/providers-schema"
	testDirs, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range testDirs {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			td := t.TempDir()
			inputDir := filepath.Join(fixtureDir, entry.Name())
			testCopyDir(t, inputDir, td)
			t.Chdir(td)

			providerSource, close := newMockProviderSource(t, map[string][]string{
				"test": {"1.2.3"},
			})
			defer close()

			p := providersSchemaFixtureProvider()
			ui := new(cli.MockUi)
			m := Meta{
				WorkingDir:       workdir.NewDir("."),
				testingOverrides: metaOverridesForProvider(p),
				Ui:               ui,
				ProviderSource:   providerSource,
			}

			// `terraform init`
			ic := &InitCommand{
				Meta: m,
			}
			if code := ic.Run([]string{}); code != 0 {
				t.Fatalf("init failed\n%s", ui.ErrorWriter)
			}

			// flush the init output from the mock ui
			ui.OutputWriter.Reset()

			// `tofu provider schemas` command
			pc := &ProvidersSchemaCommand{Meta: m}
			if code := pc.Run([]string{"-json"}); code != 0 {
				t.Fatalf("wrong exit status %d; want 0\nstderr: %s", code, ui.ErrorWriter.String())
			}
			var got, want providerSchemas

			gotString := ui.OutputWriter.String()
			if err := json.Unmarshal([]byte(gotString), &got); err != nil {
				t.Fatal(err)
			}

			wantFile, err := os.Open("output.json")
			if err != nil {
				t.Fatalf("err: %s", err)
			}
			defer wantFile.Close()
			byteValue, err := io.ReadAll(wantFile)
			if err != nil {
				t.Fatalf("err: %s", err)
			}
			if err := json.Unmarshal([]byte(byteValue), &want); err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(got, want) {
				t.Fatalf("wrong result:\n %v\n", cmp.Diff(got, want))
			}
		})
	}
}

type providerSchemas struct {
	FormatVersion string                    `json:"format_version"`
	Schemas       map[string]providerSchema `json:"provider_schemas"`
}

type providerSchema struct {
	Provider          interface{}            `json:"provider,omitempty"`
	ResourceSchemas   map[string]interface{} `json:"resource_schemas,omitempty"`
	DataSourceSchemas map[string]interface{} `json:"data_source_schemas,omitempty"`
	Functions         map[string]interface{} `json:"functions,omitempty"`
}

// testProvider returns a mock provider that is configured for basic
// operation with the configuration in testdata/providers-schema.
func providersSchemaFixtureProvider() *tofu.MockProvider {
	p := testProvider()
	p.GetProviderSchemaResponse = providersSchemaFixtureSchema()
	return p
}

// providersSchemaFixtureSchema returns a schema suitable for processing the
// configuration in testdata/providers-schema.ß
func providersSchemaFixtureSchema() *providers.GetProviderSchemaResponse {
	return &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"region": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id":  {Type: cty.String, Optional: true, Computed: true},
						"ami": {Type: cty.String, Optional: true},
						"volumes": {
							NestedType: &configschema.Object{
								Nesting: configschema.NestingList,
								Attributes: map[string]*configschema.Attribute{
									"size":        {Type: cty.String, Required: true},
									"mount_point": {Type: cty.String, Required: true},
								},
							},
							Optional: true,
						},
					},
				},
			},
		},
		Functions: map[string]providers.FunctionSpec{
			"test_func": {
				Description: "a basic string function",
				Return:      cty.String,
				Summary:     "test",
				Parameters: []providers.FunctionParameterSpec{{
					Name: "input",
					Type: cty.Number,
				}},
				VariadicParameter: &providers.FunctionParameterSpec{
					Name: "variadic_input",
					Type: cty.List(cty.Bool),
				},
			},
		},
	}
}
