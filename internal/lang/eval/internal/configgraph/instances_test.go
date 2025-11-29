// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"maps"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// singleInstanceSelectorForTesting returns an [InstanceSelector] that
// reports a single no-key instance, similar to when there are no repetition
// arguments at all or when "enabled = true".
func singleInstanceSelectorForTesting() InstanceSelector {
	return &instanceSelectorForTesting{
		keyType: addrs.NoKeyType,
		instances: map[addrs.InstanceKey]instances.RepetitionData{
			addrs.NoKey: instances.RepetitionData{},
		},
	}
}

// disabledInstanceSelectorForTesting returns an [InstanceSelector] that
// mimics what happens when "enabled = false".
//
//nolint:unused // The test coverage here is currently lacking, but something like this is likely to be needed when we improve that in future so we'll keep this here as an example for now.
func disabledInstanceSelectorForTesting() InstanceSelector {
	return &instanceSelectorForTesting{
		keyType:   addrs.NoKeyType,
		instances: nil,
	}
}

// countInstanceSelectorForTesting returns an [InstanceSelector] that behaves
// similarly to how we treat the "count" meta-argument.
//
//nolint:unused // The test coverage here is currently lacking, but something like this is likely to be needed when we improve that in future so we'll keep this here as an example for now.
func countInstanceSelectorForTesting(count int) InstanceSelector {
	insts := make(map[addrs.InstanceKey]instances.RepetitionData, count)
	for i := range count {
		insts[addrs.IntKey(i)] = instances.RepetitionData{
			CountIndex: cty.NumberIntVal(int64(i)),
		}
	}
	return &instanceSelectorForTesting{
		keyType:   addrs.IntKeyType,
		instances: insts,
	}
}

// countInstanceSelectorForTesting returns an [InstanceSelector] that behaves
// similarly to how we treat the "for_each" meta-argument.
//
//nolint:unused // The test coverage here is currently lacking, but something like this is likely to be needed when we improve that in future so we'll keep this here as an example for now.
func forEachInstanceSelectorForTesting(elems map[string]cty.Value) InstanceSelector {
	insts := make(map[addrs.InstanceKey]instances.RepetitionData, len(elems))
	for key, val := range elems {
		insts[addrs.StringKey(key)] = instances.RepetitionData{
			EachKey:   cty.StringVal(key),
			EachValue: val,
		}
	}
	return &instanceSelectorForTesting{
		keyType:   addrs.StringKeyType,
		instances: insts,
	}
}

type instanceSelectorForTesting struct {
	keyType        addrs.InstanceKeyType
	instances      map[addrs.InstanceKey]instances.RepetitionData
	instancesMarks cty.ValueMarks
}

// InstanceKeyType implements InstanceSelector.
func (i *instanceSelectorForTesting) InstanceKeyType() addrs.InstanceKeyType {
	return i.keyType
}

// Instances implements InstanceSelector.
func (i *instanceSelectorForTesting) Instances(ctx context.Context) (Maybe[InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics) {
	return Known(InstancesSeq(maps.All(i.instances))), i.instancesMarks, nil
}

// InstancesSourceRange implements InstanceSelector.
func (i *instanceSelectorForTesting) InstancesSourceRange() *tfdiags.SourceRange {
	return nil
}
