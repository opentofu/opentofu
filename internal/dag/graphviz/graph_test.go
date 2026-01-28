// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graphviz

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/dag"
)

func TestWriteDirectedGraph(t *testing.T) {
	g := &Graph{
		Content: &dag.Graph{},
		Attrs: map[string]Value{
			"rankdir": Val("LR"),
			"pad":     Val(1),
		},
		DefaultNodeAttrs: map[string]Value{
			"shape": Val("rectangle"),
		},
		DefaultEdgeAttrs: map[string]Value{
			"color": Val("red"),
		},
		DefaultEdgeDirectionOut: EdgeAttachmentSouth,
		DefaultEdgeDirectionIn:  EdgeAttachmentNorth,
	}
	noAttrs := g.Content.Add(Node{
		ID: "no_attrs",
	})
	oneAttr := g.Content.Add(Node{
		ID: "one_attr",
		Attrs: map[string]Value{
			"shape": Val("circle"),
		},
	})
	manyAttrs := g.Content.Add(Node{
		ID: "many_attrs",
		Attrs: map[string]Value{
			"shape": Val("underline"),
			"label": Val("I have many attributes!"),
		},
	})
	complexAttrs := g.Content.Add(Node{
		ID: "complex_attrs",
		Attrs: map[string]Value{
			"quoted label":    Val("..."),
			"htmllike":        Val(HTMLLikeString(`<b>Hello!</b>`)),
			"special_escapes": Val(PrequotedValue(`"foo\lbar\r"`)),
		},
	})
	g.Content.Connect(dag.BasicEdge(noAttrs, oneAttr))
	g.Content.Connect(dag.BasicEdge(complexAttrs, manyAttrs))

	var buf strings.Builder
	err := WriteDirectedGraph(g, &buf)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := strings.TrimSpace(`
digraph {
  pad="1";
  rankdir=LR;
  node [shape=rectangle];
  edge [color=red];
  complex_attrs [htmllike=<<b>Hello!</b>>,"quoted label"="...",special_escapes="foo\lbar\r"];
  many_attrs [label="I have many attributes!",shape=underline];
  no_attrs;
  one_attr [shape=circle];
  complex_attrs:s -> many_attrs:n;
  no_attrs:s -> one_attr:n;
}
`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result:\n" + diff)
	}
}
