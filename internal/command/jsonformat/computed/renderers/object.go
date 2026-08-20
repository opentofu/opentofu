// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package renderers

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"
	"github.com/opentofu/opentofu/internal/plans"
)

var _ computed.DiffRenderer = (*objectRenderer)(nil)

func Object(attributes map[string]computed.Diff) computed.DiffRenderer {
	return &objectRenderer{
		attributes:         attributes,
		overrideNullSuffix: true,
	}
}

func NestedObject(attributes map[string]computed.Diff) computed.DiffRenderer {
	return &objectRenderer{
		attributes:         attributes,
		overrideNullSuffix: false,
	}
}

type objectRenderer struct {
	NoWarningsRenderer

	attributes         map[string]computed.Diff
	overrideNullSuffix bool
}

func (renderer objectRenderer) RenderHuman(diff computed.Diff, indent int, opts computed.RenderHumanOpts) string {
	if len(renderer.attributes) == 0 {
		return fmt.Sprintf("{}%s%s", nullSuffix(diff.Action, opts), forcesReplacement(diff.Replace, opts))
	}

	attributeOpts := opts.Clone()
	attributeOpts.OverrideNullSuffix = renderer.overrideNullSuffix

	// We need to keep track of our keys in two ways. The first is the order in
	// which we will display them. The second is a mapping to their safely
	// escaped equivalent.

	maximumKeyLen := 0
	var keys []string
	escapedKeys := make(map[string]string)
	for key := range renderer.attributes {
		keys = append(keys, key)
		escapedKey := EnsureValidAttributeName(key)
		escapedKeys[key] = escapedKey
		if maximumKeyLen < len(escapedKey) {
			maximumKeyLen = len(escapedKey)
		}
	}
	sort.Strings(keys)

	unchangedAttributes := 0
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "{%s\n", forcesReplacement(diff.Replace, opts))
	for _, key := range keys {
		attribute := renderer.attributes[key]

		if importantAttribute(key) {
			importantAttributeOpts := attributeOpts.Clone()
			importantAttributeOpts.ShowUnchangedChildren = true

			for _, warning := range attribute.WarningsHuman(indent+1, importantAttributeOpts) {
				fmt.Fprintf(&buf, "%s%s\n", formatIndent(indent+1), warning)
			}
			fmt.Fprintf(&buf, "%s%s%-*s = %s\n", formatIndent(indent+1), writeDiffActionSymbol(attribute.Action, importantAttributeOpts), maximumKeyLen, escapedKeys[key], attribute.RenderHuman(indent+1, importantAttributeOpts))
			continue
		}

		if attribute.Action == plans.NoOp && !opts.ShowUnchangedChildren {
			// Don't render NoOp operations when we are compact display.
			unchangedAttributes++
			continue
		}

		for _, warning := range attribute.WarningsHuman(indent+1, opts) {
			fmt.Fprintf(&buf, "%s%s\n", formatIndent(indent+1), warning)
		}
		fmt.Fprintf(&buf, "%s%s%-*s = %s\n", formatIndent(indent+1), writeDiffActionSymbol(attribute.Action, attributeOpts), maximumKeyLen, escapedKeys[key], attribute.RenderHuman(indent+1, attributeOpts))
	}

	if unchangedAttributes > 0 {
		fmt.Fprintf(&buf, "%s%s%s\n", formatIndent(indent+1), writeDiffActionSymbol(plans.NoOp, opts), unchanged("attribute", unchangedAttributes, opts))
	}

	fmt.Fprintf(&buf, "%s%s}%s", formatIndent(indent), writeDiffActionSymbol(plans.NoOp, opts), nullSuffix(diff.Action, opts))
	return buf.String()
}
