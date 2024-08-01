// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
)

func TestProvisionerBlock_decode(t *testing.T) {
	tests := map[string]struct {
		input *hcl.Block
		want  *Provisioner
		err   string
	}{
		"refer terraform.workspace when destroy": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"command": {
							Name: "command",
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
		"refer tofu.workspace when destroy": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"command": {
							Name: "command",
							Expr: hcltest.MockExprTraversalSrc("tofu.workspace"),
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
		"refer unknown.workspace when destroy": {
			input: &hcl.Block{
				Type:   "provisioner",
				Labels: []string{"local-exec"},
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"when": {
							Name: "when",
							Expr: hcltest.MockExprTraversalSrc("destroy"),
						},
						"command": {
							Name: "command",
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
