package configs

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func TestResource_IgnoreChanges(t *testing.T) {
	cases := []struct {
		input            string
		IgnoreChanges    []hcl.Traversal
		IgnoreAllChanges bool
		parseError       string
		decodeError      string
	}{{
		input:      `*`,
		parseError: `<test>:4,23-24: Invalid expression; Expected the start of an expression, but found an invalid expression token.`,
	}, {
		input:       `"*"`,
		decodeError: `<test>:4,23-26: Invalid ignore_changes ruleset; Expected "all" or [items], not *.`,
	}, {
		input:      `[*]`,
		parseError: `<test>:4,24-25: Invalid expression; Expected the start of an expression, but found an invalid expression token.`,
	}, {
		input:       `["*"]`,
		decodeError: `<test>:4,23-28: Invalid ignore_changes wildcard; The ["*"] form of ignore_changes wildcard is was deprecated and is now invalid. Use "ignore_changes = all" to ignore changes to all attributes.`,
	}, {
		input:       `["*", foo]`,
		decodeError: `<test>:4,23-33: Invalid ignore_changes wildcard; The ["*"] form of ignore_changes wildcard is was deprecated and is now invalid. Use "ignore_changes = all" to ignore changes to all attributes.`,
	}, {
		input:            `all`,
		IgnoreAllChanges: true,
	}, {
		input:            `"all"`,
		IgnoreAllChanges: true,
	}, {
		input:         `[all]`,
		IgnoreChanges: []hcl.Traversal{hcl.Traversal{hcl.TraverseAttr{Name: "all"}}},
	}, {
		input:         `["all"]`,
		IgnoreChanges: []hcl.Traversal{hcl.Traversal{hcl.TraverseAttr{Name: "all"}}},
	}, {
		input: `[]`,
	}, {
		input:         `[foo]`,
		IgnoreChanges: []hcl.Traversal{hcl.Traversal{hcl.TraverseAttr{Name: "foo"}}},
	}, {
		input:         `["foo"]`,
		IgnoreChanges: []hcl.Traversal{hcl.Traversal{hcl.TraverseAttr{Name: "foo"}}},
	}, {
		input:       "local.value",
		decodeError: `<test>:4,23-34: Invalid ignore_changes ruleset; Expected "all" or [items], not value.`,
	}, {
		input:         "local.listy",
		IgnoreChanges: []hcl.Traversal{hcl.Traversal{hcl.TraverseAttr{Name: "y"}}},
	}, {
		// This test case and the next are a bit weird and potentially confusing to end users.
		input:         `[local.value]`,
		IgnoreChanges: []hcl.Traversal{hcl.Traversal{hcl.TraverseAttr{Name: "value"}}},
	}, {
		input:         `[local.value_not_found]`,
		IgnoreChanges: []hcl.Traversal{hcl.Traversal{hcl.TraverseAttr{Name: "local"}, hcl.TraverseAttr{Name: "value_not_found"}}},
	}, {
		input: `[foo, "bar"]`,
		IgnoreChanges: []hcl.Traversal{
			hcl.Traversal{hcl.TraverseAttr{Name: "foo"}},
			hcl.Traversal{hcl.TraverseAttr{Name: "bar"}},
		},
	}, {
		input: `[foo, local.value]`,
		IgnoreChanges: []hcl.Traversal{
			hcl.Traversal{hcl.TraverseAttr{Name: "foo"}},
			hcl.Traversal{hcl.TraverseAttr{Name: "local"}, hcl.TraverseAttr{Name: "value"}},
		},
	}, {
		input: `["foo", local.value]`,
		IgnoreChanges: []hcl.Traversal{
			hcl.Traversal{hcl.TraverseAttr{Name: "foo"}},
			hcl.Traversal{hcl.TraverseAttr{Name: "value"}},
		},
	}, {
		input: `concat(["foo"], ["bar"])`,
		IgnoreChanges: []hcl.Traversal{
			hcl.Traversal{hcl.TraverseAttr{Name: "foo"}},
			hcl.Traversal{hcl.TraverseAttr{Name: "bar"}},
		},
	}, {
		input:       `[foo, []]`,
		decodeError: `<test>:4,29-31: Invalid expression; A single static variable reference is required: only attribute access and indexing with constant keys. No calculations, function calls, template expressions, etc are allowed here.`,
	}, {
		input:       `["foo", []]`,
		decodeError: `<test>:4,23-34: Invalid ignore_changes input; Expected collection of string, not tuple`,
	}, {
		input:       `true`,
		decodeError: `<test>:4,23-27: Invalid ignore_changes ruleset; Unexpected type bool`,
	}, {
		input:       `[foo, "bar..."]`,
		decodeError: `<test>:4,33-36: Invalid character; Expected an attribute access or an index operator., and 1 other diagnostic(s)`,
	}, {
		input:       `["foo", "bar..."]`,
		decodeError: `<test>:4,23-40: Invalid ignore_changes input; Unable to parse "bar..." as traversal, and 1 other diagnostic(s)`,
	}, {
		input:       `local.missing`,
		decodeError: `<test>:4,23-36: Undefined local; Undefined local local.missing`,
	}}

	eval := NewStaticEvaluator(&Module{
		Locals: map[string]*Local{
			"value": &Local{
				Name: "value",
				Expr: hcl.StaticExpr(cty.StringVal("value"), hcl.Range{}),
			},
			"listy": &Local{
				Name: "listy",
				Expr: hcl.StaticExpr(cty.ListVal([]cty.Value{cty.StringVal("y")}), hcl.Range{}),
			},
		},
	}, RootModuleCallForTesting())

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			config := fmt.Sprintf(`
			resource "provider_type" "name" {
				lifecycle {
					ignore_changes = %s
				}
			}
			`, tc.input)

			parsed, parsedDiags := hclsyntax.ParseConfig([]byte(config), "<test>", hcl.InitialPos)

			if parsedDiags.HasErrors() || tc.parseError != "" {
				if tc.parseError != parsedDiags.Error() {
					t.Errorf("Expected error %q, got %q", tc.parseError, parsedDiags.Error())
				}
				return
			}

			content, contentDiags := parsed.Body.Content(configFileSchema)
			if contentDiags.HasErrors() {
				t.Fatal(contentDiags.Error())
			}

			resource, resourceDiags := decodeResourceBlock(content.Blocks[0], false)
			if resourceDiags.HasErrors() {
				t.Fatal(resourceDiags.Error())
			}

			decodeDiags := resource.decodeStaticFields(eval)
			if decodeDiags.HasErrors() || tc.decodeError != "" {
				if tc.decodeError != decodeDiags.Error() {
					t.Errorf("Expected error %q, got %q", tc.decodeError, decodeDiags.Error())
				}
				return
			}

			if tc.IgnoreAllChanges != resource.Managed.IgnoreAllChanges {
				t.Errorf("Expected ignore changes all %v, got %v", tc.IgnoreAllChanges, resource.Managed.IgnoreAllChanges)
				return
			}

			if len(tc.IgnoreChanges) != len(resource.Managed.IgnoreChanges) {
				t.Errorf("Expected %v ignore changes, got %v", len(tc.IgnoreChanges), len(resource.Managed.IgnoreChanges))
			}

			// Compare without source ranges as those are not ever used in the code and harder to fake
			for i := range tc.IgnoreChanges {
				expectTrav := tc.IgnoreChanges[i]
				gotTrav := resource.Managed.IgnoreChanges[i]
				if len(expectTrav) != len(gotTrav) {
					t.Errorf("Expected %#v, got %#v", expectTrav, gotTrav)
					continue
				}
				for j := range expectTrav {
					expectPart := expectTrav[j]
					gotPart := gotTrav[j]
					switch expectPart.(type) {
					case hcl.TraverseAttr:
						expectName := expectPart.(hcl.TraverseAttr).Name
						gotName := gotPart.(hcl.TraverseAttr).Name
						if expectName != gotName {
							t.Errorf("Expected %#v, got %#v", expectTrav, gotTrav)
							break
						}
					case hcl.TraverseIndex:
						expectKey := expectPart.(hcl.TraverseIndex).Key
						gotKey := gotPart.(hcl.TraverseIndex).Key
						if expectKey != gotKey {
							t.Errorf("Expected %#v, got %#v", expectTrav, gotTrav)
							break
						}
					default:
						panic("unsupported")
					}
				}
			}
		})
	}
}
