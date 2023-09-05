// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package collections

import (
	"github.com/opentffoundation/opentf/internal/command/jsonformat/computed"
	"github.com/opentffoundation/opentf/internal/plans"
)

type ProcessKey func(key string) computed.Diff

func TransformMap[Input any](before, after map[string]Input, keys []string, process ProcessKey) (map[string]computed.Diff, plans.Action) {
	current := plans.NoOp
	if before != nil && after == nil {
		current = plans.Delete
	}
	if before == nil && after != nil {
		current = plans.Create
	}

	elements := make(map[string]computed.Diff)
	for _, key := range keys {
		elements[key] = process(key)
		current = CompareActions(current, elements[key].Action)
	}

	return elements, current
}
