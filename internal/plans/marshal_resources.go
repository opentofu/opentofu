// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

// MarshalResourceChanges converts the provided internal representation of
// ResourceInstanceChangeSrc objects into the public structured JSON changes.
func MarshalResourceChanges(resources []*ResourceInstanceChangeSrc, schemas interface{}) ([]JSONResourceChange, error) {
	var ret []JSONResourceChange

	var sortedResources []*ResourceInstanceChangeSrc
	sortedResources = append(sortedResources, resources...)
	sort.Slice(sortedResources, func(i, j int) bool {
		if !sortedResources[i].Addr.Equal(sortedResources[j].Addr) {
			return sortedResources[i].Addr.Less(sortedResources[j].Addr)
		}
		return sortedResources[i].DeposedKey < sortedResources[j].DeposedKey
	})

	for _, rc := range sortedResources {
		var r JSONResourceChange
		addr := rc.Addr
		r.Address = addr.String()
		if !addr.Equal(rc.PrevRunAddr) {
			r.PreviousAddress = rc.PrevRunAddr.String()
		}

		dataSource := addr.Resource.Resource.Mode == addrs.DataResourceMode
		// We create "delete" actions for data resources so we can clean up
		// their entries in state, but this is an implementation detail that
		// users shouldn't see.
		if dataSource && rc.Action == Delete {
			continue
		}

		schema, _ := GetResourceSchema(schemas, rc.ProviderAddr.Provider, addr.Resource.Resource.Mode, addr.Resource.Resource.Type)
		if schema == nil {
			return nil, fmt.Errorf("no schema found for %s (in provider %s)", r.Address, rc.ProviderAddr.Provider)
		}

		changeV, err := rc.Decode(schema.ImpliedType())
		if err != nil {
			return nil, err
		}
		// We drop the marks from the change, as decoding is only an
		// intermediate step to re-encode the values as json
		changeV.Before, _ = changeV.Before.UnmarkDeep()
		changeV.After, _ = changeV.After.UnmarkDeep()

		var before, after []byte
		var beforeSensitive, afterSensitive []byte
		var afterUnknown cty.Value

		if changeV.Before != cty.NilVal {
			before, err = ctyjson.Marshal(changeV.Before, changeV.Before.Type())
			if err != nil {
				return nil, err
			}
			marks := rc.BeforeValMarks
			if ContainsSensitive(schema) {
				marks = append(marks, GetValueMarks(schema, changeV.Before, nil)...)
			}
			bs := SensitiveAsBoolWithPathValueMarks(changeV.Before, marks)
			beforeSensitive, err = ctyjson.Marshal(bs, bs.Type())
			if err != nil {
				return nil, err
			}
		}
		if changeV.After != cty.NilVal {
			if changeV.After.IsWhollyKnown() {
				after, err = ctyjson.Marshal(changeV.After, changeV.After.Type())
				if err != nil {
					return nil, err
				}
				afterUnknown = cty.EmptyObjectVal
			} else {
				filteredAfter := OmitUnknowns(changeV.After)
				if filteredAfter.IsNull() {
					after = nil
				} else {
					after, err = ctyjson.Marshal(filteredAfter, filteredAfter.Type())
					if err != nil {
						return nil, err
					}
				}
				afterUnknown = unknownAsBool(changeV.After)
			}
			marks := rc.AfterValMarks
			if ContainsSensitive(schema) {
				marks = append(marks, GetValueMarks(schema, changeV.After, nil)...)
			}
			as := SensitiveAsBoolWithPathValueMarks(changeV.After, marks)
			afterSensitive, err = ctyjson.Marshal(as, as.Type())
			if err != nil {
				return nil, err
			}
		}

		a, err := ctyjson.Marshal(afterUnknown, afterUnknown.Type())
		if err != nil {
			return nil, err
		}
		replacePaths, err := encodePaths(rc.RequiredReplace)
		if err != nil {
			return nil, err
		}

		var importing *JSONImporting
		if rc.Importing != nil {
			importing = &JSONImporting{ID: rc.Importing.ID}
		}

		r.Change = JSONChange{
			Actions:         actionString(rc.Action.String()),
			Before:          json.RawMessage(before),
			After:           json.RawMessage(after),
			AfterUnknown:    a,
			BeforeSensitive: json.RawMessage(beforeSensitive),
			AfterSensitive:  json.RawMessage(afterSensitive),
			ReplacePaths:    replacePaths,
			Importing:       importing,
			GeneratedConfig: rc.GeneratedConfig,
		}

		if rc.DeposedKey != states.NotDeposed {
			r.Deposed = rc.DeposedKey.String()
		}

		key := addr.Resource.Key
		if key != nil {
			value := key.Value()
			if r.Index, err = ctyjson.Marshal(value, value.Type()); err != nil {
				return nil, err
			}
		}

		switch addr.Resource.Resource.Mode {
		case addrs.ManagedResourceMode:
			r.Mode = "managed"
		case addrs.DataResourceMode:
			r.Mode = "data"
		default:
			return nil, fmt.Errorf("resource %s has an unsupported mode %s", r.Address, addr.Resource.Resource.Mode.String())
		}
		r.ModuleAddress = addr.Module.String()
		r.Name = addr.Resource.Resource.Name
		r.Type = addr.Resource.Resource.Type
		r.ProviderName = rc.ProviderAddr.Provider.String()

		switch rc.ActionReason {
		case ResourceInstanceChangeNoReason:
			r.ActionReason = "" // will be omitted in output
		case ResourceInstanceReplaceBecauseCannotUpdate:
			r.ActionReason = JSONResourceInstanceReplaceBecauseCannotUpdate
		case ResourceInstanceReplaceBecauseTainted:
			r.ActionReason = JSONResourceInstanceReplaceBecauseTainted
		case ResourceInstanceReplaceByRequest:
			r.ActionReason = JSONResourceInstanceReplaceByRequest
		case ResourceInstanceReplaceByTriggers:
			r.ActionReason = JSONResourceInstanceReplaceByTriggers
		case ResourceInstanceDeleteBecauseNoResourceConfig:
			r.ActionReason = JSONResourceInstanceDeleteBecauseNoResourceConfig
		case ResourceInstanceDeleteBecauseWrongRepetition:
			r.ActionReason = JSONResourceInstanceDeleteBecauseWrongRepetition
		case ResourceInstanceDeleteBecauseCountIndex:
			r.ActionReason = JSONResourceInstanceDeleteBecauseCountIndex
		case ResourceInstanceDeleteBecauseEachKey:
			r.ActionReason = JSONResourceInstanceDeleteBecauseEachKey
		case ResourceInstanceDeleteBecauseNoModule:
			r.ActionReason = JSONResourceInstanceDeleteBecauseNoModule
		case ResourceInstanceDeleteBecauseNoMoveTarget:
			r.ActionReason = JSONResourceInstanceDeleteBecauseNoMoveTarget
		case ResourceInstanceReadBecauseConfigUnknown:
			r.ActionReason = JSONResourceInstanceReadBecauseConfigUnknown
		case ResourceInstanceReadBecauseDependencyPending:
			r.ActionReason = JSONResourceInstanceReadBecauseDependencyPending
		case ResourceInstanceReadBecauseCheckNested:
			r.ActionReason = JSONResourceInstanceReadBecauseCheckNested
		default:
			return nil, fmt.Errorf("resource %s has an unsupported action reason %s", r.Address, rc.ActionReason)
		}

		ret = append(ret, r)
	}

	return ret, nil
}

