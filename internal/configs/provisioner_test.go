package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
)

func TestProvisionerBlock_decode_when_destroy(t *testing.T) {
	tests := map[string]struct {
		input *hcl.Block
		want  *Provisioner
		err   string
	}{
		"refer terraform.workspace": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"tf_workspace": {
							Name: "tf_workspace",
							Expr: hcltest.MockExprTraversalSrc("terraform.workspace"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
		"refer self.foo": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"self_foo": {
							Name: "self_foo",
							Expr: hcltest.MockExprTraversalSrc("self.foo"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
		"refer path.foo": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"path_foo": {
							Name: "path_foo",
							Expr: hcltest.MockExprTraversalSrc("path.foo"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
		"refer unknown.workspace": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"unknown_workspace": {
							Name: "unknown_workspace",
							Expr: hcltest.MockExprTraversalSrc("unknown.workspace"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
			err: "Invalid reference from destroy provisioner",
		},
		"refer count.index": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"count_index": {
							Name: "count_index",
							Expr: hcltest.MockExprTraversalSrc("count.index"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
		"refer each.key": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"each_key": {
							Name: "each_key",
							Expr: hcltest.MockExprTraversalSrc("each.key"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
		"handle on_failure with continue": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"on_failure": {
							Name: "on_failure",
							Expr: hcltest.MockExprTraversalSrc("continue"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureContinue,
				DeclRange: blockRange,
			},
		},
		"handle on_failure with fail": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"on_failure": {
							Name: "on_failure",
							Expr: hcltest.MockExprTraversalSrc("fail"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeProvisionerBlock(test.input)

			if diags.HasErrors() {
				if test.err == "" {
					t.Fatalf("unexpected error: %s", diags.Errs())
				}
				if gotErr := diags[0].Summary; gotErr != test.err {
					t.Errorf("wrong error, got %q, want %q", gotErr, test.err)
				}
			} else if test.err != "" {
				t.Fatal("expected error")
			}

			if !cmp.Equal(got, test.want, cmpopts.IgnoreInterfaces(struct{ hcl.Body }{})) {
				t.Fatalf("wrong result: %s", cmp.Diff(got, test.want))
			}
		})
	}
}

func TestProvisionerBlock_decode_when_create(t *testing.T) {
	tests := map[string]struct {
		input *hcl.Block
		want  *Provisioner
		err   string
	}{
		"handle on_failure with continue": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("create"),
						},
						"on_failure": {
							Name: "on_failure",
							Expr: hcltest.MockExprTraversalSrc("continue"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenCreate,
				OnFailure: ProvisionerOnFailureContinue,
				DeclRange: blockRange,
			},
		},
		"handle on_failure with fail": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("create"),
						},
						"on_failure": {
							Name: "on_failure",
							Expr: hcltest.MockExprTraversalSrc("fail"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenCreate,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeProvisionerBlock(test.input)

			if diags.HasErrors() {
				if test.err == "" {
					t.Fatalf("unexpected error: %s", diags.Errs())
				}
				if gotErr := diags[0].Summary; gotErr != test.err {
					t.Errorf("wrong error, got %q, want %q", gotErr, test.err)
				}
			} else if test.err != "" {
				t.Fatal("expected error")
			}

			if !cmp.Equal(got, test.want, cmpopts.IgnoreInterfaces(struct{ hcl.Body }{})) {
				t.Fatalf("wrong result: %s", cmp.Diff(got, test.want))
			}
		})
	}
}

func TestProvisionerBlock_decode_with_connection_block(t *testing.T) {
	tests := map[string]struct {
		input *hcl.Block
		want  *Provisioner
		err   string
	}{
		"refer terraform.workspace": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"tf_workspace": {
										Name: "tf_workspace",
										Expr: hcltest.MockExprTraversalSrc("terraform.workspace"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:       "local-exec",
				Connection: &Connection{},
				When:       ProvisionerWhenDestroy,
				OnFailure:  ProvisionerOnFailureFail,
				DeclRange:  blockRange,
			},
		},
		"refer self.foo": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"self_foo": {
										Name: "self_foo",
										Expr: hcltest.MockExprTraversalSrc("self.foo"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:       "local-exec",
				Connection: &Connection{},
				When:       ProvisionerWhenDestroy,
				OnFailure:  ProvisionerOnFailureFail,
				DeclRange:  blockRange,
			},
		},
		"refer path.foo": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"path_foo": {
										Name: "path_foo",
										Expr: hcltest.MockExprTraversalSrc("path.foo"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:       "local-exec",
				Connection: &Connection{},
				When:       ProvisionerWhenDestroy,
				OnFailure:  ProvisionerOnFailureFail,
				DeclRange:  blockRange,
			},
		},
		"refer unknown.workspace": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"unknown_workspace": {
										Name: "unknown_workspace",
										Expr: hcltest.MockExprTraversalSrc("unknown.workspace"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:       "local-exec",
				Connection: &Connection{},
				When:       ProvisionerWhenDestroy,
				OnFailure:  ProvisionerOnFailureFail,
				DeclRange:  blockRange,
			},
			err: "Invalid reference from destroy provisioner",
		},
		"refer count.index": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"count_index": {
										Name: "count_index",
										Expr: hcltest.MockExprTraversalSrc("count.index"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:       "local-exec",
				Connection: &Connection{},
				When:       ProvisionerWhenDestroy,
				OnFailure:  ProvisionerOnFailureFail,
				DeclRange:  blockRange,
			},
		},
		"refer each.key": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"each_key": {
										Name: "each_key",
										Expr: hcltest.MockExprTraversalSrc("each.key"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:       "local-exec",
				Connection: &Connection{},
				When:       ProvisionerWhenDestroy,
				OnFailure:  ProvisionerOnFailureFail,
				DeclRange:  blockRange,
			},
		},
		"duplicated connection blocks": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"each_key": {
										Name: "each_key",
										Expr: hcltest.MockExprTraversalSrc("each.key"),
									},
								},
							}),
						},
						{
							Type: "connection",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"each_key": {
										Name: "each_key",
										Expr: hcltest.MockExprTraversalSrc("each.key"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:       "local-exec",
				Connection: &Connection{},
				When:       ProvisionerWhenDestroy,
				OnFailure:  ProvisionerOnFailureFail,
				DeclRange:  blockRange,
			},
			err: "Duplicate connection block",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeProvisionerBlock(test.input)

			if diags.HasErrors() {
				if test.err == "" {
					t.Fatalf("unexpected error: %s", diags.Errs())
				}
				if gotErr := diags[0].Summary; gotErr != test.err {
					t.Errorf("wrong error, got %q, want %q", gotErr, test.err)
				}
			} else if test.err != "" {
				t.Fatal("expected error")
			}

			if !cmp.Equal(got, test.want, cmpopts.IgnoreInterfaces(struct{ hcl.Body }{})) {
				t.Fatalf("wrong result: %s", cmp.Diff(got, test.want))
			}
		})
	}
}

func TestProvisionerBlock_decode_with_escape_block(t *testing.T) {
	tests := map[string]struct {
		input *hcl.Block
		want  *Provisioner
		err   string
	}{
		"only one escape block": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "_",
							Body: hcltest.MockBody(&hcl.BodyContent{}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
		},
		"duplicated escape blocks": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "_",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"each_key": {
										Name: "each_key",
										Expr: hcltest.MockExprTraversalSrc("each.key"),
									},
								},
							}),
						},
						{
							Type: "_",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"each_key": {
										Name: "each_key",
										Expr: hcltest.MockExprTraversalSrc("each.key"),
									},
								},
							}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenDestroy,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
			err: "Duplicate escaping block",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeProvisionerBlock(test.input)

			if diags.HasErrors() {
				if test.err == "" {
					t.Fatalf("unexpected error: %s", diags.Errs())
				}
				if gotErr := diags[0].Summary; gotErr != test.err {
					t.Errorf("wrong error, got %q, want %q", gotErr, test.err)
				}
			} else if test.err != "" {
				t.Fatal("expected error")
			}

			if !cmp.Equal(got, test.want, cmpopts.IgnoreInterfaces(struct{ hcl.Body }{})) {
				t.Fatalf("wrong result: %s", cmp.Diff(got, test.want))
			}
		})
	}
}