// MarshalOutputChanges converts the provided internal representation of
// Changes objects into the structured JSON representation.
func MarshalOutputChanges(changes *Changes) (map[string]JSONChange, error) {
	if changes == nil {
		// Nothing to do!
		return nil, nil
	}

	outputChanges := make(map[string]JSONChange, len(changes.Outputs))
	for _, oc := range changes.Outputs {

		// Skip output changes that are not from the root module.
		// These are automatically stripped from plans that are written to disk
		// elsewhere, we just need to duplicate the logic here in case anyone
		// is converting this plan directly from memory.
		if !oc.Addr.Module.IsRoot() {
			continue
		}

		changeV, err := oc.Decode()
		if err != nil {
			return nil, err
		}
		// We drop the marks from the change, as decoding is only an
		// intermediate step to re-encode the values as json
		changeV.Before, _ = changeV.Before.UnmarkDeep()
		changeV.After, _ = changeV.After.UnmarkDeep()

		var before, after []byte
		var afterUnknown cty.Value

		if changeV.Before != cty.NilVal {
			before, err = ctyjson.Marshal(changeV.Before, changeV.Before.Type())
			if err != nil {
				return nil, err
			}
		}
		if changeV.After != cty.NilVal {
			if changeV.After.IsWhollyKnown() {
				after, err = ctyjson.Marshal(changeV.After, changeV.After.Type())
				if err != nil {
					return nil, err
				}
				afterUnknown = cty.False
			} else {
				filteredAfter := OmitUnknowns(changeV.After)
				if filteredAfter.IsNull() {
					after = nil
				} else {
					after, err = ctyjson.Marshal(filteredAfter, filteredAfter.Type())
					if err != nil {
						return nil, err
					}
				}
				afterUnknown = unknownAsBool(changeV.After)
			}
		}

		// The only information we have in the plan about output sensitivity is
		// a boolean which is true if the output was or is marked sensitive. As
		// a result, BeforeSensitive and AfterSensitive will be identical, and
		// either false or true.
		outputSensitive := cty.False
		if oc.Sensitive {
			outputSensitive = cty.True
		}
		sensitive, err := ctyjson.Marshal(outputSensitive, outputSensitive.Type())
		if err != nil {
			return nil, err
		}

		a, _ := ctyjson.Marshal(afterUnknown, afterUnknown.Type())

		c := JSONChange{
			Actions:         actionString(oc.Action.String()),
			Before:          json.RawMessage(before),
			After:           json.RawMessage(after),
			AfterUnknown:    a,
			BeforeSensitive: json.RawMessage(sensitive),
			AfterSensitive:  json.RawMessage(sensitive),

			// Just to be explicit, outputs cannot be imported so this is always
			// nil.
			Importing: nil,
		}

		outputChanges[oc.Addr.OutputValue.Name] = c
	}

	return outputChanges, nil
}

func actionString(action string) []string {
	switch action {
	case "NoOp":
		return []string{"no-op"}
	case "Create":
		return []string{"create"}
	case "Delete":
		return []string{"delete"}
	case "Update":
		return []string{"update"}
	case "CreateThenDelete":
		return []string{"create", "delete"}
	case "Read":
		return []string{"read"}
	case "DeleteThenCreate":
		return []string{"delete", "create"}
	case "Forget":
		return []string{"forget"}
	default:
		return []string{action}
	}
}