func TestProvisionerBlock_decode_with_invalid_keyword(t *testing.T) {
	tests := map[string]struct {
		input *hcl.Block
		want  *Provisioner
		err   string
	}{
		"with removed keyword `chef`": {
			input: &hcl.Block{
				Type:        "provisioner",
				Labels:      []string{"chef"},
				Body:        hcltest.MockBody(&hcl.BodyContent{}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: nil,
			err:  `The "chef" provisioner has been removed`,
		},
		"with removed keyword `habitat`": {
			input: &hcl.Block{
				Type:        "provisioner",
				Labels:      []string{"habitat"},
				Body:        hcltest.MockBody(&hcl.BodyContent{}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: nil,
			err:  `The "habitat" provisioner has been removed`,
		},
		"with removed keyword `puppet`": {
			input: &hcl.Block{
				Type:        "provisioner",
				Labels:      []string{"puppet"},
				Body:        hcltest.MockBody(&hcl.BodyContent{}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: nil,
			err:  `The "puppet" provisioner has been removed`,
		},
		"with removed keyword `salt-masterless`": {
			input: &hcl.Block{
				Type:        "provisioner",
				Labels:      []string{"salt-masterless"},
				Body:        hcltest.MockBody(&hcl.BodyContent{}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: nil,
			err:  `The "salt-masterless" provisioner has been removed`,
		},
		"with invalid keyword for `when`": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("unknown"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenCreate,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
			err: `Invalid "when" keyword`,
		},
		"with invalid keyword for `on_failure`": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("create"),
						},
						"on_failure": {
							Name: "on_failure",
							Expr: hcltest.MockExprTraversalSrc("unknown"),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenCreate,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
			err: `Invalid "on_failure" keyword`,
		},
		"with reserved keyword `lifecycle` on block": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("create"),
						},
					},
					Blocks: []*hcl.Block{
						{
							Type: "lifecycle",
							Body: hcltest.MockBody(&hcl.BodyContent{}),
						},
					},
				}),
				DefRange:    blockRange,
				LabelRanges: []hcl.Range{hcl.Range{}},
			},
			want: &Provisioner{
				Type:      "local-exec",
				When:      ProvisionerWhenCreate,
				OnFailure: ProvisionerOnFailureFail,
				DeclRange: blockRange,
			},
			err: `Reserved block type name in provisioner block`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeProvisionerBlock(test.input)

			if diags.HasErrors() {
				if test.err == "" {
					t.Fatalf("unexpected error: %s", diags.Errs())
				}
				if gotErr := diags[0].Summary; gotErr != test.err {
					t.Errorf("wrong error, got %q, want %q", gotErr, test.err)
				}
			} else if test.err != "" {
				t.Fatal("expected error")
			}

			if !cmp.Equal(got, test.want, cmpopts.IgnoreInterfaces(struct{ hcl.Body }{})) {
				t.Fatalf("wrong result: %s", cmp.Diff(got, test.want))
			}
		})
	}
}
